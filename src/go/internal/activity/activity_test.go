package activity

import (
	"context"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/fitglue/server/src/go/internal/infra"
)

func TestGetActivity(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			if runID == "missing" {
				return nil, nil
			}
			return &pbpipeline.PipelineRun{
				ActivityId:       runID,
				Source:           "SOURCE_STRAVA",
				Title:            "Test Run",
				Type:             pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
				EnrichedEventUri: "gs://test-bucket/activity/123.json",
			}, nil
		},
	}

	blobStore := &MockBlobStore{
		GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
			event := &pbevents.EnrichedActivityEvent{
				ActivityData: &pbactivity.StandardizedActivity{
					Name: "Mocked GCS Activity",
				},
			}
			return protojson.MarshalOptions{UseProtoNames: false}.Marshal(event)
		},
	}

	svc := NewService(store, blobStore, nil, "test-bucket", "test-showcase-bucket", logger)

	// Test Found Activity
	res, err := svc.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if res.Name != "Mocked GCS Activity" {
		t.Errorf("expected Mocked GCS Activity, got %s", res.Name)
	}

	// Test Missing Activity
	res, err = svc.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "missing"})
	if err == nil {
		t.Fatal("expected error for missing activity")
	}
}

func TestDeleteActivity(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	deleteCalled := false
	blobDeleteCount := 0

	store := &MockActivityStore{
		GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				ActivityId:         runID,
				EnrichedEventUri:   "gs://test-bucket/abc.json",
				OriginalPayloadUri: "gs://test-bucket/def.json",
			}, nil
		},
		DeletePipelineRunFunc: func(ctx context.Context, userID, runID string) error {
			deleteCalled = true
			return nil
		},
	}

	blobStore := &MockBlobStore{
		DeleteFunc: func(ctx context.Context, bucket, object string) error {
			blobDeleteCount++
			return nil
		},
	}

	svc := NewService(store, blobStore, nil, "test-bucket", "test-showcase-bucket", logger)

	_, err := svc.DeleteActivity(ctx, &pbsvc.DeleteActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if !deleteCalled {
		t.Error("expected PipelineRun delete to be called")
	}
	if blobDeleteCount != 2 {
		t.Errorf("expected 2 blob deletions, got %d", blobDeleteCount)
	}
}
