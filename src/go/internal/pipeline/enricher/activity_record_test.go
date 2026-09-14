package enricher

import (
	"testing"
	"time"

	activityPkg "github.com/fitglue/server/src/go/pkg/domain/activity"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func sampleSource() *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		Source:     pbactivity.ActivitySource_SOURCE_STRAVA,
		ExternalId: "ext-123",
		Name:       "Original Name",
		Type:       pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
	}
}

func TestBuildActivityRecord_FreshRecordLayering(t *testing.T) {
	now := time.Date(2026, 9, 14, 12, 0, 0, 0, time.UTC)
	source := sampleSource()
	derived := &pbactivity.StandardizedActivity{
		Source:      pbactivity.ActivitySource_SOURCE_STRAVA,
		ExternalId:  "ext-123",
		Name:        "Enriched Name",
		Description: "weather + AI summary",
		Type:        pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
	}
	merged := &pbactivity.ActivityEnrichments{}
	execs := []ProviderExecution{
		{ProviderName: "weather", ExecutionID: "e1", Status: "SUCCESS", DurationMs: 12},
		{ProviderName: "ai_companion", ExecutionID: "e2", Status: "SUCCESS", DurationMs: 340},
		{ProviderName: "personal_records", ExecutionID: "e3", Status: "SKIPPED"},
	}
	enrichByProvider := map[string]*pbactivity.ActivityEnrichments{
		"weather": {Weather: &pbactivity.WeatherSummary{}},
	}

	rec := buildActivityRecord(
		nil,
		"user-1", "act-1", "pipe-1", "run-1",
		pbactivity.ActivitySource_SOURCE_STRAVA, "ext-123",
		source, derived, merged,
		"gs://bucket/payloads/user-1/act-1.json", now,
		execs, enrichByProvider,
	)

	if rec.ActivityId != "act-1" || rec.UserId != "user-1" || rec.PipelineRunId != "run-1" {
		t.Fatalf("record identity fields wrong: %+v", rec)
	}
	if rec.SchemaVersion != activityPkg.ActivityRecordSchemaVersion {
		t.Errorf("schema version = %d, want %d", rec.SchemaVersion, activityPkg.ActivityRecordSchemaVersion)
	}

	// Source layer: immutable parsed snapshot + raw payload retention boundary.
	sl := rec.GetSourceLayer()
	if sl == nil || sl.Parsed == nil {
		t.Fatal("expected a populated source layer")
	}
	if sl.Parsed.Name != "Original Name" {
		t.Errorf("source layer should hold the pre-enrichment name, got %q", sl.Parsed.Name)
	}
	if sl.RawPayloadUri != "gs://bucket/payloads/user-1/act-1.json" {
		t.Errorf("raw payload uri not stored: %q", sl.RawPayloadUri)
	}
	wantExpiry := now.Add(activityPkg.RawPayloadRetention)
	if !sl.RawPayloadExpiresAt.AsTime().Equal(wantExpiry) {
		t.Errorf("raw payload expiry = %v, want %v", sl.RawPayloadExpiresAt.AsTime(), wantExpiry)
	}

	// Enricher layers: one per execution, in order, with typed enrichments attached.
	if len(rec.EnricherLayers) != 3 {
		t.Fatalf("expected 3 enricher layers, got %d", len(rec.EnricherLayers))
	}
	if rec.EnricherLayers[0].ProviderName != "weather" || rec.EnricherLayers[0].Enrichments == nil {
		t.Errorf("weather layer missing typed enrichments: %+v", rec.EnricherLayers[0])
	}
	if rec.EnricherLayers[2].Status != "SKIPPED" {
		t.Errorf("skipped layer status = %q", rec.EnricherLayers[2].Status)
	}

	// Overlay starts empty; derived + merged enrichments captured.
	if rec.UserEditOverlay == nil || rec.UserEditOverlay.Name != nil {
		t.Errorf("expected an empty user-edit overlay, got %+v", rec.UserEditOverlay)
	}
	if rec.DerivedActivity == nil || rec.DerivedActivity.Name != "Enriched Name" {
		t.Errorf("derived activity not captured: %+v", rec.DerivedActivity)
	}
	if rec.Enrichments == nil {
		t.Error("merged enrichments not captured")
	}
	if !rec.CreatedAt.AsTime().Equal(now) || !rec.UpdatedAt.AsTime().Equal(now) {
		t.Error("timestamps not stamped")
	}
}

