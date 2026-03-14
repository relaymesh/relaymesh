package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/relaymesh/relaymesh/pkg/core"
)

// RunConfig loads config from a path and starts the server with signal handling.
func RunConfig(configPath string) error {
	logger := core.NewLogger("server")
	config, err := core.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()
	return Run(ctx, config, logger)
}

// Run starts the server until the context is canceled.
func Run(ctx context.Context, config core.Config, logger *log.Logger) error {
	return RunWithMiddleware(ctx, config, logger)
}

// RunWithMiddleware starts the server with HTTP middleware applied to all handlers.
func RunWithMiddleware(ctx context.Context, config core.Config, logger *log.Logger, middlewares ...Middleware) error {
	if logger == nil {
		logger = core.NewLogger("server")
	}
	handler, cleanup, err := BuildHandler(ctx, config, logger, middlewares...)
	if err != nil {
		return err
	}
	defer cleanup()

	addr := ":" + strconv.Itoa(config.Server.Port)
	if config.Endpoint != "" {
		logger.Printf("server endpoint=%s", config.Endpoint)
	}
	server := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       time.Duration(config.Server.ReadTimeoutMS) * time.Millisecond,
		WriteTimeout:      time.Duration(config.Server.WriteTimeoutMS) * time.Millisecond,
		IdleTimeout:       time.Duration(config.Server.IdleTimeoutMS) * time.Millisecond,
		ReadHeaderTimeout: time.Duration(config.Server.ReadHeaderMS) * time.Millisecond,
	}

	errCh := make(chan error, 1)
	go func() {
		logger.Printf("listening on %s", addr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Printf("shutdown: %v", err)
		}
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("listen: %w", err)
		}
		return nil
	}
}
