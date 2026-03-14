package worker

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"

	relaymessage "github.com/relaymesh/relaybus/sdk/core/go/message"

	"github.com/relaymesh/relaymesh/pkg/auth"
	"github.com/relaymesh/relaymesh/pkg/storage"
)

// Worker is a message-processing worker that subscribes to topics, decodes
// messages, and dispatches them to handlers.
type Worker struct {
	subscriber  Subscriber
	codec       Codec
	retry       RetryPolicy
	logger      Logger
	concurrency int
	retryCount  int
	topics      []string

	topicHandlers  map[string]Handler
	topicDrivers   map[string]string
	typeHandlers   map[string]Handler
	middleware     []Middleware
	clientProvider ClientProvider
	listeners      []Listener
	driverSubs     map[string]Subscriber
	endpoint       string
	apiKey         string
	oauth2Config   *auth.OAuth2Config
	tenantID       string
	ruleHandlers   map[string]Handler
}

// New creates a new Worker with the given options.
func New(opts ...Option) *Worker {
	w := &Worker{
		codec:         DefaultCodec{},
		retry:         NoRetry{},
		logger:        stdLogger{},
		concurrency:   10,
		retryCount:    0,
		topicHandlers: make(map[string]Handler),
		topicDrivers:  make(map[string]string),
		typeHandlers:  make(map[string]Handler),
		driverSubs:    make(map[string]Subscriber),
		tenantID:      envTenantID(),
		ruleHandlers:  make(map[string]Handler),
	}
	for _, opt := range opts {
		opt(w)
	}
	log.Printf("worker tenant context=%q", w.tenantIDValue())
	w.bindClientProvider()
	return w
}

// bindClientProvider propagates the worker's API settings into providers that opt in.
func (w *Worker) bindClientProvider() {
	if w == nil || w.clientProvider == nil {
		return
	}
	if binder, ok := w.clientProvider.(apiClientBinder); ok {
		binder.BindAPIClient(apiClientConfig{
			BaseURL: w.apiBaseURL(),
			APIKey:  w.apiKeyValue(),
			OAuth2:  w.oauth2Value(),
		})
	}
}

// HandleRule registers a handler for the specified rule id.
func (w *Worker) HandleRule(ruleID string, h Handler) {
	if h == nil {
		return
	}
	ruleID = strings.TrimSpace(ruleID)
	if ruleID == "" {
		return
	}
	w.ruleHandlers[ruleID] = h
}

// HandleType registers a handler for a specific event type.
func (w *Worker) HandleType(eventType string, h Handler) {
	if h == nil || eventType == "" {
		return
	}
	w.typeHandlers[eventType] = h
}

// Run starts the worker, subscribing to topics and processing messages.
// It blocks until the context is canceled.
func (w *Worker) Run(ctx context.Context) error {
	if err := w.prepareRuleSubscriptions(ctx); err != nil {
		return err
	}
	if len(w.topics) == 0 {
		return errors.New("at least one topic is required")
	}
	if w.subscriber != nil {
		return w.runWithSubscriber(ctx, w.subscriber, unique(w.topics))
	}

	driverTopics, err := w.topicsByDriver()
	if err != nil {
		return err
	}
	if err := w.buildDriverSubscribers(ctx, driverTopics); err != nil {
		return err
	}

	w.notifyStart(ctx)
	defer w.notifyExit(ctx)
	sem := make(chan struct{}, w.concurrency)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(driverTopics))
	var wg sync.WaitGroup

	for driverID, topics := range driverTopics {
		sub := w.driverSubs[driverID]
		if sub == nil {
			return fmt.Errorf("subscriber not initialized for driver: %s", driverID)
		}
		for _, topic := range unique(topics) {
			driverID := driverID
			topic := topic
			wg.Add(1)
			go func(sub Subscriber) {
				defer wg.Done()
				err := sub.Start(ctx, topic, func(ctx context.Context, msg relaymessage.Message) error {
					sem <- struct{}{}
					defer func() { <-sem }()
					shouldNack := w.handleMessage(ctx, topic, &msg)
					if shouldNack && shouldRequeue(&msg) {
						return errRelaybusNack
					}
					return nil
				})
				if err != nil && ctx.Err() == nil {
					w.notifyError(ctx, nil, err)
					errCh <- fmt.Errorf("driver %s topic %s: %w", driverID, topic, err)
					cancel()
				}
			}(sub)
		}
	}

	<-ctx.Done()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (w *Worker) runWithSubscriber(ctx context.Context, sub Subscriber, topics []string) error {
	if sub == nil {
		return errors.New("subscriber is required")
	}

	w.notifyStart(ctx)
	defer w.notifyExit(ctx)
	sem := make(chan struct{}, w.concurrency)

	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	errCh := make(chan error, len(topics))
	var wg sync.WaitGroup

	for _, topic := range topics {
		topic := topic
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := sub.Start(ctx, topic, func(ctx context.Context, msg relaymessage.Message) error {
				sem <- struct{}{}
				defer func() { <-sem }()
				shouldNack := w.handleMessage(ctx, topic, &msg)
				if shouldNack && shouldRequeue(&msg) {
					return errRelaybusNack
				}
				return nil
			})
			if err != nil && ctx.Err() == nil {
				errCh <- err
				cancel()
			}
		}()
	}

	<-ctx.Done()
	wg.Wait()

	select {
	case err := <-errCh:
		return err
	default:
		return nil
	}
}

