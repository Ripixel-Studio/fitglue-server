// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	parkrunutil "github.com/fitglue/server/src/go/pkg/parkrun"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/api/idtoken"
)

// Outcomes of a single resolve attempt. These are also the keys tallied in the
// /pubsub/parkrun-check response body, so they must stay stable.
const (
	outcomeResolved = "resolved"
	outcomeExpired  = "expired"
	outcomeSkipped  = "skipped"
)

// Reason codes explaining why an input landed on its outcome. The reason is the
// discriminating signal surfaced to error-tracking and returned by the on-demand
// re-check, so it is deliberately machine-greppable.
const (
	reasonResolved        = "resolved"
	reasonExpiredDeadline = "deadline_elapsed"
	reasonNoIntegration   = "no_integration"
	reasonAlreadyExpired  = "already_expired"
	reasonMissingMetadata = "missing_metadata"
	reasonBadDate         = "bad_expected_date"
	reasonUserLookup      = "user_lookup_failed"
	reasonFetchFailed     = "fetch_failed"
	reasonNotPublished    = "results_not_published"
	reasonSubmitFailed    = "submit_failed"
)

// ParkrunDiagnostic captures the outcome of one resolve attempt plus every
// discriminating signal a human needs to tell "the Playwright fetch is broken"
// apart from "the stored slug/country don't match the athlete's results row".
// It is emitted to error-tracking for still-unresolved inputs (see emitDiagnostic)
// and returned verbatim by the on-demand re-check endpoint (see HandleRecheck).
type ParkrunDiagnostic struct {
	InputID     string `json:"input_id"`
	UserID      string `json:"user_id"`
	Outcome     string `json:"outcome"` // resolved | expired | skipped
	Reason      string `json:"reason"`
	EventSlug   string `json:"event_slug,omitempty"`
	Country     string `json:"country,omitempty"` // country host used to build the fetch URL
	URL         string `json:"url,omitempty"`     // resolved parkrunner URL that was fetched
	HTMLBytes   int    `json:"html_bytes"`        // length of HTML the fetcher returned (0 on fetch failure)
	RowsParsed  int    `json:"rows_parsed"`       // valid data rows the parser found
	SlugMatched bool   `json:"slug_matched"`      // any row matched the target event slug?
	DateMatched bool   `json:"date_matched"`      // any slug-matching row also matched the expected date?
	FetchError  string `json:"fetch_error,omitempty"`
	Position    int    `json:"position,omitempty"` // populated only when resolved
	Time        string `json:"time,omitempty"`     // populated only when resolved
}

// surfaceable reports whether this diagnostic describes an input that is still
// stuck in the auto-poll set and should therefore be surfaced to error-tracking.
// Resolved and expired inputs have left the set; an already-EXPIRED input is a
// manual-entry prompt that intentionally no longer polls — none of those are noise
// worth an alert.
func (d ParkrunDiagnostic) surfaceable() bool {
	return d.Outcome == outcomeSkipped && d.Reason != reasonAlreadyExpired
}

// fetchFunc matches parkrunutil.FetchResultsForAthleteWithDiag. It is a field so
// tests can drive processInput deterministically without hitting the network / WAF.
type fetchFunc func(ctx context.Context, logger *slog.Logger, athleteID, countryURL, eventSlug string, expectedDate time.Time) (*parkrunutil.Result, parkrunutil.FetchDiagnostics, error)

// verifyIdentityFunc validates a bearer identity token for the given audience.
// Defaults to Google OIDC validation; overridable in tests.
type verifyIdentityFunc func(ctx context.Context, token, audience string) error

// ParkrunChecker polls for parkrun results for all WAITING pending inputs.
// Triggered by Cloud Scheduler → Pub/Sub push → /pubsub/parkrun-check.
type ParkrunChecker struct {
	db        shared.Database
	svc       *Service
	logger    infra.Logger
	publisher shared.Publisher

	fetch          fetchFunc
	verifyIdentity verifyIdentityFunc
}

func NewParkrunChecker(db shared.Database, svc *Service, logger infra.Logger, publisher shared.Publisher) *ParkrunChecker {
	return &ParkrunChecker{
		db:             db,
		svc:            svc,
		logger:         logger,
		publisher:      publisher,
		fetch:          parkrunutil.FetchResultsForAthleteWithDiag,
		verifyIdentity: defaultVerifyIdentity,
	}
}

// defaultVerifyIdentity validates that token is a live, Google-signed identity
// token minted for this service's URL — the same OIDC trust model Cloud Run
// enforces on the Pub/Sub push subscriptions that target this backend service.
func defaultVerifyIdentity(ctx context.Context, token, audience string) error {
	_, err := idtoken.Validate(ctx, token, audience)
	return err
}

