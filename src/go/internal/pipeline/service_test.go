// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// MockStore
type MockPipelineStore struct {
	Pipelines     map[string]*pipeline.PipelineConfig
	PendingInputs map[string]*pipeline.PendingInput
	Runs          map[string]*pipeline.PipelineRun
}

func NewMockStore() *MockPipelineStore {
	return &MockPipelineStore{
		Pipelines:     make(map[string]*pipeline.PipelineConfig),
		PendingInputs: make(map[string]*pipeline.PendingInput),
		Runs:          make(map[string]*pipeline.PipelineRun),
	}
}

func (m *MockPipelineStore) key(userID, id string) string {
	return userID + "_" + id
}

func (m *MockPipelineStore) ListPipelines(ctx context.Context, userID string) ([]*pipeline.PipelineConfig, error) {
	var results []*pipeline.PipelineConfig
	for _, p := range m.Pipelines {
		// Just a mock, we don't strictly filter by user here since tests will isolate
		results = append(results, p)
	}
	return results, nil
}

func (m *MockPipelineStore) GetPipeline(ctx context.Context, userID, pipelineID string) (*pipeline.PipelineConfig, error) {
	return m.Pipelines[m.key(userID, pipelineID)], nil
}

func (m *MockPipelineStore) CreatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error) {
	m.Pipelines[m.key(userID, cfg.Id)] = cfg
	return cfg, nil
}

func (m *MockPipelineStore) UpdatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error) {
	m.Pipelines[m.key(userID, cfg.Id)] = cfg
	return cfg, nil
}

func (m *MockPipelineStore) DeletePipeline(ctx context.Context, userID, pipelineID string) error {
	delete(m.Pipelines, m.key(userID, pipelineID))
	return nil
}

func (m *MockPipelineStore) ListPendingInputs(ctx context.Context, userID string) ([]*pipeline.PendingInput, error) {
	var results []*pipeline.PendingInput
	for _, p := range m.PendingInputs {
		results = append(results, p)
	}
	return results, nil
}

func (m *MockPipelineStore) GetPendingInput(ctx context.Context, userID, inputID string) (*pipeline.PendingInput, error) {
	return m.PendingInputs[m.key(userID, inputID)], nil
}

func (m *MockPipelineStore) UpdatePendingInput(ctx context.Context, userID string, input *pipeline.PendingInput) error {
	m.PendingInputs[m.key(userID, input.ActivityId)] = input
	return nil
}

func (m *MockPipelineStore) GetPipelineRun(ctx context.Context, userID, runID string) (*pipeline.PipelineRun, error) {
	return m.Runs[m.key(userID, runID)], nil
}

func (m *MockPipelineStore) FindPipelineRunByActivityId(ctx context.Context, userID, activityID string) (*pipeline.PipelineRun, error) {
	for _, run := range m.Runs {
		if run.ActivityId == activityID {
			return run, nil
		}
	}
	return nil, nil
}

func (m *MockPipelineStore) FindPipelineRunByPendingInputId(ctx context.Context, userID, pendingInputID string) (*pipeline.PipelineRun, error) {
	for _, run := range m.Runs {
		if run.PendingInputId != nil && *run.PendingInputId == pendingInputID {
			return run, nil
		}
	}
	return nil, nil
}

func (m *MockPipelineStore) FindPipelineRunBySourceActivityID(ctx context.Context, userID, pipelineID, sourceActivityID string) (*pipeline.PipelineRun, error) {
	for _, run := range m.Runs {
		if run.PipelineId == pipelineID && run.SourceActivityId == sourceActivityID {
			return run, nil
		}
	}
	return nil, nil
}

func (m *MockPipelineStore) FindAnyPipelineRunBySourceActivityID(ctx context.Context, userID, sourceActivityID string) (*pipeline.PipelineRun, error) {
	for _, run := range m.Runs {
		if run.SourceActivityId == sourceActivityID {
			return run, nil
		}
	}
	return nil, nil
}

func (m *MockPipelineStore) ListPipelineRuns(ctx context.Context, userID, pipelineID string, limit int32, pageToken string, since, until *time.Time) ([]*pipeline.PipelineRun, string, error) {
	var results []*pipeline.PipelineRun
	for _, r := range m.Runs {
		if pipelineID == "" || r.PipelineId == pipelineID {
			results = append(results, r)
		}
	}
	return results, "", nil
}

func (m *MockPipelineStore) UpdatePipelineRun(ctx context.Context, userID, runID string, updateData map[string]interface{}) error {
	// For a mock, we can just update the internal map or do nothing.
	return nil
}

func (m *MockPipelineStore) AdminListPipelineRuns(ctx context.Context, status, source, userID string, limit int32) ([]*pipeline.PipelineRun, error) {
	return nil, nil
}

// MockPublisher
type MockPublisher struct {
	PublishedEvents []cloudevents.Event
}

func (m *MockPublisher) PublishCloudEvent(ctx context.Context, topic string, ce cloudevents.Event) (string, error) {
	m.PublishedEvents = append(m.PublishedEvents, ce)
	return fmt.Sprintf("msg_%d", len(m.PublishedEvents)), nil
}