func (w *Worker) topicsByDriver() (map[string][]string, error) {
	if len(w.topicDrivers) == 0 {
		return nil, errors.New("driver id is required for topics")
	}
	out := make(map[string][]string, len(w.topicDrivers))
	for topic, driverID := range w.topicDrivers {
		trimmed := strings.TrimSpace(driverID)
		if trimmed == "" {
			return nil, fmt.Errorf("driver id is required for topic: %s", topic)
		}
		out[trimmed] = append(out[trimmed], topic)
	}
	return out, nil
}

func (w *Worker) buildDriverSubscribers(ctx context.Context, driverTopics map[string][]string) error {
	for driverID := range driverTopics {
		if _, ok := w.driverSubs[driverID]; ok {
			continue
		}
		tenantID := w.tenantIDValue()
		log.Printf("worker building driver subscriber driver=%s tenant=%s", driverID, tenantID)
		tenantCtx := storage.WithTenant(ctx, tenantID)
		record, err := w.driversClient().GetDriverByID(tenantCtx, driverID)
		if err != nil {
			return err
		}
		if record == nil {
			return fmt.Errorf("driver not found: %s", driverID)
		}
		if !record.Enabled {
			return fmt.Errorf("driver is disabled: %s", driverID)
		}
		cfg, err := SubscriberConfigFromDriver(record.Name, record.ConfigJSON)
		if err != nil {
			return err
		}
		sub, err := BuildSubscriber(cfg)
		if err != nil {
			return err
		}
		w.driverSubs[driverID] = sub
	}
	return nil
}

// Close gracefully shuts down the worker and its subscriber.
func (w *Worker) Close() error {
	if w.subscriber == nil {
		for _, sub := range w.driverSubs {
			if sub == nil {
				continue
			}
			if err := sub.Close(); err != nil {
				return err
			}
		}
		return nil
	}
	return w.subscriber.Close()
}

