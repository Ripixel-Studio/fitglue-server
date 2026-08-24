package pipeline

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

func agentRequest(method, path, token string, body []byte) *http.Request {
	r := httptest.NewRequest(method, "https://pipeline.example.run.app"+path, bytes.NewReader(body))
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	return r
}

func newarkInput(state string, expected string) *pbpipeline.PendingInput {
	md := map[string]string{
		"parkrun_event_slug": "newark",
		"parkrun_event_name": "Newark parkrun",
		"parkrun_country":    "www.parkrun.org.uk",
		"expected_date":      expected,
	}
	if state != "" {
		md["parkrun_results_state"] = state
	}
	return &pbpipeline.PendingInput{
		ActivityId:       "SOURCE_STRAVA:19846909826:parkrun",
		UserId:           "u1",
		Status:           pbpipeline.PendingInput_STATUS_WAITING,
		ProviderMetadata: md,
	}
}

func newarkUser() *user.Record {
	u := parkrunUser()
	u.Integrations.Parkrun.AthleteId = "6790488"
	u.Integrations.Parkrun.CountryUrl = "www.parkrun.org.uk"
	return u
}

// Both agent endpoints enforce the identity token exactly like the recheck endpoint.
func TestParkrunAgent_Auth(t *testing.T) {
	c := NewParkrunChecker(&mocks.MockDatabase{}, nil, infra.NewLogger(), nil)

	w := httptest.NewRecorder()
	c.HandlePending(w, agentRequest(http.MethodGet, "/internal/parkrun-pending", "", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("pending without token: got %d, want 401", w.Code)
	}
	w = httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "", []byte(`{}`)))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("submit without token: got %d, want 401", w.Code)
	}

	c.verifyIdentity = func(_ context.Context, _, _ string) error { return errors.New("bad") }
	w = httptest.NewRecorder()
	c.HandlePending(w, agentRequest(http.MethodGet, "/internal/parkrun-pending", "x.y.z", nil))
	if w.Code != http.StatusForbidden {
		t.Fatalf("pending with rejected token: got %d, want 403", w.Code)
	}
	w = httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "x.y.z", []byte(`{}`)))
	if w.Code != http.StatusForbidden {
		t.Fatalf("submit with rejected token: got %d, want 403", w.Code)
	}
}

