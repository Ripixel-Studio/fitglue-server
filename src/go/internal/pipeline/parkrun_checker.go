// nolint:proto-json
package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/notificationpub"
	parkrunutil "github.com/fitglue/server/src/go/pkg/parkrun"
	pbnotification "github.com/fitglue/server/src/go/pkg/types/pb/models/notification"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

// ParkrunChecker polls for parkrun results for all WAITING pending inputs.
// Triggered by Cloud Scheduler → Pub/Sub push → /pubsub/parkrun-check.
type ParkrunChecker struct {
	db        shared.Database
	svc       *Service
	logger    infra.Logger
	publisher shared.Publisher
}

func NewParkrunChecker(db shared.Database, svc *Service, logger infra.Logger, publisher shared.Publisher) *ParkrunChecker {
	return &ParkrunChecker{db: db, svc: svc, logger: logger, publisher: publisher}
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

	resolved, expired, skipped := 0, 0, 0
	for _, input := range inputs {
		switch c.processInput(ctx, input) {
		case "resolved":
			resolved++
		case "expired":
			expired++
		default:
			skipped++
		}
	}

	c.logger.Info(ctx, "parkrun checker: run complete",
		"resolved", resolved, "expired", expired, "skipped", skipped)

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":   "ok",
		"resolved": resolved,
		"expired":  expired,
		"skipped":  skipped,
	})
}

// processInput attempts to resolve one pending input. Returns "resolved", "expired", or "skipped".
func (c *ParkrunChecker) processInput(ctx context.Context, input *pbpipeline.PendingInput) string {
	// Already expired to manual entry — it stays WAITING so the user can still submit
	// stats by hand, but it has left the auto-poll set. Do nothing: no re-notify, no
	// re-expire, no fetch. This must run before the deadline check below, since an
	// expired input is (by definition) also past its deadline.
	if input.ProviderMetadata["parkrun_results_state"] == "EXPIRED" {
		return "skipped"
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
			return "expired"
		}
		c.notifyExpired(ctx, input)
		return "expired"
	}

	// Need the user's parkrun integration for athlete ID + country URL.
	user, err := c.db.GetUser(ctx, input.UserId)
	if err != nil {
		c.logger.Error(ctx, "parkrun checker: failed to get user",
			"error", err, "user_id", input.UserId)
		return "skipped"
	}
	if user.Integrations == nil || user.Integrations.Parkrun == nil || !user.Integrations.Parkrun.Enabled {
		// Integration removed — can never resolve. Expire immediately.
		c.logger.Warn(ctx, "parkrun checker: user has no parkrun integration, expiring",
			"user_id", input.UserId, "input_id", input.ActivityId)
		c.db.UpdatePendingInput(ctx, input.UserId, input.ActivityId, map[string]interface{}{ //nolint:errcheck
			"status":     int32(pbpipeline.PendingInput_STATUS_COMPLETED),
			"updated_at": time.Now(),
		})
		return "expired"
	}

	integration := user.Integrations.Parkrun
	eventSlug := input.ProviderMetadata["parkrun_event_slug"]
	eventName := input.ProviderMetadata["parkrun_event_name"]
	expectedDateStr := input.ProviderMetadata["expected_date"] // stored as DD/MM/YYYY

	if eventSlug == "" || expectedDateStr == "" {
		c.logger.Error(ctx, "parkrun checker: input missing required metadata",
			"input_id", input.ActivityId, "event_slug", eventSlug, "expected_date", expectedDateStr)
		return "skipped"
	}

	expectedDate, err := time.Parse("02/01/2006", expectedDateStr)
	if err != nil {
		c.logger.Error(ctx, "parkrun checker: could not parse expected_date",
			"date", expectedDateStr, "error", err, "input_id", input.ActivityId)
		return "skipped"
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

	results, diag, err := parkrunutil.FetchResultsForAthleteWithDiag(ctx, slog.Default(),
		integration.AthleteId, countryURL, eventSlug, expectedDate)
	if err != nil {
		// Transient failure — log and retry at next scheduled check.
		c.logger.Warn(ctx, "parkrun checker: fetch failed, will retry next cycle",
			"error", err, "input_id", input.ActivityId, "event", eventSlug,
			"url", diag.URL, "country_url", countryURL)
		return "skipped"
	}
	if results == nil {
		// No matching result. Could be "not published yet" (normal for morning
		// runs) OR a bad fetch — log the diagnostics so the two are distinguishable.
		c.logger.Info(ctx, "parkrun checker: results not yet available",
			"event", eventSlug, "date", expectedDateStr, "input_id", input.ActivityId,
			"url", diag.URL, "html_bytes", diag.HTMLBytes, "rows_parsed", diag.RowsParsed,
			"slug_matched", diag.SlugMatched, "date_matched", diag.DateMatched)
		return "skipped"
	}

	desc := parkrunutil.FormatResultsDescription(results, eventName)
	_, submitErr := c.svc.SubmitInput(ctx, &pbsvc.SubmitInputRequest{
		UserId:         input.UserId,
		PendingInputId: input.ActivityId,
		InputData: map[string]string{
			"description": desc,
			"position":    strconv.Itoa(results.Position),
			"time":        results.Time,
			"age_grade":   results.AgeGrade,
		},
	})
	if submitErr != nil {
		c.logger.Error(ctx, "parkrun checker: submit failed",
			"error", submitErr, "input_id", input.ActivityId)
		return "skipped"
	}

	c.logger.Info(ctx, "parkrun checker: resolved pending input",
		"input_id", input.ActivityId, "user_id", input.UserId,
		"position", results.Position, "time", results.Time)
	return "resolved"
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