func (w *Worker) handleMessage(ctx context.Context, topic string, msg *relaymessage.Message) bool {
	logID := ""
	if msg != nil {
		if msg.Metadata != nil {
			logID = msg.Metadata[MetadataKeyLogID]
			if tenantID := strings.TrimSpace(msg.Metadata[MetadataKeyTenantID]); tenantID != "" {
				ctx = WithTenantID(ctx, tenantID)
			}
		}
	}
	evt, err := w.codec.Decode(topic, msg)
	if err != nil {
		w.logger.Printf("decode failed: %v", err)
		w.updateEventLogStatus(ctx, logID, EventLogStatusFailed, err)
		w.notifyError(ctx, nil, err)
		decision := w.retry.OnError(ctx, nil, err)
		return decision.Retry || decision.Nack
	}

	if w.clientProvider != nil {
		client, err := w.clientProvider.Client(ctx, evt)
		if err != nil {
			w.logger.Printf("client init failed: %v", err)
			w.updateEventLogStatus(ctx, logID, EventLogStatusFailed, err)
			w.notifyError(ctx, evt, err)
			decision := w.retry.OnError(ctx, evt, err)
			return decision.Retry || decision.Nack
		}
		evt.Client = client
	}

	if reqID := evt.Metadata[MetadataKeyRequestID]; reqID != "" {
		w.logger.Printf("request_id=%s topic=%s provider=%s type=%s", reqID, evt.Topic, evt.Provider, evt.Type)
	}

	w.notifyMessageStart(ctx, evt)

	handler := w.topicHandlers[topic]
	if handler == nil {
		handler = w.typeHandlers[evt.Type]
	}
	if handler == nil {
		w.logger.Printf("no handler for topic=%s type=%s", topic, evt.Type)
		w.notifyMessageFinish(ctx, evt, nil)
		w.updateEventLogStatus(ctx, logID, EventLogStatusSuccess, nil)
		return false
	}

	wrapped := w.wrap(handler)
	var handlerErr error
	attempts := w.retryCount + 1
	for i := 0; i < attempts; i++ {
		handlerErr = wrapped(ctx, evt)
		if handlerErr == nil {
			break
		}
	}
	if handlerErr != nil {
		w.notifyMessageFinish(ctx, evt, handlerErr)
		w.notifyError(ctx, evt, handlerErr)
		w.updateEventLogStatus(ctx, logID, EventLogStatusFailed, handlerErr)
		decision := w.retry.OnError(ctx, evt, handlerErr)
		return decision.Retry || decision.Nack
	}
	w.notifyMessageFinish(ctx, evt, nil)
	w.updateEventLogStatus(ctx, logID, EventLogStatusSuccess, nil)
	return false
}

func (w *Worker) wrap(h Handler) Handler {
	wrapped := h
	for i := len(w.middleware) - 1; i >= 0; i-- {
		wrapped = w.middleware[i](wrapped)
	}
	return wrapped
}

func unique(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

var errRelaybusNack = errors.New("message nack requested")

func shouldRequeue(msg *relaymessage.Message) bool {
	if msg == nil || msg.Metadata == nil {
		return false
	}
	return strings.ToLower(msg.Metadata[MetadataKeyDriver]) == "amqp"
}

func (w *Worker) prepareRuleSubscriptions(ctx context.Context) error {
	if len(w.ruleHandlers) == 0 {
		return nil
	}
	client := w.rulesClient()
	for ruleID, handler := range w.ruleHandlers {
		if handler == nil {
			continue
		}
		record, err := client.GetRule(ctx, ruleID)
		if err != nil {
			return err
		}
		if len(record.Emit) == 0 {
			return fmt.Errorf("rule %s has no emit topic", ruleID)
		}
		topic := strings.TrimSpace(record.Emit[0])
		if topic == "" {
			return fmt.Errorf("rule %s emit topic empty", ruleID)
		}
		driverID := strings.TrimSpace(record.DriverID)
		if driverID == "" {
			return fmt.Errorf("rule %s driver_id is required", ruleID)
		}
		if _, ok := w.topicDrivers[topic]; ok {
			w.logger.Printf("overwriting handler for topic=%s due to rule=%s", topic, ruleID)
		}
		w.topicHandlers[topic] = handler
		w.topicDrivers[topic] = driverID
		w.topics = append(w.topics, topic)
	}
	return nil
}

func (w *Worker) notifyStart(ctx context.Context) {
	for _, listener := range w.listeners {
		if listener.OnStart != nil {
			listener.OnStart(ctx)
		}
	}
}

func (w *Worker) notifyExit(ctx context.Context) {
	for _, listener := range w.listeners {
		if listener.OnExit != nil {
			listener.OnExit(ctx)
		}
	}
}

func (w *Worker) notifyMessageStart(ctx context.Context, evt *Event) {
	for _, listener := range w.listeners {
		if listener.OnMessageStart != nil {
			listener.OnMessageStart(ctx, evt)
		}
	}
}

func (w *Worker) notifyMessageFinish(ctx context.Context, evt *Event, err error) {
	for _, listener := range w.listeners {
		if listener.OnMessageFinish != nil {
			listener.OnMessageFinish(ctx, evt, err)
		}
	}
}

func (w *Worker) notifyError(ctx context.Context, evt *Event, err error) {
	for _, listener := range w.listeners {
		if listener.OnError != nil {
			listener.OnError(ctx, evt, err)
		}
	}
}
