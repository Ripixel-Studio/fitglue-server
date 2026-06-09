package firestore

import (
	"testing"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

func TestFirestoreToPipeline_ProviderTypeNumeric(t *testing.T) {
	m := map[string]interface{}{
		"id":     "p1",
		"source": "SOURCE_HEVY",
		"enrichers": []interface{}{
			map[string]interface{}{
				"provider_type": int64(2), // ENRICHER_PROVIDER_WORKOUT_SUMMARY
				"typed_config":  map[string]interface{}{},
			},
		},
		"destinations": []interface{}{},
	}

	pipeline := FirestoreToPipeline(m)

	if len(pipeline.Enrichers) != 1 {
		t.Fatalf("Expected 1 enricher, got %d", len(pipeline.Enrichers))
	}
	if pipeline.Enrichers[0].ProviderType != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WORKOUT_SUMMARY {
		t.Errorf("Expected ENRICHER_PROVIDER_WORKOUT_SUMMARY, got %v", pipeline.Enrichers[0].ProviderType)
	}
}

func TestFirestoreToPipeline_ProviderTypeFloat(t *testing.T) {
	m := map[string]interface{}{
		"id":     "p1",
		"source": "SOURCE_HEVY",
		"enrichers": []interface{}{
			map[string]interface{}{
				"provider_type": float64(15), // ENRICHER_PROVIDER_AI_COMPANION
				"typed_config":  map[string]interface{}{},
			},
		},
		"destinations": []interface{}{},
	}

	pipeline := FirestoreToPipeline(m)

	if len(pipeline.Enrichers) != 1 {
		t.Fatalf("Expected 1 enricher, got %d", len(pipeline.Enrichers))
	}
	if pipeline.Enrichers[0].ProviderType != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION {
		t.Errorf("Expected ENRICHER_PROVIDER_AI_COMPANION, got %v", pipeline.Enrichers[0].ProviderType)
	}
}

func TestFirestoreToPipeline_ProviderTypeString(t *testing.T) {
	m := map[string]interface{}{
		"id":     "p1",
		"source": "SOURCE_HEVY",
		"enrichers": []interface{}{
			map[string]interface{}{
				"provider_type": "ENRICHER_PROVIDER_WORKOUT_SUMMARY",
				"typed_config":  map[string]interface{}{},
			},
		},
		"destinations": []interface{}{},
	}

	pipeline := FirestoreToPipeline(m)

	if len(pipeline.Enrichers) != 1 {
		t.Fatalf("Expected 1 enricher, got %d", len(pipeline.Enrichers))
	}
	if pipeline.Enrichers[0].ProviderType != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WORKOUT_SUMMARY {
		t.Errorf("Expected ENRICHER_PROVIDER_WORKOUT_SUMMARY, got %v", pipeline.Enrichers[0].ProviderType)
	}
}

func TestFirestoreToPipeline_ProviderTypeUnknownString(t *testing.T) {
	m := map[string]interface{}{
		"id":     "p1",
		"source": "SOURCE_HEVY",
		"enrichers": []interface{}{
			map[string]interface{}{
				"provider_type": "NOT_A_REAL_PROVIDER",
				"typed_config":  map[string]interface{}{},
			},
		},
		"destinations": []interface{}{},
	}

	pipeline := FirestoreToPipeline(m)

	if len(pipeline.Enrichers) != 1 {
		t.Fatalf("Expected 1 enricher, got %d", len(pipeline.Enrichers))
	}
	if pipeline.Enrichers[0].ProviderType != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_UNSPECIFIED {
		t.Errorf("Expected ENRICHER_PROVIDER_UNSPECIFIED for unknown string, got %v", pipeline.Enrichers[0].ProviderType)
	}
}

func TestFirestoreToPipeline_DestinationStringAllTypes(t *testing.T) {
	tests := []struct {
		input    string
		expected pbplugin.DestinationType
	}{
		{"DESTINATION_STRAVA", pbplugin.DestinationType_DESTINATION_STRAVA},
		{"DESTINATION_SHOWCASE", pbplugin.DestinationType_DESTINATION_SHOWCASE},
		{"DESTINATION_HEVY", pbplugin.DestinationType_DESTINATION_HEVY},
		{"DESTINATION_TRAININGPEAKS", pbplugin.DestinationType_DESTINATION_TRAININGPEAKS},
		{"DESTINATION_INTERVALS", pbplugin.DestinationType_DESTINATION_INTERVALS},
		{"DESTINATION_GOOGLESHEETS", pbplugin.DestinationType_DESTINATION_GOOGLESHEETS},
		{"DESTINATION_GITHUB", pbplugin.DestinationType_DESTINATION_GITHUB},
		{"DESTINATION_MOCK", pbplugin.DestinationType_DESTINATION_MOCK},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := map[string]interface{}{
				"id":           "p1",
				"source":       "SOURCE_HEVY",
				"enrichers":    []interface{}{},
				"destinations": []interface{}{tt.input},
			}

			pipeline := FirestoreToPipeline(m)

			if len(pipeline.Destinations) != 1 {
				t.Fatalf("Expected 1 destination, got %d", len(pipeline.Destinations))
			}
			if pipeline.Destinations[0] != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, pipeline.Destinations[0])
			}
		})
	}
}

