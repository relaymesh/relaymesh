package worker

import (
	"strings"

	"github.com/relaymesh/relaymesh/pkg/auth"
)

// Option is a function that configures a Worker.
type Option func(*Worker)

// WithSubscriber sets the Relaybus subscriber for the worker.
func WithSubscriber(sub Subscriber) Option {
	return func(w *Worker) {
		w.subscriber = sub
	}
}

// WithConcurrency sets the number of concurrent message processors.
func WithConcurrency(n int) Option {
	return func(w *Worker) {
		if n > 0 {
			w.concurrency = n
		}
	}
}

// WithCodec sets the codec for decoding messages.
func WithCodec(c Codec) Option {
	return func(w *Worker) {
		if c != nil {
			w.codec = c
		}
	}
}

// WithMiddleware adds middleware to the worker's handler chain.
func WithMiddleware(mw ...Middleware) Option {
	return func(w *Worker) {
		w.middleware = append(w.middleware, mw...)
	}
}

// WithRetry sets the retry policy for the worker.
func WithRetry(policy RetryPolicy) Option {
	return func(w *Worker) {
		if policy != nil {
			w.retry = policy
		}
	}
}

func WithRetryCount(count int) Option {
	return func(w *Worker) {
		if count < 0 {
			count = 0
		}
		w.retryCount = count
	}
}

// WithLogger sets the logger for the worker.
func WithLogger(l Logger) Option {
	return func(w *Worker) {
		if l != nil {
			w.logger = l
		}
	}
}

// WithClientProvider sets the client provider for the worker.
func WithClientProvider(provider ClientProvider) Option {
	return func(w *Worker) {
		w.clientProvider = provider
	}
}

// WithListener adds a listener to the worker.
func WithListener(listener Listener) Option {
	return func(w *Worker) {
		w.listeners = append(w.listeners, listener)
	}
}

// WithEndpoint sets the API endpoint for driver/rules lookups.
func WithEndpoint(endpoint string) Option {
	return func(w *Worker) {
		w.endpoint = strings.TrimSpace(endpoint)
	}
}

// WithAPIKey sets the API key used for API calls.
func WithAPIKey(key string) Option {
	return func(w *Worker) {
		w.apiKey = strings.TrimSpace(key)
	}
}

// WithOAuth2Config sets OAuth2 client credentials for API calls.
func WithOAuth2Config(cfg auth.OAuth2Config) Option {
	return func(w *Worker) {
		copyCfg := cfg
		w.oauth2Config = &copyCfg
	}
}

// WithTenant sets the tenant ID used by the worker when calling control-plane APIs.
func WithTenant(tenant string) Option {
	return func(w *Worker) {
		w.tenantID = strings.TrimSpace(tenant)
	}
}
