package activity

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

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

func TestExportData_EnqueuesJob(t *testing.T) {
	var created *ExportJobRecord
	store := &MockActivityStore{
		CreateExportJobFunc: func(_ context.Context, _ string, job *ExportJobRecord) error {
			created = job
			return nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	resp, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Job == nil || resp.Job.JobId == "" {
		t.Fatal("expected a job with an id")
	}
	if resp.Job.Status != pbsvc.ExportJobStatus_EXPORT_JOB_STATUS_PENDING {
		t.Errorf("expected PENDING, got %v", resp.Job.Status)
	}
	if created == nil || created.Status != ExportStatusPending {
		t.Error("expected a PENDING job to be persisted")
	}
}

func TestExportData_DeduplicatesActiveJob(t *testing.T) {
	createCalled := false
	store := &MockActivityStore{
		LatestActiveExportJobFunc: func(_ context.Context, _ string) (*ExportJobRecord, error) {
			return &ExportJobRecord{JobID: "existing", Status: ExportStatusProcessing}, nil
		},
		CreateExportJobFunc: func(_ context.Context, _ string, _ *ExportJobRecord) error {
			createCalled = true
			return nil
		},
	}
	svc := newTestSvc(store, &MockBlobStore{})
	resp, err := svc.ExportData(context.Background(), &pbsvc.ExportDataRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Job.JobId != "existing" {
		t.Errorf("expected existing job id, got %q", resp.Job.JobId)
	}
	if createCalled {
		t.Error("should not create a new job when one is in flight")
	}
}

func TestGetExportJob_NotFound(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	_, err := svc.GetExportJob(context.Background(), &pbsvc.GetExportJobRequest{UserId: "u1", JobId: "missing"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestGetExportJob_ReadySignsURL(t *testing.T) {
	store := &MockActivityStore{
		GetExportJobFunc: func(_ context.Context, _, _ string) (*ExportJobRecord, error) {
			return &ExportJobRecord{
				JobID:      "j1",
				Status:     ExportStatusReady,
				ObjectPath: "exports/u1/j1.zip",
				SizeBytes:  1234,
				CreatedAt:  time.Now(),
			}, nil
		},
	}
	signed := ""
	blob := &MockBlobStore{
		SignedURLFunc: func(_ context.Context, _, path, _ string, _ int64, _ time.Duration) (string, error) {
			signed = path
			return "https://signed/" + path, nil
		},
	}
	svc := newTestSvc(store, blob)
	job, err := svc.GetExportJob(context.Background(), &pbsvc.GetExportJobRequest{UserId: "u1", JobId: "j1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if job.DownloadUrl == "" {
		t.Error("expected a signed download URL for a READY job")
	}
	if signed != "exports/u1/j1.zip" {
		t.Errorf("expected to sign the stored object path, got %q", signed)
	}
}

func TestExportPipelineRun_NotFound(t *testing.T) {
	svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
	_, err := svc.ExportPipelineRun(context.Background(), &pbsvc.ExportPipelineRunRequest{UserId: "u1", RunId: "r1"})
	if status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound, got %v", err)
	}
}

func TestExportPipelineRun_BuildsAndSigns(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{Id: "r1", Title: "Morning Run"}, nil
		},
	}
	var written []byte
	blob := &MockBlobStore{
		WriteFunc: func(_ context.Context, _, _ string, data []byte) error {
			written = data
			return nil
		},
	}
	svc := newTestSvc(store, blob)
	resp, err := svc.ExportPipelineRun(context.Background(), &pbsvc.ExportPipelineRunRequest{UserId: "u1", RunId: "r1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.DownloadUrl == "" {
		t.Error("expected a download URL")
	}
	// The written object should be a valid ZIP containing run.json.
	names := zipEntryNames(t, written)
	if !names["run.json"] || !names["README.txt"] {
		t.Errorf("expected run.json and README.txt in per-run zip, got %v", names)
	}
}

func TestBuildAccountArchive_ContainsExpectedFiles(t *testing.T) {
	store := &MockActivityStore{
		ListPipelineRunsFunc: func(_ context.Context, _ string, _ int32, _ string) ([]*pbpipeline.PipelineRun, string, error) {
			return []*pbpipeline.PipelineRun{
				{Id: "run-1", Title: "Run", EnrichedEventUri: "gs://b/enriched/run-1.json"},
			}, "", nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, uri string) ([]byte, error) {
			if strings.Contains(uri, "enriched") {
				return []byte(`{"fit_file_uri":"gs://b/fit/run-1.fit","access_token":"SECRET"}`), nil
			}
			if strings.Contains(uri, "fit") {
				return []byte("FITBINARY"), nil
			}
			return nil, errors.New("not found")
		},
	}
	svc := newTestSvc(store, blob)
	data, err := svc.buildAccountArchive(context.Background(), "u1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	names := zipEntryNames(t, data)
	for _, want := range []string{"manifest.json", "pipeline_runs.json", "showcased_activities.json", "README.txt", "runs/run-1/run.json", "runs/run-1/enriched.json", "runs/run-1/activity.fit"} {
		if !names[want] {
			t.Errorf("expected %q in account archive, got %v", want, names)
		}
	}
	// The enriched payload's access_token must be redacted in the archive.
	if body := zipEntry(t, data, "runs/run-1/enriched.json"); strings.Contains(string(body), "SECRET") {
		t.Error("expected access_token to be redacted in enriched.json")
	}
}

func TestRedactJSON_MasksSensitiveKeys(t *testing.T) {
	in := []byte(`{"name":"ok","access_token":"x","nested":{"refreshToken":"y","keep":"z"},"list":[{"client_secret":"s"}]}`)
	out := string(redactRawJSON(in))
	for _, leaked := range []string{`"x"`, `"y"`, `"s"`} {
		if strings.Contains(out, leaked) {
			t.Errorf("expected sensitive value removed, found in %s", out)
		}
	}
	for _, kept := range []string{`"ok"`, `"z"`} {
		if !strings.Contains(out, kept) {
			t.Errorf("expected non-sensitive value kept, missing from %s", out)
		}
	}
}

func TestExtractFitURI(t *testing.T) {
	if got := extractFitURI([]byte(`{"fit_file_uri":"gs://b/x.fit"}`)); got != "gs://b/x.fit" {
		t.Errorf("snake_case: got %q", got)
	}
	if got := extractFitURI([]byte(`{"fitFileUri":"gs://b/y.fit"}`)); got != "gs://b/y.fit" {
		t.Errorf("camelCase: got %q", got)
	}
	if got := extractFitURI([]byte(`not json`)); got != "" {
		t.Errorf("invalid json: expected empty, got %q", got)
	}
}

// zipEntryNames returns the set of entry names in a zip archive.
func zipEntryNames(t *testing.T, data []byte) map[string]bool {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}
	names := map[string]bool{}
	for _, f := range zr.File {
		names[f.Name] = true
	}
	return names
}

// zipEntry returns the bytes of a named entry.
func zipEntry(t *testing.T, data []byte, name string) []byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("failed to read zip: %v", err)
	}
	for _, f := range zr.File {
		if f.Name == name {
			rc, err := f.Open()
			if err != nil {
				t.Fatalf("open %s: %v", name, err)
			}
			defer rc.Close()
			b, _ := io.ReadAll(rc)
			return b
		}
	}
	t.Fatalf("entry %s not found", name)
	return nil
}