// HandleCheck is the HTTP handler for /pubsub/parkrun-check.
// Returns 200 always (per-input errors are logged but don't fail the batch).
// Returns 500 only if the Firestore query itself fails, so Pub/Sub retries.
func (c *ParkrunChecker) HandleCheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	inputs, err := c.db.ListPendingInputsByEnricher(ctx, "parkrun", pbpipeline.PendingInput_STATUS_WAITING)
	if err != nil {
		c.logger.Error(ctx, "parkrun checker: failed to list pending inputs", "error", err)
		http.Error(w, "failed to list pending inputs", http.StatusInternalServerError)
		return
	}

	c.logger.Info(ctx, "parkrun checker: starting run", "pending_count", len(inputs))

	resolved, expired, skipped, unresolved := 0, 0, 0, 0
	for _, input := range inputs {
		diag := c.processInput(ctx, input)
		switch diag.Outcome {
		case outcomeResolved:
			resolved++
		case outcomeExpired:
			expired++
		default:
			skipped++
		}
		// Surface every still-stuck input so its diagnostics reach error-tracking
		// (Sentry → Slack) without anyone tailing Cloud Run logs. One line per
		// unresolved input per run.
		if diag.surfaceable() {
			unresolved++
			c.emitDiagnostic(ctx, diag)
		}
	}

	c.logger.Info(ctx, "parkrun checker: run complete",
		"resolved", resolved, "expired", expired, "skipped", skipped, "unresolved", unresolved)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":     "ok",
		"resolved":   resolved,
		"expired":    expired,
		"skipped":    skipped,
		"unresolved": unresolved,
	})
}

// emitDiagnostic surfaces a still-unresolved input's diagnostics to error-tracking.
// It logs at Error level so the SentryHandler forwards it to Sentry (and onward to
// the team's Slack alert channel). The message is constant so all such events group
// into a single Sentry issue; the per-input signals ride along as context. Note the
// fetch error is carried under "fetch_error" (a string), NOT "error", so this stays a
// grouped CaptureMessage rather than a distinct CaptureException per input.
func (c *ParkrunChecker) emitDiagnostic(ctx context.Context, d ParkrunDiagnostic) {
	c.logger.Error(ctx, "parkrun checker: pending input still unresolved this run",
		"reason", d.Reason,
		"input_id", d.InputID,
		"user_id", d.UserID,
		"event_slug", d.EventSlug,
		"country", d.Country,
		"url", d.URL,
		"html_bytes", d.HTMLBytes,
		"rows_parsed", d.RowsParsed,
		"slug_matched", d.SlugMatched,
		"date_matched", d.DateMatched,
		"fetch_error", d.FetchError,
	)
}

// HandleRecheck is the HTTP handler for /internal/parkrun-recheck. It runs a real
// resolve attempt for a single pending input right now and returns the diagnostic,
// so an operator can probe one input without waiting for the next 2-hourly scheduler
// fire. Because it drives processInput, it CAN resolve or expire the input as a side
// effect — it is a real re-check, not a dry run.
//
// Guarded by the same OIDC trust boundary as the other internal endpoints on this
// backend service: the caller must present a Google-signed identity token minted for
// this service's URL. Invoke with, e.g.:
//
//	curl -X POST -H "Authorization: Bearer $(gcloud auth print-identity-token \
//	  --audiences=https://<pipeline-service-url>)" \
//	  "https://<pipeline-service-url>/internal/parkrun-recheck?user_id=U&input_id=I"
func (c *ParkrunChecker) HandleRecheck(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	authz := r.Header.Get("Authorization")
	if !strings.HasPrefix(authz, "Bearer ") {
		http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
		return
	}
	token := strings.TrimPrefix(authz, "Bearer ")
	audience := "https://" + r.Host
	if err := c.verifyIdentity(ctx, token, audience); err != nil {
		c.logger.Warn(ctx, "parkrun recheck: identity verification failed", "error", err)
		http.Error(w, "invalid identity token", http.StatusForbidden)
		return
	}

	userID := r.URL.Query().Get("user_id")
	inputID := r.URL.Query().Get("input_id")
	if userID == "" || inputID == "" {
		http.Error(w, "user_id and input_id query parameters are required", http.StatusBadRequest)
		return
	}

	input, err := c.db.GetPendingInput(ctx, userID, inputID)
	if err != nil {
		c.logger.Error(ctx, "parkrun recheck: failed to load pending input",
			"error", err, "user_id", userID, "input_id", inputID)
		http.Error(w, "pending input not found", http.StatusNotFound)
		return
	}

	diag := c.processInput(ctx, input)
	c.logger.Info(ctx, "parkrun recheck: on-demand check complete",
		"input_id", inputID, "user_id", userID, "outcome", diag.Outcome, "reason", diag.Reason)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(diag)
}

