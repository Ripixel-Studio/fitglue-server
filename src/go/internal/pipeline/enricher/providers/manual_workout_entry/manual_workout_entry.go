package manual_workout_entry

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers/user_input"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	pendinginput "github.com/fitglue/server/src/go/pkg/pending_input"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"

	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"

	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

// Provider pauses the pipeline for activities that have no strength data,
// requests manual exercise/set/rep entry from the user, then populates
// Sessions[0].StrengthSets so downstream enrichers (workout_summary) can render it.
type Provider struct {
	service *bootstrap.Service
}

func init() {
	providers.Register(&Provider{})
}

func (p *Provider) SetService(s *bootstrap.Service) {
	p.service = s
}

func (p *Provider) Name() string { return "manual-workout-entry" }

func (p *Provider) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MANUAL_WORKOUT_ENTRY
}

// Enrich checks whether the activity already has strength data. If not, it
// raises a WaitForInputError so the orchestrator can create a pending-input
// document and notify the user via the app.
func (p *Provider) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, userRec *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	// Already has strength data — nothing to do.
	if len(activity.Sessions) > 0 && len(activity.Sessions[0].StrengthSets) > 0 {
		logger.Debug("manual-workout-entry: skipping — activity already has strength sets")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "activity already has strength data",
		}, nil
	}

	// User dismissed the prompt — skip gracefully.
	if doNotRetry {
		logger.Debug("manual-workout-entry: skipping — do-not-retry flag set")
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "user dismissed manual workout entry",
		}, nil
	}

	if p.service == nil {
		return nil, fmt.Errorf("manual-workout-entry: service not initialized")
	}

	stableID := pendinginput.GenerateID(activity.Source.String(), activity.ExternalId, p.Name())

	pending, err := p.service.DB.GetPendingInput(ctx, userRec.UserId, stableID)
	if err == nil && pending != nil {
		if pending.Status == pbpipeline.PendingInput_STATUS_COMPLETED {
			logger.Debug("manual-workout-entry: pending input completed — applying inline")
			return p.EnrichResume(ctx, activity, userRec, pending)
		}
		if pending.Status == pbpipeline.PendingInput_STATUS_WAITING {
			logger.Debug("manual-workout-entry: still waiting for user input")
			return nil, buildWaitError(stableID, p.Name())
		}
	}

	logger.Debug("manual-workout-entry: requesting workout data via pending input", "activity_id", stableID)
	return nil, buildWaitError(stableID, p.Name())
}

// EnrichResume is called during resume mode. It parses the JSON workout_data
// from the completed pending input and populates Sessions[0].StrengthSets.
func (p *Provider) EnrichResume(_ context.Context, activity *pbactivity.StandardizedActivity, _ *user.Record, pendingInput *pbpipeline.PendingInput) (*providers.EnrichmentResult, error) {
	rawData := pendingInput.InputData["workout_data"]
	if rawData == "" {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"manual_workout_entry_status": "no_data"},
		}, nil
	}

	strengthSets, err := parseWorkoutData(rawData)
	if err != nil {
		return nil, fmt.Errorf("manual-workout-entry: failed to parse workout_data: %w", err)
	}

	if len(strengthSets) == 0 {
		return &providers.EnrichmentResult{
			Metadata: map[string]string{"manual_workout_entry_status": "empty"},
		}, nil
	}

	// Ensure at least one session exists to write into.
	if len(activity.Sessions) == 0 {
		activity.Sessions = []*pbactivity.Session{{}}
	}
	activity.Sessions[0].StrengthSets = strengthSets

	return &providers.EnrichmentResult{
		Metadata: map[string]string{
			"manual_workout_entry_status": "applied",
			"strength_set_count":          fmt.Sprintf("%d", len(strengthSets)),
		},
	}, nil
}

// inputExercise is the JSON structure sent by the client.
type inputExercise struct {
	Exercise          string     `json:"exercise"`
	Notes             string     `json:"notes"`
	SupersetID        string     `json:"superset_id"`
	TimestampOffsetS  int32      `json:"timestamp_offset_s"`
	Sets              []inputSet `json:"sets"`
}

type inputSet struct {
	Reps     int32   `json:"reps"`
	WeightKg float64 `json:"weight_kg"`
	SetType  string  `json:"set_type"`
}

// parseWorkoutData converts the raw JSON string from the pending input into
// a flat slice of StrengthSet protos (one per set, not per exercise).
func parseWorkoutData(raw string) ([]*pbactivity.StrengthSet, error) {
	var exercises []inputExercise
	if err := json.Unmarshal([]byte(raw), &exercises); err != nil {
		return nil, err
	}

	var sets []*pbactivity.StrengthSet
	for _, ex := range exercises {
		for _, s := range ex.Sets {
			sets = append(sets, &pbactivity.StrengthSet{
				ExerciseName: ex.Exercise,
				Reps:         s.Reps,
				WeightKg:     s.WeightKg,
				SetType:      normaliseSetType(s.SetType),
				SupersetId:   ex.SupersetID,
				Notes:        ex.Notes,
			})
		}
	}

	return sets, nil
}

// normaliseSetType maps the client set_type string to the canonical string
// expected by the workout_summary enricher.
func normaliseSetType(raw string) string {
	switch raw {
	case "warmup":
		return "warmup"
	case "failure":
		return "failure"
	case "dropset":
		return "dropset"
	default:
		return "normal"
	}
}

func buildWaitError(stableID, providerName string) *user_input.WaitForInputError {
	return &user_input.WaitForInputError{
		ActivityID:         stableID,
		RequiredFields:     []string{"workout_data"},
		EnricherProviderID: providerName,
		Metadata: map[string]string{
			"display.field_types": `{"workout_data":"workout_entry"}`,
			"display.summary":     "What exercises did you do during this activity?",
			"display.title":       "Log Your Workout",
		},
	}
}
