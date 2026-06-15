package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestExportData_Validation(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	_, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestExportData_ListPipelineRunsError(t *testing.T) {
	store := &MockActivityStore{
		ListPipelineRunsFunc: func(_ context.Context, _ string, _ int32, _ string) ([]*pbpipeline.PipelineRun, string, error) {
			return nil, "", errors.New("db error")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestExportData_ListShowcasesError(t *testing.T) {
	store := &MockActivityStore{
		ListShowcasedActivitiesByUserFunc: func(_ context.Context, _ string, _ int32, _ int32) ([]*pbactivity.ShowcasedActivity, int32, error) {
			return nil, 0, errors.New("db error")
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	_, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestExportData_WriteError(t *testing.T) {
	blob := &MockBlobStore{
		WriteFunc: func(_ context.Context, _, _ string, _ []byte) error {
			return errors.New("gcs write error")
		},
	}
	svc := newTestSvc(&MockActivityStore{}, blob)
	_, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestExportData_SignedURLError(t *testing.T) {
	blob := &MockBlobStore{
		SignedURLFunc: func(_ context.Context, _, _, _ string, _ int64, _ time.Duration) (string, error) {
			return "", errors.New("signing error")
		},
	}
	svc := newTestSvc(&MockActivityStore{}, blob)
	_, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if status.Code(err) != codes.Internal {
		t.Errorf("expected Internal, got %v", err)
	}
}

func TestExportData_Success(t *testing.T) {
	store := &MockActivityStore{
		ListPipelineRunsFunc: func(_ context.Context, _ string, _ int32, _ string) ([]*pbpipeline.PipelineRun, string, error) {
			return []*pbpipeline.PipelineRun{
				{ActivityId: "a1", Title: "Morning Run", Source: "SOURCE_STRAVA"},
			}, "", nil
		},
		ListShowcasedActivitiesByUserFunc: func(_ context.Context, _ string, _ int32, _ int32) ([]*pbactivity.ShowcasedActivity, int32, error) {
			return []*pbactivity.ShowcasedActivity{
				{ShowcaseId: "sc1"},
			}, 1, nil
		},
	}
	var writtenData []byte
	blob := &MockBlobStore{
		WriteFunc: func(_ context.Context, _, _ string, data []byte) error {
			writtenData = data
			return nil
		},
	}
	svc := newTestSvc(store, blob)
	resp, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.DownloadUrl == "" {
		t.Error("expected non-empty download_url")
	}
	if len(writtenData) == 0 {
		t.Error("expected data to be written to GCS")
	}
}

func TestExportData_EmptyData(t *testing.T) {
	// No pipeline runs, no showcases — should still succeed with empty arrays
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	resp, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.DownloadUrl == "" {
		t.Error("expected non-empty download_url")
	}
}