// processInput attempts to resolve one pending input, returning a diagnostic that
// records both the outcome and the discriminating signals of the attempt.
func (c *ParkrunChecker) processInput(ctx context.Context, input *pbpipeline.PendingInput) ParkrunDiagnostic {
	diag := ParkrunDiagnostic{
		InputID:   input.ActivityId,
		UserID:    input.UserId,
		EventSlug: input.ProviderMetadata["parkrun_event_slug"],
	}

	// Already expired to manual entry — it stays WAITING so the user can still submit
	// stats by hand, but it has left the auto-poll set. Do nothing: no re-notify, no
	// re-expire, no fetch. This must run before the deadline check below, since an
	// expired input is (by definition) also past its deadline.
	if input.ProviderMetadata["parkrun_results_state"] == "EXPIRED" {
		diag.Outcome, diag.Reason = outcomeSkipped, reasonAlreadyExpired
		return diag
	}

	// Deadline elapsed without results — the official results won't come now. Rather than
	// closing the input (STATUS_COMPLETED is terminal AND un-submittable, so the user could
	// never enter stats and the input silently vanished), flip it into a manual-entry prompt:
	// keep it WAITING, drop auto_populated, and mark parkrun_results_state=EXPIRED so the
	// skip above removes it from future auto-poll runs. The input already carries the
	// display.* metadata the client needs to render the "Enter Parkrun Results" form.
	if input.AutoDeadline != nil && time.Now().After(input.AutoDeadline.AsTime()) {
		c.logger.Info(ctx, "parkrun checker: deadline elapsed, prompting manual entry",
			"input_id", input.ActivityId, "user_id", input.UserId,
			"deadline", input.AutoDeadline.AsTime().Format(time.RFC3339))
		diag.Outcome, diag.Reason = outcomeExpired, reasonExpiredDeadline
		if err := c.db.UpdatePendingInput(ctx, input.UserId, input.ActivityId, map[string]interface{}{
			"status":         int32(pbpipeline.PendingInput_STATUS_WAITING),
			"auto_populated": false,
			// Nested map so firestore.MergeAll updates only this leaf and preserves the
			// other provider_metadata keys (event slug, display.* form config, …).
			"provider_metadata": map[string]interface{}{"parkrun_results_state": "EXPIRED"},
			"updated_at":        time.Now(),
		}); err != nil {
			// State didn't persist — don't notify. The input stays WAITING without the
			// EXPIRED marker, so the next run will retry the transition (and only then
			// notify), keeping notifications exactly-once on the happy path.
			c.logger.Error(ctx, "parkrun checker: failed to mark input expired",
				"error", err, "input_id", input.ActivityId)
			return diag
		}
		c.notifyExpired(ctx, input)
		return diag
	}

	// Need the user's parkrun integration for athlete ID + country URL.
	user, err := c.db.GetUser(ctx, input.UserId)
	if err != nil {
		c.logger.Error(ctx, "parkrun checker: failed to get user",
			"error", err, "user_id", input.UserId)
		diag.Outcome, diag.Reason = outcomeSkipped, reasonUserLookup
		return diag
	}
	if user.Integrations == nil || user.Integrations.Parkrun == nil || !user.Integrations.Parkrun.Enabled {
		// Integration removed — can never resolve. Expire immediately.
		c.logger.Warn(ctx, "parkrun checker: user has no parkrun integration, expiring",
			"user_id", input.UserId, "input_id", input.ActivityId)
		c.db.UpdatePendingInput(ctx, input.UserId, input.ActivityId, map[string]interface{}{ //nolint:errcheck
			"status":     int32(pbpipeline.PendingInput_STATUS_COMPLETED),
			"updated_at": time.Now(),
		})
		diag.Outcome, diag.Reason = outcomeExpired, reasonNoIntegration
		return diag
	}

	integration := user.Integrations.Parkrun
	eventSlug := input.ProviderMetadata["parkrun_event_slug"]
	eventName := input.ProviderMetadata["parkrun_event_name"]
	expectedDateStr := input.ProviderMetadata["expected_date"] // stored as DD/MM/YYYY

	if eventSlug == "" || expectedDateStr == "" {
		c.logger.Error(ctx, "parkrun checker: input missing required metadata",
			"input_id", input.ActivityId, "event_slug", eventSlug, "expected_date", expectedDateStr)
		diag.Outcome, diag.Reason = outcomeSkipped, reasonMissingMetadata
		return diag
	}

	expectedDate, err := time.Parse("02/01/2006", expectedDateStr)
	if err != nil {
		c.logger.Error(ctx, "parkrun checker: could not parse expected_date",
			"date", expectedDateStr, "error", err, "input_id", input.ActivityId)
		diag.Outcome, diag.Reason = outcomeSkipped, reasonBadDate
		return diag
	}

	// Resolve the country host for the athlete's results page. The athlete page
	// lives on the runner's HOME domain, so prefer the integration's CountryUrl;
	// fall back to the canonical host stored on the pending input
	// (provider_metadata["parkrun_country"], e.g. "www.parkrun.org.uk"). The
	// value is normalized inside FetchResultsForAthlete — see NormalizeCountryHost.
	countryURL := integration.CountryUrl
	if countryURL == "" {
		countryURL = input.ProviderMetadata["parkrun_country"]
	}
	diag.Country = countryURL

	results, fdiag, err := c.fetch(ctx, slog.Default(),
		integration.AthleteId, countryURL, eventSlug, expectedDate)
	diag.URL = fdiag.URL
	diag.HTMLBytes = fdiag.HTMLBytes
	diag.RowsParsed = fdiag.RowsParsed
	diag.SlugMatched = fdiag.SlugMatched
	diag.DateMatched = fdiag.DateMatched
	if err != nil {
		// Transient failure — retry at next scheduled check. Surfaced by the caller.
		diag.Outcome, diag.Reason, diag.FetchError = outcomeSkipped, reasonFetchFailed, err.Error()
		return diag
	}
	if results == nil {
		// No matching result. Could be "not published yet" (normal for morning runs)
		// OR a bad fetch — the html_bytes / rows_parsed / slug_matched signals on the
		// surfaced diagnostic tell the two apart.
		diag.Outcome, diag.Reason = outcomeSkipped, reasonNotPublished
		return diag
	}

	desc := parkrunutil.FormatResultsDescription(results, eventName)
	_, submitErr := c.svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
		UserId:         input.UserId,
		PendingInputId: input.ActivityId,
		// The stats beyond position/time/age_grade (total run count + PB flags) must
		// ride along in InputData: EnrichResume rebuilds the ParkrunSummary card solely
		// from these values, so anything omitted here renders as a zero/false on the
		// card (e.g. "0 TOTAL RUNS", missing PB stamps) even though the fetch succeeded.
		InputData: map[string]string{
			"description":     desc,
			"position":        strconv.Itoa(results.Position),
			"time":            results.Time,
			"age_grade":       results.AgeGrade,
			"total_parkruns":  strconv.Itoa(results.TotalAllTime),
			"is_time_pb":      strconv.FormatBool(results.TimeAllTimePB),
			"is_age_grade_pb": strconv.FormatBool(results.AgeGradeAllTimePB),
		},
	})
	if submitErr != nil {
		c.logger.Error(ctx, "parkrun checker: submit failed",
			"error", submitErr, "input_id", input.ActivityId)
		diag.Outcome, diag.Reason, diag.FetchError = outcomeSkipped, reasonSubmitFailed, submitErr.Error()
		return diag
	}

	c.logger.Info(ctx, "parkrun checker: resolved pending input",
		"input_id", input.ActivityId, "user_id", input.UserId,
		"position", results.Position, "time", results.Time)
	diag.Outcome, diag.Reason = outcomeResolved, reasonResolved
	diag.Position, diag.Time = results.Position, results.Time
	return diag
}

