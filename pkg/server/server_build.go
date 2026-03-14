package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"connectrpc.com/connect"
	"connectrpc.com/validate"
	"github.com/rs/cors"

	"github.com/relaymesh/relaymesh/pkg/api"
	oidchelper "github.com/relaymesh/relaymesh/pkg/auth/oidc"
	"github.com/relaymesh/relaymesh/pkg/core"
	cloudv1connect "github.com/relaymesh/relaymesh/pkg/gen/cloud/v1/cloudv1connect"
	"github.com/relaymesh/relaymesh/pkg/oauth"
	"github.com/relaymesh/relaymesh/pkg/webhook"

	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
)

// BuildHandler constructs the HTTP handler and returns a cleanup function.
func BuildHandler(ctx context.Context, config core.Config, logger *log.Logger, middlewares ...Middleware) (http.Handler, func(), error) {
	if logger == nil {
		logger = core.NewLogger("server")
	}
	var closers []func()
	addCloser := func(fn func()) {
		if fn != nil {
			closers = append(closers, fn)
		}
	}
	cleanup := func() {
		for i := len(closers) - 1; i >= 0; i-- {
			closers[i]()
		}
	}
	fail := func(err error) (http.Handler, func(), error) {
		cleanup()
		return nil, nil, err
	}

	ruleEngine, err := core.NewRuleEngine(core.RulesConfig{
		Rules:  config.Rules,
		Strict: config.RulesStrict,
		Logger: logger,
	})
	if err != nil {
		return fail(fmt.Errorf("compile rules: %w", err))
	}

	stores, err := openStores(config, logger, addCloser)
	if err != nil {
		return fail(err)
	}

	caches, err := buildCaches(ctx, stores, config, logger, addCloser)
	if err != nil {
		return fail(err)
	}

	publisher, err := buildPublisher(config, caches.driverCache)
	if err != nil {
		return fail(err)
	}
	addCloser(func() { _ = publisher.Close() })

	mux := http.NewServeMux()
	mux.Handle("/healthz", healthHandler())
	mux.Handle("/readyz", healthHandler())
	validationInterceptor := validate.NewInterceptor()
	connectOpts := []connect.HandlerOption{
		connect.WithInterceptors(validationInterceptor),
	}
	var verifier *oidchelper.Verifier
	if config.Auth.OAuth2.Enabled {
		created, err := oidchelper.NewVerifier(ctx, config.Auth.OAuth2)
		if err != nil {
			return fail(fmt.Errorf("oauth2 verifier: %w", err))
		}
		verifier = created
		authHandler := newOAuth2Handler(config.Auth.OAuth2, logger)
		mux.HandleFunc("/auth/login", authHandler.Login)
		mux.HandleFunc("/auth/callback", authHandler.Callback)
		logger.Printf("auth=oauth2 enabled issuer=%s", config.Auth.OAuth2.Issuer)
	}
	if verifier != nil {
		connectOpts = append(connectOpts, connect.WithInterceptors(newAuthInterceptor(verifier, logger)))
	}
	connectOpts = append(connectOpts, connect.WithInterceptors(newTenantInterceptor()))

	webhookRegistry := webhook.DefaultRegistry()
	oauthRegistry := oauth.DefaultRegistry()

	mux.Handle("/", &oauth.StartHandler{
		Providers:             config.Providers,
		Endpoint:              config.Endpoint,
		Logger:                logger,
		ProviderInstanceStore: stores.instanceStore,
		ProviderInstanceCache: caches.instanceCache,
		Registry:              oauthRegistry,
	})
	{
		installSvc := &api.InstallationsService{
			Store:     stores.installStore,
			Providers: config.Providers,
			Logger:    logger,
		}
		path, handler := cloudv1connect.NewInstallationsServiceHandler(installSvc, connectOpts...)
		mux.Handle(path, handler)
	}
	{
		namespaceSvc := &api.NamespacesService{
			Store:                 stores.namespaceStore,
			InstallStore:          stores.installStore,
			ProviderInstanceStore: stores.instanceStore,
			ProviderInstanceCache: caches.instanceCache,
			Providers:             config.Providers,
			Endpoint:              config.Endpoint,
			Logger:                logger,
		}
		path, handler := cloudv1connect.NewNamespacesServiceHandler(namespaceSvc, connectOpts...)
		mux.Handle(path, handler)
	}
	{
		rulesSvc := &api.RulesService{
			Store:       stores.ruleStore,
			DriverStore: stores.driverStore,
			Engine:      ruleEngine,
			Strict:      config.RulesStrict,
			Logger:      logger,
		}
		path, handler := cloudv1connect.NewRulesServiceHandler(rulesSvc, connectOpts...)
		mux.Handle(path, handler)
	}
	{
		driversSvc := &api.DriversService{
			Store:  stores.driverStore,
			Cache:  caches.driverCache,
			Logger: logger,
		}
		path, handler := cloudv1connect.NewDriversServiceHandler(driversSvc, connectOpts...)
		mux.Handle(path, handler)
	}
	{
		providerSvc := &api.ProvidersService{
			Store:  stores.instanceStore,
			Cache:  caches.instanceCache,
			Logger: logger,
		}
		path, handler := cloudv1connect.NewProvidersServiceHandler(providerSvc, connectOpts...)
		mux.Handle(path, handler)
	}
	{
		scmSvc := &api.SCMService{
			Store:                 stores.installStore,
			ProviderInstanceStore: stores.instanceStore,
			ProviderInstanceCache: caches.instanceCache,
			Providers:             config.Providers,
			Logger:                logger,
		}
		path, handler := cloudv1connect.NewSCMServiceHandler(scmSvc, connectOpts...)
		mux.Handle(path, handler)
	}
	{
		eventLogSvc := &api.EventLogsService{
			Store:       stores.logStore,
			RuleStore:   stores.ruleStore,
			DriverStore: stores.driverStore,
			Publisher:   publisher,
			RulesStrict: config.RulesStrict,
			Logger:      logger,
		}
		path, handler := cloudv1connect.NewEventLogsServiceHandler(eventLogSvc, connectOpts...)
		mux.Handle(path, handler)
	}

	webhookOpts := webhook.HandlerOptions{
		Rules:              ruleEngine,
		Publisher:          publisher,
		Logger:             logger,
		MaxBodyBytes:       config.Server.MaxBodyBytes,
		DebugEvents:        config.Server.DebugEvents,
		InstallStore:       stores.installStore,
		NamespaceStore:     stores.namespaceStore,
		EventLogStore:      stores.logStore,
		RuleStore:          stores.ruleStore,
		DriverStore:        stores.driverStore,
		RulesStrict:        config.RulesStrict,
		DynamicDriverCache: caches.dynamicDriverCache,
	}

	for _, provider := range webhookRegistry.Providers() {
		providerCfg, ok := config.Providers.ProviderConfigFor(provider.Name())
		if !ok {
			continue
		}
		handler, err := provider.NewHandler(providerCfg, webhookOpts)
		if err != nil {
			return fail(fmt.Errorf("%s handler: %w", provider.Name(), err))
		}
		path := provider.WebhookPath(providerCfg)
		mux.Handle(path, handler)
		oauthCallback := ""
		if oauthProvider, ok := oauthRegistry.Provider(provider.Name()); ok {
			oauthCallback = oauthProvider.CallbackPath()
		}
		extra := strings.TrimSpace(provider.WebhookLogFields(providerCfg))
		if extra != "" {
			extra = " " + extra
		}
		logger.Printf("provider=%s webhook=enabled path=%s oauth_callback=%s%s", provider.Name(), path, oauthCallback, extra)
	}

	redirectBase := config.RedirectBaseURL
	oauthOpts := oauth.HandlerOptions{
		Providers:             config.Providers,
		Store:                 stores.installStore,
		NamespaceStore:        stores.namespaceStore,
		ProviderInstanceStore: stores.instanceStore,
		ProviderInstanceCache: caches.instanceCache,
		Logger:                logger,
		RedirectBase:          redirectBase,
		Endpoint:              config.Endpoint,
	}
	for _, provider := range oauthRegistry.Providers() {
		providerCfg, ok := config.Providers.ProviderConfigFor(provider.Name())
		if !ok {
			continue
		}
		mux.Handle(provider.CallbackPath(), provider.NewHandler(providerCfg, oauthOpts))
	}

	corsHandler := cors.New(cors.Options{
		AllowedMethods: []string{
			http.MethodHead,
			http.MethodGet,
			http.MethodPost,
			http.MethodPut,
			http.MethodPatch,
			http.MethodDelete,
		},
		AllowOriginFunc: func(_ string) bool { return true },
		AllowedHeaders:  []string{"*"},
		ExposedHeaders: []string{
			"Accept",
			"Accept-Encoding",
			"Accept-Post",
			"Connect-Accept-Encoding",
			"Connect-Content-Encoding",
			"Content-Encoding",
			"Grpc-Accept-Encoding",
			"Grpc-Encoding",
			"Grpc-Message",
			"Grpc-Status",
			"Grpc-Status-Details-Bin",
		},
		MaxAge: int(2 * time.Hour / time.Second),
	})
	appHandler := applyMiddlewares(mux, middlewares)
	appHandler = requestLogMiddleware(logger)(appHandler)
	handler := h2c.NewHandler(corsHandler.Handler(appHandler), &http2.Server{})

	return handler, cleanup, nil
}
