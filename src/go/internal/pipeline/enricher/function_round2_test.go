// nolint:proto-json
package enricher

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	user "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// installEnricherSvc injects a fully-mocked global bootstrap.Service and a mock
// provider registry, returning a cleanup func. initService() short-circuits
// because svc is non-nil, so no real GCP clients are created.
func installEnricherSvc(t *testing.T) {
	t.Helper()
	mockDB := &mocks.MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "test-pipeline-1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
	}
	mockPub := &mocks.MockPublisher{
		PublishCloudEventFunc: func(ctx context.Context, topic string, e cloudevents.Event) (string, error) {
			return "msg-123", nil
		},
	}
	mockStore := &mocks.MockBlobStore{
		WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error { return nil },
	}

	providers.ClearRegistry()
	providers.Register(&MockProvider{
		NameFunc:         func() string { return "mock-enricher" },
		ProviderTypeFunc: func() pbplugin.EnricherProviderType { return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK },
		EnrichFunc: func(ctx context.Context, _ *slog.Logger, activity *pbactivity.StandardizedActivity, u *user.Record, inputConfig map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
			return &providers.EnrichmentResult{Name: "Mock", Description: "ok"}, nil
		},
	})

	prev := svc
	svc = &bootstrap.Service{
		DB:     mockDB,
		Pub:    mockPub,
		Store:  mockStore,
		Config: &bootstrap.Config{ProjectID: "test-project"},
	}
	t.Cleanup(func() {
		providers.ClearRegistry()
		svc = prev
	})
}

