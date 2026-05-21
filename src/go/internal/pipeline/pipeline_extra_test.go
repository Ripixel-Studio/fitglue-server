// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ErrorStore wraps MockPipelineStore to inject errors on specific methods.
type ErrorStore struct {
	*MockPipelineStore
	listPipelinesErr     error
	getPipelineErr       error
	createPipelineErr    error
	updatePipelineErr    error
	deletePipelineErr    error
	listPendingErr       error
	getPendingErr        error
	updatePendingErr     error
	getPipelineRunErr    error
	listPipelineRunsErr  error
	findRunByActivityErr error
}

func (e *ErrorStore) ListPipelines(ctx context.Context, userID string) ([]*pipeline.PipelineConfig, error) {
	if e.listPipelinesErr != nil {
		return nil, e.listPipelinesErr
	}
	return e.MockPipelineStore.ListPipelines(ctx, userID)
}
func (e *ErrorStore) GetPipeline(ctx context.Context, userID, pipelineID string) (*pipeline.PipelineConfig, error) {
	if e.getPipelineErr != nil {
		return nil, e.getPipelineErr
	}
	return e.MockPipelineStore.GetPipeline(ctx, userID, pipelineID)
}
func (e *ErrorStore) CreatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error) {
	if e.createPipelineErr != nil {
		return nil, e.createPipelineErr
	}
	return e.MockPipelineStore.CreatePipeline(ctx, userID, cfg)
}
func (e *ErrorStore) UpdatePipeline(ctx context.Context, userID string, cfg *pipeline.PipelineConfig) (*pipeline.PipelineConfig, error) {
	if e.updatePipelineErr != nil {
		return nil, e.updatePipelineErr
	}
	return e.MockPipelineStore.UpdatePipeline(ctx, userID, cfg)
}
func (e *ErrorStore) DeletePipeline(ctx context.Context, userID, pipelineID string) error {
	if e.deletePipelineErr != nil {
		return e.deletePipelineErr
	}
	return e.MockPipelineStore.DeletePipeline(ctx, userID, pipelineID)
}
func (e *ErrorStore) ListPendingInputs(ctx context.Context, userID string) ([]*pipeline.PendingInput, error) {
	if e.listPendingErr != nil {
		return nil, e.listPendingErr
	}
	return e.MockPipelineStore.ListPendingInputs(ctx, userID)
}
func (e *ErrorStore) GetPendingInput(ctx context.Context, userID, inputID string) (*pipeline.PendingInput, error) {
	if e.getPendingErr != nil {
		return nil, e.getPendingErr
	}
	return e.MockPipelineStore.GetPendingInput(ctx, userID, inputID)
}
func (e *ErrorStore) UpdatePendingInput(ctx context.Context, userID string, input *pipeline.PendingInput) error {
	if e.updatePendingErr != nil {
		return e.updatePendingErr
	}
	return e.MockPipelineStore.UpdatePendingInput(ctx, userID, input)
}
func (e *ErrorStore) GetPipelineRun(ctx context.Context, userID, runID string) (*pipeline.PipelineRun, error) {
	if e.getPipelineRunErr != nil {
		return nil, e.getPipelineRunErr
	}
	return e.MockPipelineStore.GetPipelineRun(ctx, userID, runID)
}
func (e *ErrorStore) ListPipelineRuns(ctx context.Context, userID, pipelineID string, limit int32, pageToken string, since, until *time.Time) ([]*pipeline.PipelineRun, string, error) {
	if e.listPipelineRunsErr != nil {
		return nil, "", e.listPipelineRunsErr
	}
	return e.MockPipelineStore.ListPipelineRuns(ctx, userID, pipelineID, limit, pageToken, since, until)
}
func (e *ErrorStore) UpdatePipelineRun(ctx context.Context, userID, runID string, data map[string]interface{}) error {
	return e.MockPipelineStore.UpdatePipelineRun(ctx, userID, runID, data)
}
func (e *ErrorStore) FindPipelineRunByActivityId(ctx context.Context, userID, activityID string) (*pipeline.PipelineRun, error) {
	if e.findRunByActivityErr != nil {
		return nil, e.findRunByActivityErr
	}
	return e.MockPipelineStore.FindPipelineRunByActivityId(ctx, userID, activityID)
}