func (m *MockPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error { return nil }

// MockBlobStore
type MockBlobStore struct {
	Blobs   map[string][]byte
	GetFn   func(ctx context.Context, uri string) ([]byte, error)
	WriteFn func(ctx context.Context, bucket, path string, data []byte) error
}

func (m *MockBlobStore) Get(ctx context.Context, uri string) ([]byte, error) {
	if m.GetFn != nil {
		return m.GetFn(ctx, uri)
	}
	b, ok := m.Blobs[uri]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return b, nil
}

func (m *MockBlobStore) Write(ctx context.Context, bucket, path string, data []byte) error {
	if m.WriteFn != nil {
		return m.WriteFn(ctx, bucket, path, data)
	}
	// Default mock behavior: store in Blobs map
	m.Blobs[fmt.Sprintf("gs://%s/%s", bucket, path)] = data
	return nil
}

// mockLogger
type mockLogger struct{}

func (m mockLogger) Debug(ctx context.Context, msg string, args ...any) {}
func (m mockLogger) Info(ctx context.Context, msg string, args ...any)  {}
func (m mockLogger) Warn(ctx context.Context, msg string, args ...any)  {}
func (m mockLogger) Error(ctx context.Context, msg string, args ...any) {}
func (m mockLogger) With(args ...any) infra.Logger                      { return m }

func TestPipelineCRUD(t *testing.T) {
	store := NewMockStore()
	publisher := &MockPublisher{}
	blobStore := &MockBlobStore{}
	svc := NewService(store, publisher, blobStore, mockLogger{}, nil)
	ctx := context.Background()

	// Create
	req := &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Name:         "My Pipeline",
			Source:       "SOURCE_STRAVA",
			Destinations: []plugin.DestinationType{1}, // Using int value for enum
		},
	}
	res, err := svc.CreatePipeline(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Id == "" {
		t.Errorf("expected pipeline ID to be generated")
	}
	// Legacy Source field should be migrated into Sources; Source cleared.
	if len(res.Sources) != 1 || res.Sources[0] != "SOURCE_STRAVA" {
		t.Errorf("expected Sources=[SOURCE_STRAVA], got %v", res.Sources)
	}
	if res.Source != "" {
		t.Errorf("expected Source to be cleared, got %q", res.Source)
	}

	pipelineID := res.Id

	// Get
	getReq := &pbsvc.GetPipelineRequest{
		UserId:     "user1",
		PipelineId: pipelineID,
	}
	getRes, err := svc.GetPipeline(ctx, getReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if getRes.Name != "My Pipeline" {
		t.Errorf("expected name 'My Pipeline', got %v", getRes.Name)
	}

	// Update
	updReq := &pbsvc.UpdatePipelineRequest{
		UserId:     "user1",
		PipelineId: pipelineID,
		Pipeline: &pipeline.PipelineConfig{
			Id:           pipelineID,
			Name:         "Updated Pipeline",
			Source:       "SOURCE_STRAVA",
			Destinations: []plugin.DestinationType{1, 2}, // Using int values
		},
	}
	updRes, err := svc.UpdatePipeline(ctx, updReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updRes.Name != "Updated Pipeline" {
		t.Errorf("expected name 'Updated Pipeline', got %v", updRes.Name)
	}

	// Delete
	delReq := &pbsvc.DeletePipelineRequest{
		UserId:     "user1",
		PipelineId: pipelineID,
	}
	_, err = svc.DeletePipeline(ctx, delReq)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify Delete
	_, err = svc.GetPipeline(ctx, getReq)
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestCreatePipelineWithMultipleSources(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	ctx := context.Background()

	res, err := svc.CreatePipeline(ctx, &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Name:         "Multi-Source Pipeline",
			Sources:      []string{"SOURCE_HEVY", "SOURCE_STRAVA"},
			Destinations: []plugin.DestinationType{1},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Sources) != 2 {
		t.Fatalf("expected 2 sources, got %d: %v", len(res.Sources), res.Sources)
	}
	if res.Source != "" {
		t.Errorf("expected legacy Source to be cleared, got %q", res.Source)
	}
}

func TestCreatePipelineRejectsDuplicateSources(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	ctx := context.Background()

	_, err := svc.CreatePipeline(ctx, &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Sources:      []string{"SOURCE_HEVY", "SOURCE_HEVY"},
			Destinations: []plugin.DestinationType{1},
		},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for duplicate sources, got %v", err)
	}
}

func TestUpdatePipelineToMultipleSources(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)
	ctx := context.Background()

	created, err := svc.CreatePipeline(ctx, &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Sources:      []string{"SOURCE_HEVY"},
			Destinations: []plugin.DestinationType{1},
		},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}

	updated, err := svc.UpdatePipeline(ctx, &pbsvc.UpdatePipelineRequest{
		UserId:     "user1",
		PipelineId: created.Id,
		Pipeline: &pipeline.PipelineConfig{
			Sources: []string{"SOURCE_HEVY", "SOURCE_STRAVA"},
		},
	})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if len(updated.Sources) != 2 {
		t.Errorf("expected 2 sources after update, got %v", updated.Sources)
	}
	if updated.Source != "" {
		t.Errorf("expected Source cleared after update, got %q", updated.Source)
	}
}