func TestFirestoreToPipeline_DestinationShortForm(t *testing.T) {
	tests := []struct {
		input    string
		expected pbplugin.DestinationType
	}{
		{"strava", pbplugin.DestinationType_DESTINATION_STRAVA},
		{"showcase", pbplugin.DestinationType_DESTINATION_SHOWCASE},
		{"hevy", pbplugin.DestinationType_DESTINATION_HEVY},
		{"mock", pbplugin.DestinationType_DESTINATION_MOCK},
		{"STRAVA", pbplugin.DestinationType_DESTINATION_STRAVA},
		{"Showcase", pbplugin.DestinationType_DESTINATION_SHOWCASE},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			m := map[string]interface{}{
				"id":           "p1",
				"source":       "SOURCE_HEVY",
				"enrichers":    []interface{}{},
				"destinations": []interface{}{tt.input},
			}

			pipeline := FirestoreToPipeline(m)

			if len(pipeline.Destinations) != 1 {
				t.Fatalf("Expected 1 destination for input %q, got %d", tt.input, len(pipeline.Destinations))
			}
			if pipeline.Destinations[0] != tt.expected {
				t.Errorf("For input %q: expected %v, got %v", tt.input, tt.expected, pipeline.Destinations[0])
			}
		})
	}
}

func TestFirestoreToPipeline_RoundTrip(t *testing.T) {
	// Test that PipelineToFirestore -> FirestoreToPipeline preserves data.
	// Note: PipelineToFirestore stores provider_type as int32, but Firestore
	// always returns integers as int64. We simulate this by converting the
	// enricher maps to use int64 values, as Firestore would.
	original := &pbpipeline.PipelineConfig{
		Id:     "test-pipe",
		Name:   "Test Pipeline",
		Source: "SOURCE_HEVY",
		Enrichers: []*pbpipeline.EnricherConfig{
			{
				ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WORKOUT_SUMMARY,
				TypedConfig:  map[string]string{"key": "val"},
			},
			{
				ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_AI_COMPANION,
				TypedConfig:  map[string]string{},
			},
		},
		Destinations: []pbplugin.DestinationType{
			pbplugin.DestinationType_DESTINATION_STRAVA,
			pbplugin.DestinationType_DESTINATION_SHOWCASE,
		},
	}

	firestoreMap := PipelineToFirestore(original)

	// Simulate Firestore's behavior: convert int32 -> int64 in enricher maps
	if eList, ok := firestoreMap["enrichers"].([]map[string]interface{}); ok {
		for _, eMap := range eList {
			if pt, ok := eMap["provider_type"].(int32); ok {
				eMap["provider_type"] = int64(pt)
			}
		}
		// Convert []map[string]interface{} to []interface{} as Firestore returns
		enricherSlice := make([]interface{}, len(eList))
		for i, e := range eList {
			enricherSlice[i] = e
		}
		firestoreMap["enrichers"] = enricherSlice
	}
	// Simulate Firestore's behavior for destinations: convert int32 -> int64
	if dList, ok := firestoreMap["destinations"].([]pbplugin.DestinationType); ok {
		destSlice := make([]interface{}, len(dList))
		for i, d := range dList {
			destSlice[i] = int64(d)
		}
		firestoreMap["destinations"] = destSlice
	}

	roundTripped := FirestoreToPipeline(firestoreMap)

	if roundTripped.Id != original.Id {
		t.Errorf("ID mismatch: %s vs %s", roundTripped.Id, original.Id)
	}
	if len(roundTripped.Enrichers) != len(original.Enrichers) {
		t.Fatalf("Enricher count mismatch: %d vs %d", len(roundTripped.Enrichers), len(original.Enrichers))
	}
	for i, e := range roundTripped.Enrichers {
		if e.ProviderType != original.Enrichers[i].ProviderType {
			t.Errorf("Enricher[%d] ProviderType mismatch: %v vs %v", i, e.ProviderType, original.Enrichers[i].ProviderType)
		}
	}
	if len(roundTripped.Destinations) != len(original.Destinations) {
		t.Fatalf("Destination count mismatch: %d vs %d", len(roundTripped.Destinations), len(original.Destinations))
	}
	for i, d := range roundTripped.Destinations {
		if d != original.Destinations[i] {
			t.Errorf("Destination[%d] mismatch: %v vs %v", i, d, original.Destinations[i])
		}
	}
}

