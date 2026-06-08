package splitter_test

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
	"github.com/fitglue/server/src/go/internal/pipeline/splitter"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// =============================================================
// Mock implementations
// =============================================================

type mockSplitterStore struct {
	pipelines []*pbpipeline.PipelineConfig
	err       error
}

func (m *mockSplitterStore) ListPipelines(_ context.Context, _ string) ([]*pbpipeline.PipelineConfig, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.pipelines, nil
}
func (m *mockSplitterStore) GetPipeline(_ context.Context, _, _ string) (*pbpipeline.PipelineConfig, error) {
	return nil, nil
}
func (m *mockSplitterStore) CreatePipeline(_ context.Context, _ string, cfg *pbpipeline.PipelineConfig) (*pbpipeline.PipelineConfig, error) {
	return cfg, nil
}
func (m *mockSplitterStore) UpdatePipeline(_ context.Context, _ string, cfg *pbpipeline.PipelineConfig) (*pbpipeline.PipelineConfig, error) {
	return cfg, nil
}
func (m *mockSplitterStore) DeletePipeline(_ context.Context, _, _ string) error { return nil }
func (m *mockSplitterStore) ListPendingInputs(_ context.Context, _ string) ([]*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *mockSplitterStore) GetPendingInput(_ context.Context, _, _ string) (*pbpipeline.PendingInput, error) {
	return nil, nil
}
func (m *mockSplitterStore) UpdatePendingInput(_ context.Context, _ string, _ *pbpipeline.PendingInput) error {
	return nil
}
func (m *mockSplitterStore) GetPipelineRun(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}
func (m *mockSplitterStore) ListPipelineRuns(_ context.Context, _, _ string, _ int32, _ string, _, _ *time.Time) ([]*pbpipeline.PipelineRun, string, error) {
	return nil, "", nil
}
func (m *mockSplitterStore) UpdatePipelineRun(_ context.Context, _, _ string, _ map[string]interface{}) error {
	return nil
}
func (m *mockSplitterStore) FindPipelineRunByActivityId(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockSplitterStore) FindPipelineRunByPendingInputId(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockSplitterStore) FindPipelineRunBySourceActivityID(_ context.Context, _, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockSplitterStore) FindAnyPipelineRunBySourceActivityID(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
	return nil, nil
}

func (m *mockSplitterStore) AdminListPipelineRuns(_ context.Context, _, _, _ string, _ int32) ([]*pbpipeline.PipelineRun, error) {
	return nil, nil
}

var _ pipeline.PipelineStore = (*mockSplitterStore)(nil)

type mockSplitterPublisher struct {
	published []event.Event
	err       error
}

func (m *mockSplitterPublisher) PublishCloudEvent(_ context.Context, _ string, e event.Event) (string, error) {
	if m.err != nil {
		return "", m.err
	}
	m.published = append(m.published, e)
	return "msg-id", nil
}

func (m *mockSplitterPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error { return nil }

var _ pipeline.Publisher = (*mockSplitterPublisher)(nil)

// mockLogger implements infra.Logger with all required methods
type mockLogger struct{}

func (m *mockLogger) Info(_ context.Context, _ string, _ ...any)  {}
func (m *mockLogger) Warn(_ context.Context, _ string, _ ...any)  {}
func (m *mockLogger) Error(_ context.Context, _ string, _ ...any) {}
func (m *mockLogger) Debug(_ context.Context, _ string, _ ...any) {}
func (m *mockLogger) With(_ ...any) infra.Logger                  { return m }

var _ infra.Logger = (*mockLogger)(nil)

// =============================================================
// Helpers
// =============================================================

func makeEvent(payload *pbevents.ActivityPayload) cloudevents.Event {
	e := cloudevents.NewEvent()
	e.SetType("com.fitglue.activity.raw")
	e.SetSource("test")
	data, _ := protojson.MarshalOptions{UseProtoNames: true}.Marshal(payload)
	_ = e.SetData("application/json", json.RawMessage(data))
	return e
}

// =============================================================
// Tests
// =============================================================

func TestSplitByPipeline_NoPipelines(t *testing.T) {
	store := &mockSplitterStore{pipelines: []*pbpipeline.PipelineConfig{}}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 0 {
		t.Errorf("expected no published events, got %d", len(pub.published))
	}
}

func TestSplitByPipeline_WithMatchingPipeline(t *testing.T) {
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe1", Name: "Hevy Pipe", Source: "SOURCE_HEVY"},
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected 1 published event, got %d", len(pub.published))
	}
}