// notifyExpired enqueues a PENDING_INPUT notification telling the user their parkrun
// results never published and they can now enter their stats manually. Fans out to
// push + email per the user's stored notification prefs (handled by the notification
// service). Data carries both IDs so the client can deep-link to the manual-entry form.
func (c *ParkrunChecker) notifyExpired(ctx context.Context, input *pbpipeline.PendingInput) {
	if c.publisher == nil {
		c.logger.Warn(ctx, "parkrun checker: publisher unavailable, EXPIRED notification not sent",
			"user_id", input.UserId, "input_id", input.ActivityId)
		return
	}
	// The client deep-links off the linked activity; fall back to the input's own ID
	// (older inputs created before linked_activity_id was populated).
	activityID := input.LinkedActivityId
	if activityID == "" {
		activityID = input.ActivityId
	}
	req := &pbnotification.NotificationRequest{
		UserId: input.UserId,
		Type:   pbnotification.NotificationType_NOTIFICATION_TYPE_PENDING_INPUT,
		Title:  "Enter your parkrun results",
		Body:   "Your parkrun results didn't publish in time — tap to enter your stats manually.",
		Data: map[string]string{
			"activity_id":      activityID,
			"pending_input_id": input.ActivityId,
		},
	}
	if err := notificationpub.Enqueue(ctx, c.publisher, req); err != nil {
		c.logger.Error(ctx, "parkrun checker: failed to enqueue EXPIRED notification",
			"error", err, "user_id", input.UserId, "input_id", input.ActivityId)
	}
}
