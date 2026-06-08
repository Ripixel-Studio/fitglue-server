package router_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/cloudevents/sdk-go/v2/event"
	"google.golang.org/protobuf/encoding/protojson"

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
	updateErr error
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
func (m *mockRouterStore) UpdatePipelineRun(_ context.Context, _, _ string, _ map[string]interface{}) error {
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
