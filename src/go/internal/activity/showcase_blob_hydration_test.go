package activity

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"strings"

	"cloud.google.com/go/storage"
	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
)

// logEntry records a single structured log call for assertion.
type logEntry struct {
	level string
	msg   string
}

// recordingLogger is an infra.Logger that captures every call so tests can
// assert the level a given condition was logged at. Only Error-level logs are
// forwarded to Sentry (see pkg/infrastructure/sentry), so the level a blob miss
// is logged at is the difference between a silent, recoverable degradation and a
// paging Sentry issue.
type recordingLogger struct {
	entries *[]logEntry
}

func newRecordingLogger() *recordingLogger { return &recordingLogger{entries: &[]logEntry{}} }

func (l *recordingLogger) record(level, msg string) {
	*l.entries = append(*l.entries, logEntry{level: level, msg: msg})
}

func (l *recordingLogger) Debug(_ context.Context, msg string, _ ...any) { l.record("DEBUG", msg) }
func (l *recordingLogger) Info(_ context.Context, msg string, _ ...any)  { l.record("INFO", msg) }
func (l *recordingLogger) Warn(_ context.Context, msg string, _ ...any)  { l.record("WARN", msg) }
func (l *recordingLogger) Error(_ context.Context, msg string, _ ...any) { l.record("ERROR", msg) }
func (l *recordingLogger) With(_ ...any) infra.Logger                    { return l }

func (l *recordingLogger) levelsFor(msgSubstr string) []string {
	var out []string
	for _, e := range *l.entries {
		if msgSubstr == "" || strings.Contains(e.msg, msgSubstr) {
			out = append(out, e.level)
		}
	}
	return out
}

func svcWithLogger(store ActivityStore, blob BlobStore, logger infra.Logger) *Service {
	svc := NewService(store, blob, nil, "test-bucket", "test-showcase-bucket", logger)
	return svc
}

// GetPublicShowcase must not treat a missing GCS blob as an Error: the request
// still succeeds (the page renders from Firestore metadata), and a missing blob
// is logged at Warn so it never reaches Sentry. This is the SERVER-4 regression:
// storage.ErrObjectNotExist (an *errors.errorString) was logged at Error on every
// public pageview of a showcase whose activity blob had been lifecycle-deleted.
func TestGetPublicShowcase_MissingBlobIsWarnNotError(t *testing.T) {
	ctx := context.Background()
	store := &MockActivityStore{
		GetPublicShowcaseFunc: func(_ context.Context, _ string) (*pbactivity.ShowcasedActivity, string, error) {
			return &pbactivity.ShowcasedActivity{ShowcaseId: "s1", ActivityDataUri: "gs://showcase-assets/gone.json"}, "owner-uid", nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, _ string) ([]byte, error) {
			return nil, storage.ErrObjectNotExist
		},
	}
	logger := newRecordingLogger()
	svc := svcWithLogger(store, blob, logger)

	res, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
	if err != nil {
		t.Fatalf("expected success despite missing blob, got error: %v", err)
	}
	if res.ShowcaseId != "s1" {
		t.Fatalf("expected showcase returned, got %+v", res)
	}

	for _, lvl := range logger.levelsFor("") {
		if lvl == "ERROR" {
			t.Fatalf("missing blob must not be logged at Error (would page Sentry); levels=%v", *logger.entries)
		}
	}
	levels := logger.levelsFor("blob missing")
	if len(levels) != 1 || levels[0] != "WARN" {
		t.Fatalf("expected a single Warn for the missing blob, got %v (all=%v)", levels, *logger.entries)
	}
}

// A genuine, unexpected GCS failure (auth, network, wrapped ErrObjectNotExist is
// specifically excluded here) must still be logged at Error so it continues to
// surface in Sentry.
func TestGetPublicShowcase_UnexpectedBlobErrorStillError(t *testing.T) {
	ctx := context.Background()
	store := &MockActivityStore{
		GetPublicShowcaseFunc: func(_ context.Context, _ string) (*pbactivity.ShowcasedActivity, string, error) {
			return &pbactivity.ShowcasedActivity{ShowcaseId: "s1", ActivityDataUri: "gs://showcase-assets/x.json"}, "owner-uid", nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, _ string) ([]byte, error) {
			return nil, errors.New("googleapi: permission denied on bucket")
		},
	}
	logger := newRecordingLogger()
	svc := svcWithLogger(store, blob, logger)

	res, err := svc.GetPublicShowcase(ctx, &pbsvc.GetPublicShowcaseRequest{ShowcaseId: "s1"})
	if err != nil {
		t.Fatalf("expected graceful success, got error: %v", err)
	}
	if res == nil {
		t.Fatal("expected showcase returned")
	}
	levels := logger.levelsFor("failed to fetch activity data")
	if len(levels) != 1 || levels[0] != "ERROR" {
		t.Fatalf("expected a single Error for unexpected GCS failure, got %v (all=%v)", levels, *logger.entries)
	}
}

// The same recoverable-miss handling applies to the private GetShowcase read path,
// which shares the hydration helper.
func TestGetShowcase_MissingBlobIsWarnNotError(t *testing.T) {
	ctx := context.Background()
	store := &MockActivityStore{
		GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
			return &pbactivity.ShowcasedActivity{ShowcaseId: "s1", ActivityDataUri: "gs://showcase-assets/gone.json"}, nil
		},
	}
	blob := &MockBlobStore{
		GetFunc: func(_ context.Context, _, _ string) ([]byte, error) {
			return nil, fmt.Errorf("read object: %w", storage.ErrObjectNotExist)
		},
	}
	logger := newRecordingLogger()
	svc := svcWithLogger(store, blob, logger)

	if _, err := svc.GetShowcase(ctx, &pbsvc.GetShowcaseRequest{UserId: "owner-uid", ShowcaseId: "s1"}); err != nil {
		t.Fatalf("expected success despite missing blob, got error: %v", err)
	}
	for _, lvl := range logger.levelsFor("") {
		if lvl == "ERROR" {
			t.Fatalf("missing blob must not be logged at Error; levels=%v", *logger.entries)
		}
	}
}
