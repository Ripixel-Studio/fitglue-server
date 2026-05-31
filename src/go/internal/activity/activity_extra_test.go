package activity

import (
	"context"
	"errors"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// cloudEventsPublisher is a no-op Publisher for tests.
type cloudEventsPublisher struct{}

func (p *cloudEventsPublisher) PublishCloudEvent(_ context.Context, _ string, _ cloudevents.Event) (string, error) {
	return "test-id", nil
}

func (p *cloudEventsPublisher) PublishJSON(_ context.Context, _ string, _ []byte) error {
	return nil
}

func newTestSvc(store *MockActivityStore, blob *MockBlobStore) *Service {
	return NewService(store, blob, &cloudEventsPublisher{}, "test-bucket", "test-showcase-bucket", infra.NewLogger())
}

func TestGetActivity_Validation(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	ctx := context.Background()

	t.Run("missing user_id", func(t *testing.T) {
		_, err := svc.GetActivity(ctx, &pbsvc.GetActivityRequest{ActivityId: "a1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("missing activity_id", func(t *testing.T) {
		_, err := svc.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})
}

func TestGetActivity_StoreError(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return nil, errors.New("db down")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.GetActivity(context.Background(), &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "a1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestGetActivity_NotFound(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return nil, nil // nil = not found
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.GetActivity(context.Background(), &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "a1"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestGetActivity_NoURI_ReturnsMetadata(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				ActivityId: "a1",
				Source:     "SOURCE_STRAVA",
				Title:      "Evening Ride",
				Type:       pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
			}, nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	resp, err := svc.GetActivity(context.Background(), &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Name != "Evening Ride" {
		t.Errorf("expected 'Evening Ride', got %q", resp.Name)
	}
}

func TestGetActivity_BlobError(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{EnrichedEventUri: "gs://b/e.json"}, nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, _ string) ([]byte, error) {
			return nil, errors.New("gcs error")
		},
	}
	svc := newTestSvc(store, blob)
	_, err := svc.GetActivity(context.Background(), &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "a1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestDeleteActivity_Validation(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	ctx := context.Background()

	t.Run("missing user_id", func(t *testing.T) {
		_, err := svc.DeleteActivity(ctx, &pbsvc.DeleteActivityRequest{ActivityId: "a1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("missing activity_id", func(t *testing.T) {
		_, err := svc.DeleteActivity(ctx, &pbsvc.DeleteActivityRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})
}

func TestDeleteActivity_StoreError(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return nil, errors.New("db error")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.DeleteActivity(context.Background(), &pbsvc.DeleteActivityRequest{UserId: "u1", ActivityId: "a1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestDeleteActivity_NotFound_Succeeds(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return nil, nil // no run found
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.DeleteActivity(context.Background(), &pbsvc.DeleteActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Errorf("expected success for not-found activity, got %v", err)
	}
}

func TestListActivities_Validation(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	_, err := svc.ListActivities(context.Background(), &pbsvc.ListActivitiesRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestListActivities_StoreError(t *testing.T) {
	store := &MockActivityStore{
		ListPipelineRunsFunc: func(_ context.Context, _ string, _ int32, _ string) ([]*pbpipeline.PipelineRun, string, error) {
			return nil, "", errors.New("db error")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.ListActivities(context.Background(), &pbsvc.ListActivitiesRequest{UserId: "u1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestListActivities_Success(t *testing.T) {
	store := &MockActivityStore{
		ListPipelineRunsFunc: func(_ context.Context, _ string, _ int32, _ string) ([]*pbpipeline.PipelineRun, string, error) {
			return []*pbpipeline.PipelineRun{
				{ActivityId: "a1", Title: "Run 1", Source: "SOURCE_STRAVA", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN},
				{ActivityId: "a2", Title: "Ride 1", Source: "SOURCE_WAHOO", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE},
			}, "next-page", nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	resp, err := svc.ListActivities(context.Background(), &pbsvc.ListActivitiesRequest{UserId: "u1", Limit: 10})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.Activities) != 2 {
		t.Errorf("expected 2 activities, got %d", len(resp.Activities))
	}
	if resp.NextPageToken != "next-page" {
		t.Errorf("expected next-page token, got %q", resp.NextPageToken)
	}
	if resp.Activities[0].Name != "Run 1" {
		t.Errorf("unexpected activity name: %q", resp.Activities[0].Name)
	}
}
