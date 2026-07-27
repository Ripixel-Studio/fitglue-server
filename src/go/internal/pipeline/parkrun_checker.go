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
	parkrunutil "github.com/fitglue/server/src/go/pkg/parkrun"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

// ParkrunChecker polls for parkrun results for all WAITING pending inputs.
// Triggered by Cloud Scheduler → Pub/Sub push → /pubsub/parkrun-check.
type ParkrunChecker struct {
	db     shared.Database
	svc    *Service
	logger infra.Logger
}

func NewParkrunChecker(db shared.Database, svc *Service, logger infra.Logger) *ParkrunChecker {
	return &ParkrunChecker{db: db, svc: svc, logger: logger}
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
	// Expire inputs past their auto-deadline — results won't come now.
	if input.AutoDeadline != nil && time.Now().After(input.AutoDeadline.AsTime()) {
		c.logger.Info(ctx, "parkrun checker: expiring past-deadline input",
			"input_id", input.ActivityId, "user_id", input.UserId,
			"deadline", input.AutoDeadline.AsTime().Format(time.RFC3339))
		if err := c.db.UpdatePendingInput(ctx, input.UserId, input.ActivityId, map[string]interface{}{
			"status":     int32(pbpipeline.PendingInput_STATUS_COMPLETED),
			"updated_at": time.Now(),
		}); err != nil {
			c.logger.Error(ctx, "parkrun checker: failed to expire input",
				"error", err, "input_id", input.ActivityId)
		}
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
