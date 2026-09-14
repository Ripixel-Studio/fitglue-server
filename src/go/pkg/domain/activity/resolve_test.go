package activity

import (
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"google.golang.org/protobuf/proto"
)

// find returns the provenance entries for a given field, preserving order.
func find(prov []*pbactivity.FieldProvenance, field string) []*pbactivity.FieldProvenance {
	var out []*pbactivity.FieldProvenance
	for _, p := range prov {
		if p.Field == field {
			out = append(out, p)
		}
	}
	return out
}

func TestResolve_NilRecord(t *testing.T) {
	if Resolve(nil) != nil {
		t.Error("expected nil for nil record")
	}
}

func TestResolve_LayersInOrderLastEnricherWins(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{
				Name: "Morning Activity",
				Type: pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT,
			},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "activity_type_detector",
				ExecutionId:  "e1",
				Contribution: &pbactivity.EnricherContribution{
					Type: activityType(pbactivity.ActivityType_ACTIVITY_TYPE_RUN),
				},
			},
			{
				ProviderName: "location_naming",
				ExecutionId:  "e2",
				Contribution: &pbactivity.EnricherContribution{
					Name: proto.String("Regent's Park Run"),
				},
			},
			{
				ProviderName: "trail_detector",
				ExecutionId:  "e3",
				Contribution: &pbactivity.EnricherContribution{
					Type: activityType(pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN),
				},
			},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name: "Regent's Park Run",
			Type: pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
	}

	res := Resolve(rec)
	if res == nil || res.Activity == nil {
		t.Fatal("expected a resolved activity")
	}
	if res.Activity.Name != "Regent's Park Run" {
		t.Errorf("resolved name = %q", res.Activity.Name)
	}

	// name was last set by the location_naming enricher (index 1).
	names := find(res.Provenance, FieldName)
	if len(names) != 1 {
		t.Fatalf("expected one name provenance entry, got %d", len(names))
	}
	if names[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER ||
		names[0].ProviderName != "location_naming" || names[0].EnricherIndex != 1 {
		t.Errorf("name provenance wrong: %+v", names[0])
	}

	// type: source set it, then two enrichers — the LAST one (trail_detector, index 2) wins.
	types := find(res.Provenance, FieldType)
	if len(types) != 1 {
		t.Fatalf("expected one type provenance entry, got %d", len(types))
	}
	if types[0].ProviderName != "trail_detector" || types[0].EnricherIndex != 2 {
		t.Errorf("type provenance should attribute to the last enricher that set it: %+v", types[0])
	}
}

func TestResolve_UserOverlayWins(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{
				Name:        "Auto Name",
				Description: "source desc",
				Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "ai_companion",
				ExecutionId:  "e1",
				Contribution: &pbactivity.EnricherContribution{
					Name:        proto.String("AI Suggested Name"),
					Description: proto.String("AI summary section"),
				},
			},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name:        "AI Suggested Name",
			Description: "source desc\n\nAI summary section",
			Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Name:        proto.String("My Title"),
			Description: proto.String("My words"),
		},
	}

	res := Resolve(rec)
	if res.Activity.Name != "My Title" || res.Activity.Description != "My words" {
		t.Fatalf("overlay not applied to resolved activity: name=%q desc=%q", res.Activity.Name, res.Activity.Description)
	}

	// name provenance is the overlay (wins over source + enricher).
	names := find(res.Provenance, FieldName)
	if len(names) != 1 || names[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY {
		t.Errorf("name should be attributed to user overlay: %+v", names)
	}

	// description: overlay replaces the whole field, so a SINGLE overlay entry — the
	// source + enricher sections must not leak through as contributors.
	descs := find(res.Provenance, FieldDescription)
	if len(descs) != 1 || descs[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY {
		t.Errorf("description overlay should replace all contributors: %+v", descs)
	}

	// type was not edited — falls through to the source.
	types := find(res.Provenance, FieldType)
	if len(types) != 1 || types[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE {
		t.Errorf("type should fall through to source: %+v", types)
	}
}

func TestResolve_TagsPerElementProvenance(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{
				Tags: []string{"strava"},
			},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "weather",
				ExecutionId:  "e1",
				Contribution: &pbactivity.EnricherContribution{Tags: []string{"sunny"}},
			},
			{
				ProviderName: "personal_records",
				ExecutionId:  "e2",
				Contribution: &pbactivity.EnricherContribution{Tags: []string{"pr", "5k-pb"}},
			},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			Tags: []string{"strava", "sunny", "pr", "5k-pb"},
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
	}

	res := Resolve(rec)
	tags := find(res.Provenance, FieldTags)
	if len(tags) != 4 {
		t.Fatalf("expected one provenance entry per tag (4), got %d", len(tags))
	}
	// Element indices must line up with the resolved tag ordering.
	want := []struct {
		layer    pbactivity.ProvenanceLayer
		provider string
		elem     int32
	}{
		{pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE, "", 0},
		{pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER, "weather", 1},
		{pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER, "personal_records", 2},
		{pbactivity.ProvenanceLayer_PROVENANCE_LAYER_ENRICHER, "personal_records", 3},
	}
	for i, w := range want {
		got := tags[i]
		if got.Layer != w.layer || got.ProviderName != w.provider || got.GetElementIndex() != w.elem {
			t.Errorf("tag[%d] provenance = %+v, want layer=%v provider=%q elem=%d", i, got, w.layer, w.provider, w.elem)
		}
	}
}