func TestFirestoreToPipeline_NonBlocking(t *testing.T) {
	m := map[string]interface{}{
		"id":     "p1",
		"source": "SOURCE_HEVY",
		"enrichers": []interface{}{
			map[string]interface{}{
				"provider_type": int64(2),
				"typed_config":  map[string]interface{}{},
				"non_blocking":  true,
			},
			map[string]interface{}{
				"provider_type": int64(3),
				"typed_config":  map[string]interface{}{},
			},
		},
		"destinations": []interface{}{},
	}

	pipeline := FirestoreToPipeline(m)

	if len(pipeline.Enrichers) != 2 {
		t.Fatalf("Expected 2 enrichers, got %d", len(pipeline.Enrichers))
	}
	if !pipeline.Enrichers[0].NonBlocking {
		t.Error("Expected Enrichers[0].NonBlocking to be true")
	}
	if pipeline.Enrichers[1].NonBlocking {
		t.Error("Expected Enrichers[1].NonBlocking to be false")
	}
}

// --- PipelineRun string enum tests ---

func TestFirestoreToPipelineRun_StringEnums(t *testing.T) {
	m := map[string]interface{}{
		"id":          "run-1",
		"pipeline_id": "pipe-1",
		"activity_id": "act-1",
		"source":      "SOURCE_HEVY",
		"title":       "Morning Run",
		"type":        "ACTIVITY_TYPE_RUN",
		"status":      "PIPELINE_RUN_STATUS_SYNCED",
		"destinations": []interface{}{
			map[string]interface{}{
				"destination": "DESTINATION_STRAVA",
				"status":      "DESTINATION_STATUS_SUCCESS",
				"external_id": "ext-1",
			},
		},
	}

	run := FirestoreToPipelineRun(m)

	if run.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("Expected ACTIVITY_TYPE_RUN, got %v", run.Type)
	}
	if run.Status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED {
		t.Errorf("Expected PIPELINE_RUN_STATUS_SYNCED, got %v", run.Status)
	}
	if len(run.Destinations) != 1 {
		t.Fatalf("Expected 1 destination, got %d", len(run.Destinations))
	}
	if run.Destinations[0].Destination != pbplugin.DestinationType_DESTINATION_STRAVA {
		t.Errorf("Expected DESTINATION_STRAVA, got %v", run.Destinations[0].Destination)
	}
	if run.Destinations[0].Status != pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS {
		t.Errorf("Expected DESTINATION_STATUS_SUCCESS, got %v", run.Destinations[0].Status)
	}
}

