package server

import "testing"

func TestIsAthleteTier(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want bool
	}{
		"proto enum name":  {"USER_TIER_ATHLETE", true},
		"legacy uppercase": {"ATHLETE", true},
		"legacy lowercase": {"athlete", true},
		"hobbyist enum":    {"USER_TIER_HOBBYIST", false},
		"empty":            {"", false},
		"non-string":       {2, false},
		"nil":              {nil, false},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := isAthleteTier(tc.in); got != tc.want {
				t.Fatalf("isAthleteTier(%v) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestAsBool(t *testing.T) {
	if !asBool(true) {
		t.Fatal("asBool(true) should be true")
	}
	if asBool("true") {
		t.Fatal("asBool(string) should be false")
	}
	if asBool(nil) {
		t.Fatal("asBool(nil) should be false")
	}
}

func TestAsInt(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want int
	}{
		"int64":   {int64(42), 42},
		"int":     {7, 7},
		"float64": {float64(13), 13},
		"string":  {"5", 0},
		"nil":     {nil, 0},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := asInt(tc.in); got != tc.want {
				t.Fatalf("asInt(%v) = %d, want %d", tc.in, got, tc.want)
			}
		})
	}
}

func TestFirstInt(t *testing.T) {
	data := map[string]interface{}{
		"sync_count_this_month": int64(9),
		"syncCountThisMonth":    int64(3),
	}
	// Prefers the first key supplied.
	if got := firstInt(data, "sync_count_this_month", "syncCountThisMonth"); got != 9 {
		t.Fatalf("firstInt snake first = %d, want 9", got)
	}
	// Falls through to a later key when the first is absent/zero.
	camelOnly := map[string]interface{}{"syncCountThisMonth": int64(4)}
	if got := firstInt(camelOnly, "sync_count_this_month", "syncCountThisMonth"); got != 4 {
		t.Fatalf("firstInt fallback = %d, want 4", got)
	}
	// Missing entirely → 0.
	if got := firstInt(map[string]interface{}{}, "a", "b"); got != 0 {
		t.Fatalf("firstInt missing = %d, want 0", got)
	}
}

func TestClassifyRunStatus(t *testing.T) {
	cases := map[string]struct {
		in   interface{}
		want string
	}{
		"synced name":         {"PIPELINE_RUN_STATUS_SYNCED", runStatusSuccess},
		"synced with pending": {"PIPELINE_RUN_STATUS_SYNCED_WITH_PENDING", runStatusSuccess},
		"legacy completed":    {"COMPLETED", runStatusSuccess},
		"failed name":         {"PIPELINE_RUN_STATUS_FAILED", runStatusFailed},
		"legacy error":        {"ERROR", runStatusFailed},
		"running":             {"PIPELINE_RUN_STATUS_RUNNING", runStatusStarted},
		"numeric synced":      {int64(2), runStatusSuccess},
		"numeric failed":      {float64(4), runStatusFailed},
		"unknown":             {"WHATEVER", runStatusStarted},
		"nil":                 {nil, runStatusStarted},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			if got := classifyRunStatus(tc.in); got != tc.want {
				t.Fatalf("classifyRunStatus(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