func TestResolve_OverlayReplacesTagSet(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{Tags: []string{"strava", "sunny"}},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{Tags: []string{"strava", "sunny"}},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Tags: []string{"custom-only"},
		},
	}

	res := Resolve(rec)
	if len(res.Activity.Tags) != 1 || res.Activity.Tags[0] != "custom-only" {
		t.Fatalf("overlay tag set not applied: %v", res.Activity.Tags)
	}
	tags := find(res.Provenance, FieldTags)
	if len(tags) != 1 || tags[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY || tags[0].GetElementIndex() != 0 {
		t.Errorf("overlay should own the whole tag set: %+v", tags)
	}
}

func TestResolve_TimeMarkerProvenance(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{
				TimeMarkers: []*pbactivity.TimeMarker{{Label: "start"}},
			},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "hybrid_race_tagger",
				ExecutionId:  "e1",
				Contribution: &pbactivity.EnricherContribution{
					TimeMarkers: []*pbactivity.TimeMarker{{Label: "run 1"}, {Label: "row"}},
				},
			},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			TimeMarkers: []*pbactivity.TimeMarker{{Label: "start"}, {Label: "run 1"}, {Label: "row"}},
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
	}

	res := Resolve(rec)
	tm := find(res.Provenance, FieldTimeMarkers)
	if len(tm) != 3 {
		t.Fatalf("expected 3 time marker provenance entries, got %d", len(tm))
	}
	if tm[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE || tm[0].GetElementIndex() != 0 {
		t.Errorf("first marker should be source: %+v", tm[0])
	}
	if tm[2].ProviderName != "hybrid_race_tagger" || tm[2].GetElementIndex() != 2 {
		t.Errorf("last marker should be attributed to the enricher: %+v", tm[2])
	}
}

func TestResolve_LegacyLayerWithoutContributionFallsThrough(t *testing.T) {
	// A layer written before contributions were captured has a nil Contribution.
	// It must not crash and must not steal provenance from the source.
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{Name: "Source Name"},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{ProviderName: "legacy", ExecutionId: "e1"}, // no contribution
		},
		DerivedActivity: &pbactivity.StandardizedActivity{Name: "Source Name"},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
	}

	res := Resolve(rec)
	names := find(res.Provenance, FieldName)
	if len(names) != 1 || names[0].Layer != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_SOURCE {
		t.Errorf("name should remain attributed to source when the only layer has no contribution: %+v", names)
	}
}

func TestResolve_NameSuffixCountsAsSettingTheName(t *testing.T) {
	rec := &pbactivity.ActivityRecord{
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{Name: "Run"},
		},
		EnricherLayers: []*pbactivity.EnricherRunLayer{
			{
				ProviderName: "personal_records",
				ExecutionId:  "e1",
				Contribution: &pbactivity.EnricherContribution{NameSuffix: proto.String(" (#5)")},
			},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{Name: "Run (#5)"},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
	}

	res := Resolve(rec)
	names := find(res.Provenance, FieldName)
	if len(names) != 1 || names[0].ProviderName != "personal_records" {
		t.Errorf("a name suffix should attribute the name to that enricher: %+v", names)
	}
}

func activityType(t pbactivity.ActivityType) *pbactivity.ActivityType { return &t }