func (e *ErrorStore) FindPipelineRunByPendingInputId(ctx context.Context, userID, pendingInputID string) (*pipeline.PipelineRun, error) {
	return e.MockPipelineStore.FindPipelineRunByPendingInputId(ctx, userID, pendingInputID)
}

func (e *ErrorStore) FindPipelineRunBySourceActivityID(ctx context.Context, userID, pipelineID, sourceActivityID string) (*pipeline.PipelineRun, error) {
	return e.MockPipelineStore.FindPipelineRunBySourceActivityID(ctx, userID, pipelineID, sourceActivityID)
}

// --- Validation error tests ---

func TestPipeline_Validation(t *testing.T) {
	svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	ctx := context.Background()

	t.Run("ListPipelines_missing_user_id", func(t *testing.T) {
		_, err := svc.ListPipelines(ctx, &pbsvc.ListPipelinesRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("GetPipeline_missing_ids", func(t *testing.T) {
		_, err := svc.GetPipeline(ctx, &pbsvc.GetPipelineRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("CreatePipeline_missing_source", func(t *testing.T) {
		_, err := svc.CreatePipeline(ctx, &pbsvc.CreatePipelineRequest{
			UserId:   "u1",
			Pipeline: &pipeline.PipelineConfig{Destinations: []plugin.DestinationType{1}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("CreatePipeline_missing_destinations", func(t *testing.T) {
		_, err := svc.CreatePipeline(ctx, &pbsvc.CreatePipelineRequest{
			UserId:   "u1",
			Pipeline: &pipeline.PipelineConfig{Source: "SOURCE_STRAVA"},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("UpdatePipeline_missing_pipeline_id", func(t *testing.T) {
		_, err := svc.UpdatePipeline(ctx, &pbsvc.UpdatePipelineRequest{
			UserId:   "u1",
			Pipeline: &pipeline.PipelineConfig{Source: "SOURCE_STRAVA", Destinations: []plugin.DestinationType{1}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("DeletePipeline_missing_ids", func(t *testing.T) {
		_, err := svc.DeletePipeline(ctx, &pbsvc.DeletePipelineRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("SubmitInput_missing_fields", func(t *testing.T) {
		_, err := svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("ListPendingInputs_missing_user_id", func(t *testing.T) {
		_, err := svc.ListPendingInputs(ctx, &pbsvc.ListPendingInputsRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("ResolvePendingInput_missing_ids", func(t *testing.T) {
		_, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("GetPipelineRun_missing_ids", func(t *testing.T) {
		_, err := svc.GetPipelineRun(ctx, &pbsvc.GetPipelineRunRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("ListPipelineRuns_missing_user_id", func(t *testing.T) {
		_, err := svc.ListPipelineRuns(ctx, &pbsvc.ListPipelineRunsRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("RepostActivity_missing_ids", func(t *testing.T) {
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})
}

// --- Store error tests ---

func TestPipeline_StoreErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("ListPipelines_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), listPipelinesErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.ListPipelines(ctx, &pbsvc.ListPipelinesRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("CreatePipeline_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), createPipelineErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.CreatePipeline(ctx, &pbsvc.CreatePipelineRequest{
			UserId: "u1",
			Pipeline: &pipeline.PipelineConfig{
				Source:       "SOURCE_STRAVA",
				Destinations: []plugin.DestinationType{1},
			},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("UpdatePipeline_storeError", func(t *testing.T) {
		ms := NewMockStore()
		ms.Pipelines["u1_p1"] = &pipeline.PipelineConfig{
			Id:           "p1",
			Source:       "SOURCE_STRAVA",
			Destinations: []plugin.DestinationType{1},
		}
		es := &ErrorStore{MockPipelineStore: ms, updatePipelineErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.UpdatePipeline(ctx, &pbsvc.UpdatePipelineRequest{
			UserId:     "u1",
			PipelineId: "p1",
			Pipeline: &pipeline.PipelineConfig{
				Source:       "SOURCE_STRAVA",
				Destinations: []plugin.DestinationType{1},
			},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("DeletePipeline_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), deletePipelineErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.DeletePipeline(ctx, &pbsvc.DeletePipelineRequest{UserId: "u1", PipelineId: "p1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("ListPendingInputs_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), listPendingErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.ListPendingInputs(ctx, &pbsvc.ListPendingInputsRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("GetPipelineRun_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), getPipelineRunErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.GetPipelineRun(ctx, &pbsvc.GetPipelineRunRequest{UserId: "u1", RunId: "r1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("ListPipelineRuns_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), listPipelineRunsErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.ListPipelineRuns(ctx, &pbsvc.ListPipelineRunsRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})
}

func TestPipeline_HappyPaths(t *testing.T) {
	ctx := context.Background()

	t.Run("ListPipelines_success", func(t *testing.T) {
		store := NewMockStore()
		store.Pipelines["u1_p1"] = &pipeline.PipelineConfig{Id: "p1", Name: "Pipeline 1"}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		resp, err := svc.ListPipelines(ctx, &pbsvc.ListPipelinesRequest{UserId: "u1"})
		if err != nil || len(resp.Pipelines) != 1 {
			t.Errorf("expected 1 pipeline, got %v, err=%v", len(resp.Pipelines), err)
		}
	})

	t.Run("ListPendingInputs_success", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{ActivityId: "i1"}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		resp, err := svc.ListPendingInputs(ctx, &pbsvc.ListPendingInputsRequest{UserId: "u1"})
		if err != nil || len(resp.Inputs) != 1 {
			t.Errorf("expected 1 input, got %v, err=%v", len(resp.Inputs), err)
		}
	})

	t.Run("GetPipelineRun_notFound", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.GetPipelineRun(ctx, &pbsvc.GetPipelineRunRequest{UserId: "u1", RunId: "missing"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("GetPipelineRun_success", func(t *testing.T) {
		store := NewMockStore()
		store.Runs["u1_r1"] = &pipeline.PipelineRun{Id: "r1"}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		resp, err := svc.GetPipelineRun(ctx, &pbsvc.GetPipelineRunRequest{UserId: "u1", RunId: "r1"})
		if err != nil || resp.Id != "r1" {
			t.Errorf("expected run r1, got %v, err=%v", resp, err)
		}
	})

	t.Run("ListPipelineRuns_success", func(t *testing.T) {
		store := NewMockStore()
		store.Runs["u1_r1"] = &pipeline.PipelineRun{Id: "r1"}
		store.Runs["u1_r2"] = &pipeline.PipelineRun{Id: "r2"}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		resp, err := svc.ListPipelineRuns(ctx, &pbsvc.ListPipelineRunsRequest{UserId: "u1"})
		if err != nil || len(resp.Runs) != 2 {
			t.Errorf("expected 2 runs, got %d, err=%v", len(resp.Runs), err)
		}
	})

	t.Run("ResolvePendingInput_notFound", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{UserId: "u1", PendingInputId: "missing"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("ResolvePendingInput_success", func(t *testing.T) {
		store := NewMockStore()
		store.PendingInputs["u1_i1"] = &pipeline.PendingInput{
			ActivityId: "i1",
			Status:     pipeline.PendingInput_STATUS_WAITING,
		}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.ResolvePendingInput(ctx, &pbsvc.ResolvePendingInputRequest{UserId: "u1", PendingInputId: "i1"})
		if err != nil {
			t.Errorf("expected success, got %v", err)
		}
	})

	t.Run("RepostActivity_invalidMode", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "a1", Mode: "bad"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for bad mode, got %v", err)
		}
	})

	t.Run("RepostActivity_missingDestination", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "a1", Mode: "missed-destination"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for missing destination, got %v", err)
		}
	})

	t.Run("RepostActivity_runNotFound", func(t *testing.T) {
		svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "missing", Mode: "full-pipeline"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("RepostActivity_noPayloadUri", func(t *testing.T) {
		store := NewMockStore()
		store.Runs["u1_r1"] = &pipeline.PipelineRun{Id: "r1", ActivityId: "a1"} // no OriginalPayloadUri
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "a1", Mode: "full-pipeline"})
		if status.Code(err) != codes.FailedPrecondition {
			t.Errorf("expected FailedPrecondition, got %v", err)
		}
	})

	t.Run("RepostActivity_gcsFetchError", func(t *testing.T) {
		store := NewMockStore()
		store.Runs["u1_r1"] = &pipeline.PipelineRun{Id: "r1", ActivityId: "a1", OriginalPayloadUri: "gs://bucket/missing.json"}
		svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "a1", Mode: "full-pipeline"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal for GCS fetch failure, got %v", err)
		}
	})

	t.Run("RepostActivity_storeError", func(t *testing.T) {
		es := &ErrorStore{MockPipelineStore: NewMockStore(), findRunByActivityErr: errors.New("db down")}
		svc := NewService(es, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "a1", Mode: "full-pipeline"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal for store error, got %v", err)
		}
	})

	t.Run("RepostActivity_fullPipeline_success", func(t *testing.T) {
		store := NewMockStore()
		payloadBytes := []byte(`{"source":"strava","title":"Morning Run"}`)
		uri := "gs://bucket/original/a1.json"
		store.Runs["u1_r1"] = &pipeline.PipelineRun{Id: "r1", ActivityId: "a1", OriginalPayloadUri: uri}
		pub := &MockPublisher{}
		blob := &MockBlobStore{Blobs: map[string][]byte{uri: payloadBytes}}
		svc := NewService(store, pub, blob, mockLogger{}, nil)

		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{UserId: "u1", ActivityId: "a1", Mode: "full-pipeline"})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if len(pub.PublishedEvents) != 1 {
			t.Fatalf("expected 1 event published, got %d", len(pub.PublishedEvents))
		}

		var p map[string]interface{}
		json.Unmarshal(pub.PublishedEvents[0].Data(), &p)
		if p["isRepost"] != true {
			t.Errorf("expected isRepost=true")
		}
		if p["repostMode"] != "full-pipeline" {
			t.Errorf("expected repostMode=full-pipeline, got %v", p["repostMode"])
		}
		if p["activityId"] != "a1" {
			t.Errorf("expected activityId=a1")
		}
	})

	t.Run("RepostActivity_missedDestination_success", func(t *testing.T) {
		store := NewMockStore()
		uri := "gs://bucket/original/a2.json"
		store.Runs["u1_r2"] = &pipeline.PipelineRun{Id: "r2", ActivityId: "a2", OriginalPayloadUri: uri}
		pub := &MockPublisher{}
		blob := &MockBlobStore{Blobs: map[string][]byte{uri: []byte(`{"source":"hevy"}`)}}
		svc := NewService(store, pub, blob, mockLogger{}, nil)

		_, err := svc.RepostActivity(ctx, &pbsvc.RepostActivityRequest{
			UserId: "u1", ActivityId: "a2", Mode: "missed-destination", Destination: "DESTINATION_HEVY",
		})
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}

		var p map[string]interface{}
		json.Unmarshal(pub.PublishedEvents[0].Data(), &p)
		if p["repostDestination"] != "DESTINATION_HEVY" {
			t.Errorf("expected repostDestination=DESTINATION_HEVY, got %v", p["repostDestination"])
		}
	})
}
