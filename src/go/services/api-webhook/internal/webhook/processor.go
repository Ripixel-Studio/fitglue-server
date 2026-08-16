package webhook

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	"github.com/fitglue/server/src/go/pkg/loopprevention"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/google/uuid"
)

// WebhookEvent represents a normalized event across all providers
type WebhookEvent struct {
	Provider    string
	ProviderUID string // The provider's internal user ID
	ActivityID  string // The external activity ID
	Event       string // "create", "update", "delete"
	RawPayload  []byte // The raw JSON body
}

// SourceProvider is the interface implemented by each integration
// to handle verification, parsing, and user resolution.
type SourceProvider interface {
	// ID returns the provider identifier (e.g. "strava", "fitbit")
	ID() string

	// VerifySubscription handles provider-specific GET/POST verification challenges
	VerifySubscription(w http.ResponseWriter, r *http.Request)

	// ParseEvent validates the incoming POST signature/payload and extracts uniform event data
	ParseEvent(r *http.Request) ([]*WebhookEvent, error)

	// FetchActivity retrieves the full payload from the provider's API.
	FetchActivity(ctx context.Context, userSvc userpb.UserServiceClient, internalUserID string, evt *WebhookEvent) (*pbevents.ActivityPayload, error)
}

// Publisher defines the outbound event bus interface
type Publisher interface {
	PublishCloudEvent(ctx context.Context, topicID string, e event.Event) (string, error)
}

// defaultEventTimeout bounds the fetch/bounceback/publish pipeline for a single
// webhook event. It is applied per event, not per batch — see processEvents.
const defaultEventTimeout = 30 * time.Second

// Processor manages routing webhooks to the correct SourceProvider
type Processor struct {
	providers map[string]SourceProvider
	userSvc   userpb.UserServiceClient
	publisher Publisher
	db        loopprevention.UploadedActivityStore
	logger    infra.Logger

	// eventTimeout bounds the pipeline for a single event (see processEvent).
	eventTimeout time.Duration

	// wg tracks in-flight background event processing spawned by HandleEvent.
	// It lets callers (tests, graceful shutdown) wait for detached work to drain.
	wg sync.WaitGroup
}

// Option customises a Processor at construction time.
type Option func(*Processor)

// WithEventTimeout overrides the per-event pipeline timeout. Primarily for tests
// that need a short deadline; production uses defaultEventTimeout.
func WithEventTimeout(d time.Duration) Option {
	return func(p *Processor) { p.eventTimeout = d }
}

