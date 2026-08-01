// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	parkrunutil "github.com/fitglue/server/src/go/pkg/parkrun"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// recordingLogger is an infra.Logger that captures Error-level calls (and their
// structured args) so tests can assert on the diagnostics surfaced to error-tracking.
type recordingLogger struct{ errs *[]map[string]any }

func kvArgs(args []any) map[string]any {
	m := map[string]any{}
	for i := 0; i+1 < len(args); i += 2 {
		if k, ok := args[i].(string); ok {
			m[k] = args[i+1]
		}
	}
	return m
}

func (l recordingLogger) Debug(_ context.Context, _ string, _ ...any) {}
func (l recordingLogger) Info(_ context.Context, _ string, _ ...any)  {}
func (l recordingLogger) Warn(_ context.Context, _ string, _ ...any)  {}
func (l recordingLogger) Error(_ context.Context, msg string, args ...any) {
	m := kvArgs(args)
	m["_msg"] = msg
	*l.errs = append(*l.errs, m)
}
func (l recordingLogger) With(_ ...any) infra.Logger { return l }

// A WAITING input that fetches cleanly but has no matching results row is still
// unresolved, so exactly one diagnostic summary is surfaced to error-tracking
// carrying the discriminating signals (url, html_bytes, rows_parsed, slug/date match).
func TestParkrunChecker_HandleCheck_SurfacesUnresolvedDiagnostic(t *testing.T) {
	base := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*user.Record, error) { return parkrunUser(), nil },
	}
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(_ context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId: "i1",
				UserId:     "u1",
				ProviderMetadata: map[string]string{
					"parkrun_event_slug": "bushy",
					"expected_date":      "26/07/2025",
				},
			}}, nil
		},
	}

	var errs []map[string]any
	c := NewParkrunChecker(db, nil, recordingLogger{errs: &errs}, nil)
	c.fetch = func(_ context.Context, _ *slog.Logger, _, _, _ string, _ time.Time) (*parkrunutil.Result, parkrunutil.FetchDiagnostics, error) {
		return nil, parkrunutil.FetchDiagnostics{
			URL:         "https://www.parkrun.org.uk/parkrunner/12345/all/",
			HTMLBytes:   48213,
			RowsParsed:  42,
			SlugMatched: true,
			DateMatched: false,
		}, nil
	}

	code, body := doCheck(t, c)
	if code != http.StatusOK {
		t.Fatalf("expected 200, got %d", code)
	}
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped, got %v", body["skipped"])
	}
	if body["unresolved"].(float64) != 1 {
		t.Errorf("expected 1 unresolved, got %v", body["unresolved"])
	}

	var summary map[string]any
	for _, e := range errs {
		if e["_msg"] == "parkrun checker: pending input still unresolved this run" {
			summary = e
			break
		}
	}
	if summary == nil {
		t.Fatalf("expected an unresolved-input diagnostic to be surfaced, got errors: %v", errs)
	}
	if summary["reason"] != reasonNotPublished {
		t.Errorf("reason = %v, want %s", summary["reason"], reasonNotPublished)
	}
	if summary["input_id"] != "i1" || summary["user_id"] != "u1" {
		t.Errorf("expected input_id=i1 user_id=u1, got %v / %v", summary["input_id"], summary["user_id"])
	}
	if summary["event_slug"] != "bushy" {
		t.Errorf("event_slug = %v, want bushy", summary["event_slug"])
	}
	if summary["url"] != "https://www.parkrun.org.uk/parkrunner/12345/all/" {
		t.Errorf("url = %v", summary["url"])
	}
	if summary["html_bytes"] != 48213 {
		t.Errorf("html_bytes = %v, want 48213", summary["html_bytes"])
	}
	if summary["rows_parsed"] != 42 {
		t.Errorf("rows_parsed = %v, want 42", summary["rows_parsed"])
	}
	if summary["slug_matched"] != true || summary["date_matched"] != false {
		t.Errorf("slug/date match = %v/%v, want true/false", summary["slug_matched"], summary["date_matched"])
	}
}