// --- ShowcasedActivity string enum tests ---

func TestFirestoreToShowcasedActivity_StringEnums(t *testing.T) {
	m := map[string]interface{}{
		"showcase_id":   "sc-1",
		"activity_id":   "act-1",
		"user_id":       "user-1",
		"title":         "Test Showcase",
		"activity_type": "ACTIVITY_TYPE_RIDE",
		"source":        "SOURCE_STRAVA",
	}

	s := FirestoreToShowcasedActivity(m)

	if s.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_RIDE {
		t.Errorf("Expected ACTIVITY_TYPE_RIDE, got %v", s.ActivityType)
	}
	if s.Source != pbactivity.ActivitySource_SOURCE_STRAVA {
		t.Errorf("Expected SOURCE_STRAVA, got %v", s.Source)
	}
}

// --- PendingInput string enum tests ---

func TestFirestoreToPendingInput_StringStatus(t *testing.T) {
	m := map[string]interface{}{
		"activity_id": "act-1",
		"status":      "STATUS_WAITING",
	}

	p := FirestoreToPendingInput(m)

	if p.Status != pbpipeline.PendingInput_STATUS_WAITING {
		t.Errorf("Expected STATUS_WAITING, got %v", p.Status)
	}
}

// --- PersonalRecord string enum tests ---

func TestFirestoreToPersonalRecord_StringActivityType(t *testing.T) {
	m := map[string]interface{}{
		"record_type":   "fastest_5k",
		"value":         float64(1200),
		"unit":          "seconds",
		"activity_type": "ACTIVITY_TYPE_RUN",
	}

	r := FirestoreToPersonalRecord(m)

	if r.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("Expected ACTIVITY_TYPE_RUN, got %v", r.ActivityType)
	}
}

// --- UploadedActivity string enum tests ---

func TestFirestoreToUploadedActivity_StringEnums(t *testing.T) {
	m := map[string]interface{}{
		"id":          "up-1",
		"user_id":     "user-1",
		"source":      "SOURCE_HEVY",
		"destination": "DESTINATION_SHOWCASE",
		"external_id": "ext-1",
	}

	r := FirestoreToUploadedActivity(m)

	if r.Source != pbactivity.ActivitySource_SOURCE_HEVY {
		t.Errorf("Expected SOURCE_HEVY, got %v", r.Source)
	}
	if r.Destination != pbplugin.DestinationType_DESTINATION_SHOWCASE {
		t.Errorf("Expected DESTINATION_SHOWCASE, got %v", r.Destination)
	}
}

// --- ShowcaseProfileEntry string enum tests ---

func TestFirestoreToShowcaseProfileEntry_StringEnums(t *testing.T) {
	m := map[string]interface{}{
		"showcase_id":   "sc-1",
		"title":         "Test Entry",
		"activity_type": "ACTIVITY_TYPE_WEIGHT_TRAINING",
		"source":        "SOURCE_HEVY",
	}

	e := FirestoreToShowcaseProfileEntry(m)

	if e.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING {
		t.Errorf("Expected ACTIVITY_TYPE_WEIGHT_TRAINING, got %v", e.ActivityType)
	}
	if e.Source != pbactivity.ActivitySource_SOURCE_HEVY {
		t.Errorf("Expected SOURCE_HEVY, got %v", e.Source)
	}
}
