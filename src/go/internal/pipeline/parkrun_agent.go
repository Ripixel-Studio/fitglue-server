// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	parkrunutil "github.com/fitglue/server/src/go/pkg/parkrun"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
)

// Off-cloud fetch agent endpoints.
//
// parkrun.org.uk sits behind AWS WAF, which since 2026-08-22 answers every
// request from Google Cloud egress with a captcha interstitial ("Human
// Verification") regardless of browser fingerprint — the Playwright fetcher on
// Cloud Run gets the same 10KB challenge page a plain curl does, while the same
// code from a residential IP gets the full results page. No in-cloud change can
// fix an IP-reputation block, so results are fetched by an agent running on a
// residential box instead. The agent is deliberately dumb: it asks this service
// what needs fetching, GETs the page, and posts the raw HTML back. All parsing,
// matching and submission stay here, so the agent never needs credentials
// beyond a Cloud Run invoker identity.
//
// Both endpoints require a Google-signed identity token for this service's
// URL, exactly like /internal/parkrun-recheck.

// agentPendingWindow bounds how far back the pending list reaches. Inputs the
// scheduled checker has already EXPIRED (deadline elapsed) stay WAITING as a
// manual-entry prompt; the agent still fetches those for a while so a stretch
// of failed cloud fetches is recovered automatically once results are reachable.
const agentPendingWindow = 21 * 24 * time.Hour

// agentMaxHTMLBytes caps the accepted HTML body. A full parkrunner /all/ page
// for a prolific runner is a few hundred KB; 4MB leaves generous headroom.
const agentMaxHTMLBytes = 4 << 20

// AgentPendingItem is one input the agent should fetch.
type AgentPendingItem struct {
	UserID       string `json:"user_id"`
	InputID      string `json:"input_id"`
	URL          string `json:"url"`
	EventSlug    string `json:"event_slug"`
	ExpectedDate string `json:"expected_date"` // DD/MM/YYYY
	State        string `json:"state"`         // "" (auto-polling) | "EXPIRED" (manual prompt)
}

// AgentSubmitRequest is the body of POST /internal/parkrun-html.
type AgentSubmitRequest struct {
	UserID  string `json:"user_id"`
	InputID string `json:"input_id"`
	URL     string `json:"url,omitempty"` // for diagnostics only
	HTML    string `json:"html"`
}

// authorizeAgent enforces the identity-token check shared by the /internal/*
// endpoints. It writes the error response itself and reports whether to continue.
func (c *ParkrunChecker) authorizeAgent(w http.ResponseWriter, r *http.Request) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" || token == r.Header.Get("Authorization") {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return false
	}
	audience := "https://" + r.Host
	if err := c.verifyIdentity(r.Context(), token, audience); err != nil {
		c.logger.Warn(r.Context(), "parkrun agent: identity verification failed", "error", err)
		http.Error(w, "invalid identity token", http.StatusForbidden)
		return false
	}
	return true
}

// HandlePending is GET /internal/parkrun-pending: the list of parkrun inputs
// still awaiting results, each with the parkrunner URL the agent should fetch.
func (c *ParkrunChecker) HandlePending(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.authorizeAgent(w, r) {
		return
	}
	ctx := r.Context()

	inputs, err := c.db.ListPendingInputsByEnricher(ctx, "parkrun", pbpipeline.PendingInput_STATUS_WAITING)
	if err != nil {
		c.logger.Error(ctx, "parkrun agent: failed to list pending inputs", "error", err)
		http.Error(w, "failed to list pending inputs", http.StatusInternalServerError)
		return
	}

	items := make([]AgentPendingItem, 0, len(inputs))
	for _, input := range inputs {
		item, ok := c.pendingItem(ctx, input, time.Now())
		if ok {
			items = append(items, item)
		}
	}

	c.logger.Info(ctx, "parkrun agent: pending list served", "pending", len(inputs), "fetchable", len(items))
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{"items": items}) //nolint:errcheck
}