// A hard fetch failure surfaces a diagnostic whose reason is fetch_failed and whose
// fetch_error carries the underlying message — the fetcher-infra vs stored-metadata
// discriminator.
func TestParkrunChecker_HandleCheck_SurfacesFetchFailure(t *testing.T) {
	base := &mocks.MockDatabase{
		GetUserFunc: func(_ context.Context, _ string) (*user.Record, error) { return parkrunUser(), nil },
	}
	db := &parkrunDB{
		MockDatabase: base,
		listFn: func(_ context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId:       "i1",
				UserId:           "u1",
				ProviderMetadata: map[string]string{"parkrun_event_slug": "bushy", "expected_date": "26/07/2025"},
			}}, nil
		},
	}

	var errs []map[string]any
	c := NewParkrunChecker(db, nil, recordingLogger{errs: &errs}, nil)
	c.fetch = func(_ context.Context, _ *slog.Logger, _, _, _ string, _ time.Time) (*parkrunutil.Result, parkrunutil.FetchDiagnostics, error) {
		return nil, parkrunutil.FetchDiagnostics{URL: "https://www.parkrun.org.uk/parkrunner/12345/all/"},
			errors.New("playwright service error: status=503")
	}

	_, body := doCheck(t, c)
	if body["unresolved"].(float64) != 1 {
		t.Fatalf("expected 1 unresolved, got %v", body["unresolved"])
	}
	var summary map[string]any
	for _, e := range errs {
		if e["_msg"] == "parkrun checker: pending input still unresolved this run" {
			summary = e
		}
	}
	if summary == nil {
		t.Fatal("expected a surfaced diagnostic for the fetch failure")
	}
	if summary["reason"] != reasonFetchFailed {
		t.Errorf("reason = %v, want %s", summary["reason"], reasonFetchFailed)
	}
	if summary["fetch_error"] != "playwright service error: status=503" {
		t.Errorf("fetch_error = %v", summary["fetch_error"])
	}
}

// An already-EXPIRED (manual-entry) input has intentionally left the auto-poll set,
// so it must NOT be surfaced as noise even though it is skipped.
func TestParkrunChecker_HandleCheck_AlreadyExpiredNotSurfaced(t *testing.T) {
	db := &parkrunDB{
		MockDatabase: &mocks.MockDatabase{},
		listFn: func(_ context.Context, _ string, _ pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			return []*pbpipeline.PendingInput{{
				ActivityId:       "i1",
				UserId:           "u1",
				ProviderMetadata: map[string]string{"parkrun_results_state": "EXPIRED"},
			}}, nil
		},
	}
	var errs []map[string]any
	c := NewParkrunChecker(db, nil, recordingLogger{errs: &errs}, nil)
	_, body := doCheck(t, c)
	if body["skipped"].(float64) != 1 {
		t.Errorf("expected 1 skipped, got %v", body["skipped"])
	}
	if body["unresolved"].(float64) != 0 {
		t.Errorf("expected 0 unresolved (EXPIRED is not noise), got %v", body["unresolved"])
	}
	if len(errs) != 0 {
		t.Errorf("expected no surfaced diagnostics for already-EXPIRED input, got %v", errs)
	}
}

func recheckRequest(userID, inputID, bearer string) *http.Request {
	req := httptest.NewRequest(http.MethodPost, "/internal/parkrun-recheck?user_id="+userID+"&input_id="+inputID, nil)
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	return req
}

// The on-demand re-check rejects unauthenticated callers: no bearer → 401, and a
// bearer the identity verifier rejects → 403.
func TestParkrunChecker_HandleRecheck_Auth(t *testing.T) {
	c := NewParkrunChecker(&mocks.MockDatabase{}, nil, infra.NewLogger(), nil)

	// No Authorization header at all.
	w := httptest.NewRecorder()
	c.HandleRecheck(w, recheckRequest("u1", "i1", ""))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without bearer token, got %d", w.Code)
	}

	// Bearer present but the identity verifier rejects it.
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return errors.New("token expired") }
	w = httptest.NewRecorder()
	c.HandleRecheck(w, recheckRequest("u1", "i1", "some.jwt.token"))
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for invalid identity token, got %d", w.Code)
	}
}

