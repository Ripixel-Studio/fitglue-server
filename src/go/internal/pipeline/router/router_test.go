package router_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/internal/pipeline"
	"github.com/fitglue/server/src/go/internal/pipeline/router"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// =============================================================
// Mocks
// =============================================================

type mockRouterStore struct {
	updateErr  error
	lastUpdate map[string]interface{}
}

func (m *mockRouterStore) ListPipelines(_ context.Context, _ string) ([]*pbpipeline.PipelineConfig, error) {
	return nil, nil
}
func (m *mockRouterStore) GetPipeline(_ context.Context, _, _ string) (*pbpipeline.PipelineConfig, error) {
	return nil, nil
}
func (m *mockRouterStore) CreatePipeline(_ context.Context, _ string, cfg *pbpipeline.PipelineConfig) (*pbpipeline.PipelineConfig, error) {
	return cfg, nil
}
func (m *mockRouterStore) UpdatePipeline(_ context.Context, _ string, cfg *pbpipeline.PipelineConfig) (*pbpipeline.PipelineConfig, error) {
	return cfg, nil
}
func (m *mockRouterStore) DeletePipeline(_ context.Context, _, _ string) error { return nil }
func (m *mockRouterStore) ListPendingInputs(_ context.Context, _ string) ([]*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *mockRouterStore) GetPendingInput(_ context.Context, _, _ string) (*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *mockRouterStore) UpdatePendingInput(_ context.Context, _ string, _ *pbpipeline.PendingInput) error {
	return nil
}
func (m *mockRouterStore) GetPipelineRun(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *mockRouterStore) ListPipelineRuns(_ context.Context, _, _ string, _ int32, _ string, _, _ *time.Time) ([]*pbpipeline.PipelineRun, string, error) {
	return nil, "", nil
}
func (m *mockRouterStore) UpdatePipelineRun(_ context.Context, _, _ string, data map[string]interface{}) error {
	m.lastUpdate = data
	return m.updateErr
}
func (m *mockRouterStore) FindPipelineRunByActivityId(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockRouterStore) FindPipelineRunByPendingInputId(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockRouterStore) FindPipelineRunBySourceActivityID(_ context.Context, _, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockRouterStore) FindAnyPipelineRunBySourceActivityID(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockRouterStore) AdminListPipelineRuns(_ context.Context, _, _, _ string, _ int32) ([]*pbpipeline.PipelineRun, error) {
	return nil, nil
}

var _ pipeline.PipelineStore = (*mockRouterStore)(nil)

type mockRouterPublisher struct {
	published int
	err       error
}

func (m *mockRouterPublisher) PublishCloudEvent(_ context.Context, _ string, _ event.Event) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.published++
	return "msg-id", nil
}

func (m *mockRouterPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error { return nil }

var _ pipeline.Publisher = (*mockRouterPublisher)(nil)

type mockBlobStore struct {
	writeErr error
}

func (m *mockBlobStore) Get(_ context.Context, _ string) ([]byte, error) { return nil, nil }
func (m *mockBlobStore) Write(_ context.Context, _, _ string, _ []byte) error {
	return m.writeErr
}

var _ pipeline.BlobStore = (*mockBlobStore)(nil)

// mockRouterLogger implements infra.Logger with all required methods
type mockRouterLogger struct{}

func (m *mockRouterLogger) Info(_ context.Context, _ string, _ ...any)  {}
func (m *mockRouterLogger) Warn(_ context.Context, _ string, _ ...any)  {}
func (m *mockRouterLogger) Error(_ context.Context, _ string, _ ...any) {}
func (m *mockRouterLogger) Debug(_ context.Context, _ string, _ ...any) {}
func (m *mockRouterLogger) With(_ ...any) infra.Logger                  { return m }

var _ infra.Logger = (*mockRouterLogger)(nil)

// =============================================================
// Helpers
// =============================================================

func makeEnrichedEvent(payload *pbevents.EnrichedActivityEvent) cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType("com.fitglue.activity.enriched")
	e.SetSource("test")
	data, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(payload)
	_ = e.SetData("application/json", json.RawMessage(data))
	return e
}

// =============================================================
// Tests
// =============================================================