// The pending list includes auto-polling AND recently-EXPIRED inputs (the cloud
// fetch may have been captcha-walled for the whole deadline window), excludes
// inputs older than the window or without a usable integration, and resolves
// the parkrunner URL from the user's home country.
func TestParkrunAgent_Pending(t *testing.T) {
	recent := time.Now().Add(-2 * 24 * time.Hour).Format("02/01/2006")
	ancient := time.Now().Add(-60 * 24 * time.Hour).Format("02/01/2006")
	noIntegration := newarkInput("", recent)
	noIntegration.UserId = "u-none"
	noIntegration.ActivityId = "i-none"
	expired := newarkInput("EXPIRED", recent)
	expired.ActivityId = "i-expired"
	old := newarkInput("EXPIRED", ancient)
	old.ActivityId = "i-old"

	db := &mocks.MockDatabase{
		ListPendingInputsByEnricherFunc: func(_ context.Context, enricher string, status pbpipeline.PendingInput_Status) ([]*pbpipeline.PendingInput, error) {
			if enricher != "parkrun" || status != pbpipeline.PendingInput_STATUS_WAITING {
				t.Fatalf("unexpected list args %s/%v", enricher, status)
			}
			return []*pbpipeline.PendingInput{newarkInput("", recent), expired, old, noIntegration}, nil
		},
		GetUserFunc: func(_ context.Context, id string) (*user.Record, error) {
			if id == "u-none" {
				return &user.Record{}, nil
			}
			return newarkUser(), nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }

	w := httptest.NewRecorder()
	c.HandlePending(w, agentRequest(http.MethodGet, "/internal/parkrun-pending", "ok", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var resp struct {
		Items []AgentPendingItem `json:"items"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Items) != 2 {
		t.Fatalf("want 2 fetchable items (auto + recently expired), got %d: %+v", len(resp.Items), resp.Items)
	}
	if resp.Items[0].URL != "https://www.parkrun.org.uk/parkrunner/6790488/all/" {
		t.Errorf("url = %s", resp.Items[0].URL)
	}
	if resp.Items[0].EventSlug != "newark" || resp.Items[0].ExpectedDate != recent || resp.Items[0].State != "" {
		t.Errorf("item 0 = %+v", resp.Items[0])
	}
	if resp.Items[1].InputID != "i-expired" || resp.Items[1].State != "EXPIRED" {
		t.Errorf("item 1 = %+v", resp.Items[1])
	}
}

// The page a residential IP gets for the athlete on 2026-08-24 (plain curl,
// 43KB) resolves the 22/08/2026 Newark row (event #617) — 257th in 43:46 — and submits it
// through the same SubmitInput path as the scheduled checker, even though the
// input had already been EXPIRED to a manual-entry prompt.
func TestParkrunAgent_SubmitHTML_ResolvesRealPage(t *testing.T) {
	page, err := os.ReadFile("testdata/parkrunner_all_2026-08-22.html")
	if err != nil {
		t.Fatal(err)
	}
	input := newarkInput("EXPIRED", "22/08/2026")
	var submitted *pbsvc.SubmitInputRequest
	db := &mocks.MockDatabase{
		GetPendingInputFunc: func(_ context.Context, userID, id string) (*pbpipeline.PendingInput, error) {
			if userID != input.UserId || id != input.ActivityId {
				return nil, errors.New("not found")
			}
			return input, nil
		},
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }
	c.submit = func(_ context.Context, req *pbsvc.SubmitInputRequest) error { submitted = req; return nil }

	body, _ := json.Marshal(AgentSubmitRequest{UserID: input.UserId, InputID: input.ActivityId, HTML: string(page)})
	w := httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "ok", body))
	if w.Code != http.StatusOK {
		t.Fatalf("got %d: %s", w.Code, w.Body.String())
	}
	var diag ParkrunDiagnostic
	if err := json.Unmarshal(w.Body.Bytes(), &diag); err != nil {
		t.Fatal(err)
	}
	if diag.Outcome != outcomeResolved || diag.Reason != reasonResolved {
		t.Fatalf("outcome/reason = %s/%s (%+v)", diag.Outcome, diag.Reason, diag)
	}
	if diag.Position != 257 || diag.Time != "43:46" {
		t.Errorf("position/time = %d/%s, want 257/43:46", diag.Position, diag.Time)
	}
	if submitted == nil {
		t.Fatal("nothing submitted")
	}
	if submitted.PendingInputId != input.ActivityId || submitted.InputData["position"] != "257" ||
		submitted.InputData["time"] != "43:46" || submitted.InputData["age_grade"] != "29.97%" {
		t.Errorf("submitted = %+v", submitted.InputData)
	}
}

// A captcha interstitial posted by the agent is reported as a fetch failure,
// never as "results not published".
func TestParkrunAgent_SubmitHTML_BotChallenge(t *testing.T) {
	page, err := os.ReadFile("testdata/aws_waf_challenge.html")
	if err != nil {
		t.Fatal(err)
	}
	input := newarkInput("", "22/08/2026")
	db := &mocks.MockDatabase{
		GetPendingInputFunc: func(_ context.Context, _, _ string) (*pbpipeline.PendingInput, error) { return input, nil },
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }
	c.submit = func(_ context.Context, _ *pbsvc.SubmitInputRequest) error { t.Fatal("must not submit"); return nil }

	body, _ := json.Marshal(AgentSubmitRequest{UserID: "u1", InputID: input.ActivityId, HTML: string(page)})
	w := httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "ok", body))
	var diag ParkrunDiagnostic
	if err := json.Unmarshal(w.Body.Bytes(), &diag); err != nil {
		t.Fatal(err)
	}
	if diag.Reason != reasonFetchFailed || diag.FetchError == "" {
		t.Fatalf("want fetch_failed with bot_challenge error, got %+v", diag)
	}
}

// Results not yet on the page: not published, nothing submitted, 200 with diagnostics.
func TestParkrunAgent_SubmitHTML_NotPublished(t *testing.T) {
	page, _ := os.ReadFile("testdata/parkrunner_all_2026-08-22.html")
	input := newarkInput("", "29/08/2026")
	db := &mocks.MockDatabase{
		GetPendingInputFunc: func(_ context.Context, _, _ string) (*pbpipeline.PendingInput, error) { return input, nil },
	}
	c := NewParkrunChecker(db, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }
	c.submit = func(_ context.Context, _ *pbsvc.SubmitInputRequest) error { t.Fatal("must not submit"); return nil }

	body, _ := json.Marshal(AgentSubmitRequest{UserID: "u1", InputID: input.ActivityId, HTML: string(page)})
	w := httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "ok", body))
	var diag ParkrunDiagnostic
	if err := json.Unmarshal(w.Body.Bytes(), &diag); err != nil {
		t.Fatal(err)
	}
	if diag.Reason != reasonNotPublished || !diag.SlugMatched || diag.DateMatched {
		t.Fatalf("want results_not_published with slug matched/date unmatched, got %+v", diag)
	}
}

func TestParkrunAgent_SubmitHTML_BadRequests(t *testing.T) {
	c := NewParkrunChecker(&mocks.MockDatabase{
		GetPendingInputFunc: func(_ context.Context, _, _ string) (*pbpipeline.PendingInput, error) { return nil, errors.New("nope") },
	}, nil, infra.NewLogger(), nil)
	c.verifyIdentity = func(_ context.Context, _, _ string) error { return nil }

	w := httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "ok", []byte(`not json`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("bad json: %d", w.Code)
	}
	w = httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "ok", []byte(`{"user_id":"u"}`)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("missing fields: %d", w.Code)
	}
	w = httptest.NewRecorder()
	c.HandleSubmitHTML(w, agentRequest(http.MethodPost, "/internal/parkrun-html", "ok", []byte(`{"user_id":"u","input_id":"i","html":"<html>"}`)))
	if w.Code != http.StatusNotFound {
		t.Errorf("unknown input: %d", w.Code)
	}
	w = httptest.NewRecorder()
	c.HandlePending(w, agentRequest(http.MethodPost, "/internal/parkrun-pending", "ok", nil))
	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("pending POST: %d", w.Code)
	}
}