func TestBuildActivityRecord_SourceLayerIsIndependentSnapshot(t *testing.T) {
	now := time.Unix(1_700_000_000, 0).UTC()
	source := sampleSource()

	rec := buildActivityRecord(
		nil, "u", "a", "p", "r",
		pbactivity.ActivitySource_SOURCE_STRAVA, "ext",
		source, sampleSource(), nil, "", now, nil, nil,
	)

	// Mutating the caller's source after the build must not affect the stored snapshot.
	source.Name = "MUTATED AFTER BUILD"
	if rec.SourceLayer.Parsed.Name != "Original Name" {
		t.Errorf("source layer aliases caller's activity: got %q", rec.SourceLayer.Parsed.Name)
	}
}

func TestBuildActivityRecord_ResumeAppendsAndPreservesImmutableLayers(t *testing.T) {
	first := time.Date(2026, 9, 14, 10, 0, 0, 0, time.UTC)
	second := time.Date(2026, 9, 14, 11, 0, 0, 0, time.UTC)

	existing := buildActivityRecord(
		nil, "u", "a", "p", "run-1",
		pbactivity.ActivitySource_SOURCE_STRAVA, "ext",
		sampleSource(), sampleSource(), nil,
		"gs://bucket/payloads/u/a.json", first,
		[]ProviderExecution{{ProviderName: "weather", Status: "SUCCESS"}}, nil,
	)
	// Simulate a user edit landing on the record between runs.
	existing.UserEditOverlay.Name = ptr("user edit")

	derived2 := &pbactivity.StandardizedActivity{Name: "Re-enriched"}
	updated := buildActivityRecord(
		existing, "u", "a", "p", "run-2",
		pbactivity.ActivitySource_SOURCE_STRAVA, "ext",
		sampleSource(), derived2, nil,
		"gs://bucket/payloads/u/a.json", second,
		[]ProviderExecution{{ProviderName: "ai_companion", Status: "SUCCESS"}}, nil,
	)

	// Enricher layers are append-only across runs.
	if len(updated.EnricherLayers) != 2 {
		t.Fatalf("expected layers appended (2), got %d", len(updated.EnricherLayers))
	}
	if updated.EnricherLayers[0].ProviderName != "weather" || updated.EnricherLayers[1].ProviderName != "ai_companion" {
		t.Errorf("append order wrong: %v / %v", updated.EnricherLayers[0].ProviderName, updated.EnricherLayers[1].ProviderName)
	}
	// Immutable source layer + created_at preserved from the first run.
	if !updated.CreatedAt.AsTime().Equal(first) {
		t.Errorf("created_at should be preserved from first run, got %v", updated.CreatedAt.AsTime())
	}
	if !updated.SourceLayer.RawPayloadExpiresAt.AsTime().Equal(first.Add(activityPkg.RawPayloadRetention)) {
		t.Error("source layer retention boundary should be preserved from first run")
	}
	// Mutable overlay preserved; derived + run id + updated_at refreshed.
	if updated.UserEditOverlay.GetName() != "user edit" {
		t.Errorf("user edit overlay lost on re-enrichment: %q", updated.UserEditOverlay.GetName())
	}
	if updated.DerivedActivity.Name != "Re-enriched" {
		t.Errorf("derived not refreshed: %q", updated.DerivedActivity.Name)
	}
	if updated.PipelineRunId != "run-2" || !updated.UpdatedAt.AsTime().Equal(second) {
		t.Error("run id / updated_at not refreshed on re-enrichment")
	}
}

func ptr(s string) *string { return &s }