func TestSplitByPipeline_DisabledPipeline(t *testing.T) {
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe1", Name: "Disabled Pipe", Source: "SOURCE_HEVY", Disabled: true},
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 0 {
		t.Errorf("expected 0 published, got %d", len(pub.published))
	}
}

func TestSplitByPipeline_PipelineIdAlreadySet_PassThrough(t *testing.T) {
	store := &mockSplitterStore{pipelines: []*pbpipeline.PipelineConfig{}}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	pipelineID := "existing-pipe"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
		PipelineId:          &pipelineID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected 1 pass-through published event, got %d", len(pub.published))
	}
}

func TestSplitByPipeline_MissingExecID(t *testing.T) {
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe1", Source: "SOURCE_HEVY"},
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	payload := &pbevents.ActivityPayload{
		UserId: "user1",
		Source: pbactivity.ActivitySource_SOURCE_HEVY,
		// No PipelineExecutionId
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error (uses fallback exec ID), got %v", err)
	}
}

func TestSplitByPipeline_StoreError(t *testing.T) {
	store := &mockSplitterStore{err: context.DeadlineExceeded}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err == nil {
		t.Error("expected error from store failure")
	}
}

func TestSplitByPipeline_PublisherError_Continue(t *testing.T) {
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe1", Source: "SOURCE_HEVY"},
			{Id: "pipe2", Source: "SOURCE_HEVY"},
		},
	}
	pub := &mockSplitterPublisher{err: context.DeadlineExceeded}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	// Publisher errors are non-fatal per-pipeline (continue loop)
	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error (publisher errors are non-fatal), got %v", err)
	}
}

func TestSplitByPipeline_InvalidEventData(t *testing.T) {
	store := &mockSplitterStore{}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	e := cloudevents.NewEvent()
	e.SetType("com.fitglue.activity.raw")
	e.SetSource("test")
	_ = e.SetData("application/json", []byte("INVALID JSON NOT PROTO"))

	err := s.SplitByPipeline(context.Background(), e)
	if err == nil {
		t.Error("expected unmarshal error")
	}
}

func TestSplitByPipeline_MultiplePipelines(t *testing.T) {
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe1", Source: "SOURCE_HEVY"},
			{Id: "pipe2", Source: "SOURCE_HEVY"},
			{Id: "pipe3", Source: "SOURCE_FITBIT"}, // non-matching source
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-123"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 2 {
		t.Errorf("expected 2 published events (matching source only), got %d", len(pub.published))
	}
}

func TestSplitByPipeline_MultiSourcePipeline(t *testing.T) {
	// A pipeline with Sources=["SOURCE_HEVY","SOURCE_STRAVA"] should match either source.
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe-multi", Sources: []string{"SOURCE_HEVY", "SOURCE_STRAVA"}},
			{Id: "pipe-fitbit", Sources: []string{"SOURCE_FITBIT"}}, // non-matching
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-ms"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_STRAVA,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected 1 published event (multi-source pipeline), got %d", len(pub.published))
	}
}

func TestSplitByPipeline_LegacySourceFieldStillMatches(t *testing.T) {
	// Pipelines written before multi-source support use the legacy Source field;
	// the splitter must still route them correctly.
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe-legacy", Source: "SOURCE_HEVY"}, // no Sources field
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-leg"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_HEVY,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected 1 published event for legacy source field, got %d", len(pub.published))
	}
}

func TestSplitByPipeline_ShortFormatSourceMatching(t *testing.T) {
	// Regression: Firestore stores "file_upload" but source.String() returns "SOURCE_FILE_UPLOAD"
	store := &mockSplitterStore{
		pipelines: []*pbpipeline.PipelineConfig{
			{Id: "pipe1", Name: "File Upload Pipe", Source: "file_upload"},
		},
	}
	pub := &mockSplitterPublisher{}
	s := splitter.NewSplitter(store, pub, &mockLogger{})

	execID := "exec-456"
	payload := &pbevents.ActivityPayload{
		UserId:              "user1",
		Source:              pbactivity.ActivitySource_SOURCE_FILE_UPLOAD,
		PipelineExecutionId: &execID,
	}

	err := s.SplitByPipeline(context.Background(), makeEvent(payload))
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}
	if len(pub.published) != 1 {
		t.Errorf("expected 1 published event for short-format source, got %d", len(pub.published))
	}
}