func TestSubmitInput(t *testing.T) {
	store := NewMockStore()
	publisher := &MockPublisher{}

	payload := map[string]interface{}{"foo": "bar"}
	payloadBytes, _ := json.Marshal(payload)

	blobStore := &MockBlobStore{
		Blobs: map[string][]byte{
			"gs://bucket/path.json": payloadBytes,
		},
	}

	svc := NewService(store, publisher, blobStore, mockLogger{}, nil)
	ctx := context.Background()

	// Setup pending input
	store.PendingInputs["user1_input1"] = &pipeline.PendingInput{
		ActivityId:         "input1",
		Status:             pipeline.PendingInput_STATUS_WAITING,
		OriginalPayloadUri: "gs://bucket/path.json",
		LinkedActivityId:   "activity1",
	}

	req := &pbsvc.SubmitInputRequest{
		UserId:         "user1",
		PendingInputId: "input1",
		InputData:      map[string]string{"answer": "42"},
	}

	_, err := svc.SubmitInput(ctx, req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify DB state
	input, _ := store.GetPendingInput(ctx, "user1", "input1")
	if input.Status != pipeline.PendingInput_STATUS_COMPLETED {
		t.Errorf("expected status COMPLETED, got %v", input.Status)
	}
	if input.InputData["answer"] != "42" {
		t.Errorf("expected input data saved")
	}

	// Verify Pub/Sub
	if len(publisher.PublishedEvents) != 1 {
		t.Fatalf("expected 1 event published, got %d", len(publisher.PublishedEvents))
	}

	eventData := publisher.PublishedEvents[0].Data()
	var publishedPayload map[string]interface{}
	json.Unmarshal(eventData, &publishedPayload)

	if publishedPayload["isResume"] != true {
		t.Errorf("expected isResume=true in published payload")
	}
	if publishedPayload["resumePendingInputId"] != "input1" {
		t.Errorf("expected resumePendingInputId=input1")
	}
	if publishedPayload["activityId"] != "activity1" {
		t.Errorf("expected activityId=activity1")
	}
}

func TestCreatePipeline_InvalidSource(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)

	req := &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Name:         "Bad Source",
			Source:       "banana",
			Destinations: []plugin.DestinationType{1},
		},
	}

	_, err := svc.CreatePipeline(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid source")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestCreatePipeline_EmptySource(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)

	req := &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Name:         "No Source",
			Source:       "",
			Destinations: []plugin.DestinationType{1},
		},
	}

	_, err := svc.CreatePipeline(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for empty source")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestCreatePipeline_NormalizesShortSource(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)

	req := &pbsvc.CreatePipelineRequest{
		UserId: "user1",
		Pipeline: &pipeline.PipelineConfig{
			Name:         "Short Source",
			Source:       "file_upload", // Short format from UI
			Destinations: []plugin.DestinationType{1},
		},
	}

	res, err := svc.CreatePipeline(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Legacy Source field is migrated to Sources and cleared.
	if len(res.Sources) != 1 || res.Sources[0] != "SOURCE_FILE_UPLOAD" {
		t.Errorf("expected Sources=[SOURCE_FILE_UPLOAD], got %v", res.Sources)
	}
	if res.Source != "" {
		t.Errorf("expected Source cleared, got %q", res.Source)
	}
}

func TestUpdatePipeline_InvalidSource(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)

	// Seed an existing pipeline
	store.Pipelines["user1_pipe1"] = &pipeline.PipelineConfig{
		Id:           "pipe1",
		Name:         "Existing",
		Source:       "SOURCE_STRAVA",
		Destinations: []plugin.DestinationType{1},
	}

	req := &pbsvc.UpdatePipelineRequest{
		UserId:     "user1",
		PipelineId: "pipe1",
		Pipeline: &pipeline.PipelineConfig{
			Source: "garbage_source",
		},
	}

	_, err := svc.UpdatePipeline(context.Background(), req)
	if err == nil {
		t.Fatal("expected error for invalid source on update")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", status.Code(err))
	}
}

func TestUpdatePipeline_NormalizesSource(t *testing.T) {
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{}, mockLogger{}, nil)

	// Seed an existing pipeline
	store.Pipelines["user1_pipe1"] = &pipeline.PipelineConfig{
		Id:           "pipe1",
		Name:         "Existing",
		Source:       "SOURCE_STRAVA",
		Destinations: []plugin.DestinationType{1},
	}

	req := &pbsvc.UpdatePipelineRequest{
		UserId:     "user1",
		PipelineId: "pipe1",
		Pipeline: &pipeline.PipelineConfig{
			Source: "hevy", // Short format
		},
	}

	res, err := svc.UpdatePipeline(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Legacy Source field is migrated to Sources and cleared.
	if len(res.Sources) != 1 || res.Sources[0] != "SOURCE_HEVY" {
		t.Errorf("expected Sources=[SOURCE_HEVY], got %v", res.Sources)
	}
	if res.Source != "" {
		t.Errorf("expected Source cleared, got %q", res.Source)
	}
}
