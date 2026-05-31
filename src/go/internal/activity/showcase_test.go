package activity

import (
	"context"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
)

func TestShowcaseOffloading(t *testing.T) {
	ctx := context.Background()
	logger := infra.NewLogger()

	var writtenData []byte

	store := &MockActivityStore{
		CreateShowcaseFunc: func(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
			return showcase, nil
		},
		UpdateShowcaseFunc: func(ctx context.Context, userID string, showcase *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
			return showcase, nil
		},
	}

	blobStore := &MockBlobStore{
		WriteFunc: func(ctx context.Context, bucket, object string, data []byte) error {
			writtenData = data
			return nil
		},
	}

	svc := NewService(store, blobStore, nil, "test-bucket", "test-showcase-bucket", logger)

	req := &pbsvc.CreateShowcaseRequest{
		UserId: "u1",
		Showcase: &pbactivity.ShowcasedActivity{
			ShowcaseId: "s1",
			ActivityData: &pbactivity.StandardizedActivity{
				Name: "Massive Workout Data",
			},
		},
	}

	res, err := svc.CreateShowcase(ctx, req)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	if res.ActivityData != nil {
		t.Error("expected ActivityData to be nil after offloading")
	}

	if !strings.HasPrefix(res.ActivityDataUri, "gs://test-bucket/showcase_data/u1/s1_data.json") {
		t.Errorf("expected GCS URI, got %s", res.ActivityDataUri)
	}

	if !strings.Contains(string(writtenData), "Massive Workout Data") {
		t.Error("expected BlobStore to contain the serialized StandardizedActivity")
	}
}
