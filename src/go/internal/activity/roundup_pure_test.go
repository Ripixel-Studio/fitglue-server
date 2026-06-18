package activity

import (
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
)

func TestPeriodKey(t *testing.T) {
	// Monday 2024-01-15 is in ISO week 3 of 2024.
	jan15 := time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)
	if got := periodKey(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, jan15); got != "week-03-2024" {
		t.Errorf("week key = %q, want week-03-2024", got)
	}
	if got := periodKey(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH, jan15); got != "month-01-2024" {
		t.Errorf("month key = %q, want month-01-2024", got)
	}
	if got := periodKey(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR, jan15); got != "year-2024" {
		t.Errorf("year key = %q, want year-2024", got)
	}
	if got := periodKey(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_UNSPECIFIED, jan15); got != "unknown" {
		t.Errorf("default key = %q, want unknown", got)
	}
}

func TestRoundupPeriodBounds_Week(t *testing.T) {
	// Wednesday 2024-01-17 -> previous completed week is Mon Jan 8 .. Mon Jan 15.
	now := time.Date(2024, 1, 17, 12, 0, 0, 0, time.UTC)
	start, end := RoundupPeriodBounds(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, now)
	if !start.Equal(time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("week start = %v, want 2024-01-08", start)
	}
	if !end.Equal(time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("week end = %v, want 2024-01-15", end)
	}
	if end.Sub(start) != 7*24*time.Hour {
		t.Errorf("week span = %v, want 168h", end.Sub(start))
	}
}

func TestRoundupPeriodBounds_WeekOnSunday(t *testing.T) {
	// Sunday should be treated as day 7 of the week (ISO), so the completed week
	// is the one ending on the most recent Monday.
	now := time.Date(2024, 1, 14, 9, 0, 0, 0, time.UTC) // Sunday
	start, end := RoundupPeriodBounds(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, now)
	if end.Weekday() != time.Monday || start.Weekday() != time.Monday {
		t.Errorf("bounds should fall on Mondays, got start=%v end=%v", start.Weekday(), end.Weekday())
	}
	if !end.Equal(time.Date(2024, 1, 8, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("week end = %v, want 2024-01-08", end)
	}
}

func TestRoundupPeriodBounds_Month(t *testing.T) {
	now := time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)
	start, end := RoundupPeriodBounds(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH, now)
	if !start.Equal(time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("month start = %v, want 2024-02-01", start)
	}
	if !end.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) {
		t.Errorf("month end = %v, want 2024-03-01", end)
	}
}

func TestRoundupPeriodBounds_Year(t *testing.T) {
	now := time.Date(2024, 7, 1, 0, 0, 0, 0, time.UTC)
	start, end := RoundupPeriodBounds(pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR, now)
	if start.Year() != 2023 || end.Year() != 2024 {
		t.Errorf("year bounds = %v..%v, want 2023..2024", start.Year(), end.Year())
	}
}

func TestParsePeriodKey(t *testing.T) {
	t.Run("month roundtrip", func(t *testing.T) {
		typ, start, end, err := parsePeriodKey("month-03-2024")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if typ != pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH {
			t.Errorf("type = %v", typ)
		}
		if !start.Equal(time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC)) || !end.Equal(time.Date(2024, 4, 1, 0, 0, 0, 0, time.UTC)) {
			t.Errorf("bounds = %v..%v", start, end)
		}
	})

	t.Run("year", func(t *testing.T) {
		typ, start, end, err := parsePeriodKey("year-2024")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if typ != pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR || start.Year() != 2024 || end.Year() != 2025 {
			t.Errorf("unexpected: %v %v %v", typ, start, end)
		}
	})

	t.Run("week produces Monday and 7-day span", func(t *testing.T) {
		_, start, end, err := parsePeriodKey("week-03-2024")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if start.Weekday() != time.Monday {
			t.Errorf("week start should be Monday, got %v", start.Weekday())
		}
		if end.Sub(start) != 7*24*time.Hour {
			t.Errorf("span = %v", end.Sub(start))
		}
	})

	t.Run("errors", func(t *testing.T) {
		bad := []string{"week-03", "month-xx-2024", "month-03-yyyy", "year-yyyy", "year", "bogus-1-2", "week-3-bad"}
		for _, k := range bad {
			if _, _, _, err := parsePeriodKey(k); err == nil {
				t.Errorf("expected error for %q", k)
			}
		}
	})
}

func TestParsePeriodKeyRoundTripWithPeriodKey(t *testing.T) {
	// periodKey(parsePeriodKey(k).start) should reproduce k for month/year.
	for _, k := range []string{"month-06-2024", "year-2024"} {
		typ, start, _, err := parsePeriodKey(k)
		if err != nil {
			t.Fatalf("parse %q: %v", k, err)
		}
		if got := periodKey(typ, start); got != k {
			t.Errorf("roundtrip %q -> %q", k, got)
		}
	}
}

func TestParsePeriodType(t *testing.T) {
	cases := map[string]struct {
		typ pbactivity.RoundupPeriodType
		ok  bool
	}{
		"week":  {pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_WEEK, true},
		"month": {pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_MONTH, true},
		"year":  {pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_YEAR, true},
		"day":   {pbactivity.RoundupPeriodType_ROUNDUP_PERIOD_TYPE_UNSPECIFIED, false},
	}
	for in, want := range cases {
		got, ok := parsePeriodType(in)
		if got != want.typ || ok != want.ok {
			t.Errorf("parsePeriodType(%q) = %v,%v want %v,%v", in, got, ok, want.typ, want.ok)
		}
	}
}

func TestExportJobMapRoundTrip(t *testing.T) {
	now := time.Date(2024, 5, 1, 10, 0, 0, 0, time.UTC)
	orig := &ExportJobRecord{
		JobID:      "job-1",
		Status:     "running",
		ObjectPath: "exports/job-1.zip",
		SizeBytes:  2048,
		Error:      "",
		CreatedAt:  now,
		UpdatedAt:  now.Add(time.Minute),
	}
	m := exportJobToMap(orig)
	if m["job_id"] != "job-1" || m["size_bytes"].(int64) != 2048 {
		t.Errorf("unexpected map: %v", m)
	}
	back := exportJobFromMap("job-1", m)
	if back.JobID != orig.JobID || back.Status != orig.Status || back.ObjectPath != orig.ObjectPath ||
		back.SizeBytes != orig.SizeBytes || !back.CreatedAt.Equal(orig.CreatedAt) || !back.UpdatedAt.Equal(orig.UpdatedAt) {
		t.Errorf("roundtrip mismatch: %+v vs %+v", back, orig)
	}
}

func TestExportJobFromMap_MissingFields(t *testing.T) {
	rec := exportJobFromMap("job-x", map[string]interface{}{})
	if rec.JobID != "job-x" || rec.Status != "" || rec.SizeBytes != 0 {
		t.Errorf("expected zero-value fields, got %+v", rec)
	}
}

func TestToInt64(t *testing.T) {
	cases := []struct {
		in   interface{}
		want int64
	}{
		{int64(5), 5},
		{int(7), 7},
		{float64(9.9), 9},
		{"nope", 0},
		{nil, 0},
	}
	for _, c := range cases {
		if got := toInt64(c.in); got != c.want {
			t.Errorf("toInt64(%v) = %d, want %d", c.in, got, c.want)
		}
	}
}