// pendingItem turns a WAITING input into an agent work item, or reports false
// when it is not fetchable (no integration, malformed metadata, outside the window).
func (c *ParkrunChecker) pendingItem(ctx context.Context, input *pbpipeline.PendingInput, now time.Time) (AgentPendingItem, bool) {
	eventSlug := input.ProviderMetadata["parkrun_event_slug"]
	expectedDateStr := input.ProviderMetadata["expected_date"]
	if eventSlug == "" || expectedDateStr == "" {
		return AgentPendingItem{}, false
	}
	expectedDate, err := time.Parse("02/01/2006", expectedDateStr)
	if err != nil {
		return AgentPendingItem{}, false
	}
	if now.Sub(expectedDate) > agentPendingWindow {
		return AgentPendingItem{}, false
	}

	user, err := c.db.GetUser(ctx, input.UserId)
	if err != nil || user.Integrations == nil || user.Integrations.Parkrun == nil || !user.Integrations.Parkrun.Enabled {
		return AgentPendingItem{}, false
	}
	integration := user.Integrations.Parkrun
	countryURL := integration.CountryUrl
	if countryURL == "" {
		countryURL = input.ProviderMetadata["parkrun_country"]
	}

	return AgentPendingItem{
		UserID:       input.UserId,
		InputID:      input.ActivityId,
		URL:          parkrunutil.BuildAthleteResultsURL(integration.AthleteId, countryURL),
		EventSlug:    eventSlug,
		ExpectedDate: expectedDateStr,
		State:        input.ProviderMetadata["parkrun_results_state"],
	}, true
}

// HandleSubmitHTML is POST /internal/parkrun-html: the agent hands back the raw
// parkrunner page for one input. The page is parsed exactly as the scheduled
// checker would parse a Playwright fetch; a match is submitted through the same
// SubmitInput path. Responds with the ParkrunDiagnostic for the attempt.
func (c *ParkrunChecker) HandleSubmitHTML(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !c.authorizeAgent(w, r) {
		return
	}
	ctx := r.Context()

	var req AgentSubmitRequest
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, agentMaxHTMLBytes))
	if err != nil {
		http.Error(w, "body too large or unreadable", http.StatusRequestEntityTooLarge)
		return
	}
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "invalid JSON body", http.StatusBadRequest)
		return
	}
	if req.UserID == "" || req.InputID == "" || req.HTML == "" {
		http.Error(w, "user_id, input_id and html are required", http.StatusBadRequest)
		return
	}

	input, err := c.db.GetPendingInput(ctx, req.UserID, req.InputID)
	if err != nil {
		c.logger.Error(ctx, "parkrun agent: failed to load pending input",
			"error", err, "user_id", req.UserID, "input_id", req.InputID)
		http.Error(w, "pending input not found", http.StatusNotFound)
		return
	}

	diag := c.resolveFromHTML(ctx, input, req.HTML)
	diag.URL = req.URL
	if diag.surfaceable() && diag.Reason != reasonNotPublished {
		c.emitDiagnostic(ctx, diag)
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(diag) //nolint:errcheck
}

// resolveFromHTML parses agent-supplied HTML for one input and submits a match.
// Unlike processInput it does not expire on deadline: an EXPIRED input is
// precisely the case the agent exists to rescue, and SubmitInput accepts any
// WAITING input.
func (c *ParkrunChecker) resolveFromHTML(ctx context.Context, input *pbpipeline.PendingInput, html string) ParkrunDiagnostic {
	diag := ParkrunDiagnostic{
		InputID:   input.ActivityId,
		UserID:    input.UserId,
		EventSlug: input.ProviderMetadata["parkrun_event_slug"],
		HTMLBytes: len(html),
	}
	if input.Status != pbpipeline.PendingInput_STATUS_WAITING {
		diag.Outcome, diag.Reason = outcomeSkipped, reasonNotWaiting
		return diag
	}
	if parkrunutil.IsBotChallenge(html) {
		diag.Outcome, diag.Reason, diag.FetchError = outcomeSkipped, reasonFetchFailed, parkrunutil.ErrBotChallenge.Error()
		return diag
	}

	eventSlug := input.ProviderMetadata["parkrun_event_slug"]
	eventName := input.ProviderMetadata["parkrun_event_name"]
	expectedDateStr := input.ProviderMetadata["expected_date"]
	if eventSlug == "" || expectedDateStr == "" {
		diag.Outcome, diag.Reason = outcomeSkipped, reasonMissingMetadata
		return diag
	}
	expectedDate, err := time.Parse("02/01/2006", expectedDateStr)
	if err != nil {
		diag.Outcome, diag.Reason = outcomeSkipped, reasonBadDate
		return diag
	}

	results, pdiag, err := parkrunutil.ParseAthleteResultsBySlugWithDiag(slogDefault(), html, eventSlug, expectedDate)
	diag.RowsParsed, diag.SlugMatched, diag.DateMatched = pdiag.RowsParsed, pdiag.SlugMatched, pdiag.DateMatched
	if err != nil {
		diag.Outcome, diag.Reason, diag.FetchError = outcomeSkipped, reasonFetchFailed, err.Error()
		return diag
	}
	if results == nil {
		diag.Outcome, diag.Reason = outcomeSkipped, reasonNotPublished
		return diag
	}
	return c.submitResults(ctx, input, eventName, results, diag)
}
