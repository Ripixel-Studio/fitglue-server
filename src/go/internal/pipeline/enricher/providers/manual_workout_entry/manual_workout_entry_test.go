package manual_workout_entry

import (
	"context"
	"io"
	"log/slog"
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func mweLogger() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestMWE_Metadata(t *testing.T) {
	p := &Provider{}
	if p.Name() != "manual-workout-entry" {
		t.Errorf("name %q", p.Name())
	}
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_MANUAL_WORKOUT_ENTRY {
		t.Errorf("type %v", p.ProviderType())
	}
	p.SetService(nil)
}

func TestMWE_Enrich_SkipsWhenStrengthExists(t *testing.T) {
	act := &pbactivity.StandardizedActivity{
		Sessions: []*pbactivity.Session{{StrengthSets: []*pbactivity.StrengthSet{{ExerciseName: "Squat"}}}},
	}
	res, err := (&Provider{}).Enrich(context.Background(), mweLogger(), act, nil, nil, false)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Skipped {
		t.Error("expected skip when strength sets already present")
	}
}

func TestMWE_Enrich_SkipsOnDoNotRetry(t *testing.T) {
	act := &pbactivity.StandardizedActivity{}
	res, err := (&Provider{}).Enrich(context.Background(), mweLogger(), act, nil, nil, true)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !res.Skipped {
		t.Error("expected skip when doNotRetry set")
	}
}

func TestMWE_Enrich_ErrorsWhenServiceNil(t *testing.T) {
	act := &pbactivity.StandardizedActivity{}
	_, err := (&Provider{}).Enrich(context.Background(), mweLogger(), act, nil, nil, false)
	if err == nil {
		t.Error("expected error when service not initialised")
	}
}

func TestMWE_EnrichResume_NoData(t *testing.T) {
	pi := &pbpipeline.PendingInput{InputData: map[string]string{}}
	res, err := (&Provider{}).EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, nil, pi)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Metadata["manual_workout_entry_status"] != "no_data" {
		t.Errorf("expected no_data, got %v", res.Metadata)
	}
}

func TestMWE_EnrichResume_Applied(t *testing.T) {
	raw := `[{"exercise":"Bench Press","notes":"felt strong","superset_id":"A","sets":[{"reps":5,"weight_kg":80,"set_type":"warmup"},{"reps":5,"weight_kg":100,"set_type":"normal"}]}]`
	pi := &pbpipeline.PendingInput{InputData: map[string]string{"workout_data": raw}}
	act := &pbactivity.StandardizedActivity{}
	res, err := (&Provider{}).EnrichResume(context.Background(), act, nil, pi)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Metadata["manual_workout_entry_status"] != "applied" {
		t.Fatalf("expected applied, got %v", res.Metadata)
	}
	if len(act.Sessions) != 1 || len(act.Sessions[0].StrengthSets) != 2 {
		t.Fatalf("expected 2 strength sets written, got %+v", act.Sessions)
	}
	first := act.Sessions[0].StrengthSets[0]
	if first.ExerciseName != "Bench Press" || first.SetType != "warmup" || first.SupersetId != "A" || first.Notes != "felt strong" {
		t.Errorf("unexpected first set: %+v", first)
	}
}

func TestMWE_EnrichResume_InvalidJSON(t *testing.T) {
	pi := &pbpipeline.PendingInput{InputData: map[string]string{"workout_data": "{not json"}}
	_, err := (&Provider{}).EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, nil, pi)
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestMWE_EnrichResume_EmptySets(t *testing.T) {
	pi := &pbpipeline.PendingInput{InputData: map[string]string{"workout_data": `[{"exercise":"X","sets":[]}]`}}
	res, err := (&Provider{}).EnrichResume(context.Background(), &pbactivity.StandardizedActivity{}, nil, pi)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if res.Metadata["manual_workout_entry_status"] != "empty" {
		t.Errorf("expected empty, got %v", res.Metadata)
	}
}

func TestNormaliseSetType(t *testing.T) {
	cases := map[string]string{"warmup": "warmup", "failure": "failure", "dropset": "dropset", "": "normal", "weird": "normal"}
	for in, want := range cases {
		if got := normaliseSetType(in); got != want {
			t.Errorf("normaliseSetType(%q)=%q want %q", in, got, want)
		}
	}
}

func TestBuildWaitError(t *testing.T) {
	we := buildWaitError("act-123", "manual-workout-entry")
	if we.ActivityID != "act-123" || we.EnricherProviderID != "manual-workout-entry" {
		t.Errorf("unexpected wait error: %+v", we)
	}
	if len(we.RequiredFields) != 1 || we.RequiredFields[0] != "workout_data" {
		t.Errorf("unexpected required fields: %v", we.RequiredFields)
	}
}
