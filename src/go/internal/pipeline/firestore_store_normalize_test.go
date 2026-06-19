package pipeline

import "testing"

func TestResolvePipelineRunStatusName(t *testing.T) {
	cases := map[string]struct {
		in       string
		wantName string
		wantOK   bool
	}{
		"full enum name":  {"PIPELINE_RUN_STATUS_SYNCED", "PIPELINE_RUN_STATUS_SYNCED", true},
		"short name":      {"SYNCED", "PIPELINE_RUN_STATUS_SYNCED", true},
		"lowercase short": {"failed", "PIPELINE_RUN_STATUS_FAILED", true},
		"numeric synced":  {"2", "PIPELINE_RUN_STATUS_SYNCED", true},
		"numeric failed":  {"4", "PIPELINE_RUN_STATUS_FAILED", true},
		"empty":           {"", "", false},
		"garbage":         {"NOT_A_STATUS", "", false},
		"out of range":    {"999", "", false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			gotName, gotOK := resolvePipelineRunStatusName(tc.in)
			if gotName != tc.wantName || gotOK != tc.wantOK {
				t.Fatalf("resolvePipelineRunStatusName(%q) = (%q,%v), want (%q,%v)",
					tc.in, gotName, gotOK, tc.wantName, tc.wantOK)
			}
		})
	}
}

func TestNormalizeRunData_DropsCamelCaseDuplicates(t *testing.T) {
	data := map[string]interface{}{
		"created_at":  "2026-06-19T00:00:00Z",
		"createdAt":   "2026-06-19T00:00:00Z", // duplicate — would break protojson
		"pipeline_id": "p1",
		"pipelineId":  "p1",
		"source":      "strava", // no snake/camel pair, must be preserved
	}
	out := normalizeRunData(data)

	if _, ok := out["createdAt"]; ok {
		t.Fatal("expected camelCase createdAt to be dropped")
	}
	if _, ok := out["pipelineId"]; ok {
		t.Fatal("expected camelCase pipelineId to be dropped")
	}
	if out["created_at"] != "2026-06-19T00:00:00Z" {
		t.Fatal("expected snake_case created_at preserved")
	}
	if out["source"] != "strava" {
		t.Fatal("expected unrelated keys preserved")
	}
}

func TestNormalizeRunData_KeepsCamelWhenNoSnakeEquivalent(t *testing.T) {
	// A doc written purely in camelCase (no snake_case dupes) decodes fine via
	// protojson json_name, so we must NOT delete those keys.
	data := map[string]interface{}{"createdAt": "2026-06-19T00:00:00Z"}
	out := normalizeRunData(data)
	if out["createdAt"] != "2026-06-19T00:00:00Z" {
		t.Fatal("expected camelCase-only key to be preserved")
	}
}

func TestNormalizeRunData_CoercesNumericStatus(t *testing.T) {
	if out := normalizeRunData(map[string]interface{}{"status": int64(2)}); out["status"] != "PIPELINE_RUN_STATUS_SYNCED" {
		t.Fatalf("int64 status not coerced: %v", out["status"])
	}
	if out := normalizeRunData(map[string]interface{}{"status": float64(4)}); out["status"] != "PIPELINE_RUN_STATUS_FAILED" {
		t.Fatalf("float64 status not coerced: %v", out["status"])
	}
	// String status is left untouched.
	if out := normalizeRunData(map[string]interface{}{"status": "PIPELINE_RUN_STATUS_RUNNING"}); out["status"] != "PIPELINE_RUN_STATUS_RUNNING" {
		t.Fatalf("string status mutated: %v", out["status"])
	}
}