func TestRouteActivity_NoDestinations(t *testing.T) {
	r := router.NewRouter(&mockRouterStore{}, &mockRouterPublisher{}, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	execID := "exec-123"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations:        nil, // no destinations
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRouteActivity_WithDestination(t *testing.T) {
	pub := &mockRouterPublisher{}
	r := router.NewRouter(&mockRouterStore{}, pub, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	execID := "exec-123"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if pub.published != 1 {
		t.Errorf("expected 1 published, got %d", pub.published)
	}
}

func TestRouteActivity_MultipleDestinations(t *testing.T) {
	pub := &mockRouterPublisher{}
	r := router.NewRouter(&mockRouterStore{}, pub, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	execID := "exec-456"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations: []pbplugin.DestinationType{
			pbplugin.DestinationType_DESTINATION_HEVY,
			pbplugin.DestinationType_DESTINATION_INTERVALS,
		},
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if pub.published != 2 {
		t.Errorf("expected 2 published events, got %d", pub.published)
	}
}

func TestRouteActivity_PublisherError_Continue(t *testing.T) {
	pub := &mockRouterPublisher{err: context.DeadlineExceeded}
	r := router.NewRouter(&mockRouterStore{}, pub, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	execID := "exec-123"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error (publisher errors are non-fatal), got %v", err)
	}
}

func TestRouteActivity_InvalidEventData(t *testing.T) {
	r := router.NewRouter(&mockRouterStore{}, &mockRouterPublisher{}, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	e := cloudevents.NewEvent()
	e.SetType("com.fitglue.activity.enriched")
	e.SetSource("test")
	_ = e.SetData("application/json", []byte("NOT VALID PROTOJSON"))

	err := r.RouteActivity(context.Background(), e)
	if err == nil {
		t.Error("expected unmarshal error")
	}
}

func TestRouteActivity_WithActivityDataURI(t *testing.T) {
	pub := &mockRouterPublisher{}
	r := router.NewRouter(&mockRouterStore{}, pub, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	execID := "exec-789"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		ActivityDataUri:     "gs://my-bucket/enriched_events/user1/exec-789.json",
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRouteActivity_NoBucket_SkipsUpload(t *testing.T) {
	pub := &mockRouterPublisher{}
	r := router.NewRouter(&mockRouterStore{}, pub, &mockBlobStore{}, "" /* empty bucket */, &mockRouterLogger{})

	execID := "exec-789"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}

func TestRouteActivity_BlobWriteError_NonFatal(t *testing.T) {
	pub := &mockRouterPublisher{}
	blob := &mockBlobStore{writeErr: context.DeadlineExceeded}
	r := router.NewRouter(&mockRouterStore{}, pub, blob, "my-bucket", &mockRouterLogger{})

	execID := "exec-789"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error (blob errors are non-fatal), got %v", err)
	}
}

// TestRouteActivity_UpdatedAtIsProtoTimestamp guards against a regression where
// updated_at was written via protojson.Format(timestamppb.Now()), which yields a
// quote-wrapped JSON string ("2026-...Z"). Firestore persisted that string verbatim,
// and it then failed to decode back into the PipelineRun proto
// ("invalid google.protobuf.Timestamp value"), wedging the run and breaking every
// magic action (repost / re-run) on the affected activity. updated_at must be a
// proto Timestamp so the Firestore client stores a native timestamp.
func TestRouteActivity_UpdatedAtIsProtoTimestamp(t *testing.T) {
	store := &mockRouterStore{}
	r := router.NewRouter(store, &mockRouterPublisher{}, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	execID := "exec-ts"
	payload := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		PipelineId:          "pipe1",
		PipelineExecutionId: &execID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
	}

	if err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload)); err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if store.lastUpdate == nil {
		t.Fatal("expected UpdatePipelineRun to be called")
	}
	ua, ok := store.lastUpdate["updated_at"]
	if !ok {
		t.Fatal("expected updated_at in update payload")
	}
	if _, isString := ua.(string); isString {
		t.Fatalf("updated_at must not be a string (would corrupt the run on decode), got string %q", ua)
	}
	if _, isTS := ua.(*timestamppb.Timestamp); !isTS {
		t.Fatalf("updated_at must be a *timestamppb.Timestamp, got %T", ua)
	}
}

func TestRouteActivity_MissingExecID(t *testing.T) {
	pub := &mockRouterPublisher{}
	r := router.NewRouter(&mockRouterStore{}, pub, &mockBlobStore{}, "my-bucket", &mockRouterLogger{})

	payload := &pbevents.EnrichedActivityEvent{
		UserId:       "user1",
		PipelineId:   "pipe1",
		Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY},
		// No PipelineExecutionId
	}

	err := r.RouteActivity(context.Background(), makeEnrichedEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
}
