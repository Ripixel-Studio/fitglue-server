package parkrun

import (
	"encoding/json"
	"strings"
	"testing"
)

// TestParkrun_ManualFormConfig guards the structured-manual form contract (Option B):
// the config the pending input ships is valid JSON, mirrors the ParkrunSummary fields
// EnrichResume reads, no longer offers the old freeform "description" textarea, and — for
// web builds that understand display.optional_fields — makes only the finish time mandatory.
//
// This lives in its own file (no protobuf imports) so the proto-json lint doesn't flag the
// encoding/json usage; the JSON here is plain map[string]string / []string form config.
func TestParkrun_ManualFormConfig(t *testing.T) {
	labels := map[string]string{}
	if err := json.Unmarshal([]byte(manualFieldLabels), &labels); err != nil {
		t.Fatalf("manualFieldLabels is not valid JSON: %v", err)
	}
	types := map[string]string{}
	if err := json.Unmarshal([]byte(manualFieldTypes), &types); err != nil {
		t.Fatalf("manualFieldTypes is not valid JSON: %v", err)
	}
	var optional []string
	if err := json.Unmarshal([]byte(manualOptionalFields), &optional); err != nil {
		t.Fatalf("manualOptionalFields is not valid JSON: %v", err)
	}

	if _, ok := types["description"]; ok {
		t.Error("freeform description field should be gone from the structured manual form")
	}
	for _, f := range []string{"time", "position", "age_grade", "total_parkruns", "is_time_pb", "is_age_grade_pb"} {
		if _, ok := labels[f]; !ok {
			t.Errorf("missing label for structured field %q", f)
		}
		if _, ok := types[f]; !ok {
			t.Errorf("missing field type for structured field %q", f)
		}
	}
	if !strings.HasPrefix(types["total_parkruns"], "number") {
		t.Errorf("total_parkruns should be a numeric input, got %q", types["total_parkruns"])
	}
	if types["is_time_pb"] != "checkbox" {
		t.Errorf("is_time_pb should be a checkbox, got %q", types["is_time_pb"])
	}

	optionalSet := map[string]bool{}
	for _, f := range optional {
		optionalSet[f] = true
	}
	if optionalSet["time"] {
		t.Error("finish time must not be optional")
	}
	requiredSet := map[string]bool{}
	for _, f := range manualRequiredFields {
		requiredSet[f] = true
	}
	if !requiredSet["time"] {
		t.Error("finish time must be in the render list (RequiredFields)")
	}
	// mandatory = RequiredFields \ optional_fields — should reduce to exactly {time}.
	mandatory := []string{}
	for _, f := range manualRequiredFields {
		if !optionalSet[f] {
			mandatory = append(mandatory, f)
		}
	}
	if len(mandatory) != 1 || mandatory[0] != "time" {
		t.Errorf("expected only 'time' mandatory after subtracting optional_fields, got %v", mandatory)
	}
	// Everything the user can't reliably supply must be optional so a blank degrades cleanly.
	for _, f := range []string{"position", "age_grade", "total_parkruns", "is_time_pb", "is_age_grade_pb"} {
		if !optionalSet[f] {
			t.Errorf("field %q should be optional so a blank value degrades cleanly", f)
		}
	}
}