// NewProcessor creates a new WebhookProcessor
func NewProcessor(logger infra.Logger, userSvc userpb.UserServiceClient, publisher Publisher, db loopprevention.UploadedActivityStore, opts ...Option) *Processor {
	p := &Processor{
		providers:    make(map[string]SourceProvider),
		userSvc:      userSvc,
		publisher:    publisher,
		db:           db,
		logger:       logger,
		eventTimeout: defaultEventTimeout,
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Register adds a new SourceProvider to the processor
func (p *Processor) Register(provider SourceProvider) {
	p.providers[provider.ID()] = provider
}

// HandleVerification routes GET requests for webhook subscription challenges
func (p *Processor) HandleVerification(w http.ResponseWriter, r *http.Request, providerID string) {
	p.logger.Info(r.Context(), "Received webhook verification challenge", "provider", providerID)
	provider, ok := p.providers[providerID]
	if !ok {
		p.logger.Error(r.Context(), "Unknown provider for verification", "provider", providerID)
		http.Error(w, "Unknown provider", http.StatusNotFound)
		return
	}
	provider.VerifySubscription(w, r)
}

// HandleEvent routes POST requests containing webhook payloads
func (p *Processor) HandleEvent(w http.ResponseWriter, r *http.Request, providerID string) {
	provider, ok := p.providers[providerID]
	if !ok {
		p.logger.Error(r.Context(), "Unknown provider for event", "provider", providerID)
		http.Error(w, "Unknown provider", http.StatusNotFound)
		return
	}

	events, err := provider.ParseEvent(r)
	if err != nil {
		p.logger.Warn(r.Context(), "Failed to parse webhook event (returning 400)", "provider", providerID, "error", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if len(events) == 0 {
		p.logger.Info(r.Context(), "Parsed webhook but no events returned", "provider", providerID)
	}

	// Acknowledge immediately — Strava retries if it doesn't receive 200 within 2s,
	// which causes duplicate pipeline runs when FetchActivity is slow (e.g. cold start).
	w.WriteHeader(http.StatusOK)

	if len(events) > 0 {
		p.wg.Add(1)
		go func() {
			defer p.wg.Done()
			p.processEvents(provider, events)
		}()
	}
}

// Wait blocks until all in-flight background event processing started by
// HandleEvent has completed. Because the work is dispatched on detached
// goroutines (so the HTTP handler can acknowledge immediately), tests use this
// to observe results deterministically instead of sleeping, and it also gives
// a shutdown hook a way to drain outstanding work.
func (p *Processor) Wait() {
	p.wg.Wait()
}

// processEvents runs the fetch/bounceback/publish pipeline for each webhook event.
// It runs in a goroutine detached from the HTTP request context.
//
// Each event gets its OWN timeout budget (see processEvent). A single provider
// webhook can carry many events (Fitbit and the mobile source both fan a batch
// out here), and every event makes several sequential network round trips —
// resolve, token refresh, provider fetch, bounceback, publish. A single deadline
// shared across the whole batch meant that once the cumulative latency of the
// earlier events crossed 30s, every remaining event's publish failed with
// context.DeadlineExceeded, which is logged at Error and so captured by Sentry
// (SERVER-6). Scoping the deadline per event removes that cross-event starvation.
func (p *Processor) processEvents(provider SourceProvider, events []*WebhookEvent) {
	for _, evt := range events {
		p.processEvent(provider, evt)
	}
}

// processEvent runs the pipeline for a single webhook event under its own
// timeout. It is a separate method (rather than an inline loop body) so the
// context is cancelled as soon as the event finishes instead of accumulating
// one deferred cancel per event until the whole batch drains.
func (p *Processor) processEvent(provider SourceProvider, evt *WebhookEvent) {
	ctx, cancel := context.WithTimeout(context.Background(), p.eventTimeout)
	defer cancel()

	// 1. Resolve internal user ID
	resolveResp, err := p.userSvc.ResolveUserByIntegration(ctx, &userpb.ResolveUserByIntegrationRequest{
		Provider:    evt.Provider,
		ProviderUid: evt.ProviderUID,
	})
	if err != nil {
		p.logger.Warn(ctx, "Skipping webhook event: User not found or resolve error", "provider", evt.Provider, "provider_uid", evt.ProviderUID, "error", err)
		return
	}

	internalUserID := resolveResp.Profile.UserId

	// 2. Fetch the full activity data using SourceProvider
	activityPayload, err := provider.FetchActivity(ctx, p.userSvc, internalUserID, evt)
	if err != nil {
		p.logger.Warn(ctx, "Skipping webhook event: Failed to fetch activity payload", "provider", evt.Provider, "user_id", internalUserID, "activity_id", evt.ActivityID, "error", err)
		return
	}
	if activityPayload == nil {
		p.logger.Info(ctx, "Webhook event ignored by provider logic (returned nil payload)", "provider", evt.Provider, "user_id", internalUserID, "activity_id", evt.ActivityID)
		return
	}

	// 3. Bounceback check — skip if this activity was uploaded by us
	if activityPayload.StandardizedActivity != nil {
		startTimeUnix := int64(0)
		if st := activityPayload.StandardizedActivity.GetStartTime(); st != nil {
			startTimeUnix = st.AsTime().Unix()
		}
		isBounceback, bbErr := loopprevention.IsBounceback(ctx, p.db, internalUserID, activityPayload.Source, activityPayload.StandardizedActivity.GetExternalId(), startTimeUnix)
		if bbErr != nil {
			p.logger.Warn(ctx, "Bounceback check failed, proceeding", "provider", evt.Provider, "error", bbErr)
		} else if isBounceback {
			p.logger.Info(ctx, "Skipping bounceback activity", "provider", evt.Provider, "activity_id", evt.ActivityID)
			return
		}
	}

	// 4. Assign a unique execution ID so each activity gets its own GCS path.
	// Without this the splitter falls back to "exec-unknown", causing all
	// activities for the same pipeline to share and overwrite one GCS file.
	execID := uuid.NewString()
	activityPayload.PipelineExecutionId = &execID

	// 5. Construct and export the CloudEvent
	ce, err := infrapubsub.NewCloudEvent(
		fmt.Sprintf("/integrations/%s/webhook", evt.Provider),
		"com.fitglue.activity.created",
		activityPayload,
	)
	if err != nil {
		p.logger.Error(ctx, "Failed to pack CloudEvent data", "provider", evt.Provider, "user_id", internalUserID, "error", err)
		return
	}

	msgID, err := p.publisher.PublishCloudEvent(ctx, "topic-raw-activity", ce)
	if err != nil {
		p.logger.Error(ctx, "Failed to publish webhook event to Pub/Sub", "provider", evt.Provider, "user_id", internalUserID, "error", err)
		return
	}

	p.logger.Info(ctx, "Successfully published webhook event to Pipeline payload topic", "provider", evt.Provider, "user_id", internalUserID, "activity_id", evt.ActivityID, "msg_id", msgID)
}