// A properly authenticated re-check loads the one input, runs a real resolve attempt,
// and returns the diagnostic as JSON.
func TestParkrunChecker_HandleRecheck_ReturnsDiagnostics(t *testing.T) {
	db := &mocks.MockDatabase{
		GetPendingInputFunc: func(_ context.Context, userID, id string) (*pbpipeline.PendingInput, error) {
			if userID != "u1" || id != "i1" {
				return nil, errors.New("not found")
			}
			return &pbpipeline.PendingInput{
				ActivityId:       "i1",
				UserId:           "u1",
				ProviderMetadata: map[string]string{"parkrun_event_slug": "bushy", "expected_date": "26/07/2025"},
			}, nil
		},
		GetUserFunc: func(_ context.Context, _ string) (*user.Record, error) { return parkrunUser(), nil },
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil } // accept
	c.fetch = func(_ context.Context, _ *slog.Logger, _, _, _ string, _ time.Time) (*parkrunutil.Result, parkrunutil.FetchDiagnostics, error) {
		return nil, parkrunutil.FetchDiagnostics{
			URL:         "https://www.parkrun.org.uk/parkrunner/12345/all/",
			HTMLBytes:   50123,
			RowsParsed:  10,
			SlugMatched: false,
			DateMatched: false,
		}, nil
	}

	w := httptest.NewRecorder()
	c.HandleRecheck(w, recheckRequest("u1", "i1", "good.jwt.token"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", w.Code, w.Body.String())
	}

	var diag ParkrunDiagnostic
	if err := json.Unmarshal(w.Body.Bytes(), &diag); err != nil {
		t.Fatalf("response not a ParkrunDiagnostic: %v", err)
	}
	if diag.Outcome != outcomeSkipped || diag.Reason != reasonNotPublished {
		t.Errorf("outcome/reason = %s/%s, want %s/%s", diag.Outcome, diag.Reason, outcomeSkipped, reasonNotPublished)
	}
	if diag.InputID != "i1" || diag.UserID != "u1" {
		t.Errorf("ids = %s/%s, want i1/u1", diag.InputID, diag.UserID)
	}
	if diag.RowsParsed != 10 || diag.HTMLBytes != 50123 {
		t.Errorf("rows/bytes = %d/%d, want 10/50123", diag.RowsParsed, diag.HTMLBytes)
	}
	if diag.URL != "https://www.parkrun.org.uk/parkrunner/12345/all/" {
		t.Errorf("url = %s", diag.URL)
	}
	if diag.SlugMatched || diag.DateMatched {
		t.Errorf("expected slug/date unmatched, got %v/%v", diag.SlugMatched, diag.DateMatched)
	}
}

// A re-check for an input that doesn't exist returns 404 (after passing auth).
func TestParkrunChecker_HandleRecheck_NotFound(t *testing.T) {
	db := &mocks.MockDatabase{
		GetPendingInputFunc: func(_ context.Context, _, _ string) (*pbpipeline.PendingInput, error) {
			return nil, errors.New("no such document")
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }
	w := httptest.NewRecorder()
	c.HandleRecheck(w, recheckRequest("u1", "missing", "good.jwt.token"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing input, got %d", w.Code)
	}
}

// Missing query parameters are rejected with 400 (auth still enforced first).
func TestParkrunChecker_HandleRecheck_MissingParams(t *testing.T) {
	c := NewParkrunChecker(&mocks.MockDatabase{}, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }
	w := httptest.NewRecorder()
	c.HandleRecheck(w, recheckRequest("", "", "good.jwt.token"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for missing params, got %d", w.Code)
	}
}
