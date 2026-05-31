package activity

import (
	"context"
	"errors"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func newTestService(store ActivityStore, blob BlobStore) *Service {
	return NewService(store, blob, nil, "test-bucket", "test-showcase-bucket", infra.NewLogger())
}

// ------- GetShowcasePreferences -------

func TestGetShowcasePreferences(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyUserId", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetShowcasePreferences(ctx, &pbsvc.GetShowcasePreferencesRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
			return nil, errors.New("db error")
		}
		svc := newTestService(store, &MockBlobStore{})
		_, err := svc.GetShowcasePreferences(ctx, &pbsvc.GetShowcasePreferencesRequest{UserId: "u1"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("NilReturnedDefaults", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
			return nil, nil
		}
		svc := newTestService(store, &MockBlobStore{})
		result, err := svc.GetShowcasePreferences(ctx, &pbsvc.GetShowcasePreferencesRequest{UserId: "u1"})
		assert.NoError(t, err)
		assert.Equal(t, "u1", result.UserId)
	})

	t.Run("Success", func(t *testing.T) {
		prefs := &pbactivity.ShowcaseProfile{UserId: "u1"}
		store := &MockActivityStore{}
		store.GetShowcasePreferencesFunc = func(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
			return prefs, nil
		}
		svc := newTestService(store, &MockBlobStore{})
		result, err := svc.GetShowcasePreferences(ctx, &pbsvc.GetShowcasePreferencesRequest{UserId: "u1"})
		assert.NoError(t, err)
		assert.Equal(t, "u1", result.UserId)
	})
}

// ------- UpdateShowcasePreferences -------

func TestUpdateShowcasePreferences(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyUserId", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcasePreferences(ctx, &pbsvc.UpdateShowcasePreferencesRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("NilPreferences", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcasePreferences(ctx, &pbsvc.UpdateShowcasePreferencesRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.UpdateShowcasePreferencesFunc = func(ctx context.Context, userID string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
			return nil, errors.New("db error")
		}
		svc := newTestService(store, &MockBlobStore{})
		_, err := svc.UpdateShowcasePreferences(ctx, &pbsvc.UpdateShowcasePreferencesRequest{
			UserId:      "u1",
			Preferences: &pbactivity.ShowcaseProfile{},
		})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("SetsUserIdOnPreferences", func(t *testing.T) {
		var captured string
		store := &MockActivityStore{}
		store.UpdateShowcasePreferencesFunc = func(ctx context.Context, userID string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
			captured = prefs.UserId
			return prefs, nil
		}
		svc := newTestService(store, &MockBlobStore{})
		_, err := svc.UpdateShowcasePreferences(ctx, &pbsvc.UpdateShowcasePreferencesRequest{
			UserId:      "u1",
			Preferences: &pbactivity.ShowcaseProfile{},
		})
		assert.NoError(t, err)
		assert.Equal(t, "u1", captured) // UserId should be injected
	})
}

// ------- GenerateShowcaseImages -------

func TestGenerateShowcaseImages(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyUserId", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GenerateShowcaseImages(ctx, &pbsvc.GenerateShowcaseImagesRequest{ShowcaseId: "s1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("EmptyShowcaseId", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GenerateShowcaseImages(ctx, &pbsvc.GenerateShowcaseImagesRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		resp, err := svc.GenerateShowcaseImages(ctx, &pbsvc.GenerateShowcaseImagesRequest{UserId: "u1", ShowcaseId: "s1"})
		assert.NoError(t, err)
		assert.NotNil(t, resp)
	})
}

// ------- GetPublicShowcase -------

func TestGetPublicShowcase(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyShowcaseId", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetPublicShowcaseFunc = func(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error) {
			return nil, "", errors.New("db error")
		}
		svc := newTestService(store, &MockBlobStore{})
		_, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("NotFound", func(t *testing.T) {
		store := &MockActivityStore{}
		store.GetPublicShowcaseFunc = func(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error) {
			return nil, "", nil
		}
		svc := newTestService(store, &MockBlobStore{})
		_, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
		assert.Equal(t, codes.NotFound, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		showcase := &pbactivity.ShowcasedActivity{ShowcaseId: "s1"}
		store := &MockActivityStore{}
		store.GetPublicShowcaseFunc = func(ctx context.Context, showcaseID string) (*pbactivity.ShowcasedActivity, string, error) {
			return showcase, "", nil
		}
		svc := newTestService(store, &MockBlobStore{})
		result, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
		assert.NoError(t, err)
		assert.Equal(t, "s1", result.ShowcaseId)
	})
}

// ------- DeleteShowcase -------

func TestDeleteShowcase(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyFields", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.DeleteShowcase(ctx, &pbsvc.DeleteShowcaseRequest{UserId: "u1"})
		assert.Equal(t, codes.InvalidArgument, status.Code(err))
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{}
		store.DeleteShowcaseFunc = func(ctx context.Context, userID, showcaseID string) error {
			return errors.New("delete failed")
		}
		svc := newTestService(store, &MockBlobStore{})
		_, err := svc.DeleteShowcase(ctx, &pbsvc.DeleteShowcaseRequest{UserId: "u1", ShowcaseId: "s1"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("Success", func(t *testing.T) {
		svc := newTestService(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.DeleteShowcase(ctx, &pbsvc.DeleteShowcaseRequest{UserId: "u1", ShowcaseId: "s1"})
		assert.NoError(t, err)
	})
}

// ------- GetActivity additional branches -------

func TestGetActivityAdditionalBranches(t *testing.T) {
	ctx := context.Background()

	t.Run("EmptyURI_BuildsFromMetadata", func(t *testing.T) {
		// When run has no URI, should build a lightweight activity from run metadata
		run := &pbpipeline.PipelineRun{
			Id:         "run1",
			ActivityId: "act1",
			Title:      "Morning Run",
			Source:     "SOURCE_STRAVA",
			StartTime:  timestamppb.Now(),
		}
		store := &MockActivityStore{
			GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
				return run, nil
			},
		}
		svc := newTestService(store, &MockBlobStore{})
		result, err := svc.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "run1"})
		assert.NoError(t, err)
		assert.Equal(t, "Morning Run", result.Name)
	})

	t.Run("BlobStoreError", func(t *testing.T) {
		run := &pbpipeline.PipelineRun{
			Id:               "run1",
			EnrichedEventUri: "gs://bucket/obj.json",
		}
		store := &MockActivityStore{
			GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
				return run, nil
			},
		}
		blob := &MockBlobStore{
			GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
				return nil, errors.New("gcs error")
			},
		}
		svc := newTestService(store, blob)
		_, err := svc.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "run1"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})

	t.Run("BlobUnparsable_ReturnsInternal", func(t *testing.T) {
		run := &pbpipeline.PipelineRun{
			Id:               "run1",
			EnrichedEventUri: "gs://bucket/obj.json",
		}
		store := &MockActivityStore{
			GetPipelineRunFunc: func(ctx context.Context, userID, runID string) (*pbpipeline.PipelineRun, error) {
				return run, nil
			},
		}
		blob := &MockBlobStore{
			GetFunc: func(ctx context.Context, bucket, object string) ([]byte, error) {
				return []byte(`this is not JSON at all!!!!`), nil
			},
		}
		svc := newTestService(store, blob)
		_, err := svc.GetActivity(ctx, &pbsvc.GetActivityRequest{UserId: "u1", ActivityId: "run1"})
		assert.Equal(t, codes.Internal, status.Code(err))
	})
}