func validPayloadBytes(t *testing.T) []byte {
	t.Helper()
	pipelineID := "test-pipeline-1"
	activity := pbevents.ActivityPayload{
		Source:     pbactivity.ActivitySource_SOURCE_HEVY,
		UserId:     "user_123",
		PipelineId: &pipelineID,
		Timestamp:  timestamppb.New(time.Now()),
		StandardizedActivity: &pbactivity.StandardizedActivity{
			StartTime: timestamppb.New(time.Now()),
			Type:      pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
			Sessions:  []*pbactivity.Session{{TotalElapsedTime: 3600}},
		},
	}
	b, err := protojson.MarshalOptions{EmitUnpopulated: true}.Marshal(&activity)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// TestEnrichActivityHTTP_PubSubPush drives the HTTP entrypoint with a Pub/Sub
// push wrapper whose data is the raw activity payload (not a CloudEvent). This
// exercises the CloudEvents-parse-fail → parseCloudEventFromPubSubPush fallback
// and the success 200 path.
func TestEnrichActivityHTTP_PubSubPush(t *testing.T) {
	installEnricherSvc(t)

	payload := validPayloadBytes(t)
	push := map[string]interface{}{
		"message": map[string]interface{}{
			"data":      base64.StdEncoding.EncodeToString(payload),
			"messageId": "m1",
		},
		"subscription": "projects/p/subscriptions/s",
	}
	body, _ := json.Marshal(push)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	EnrichActivityHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestEnrichActivityHTTP_BadBody exercises the 400 path: a request body that is
// neither a CloudEvent nor a valid Pub/Sub push envelope.
func TestEnrichActivityHTTP_BadBody(t *testing.T) {
	installEnricherSvc(t)

	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader([]byte("not json at all")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	EnrichActivityHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestEnrichActivityHTTP_HandlerError exercises the 500 path: a structurally
// valid CloudEvent whose payload is missing userId, so enrichHandler returns an
// error and the HTTP handler returns 500 for Pub/Sub to NACK.
func TestEnrichActivityHTTP_HandlerError(t *testing.T) {
	installEnricherSvc(t)

	// Empty-object payload → unmarshals fine but userId is empty.
	e := cloudevents.NewEvent()
	e.SetID("e1")
	e.SetType("com.fitglue.activity.created")
	e.SetSource("/test")
	_ = e.SetData(cloudevents.ApplicationJSON, []byte(`{}`))
	ceBytes, _ := json.Marshal(e)

	push := map[string]interface{}{
		"message": map[string]interface{}{
			"data":      base64.StdEncoding.EncodeToString(ceBytes),
			"messageId": "m2",
		},
	}
	body, _ := json.Marshal(push)
	req := httptest.NewRequest(http.MethodPost, "/", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	EnrichActivityHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on handler error, got %d body=%s", w.Code, w.Body.String())
	}
}

// TestEnrichActivity_CloudEventEntry runs the EventArc entrypoint with a valid
// payload through to a published event, covering the EnrichActivity wrapper.
func TestEnrichActivity_CloudEventEntry(t *testing.T) {
	installEnricherSvc(t)

	e := cloudevents.NewEvent()
	e.SetID("e1")
	e.SetType("com.fitglue.activity.created")
	e.SetSource("/test")
	_ = e.SetData(cloudevents.ApplicationJSON, validPayloadBytes(t))

	if err := EnrichActivity(context.Background(), e); err != nil {
		t.Fatalf("EnrichActivity: %v", err)
	}
}

// installEnricherSvcWithProvider is like installEnricherSvc but lets the caller
// supply the enricher behaviour and capture published topics.
func installEnricherSvcWithProvider(t *testing.T, enrich func(ctx context.Context, l *slog.Logger, a *pbactivity.StandardizedActivity, u *user.Record, cfg map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error)) *[]string {
	t.Helper()
	mockDB := &mocks.MockDatabase{
		GetUserFunc: func(ctx context.Context, id string) (*user.Record, error) {
			return &user.Record{UserProfile: &pbuser.UserProfile{UserId: id}}, nil
		},
		GetUserPipelinesFunc: func(ctx context.Context, userId string) ([]*pbpipeline.PipelineConfig, error) {
			return []*pbpipeline.PipelineConfig{{
				Id:           "test-pipeline-1",
				Source:       "SOURCE_HEVY",
				Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
				Enrichers: []*pbpipeline.EnricherConfig{
					{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK},
				},
			}}, nil
		},
	}
	topics := &[]string{}
	mockPub := &mocks.MockPublisher{
		PublishCloudEventFunc: func(ctx context.Context, topic string, e cloudevents.Event) (string, error) {
			*topics = append(*topics, topic)
			return "msg-123", nil
		},
	}
	mockStore := &mocks.MockBlobStore{
		WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error { return nil },
	}

	providers.ClearRegistry()
	providers.Register(&MockProvider{
		NameFunc:         func() string { return "mock-enricher" },
		ProviderTypeFunc: func() pbplugin.EnricherProviderType { return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MOCK },
		EnrichFunc:       enrich,
	})

	prev := svc
	svc = &bootstrap.Service{
		DB:     mockDB,
		Pub:    mockPub,
		Store:  mockStore,
		Config: &bootstrap.Config{ProjectID: "test-project"},
	}
	t.Cleanup(func() {
		providers.ClearRegistry()
		svc = prev
	})
	return topics
}

// TestEnrichActivity_LagRetryOffload covers enrichHandler's retryable-error path:
// a fresh (non-lag) message whose enricher returns a RetryableError is offloaded
// to the lag topic and ACKed.
func TestEnrichActivity_LagRetryOffload(t *testing.T) {
	topics := installEnricherSvcWithProvider(t, func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
		return nil, providers.NewRetryableError(errors.New("lag"), time.Minute, "source not ready")
	})

	e := cloudevents.NewEvent()
	e.SetID("e-lag")
	e.SetType("com.fitglue.activity.created")
	e.SetSource("/test")
	e.SetTime(time.Now()) // fresh, so not force-exhausted
	_ = e.SetData(cloudevents.ApplicationJSON, validPayloadBytes(t))

	// Should ACK (no error) after offloading to the lag topic.
	if err := EnrichActivity(context.Background(), e); err != nil {
		t.Fatalf("expected ACK after lag offload, got %v", err)
	}
	var sawLagTopic bool
	for _, topic := range *topics {
		if topic == shared.TopicEnrichmentLag {
			sawLagTopic = true
		}
	}
	if !sawLagTopic {
		t.Errorf("expected publish to lag topic, got topics=%v", *topics)
	}
}

// TestEnrichActivity_SkippedNoEvents covers the no-events SKIPPED return in
// enrichHandler: an enricher that halts the pipeline yields zero events.
func TestEnrichActivity_SkippedNoEvents(t *testing.T) {
	installEnricherSvcWithProvider(t, func(ctx context.Context, _ *slog.Logger, _ *pbactivity.StandardizedActivity, _ *user.Record, _ map[string]string, _ bool) (*providers.EnrichmentResult, error) {
		return &providers.EnrichmentResult{HaltPipeline: true, HaltReason: "filtered", Metadata: map[string]string{}}, nil
	})

	e := cloudevents.NewEvent()
	e.SetID("e-skip")
	e.SetType("com.fitglue.activity.created")
	e.SetSource("/test")
	_ = e.SetData(cloudevents.ApplicationJSON, validPayloadBytes(t))

	if err := EnrichActivity(context.Background(), e); err != nil {
		t.Fatalf("skipped pipeline should ACK, got %v", err)
	}
}
