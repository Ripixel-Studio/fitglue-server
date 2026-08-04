package parkrun

import (
	"strings"
	"testing"
)

func TestOrdinal(t *testing.T) {
	cases := map[int]string{
		0:   "0",
		-1:  "-1",
		1:   "1st",
		2:   "2nd",
		3:   "3rd",
		4:   "4th",
		10:  "10th",
		11:  "11th",
		12:  "12th",
		13:  "13th",
		21:  "21st",
		22:  "22nd",
		23:  "23rd",
		24:  "24th",
		100: "100th",
		101: "101st",
		111: "111th",
		112: "112th",
		113: "113th",
		121: "121st",
	}
	for n, want := range cases {
		if got := Ordinal(n); got != want {
			t.Errorf("Ordinal(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestParseTimeToSeconds(t *testing.T) {
	cases := []struct {
		in   string
		want int
	}{
		{"24:30", 24*60 + 30},
		{"01:02:03", 3600 + 2*60 + 3},
		{"00:00", 0},
		{"", 0},        // single part -> default 0
		{"1:2:3:4", 0}, // too many parts -> default 0
		{"abc:def", 0}, // unparseable -> 0
		{"5:00", 300},
	}
	for _, c := range cases {
		if got := parseTimeToSeconds(c.in); got != c.want {
			t.Errorf("parseTimeToSeconds(%q) = %d, want %d", c.in, got, c.want)
		}
	}
}

func TestParseAgeGrade(t *testing.T) {
	cases := []struct {
		in   string
		want float64
	}{
		{"54.76%", 54.76},
		{"60%", 60},
		{"", 0},
		{"bad", 0},
	}
	for _, c := range cases {
		if got := parseAgeGrade(c.in); got != c.want {
			t.Errorf("parseAgeGrade(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestExtractEventSlugFromRow(t *testing.T) {
	cases := []struct {
		name string
		row  string
		want string
	}{
		{"https uk", `<a href="https://www.parkrun.org.uk/newark/results/">Newark</a>`, "newark"},
		{"http", `<a href="http://www.parkrun.org.uk/Bushy/results/">Bushy</a>`, "bushy"},
		{"no match", `<a href="https://www.parkrun.org.uk/newark/">Newark</a>`, ""},
		{"empty", ``, ""},
	}
	for _, c := range cases {
		if got := extractEventSlugFromRow(c.row); got != c.want {
			t.Errorf("%s: extractEventSlugFromRow = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestStripTags(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"<a href=\"x\">Newark</a>", "Newark"},
		{"  <span>24:30</span>  ", "24:30"},
		{"plain", "plain"},
		{"<b><i>nested</i></b>", "nested"},
	}
	for _, c := range cases {
		if got := stripTags(c.in); got != c.want {
			t.Errorf("stripTags(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestIsTimePB(t *testing.T) {
	cases := []struct {
		name     string
		target   string
		allTimes []string
		want     bool
	}{
		{"zero target", "", []string{"25:00"}, false},
		{"single run not a PB", "24:00", []string{"24:00"}, false},
		{"new fastest with history", "23:00", []string{"23:00", "25:00", "24:00"}, true},
		{"slower than a prior run", "26:00", []string{"26:00", "24:00"}, false},
		{"faster ignores zero-parse entries", "23:00", []string{"23:00", "bad"}, true},
	}
	for _, c := range cases {
		if got := isTimePB(c.target, c.allTimes, ""); got != c.want {
			t.Errorf("%s: isTimePB = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsPositionPB(t *testing.T) {
	cases := []struct {
		name      string
		target    int
		positions []int
		want      bool
	}{
		{"single run not PB", 10, []int{10}, false},
		{"best position with history", 5, []int{5, 10, 8}, true},
		{"worse than prior", 12, []int{12, 5}, false},
	}
	for _, c := range cases {
		if got := isPositionPB(c.target, c.positions); got != c.want {
			t.Errorf("%s: isPositionPB = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsAgeGradePB(t *testing.T) {
	cases := []struct {
		name   string
		target float64
		all    []float64
		want   bool
	}{
		{"single run not PB", 55, []float64{55}, false},
		{"highest with history", 60, []float64{60, 55, 50}, true},
		{"lower than prior", 50, []float64{50, 60}, false},
	}
	for _, c := range cases {
		if got := isAgeGradePB(c.target, c.all); got != c.want {
			t.Errorf("%s: isAgeGradePB = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestIsTimePBThisYear(t *testing.T) {
	if isTimePBThisYear("", []string{"25:00"}) {
		t.Error("zero target should not be PB")
	}
	if !isTimePBThisYear("23:00", []string{"23:00", "24:00"}) {
		t.Error("expected this-year time PB")
	}
	if isTimePBThisYear("26:00", []string{"26:00", "24:00"}) {
		t.Error("slower should not be PB")
	}
}

func TestIsPositionPBThisYear(t *testing.T) {
	if !isPositionPBThisYear(5, []int{5, 10}) {
		t.Error("expected this-year position PB")
	}
	if isPositionPBThisYear(12, []int{12, 5}) {
		t.Error("worse should not be PB")
	}
}

func TestIsAgeGradePBThisYear(t *testing.T) {
	if !isAgeGradePBThisYear(60, []float64{60, 55}) {
		t.Error("expected this-year age grade PB")
	}
	if isAgeGradePBThisYear(50, []float64{50, 60}) {
		t.Error("lower should not be PB")
	}
}

func TestFormatResultsDescription(t *testing.T) {
	if got := FormatResultsDescription(nil, "Newark"); got != "" {
		t.Errorf("nil result should give empty string, got %q", got)
	}

	res := &Result{
		Time:               "24:30",
		Position:           15,
		AgeGrade:           "55.00%",
		TimeAllTimePB:      true,
		TimeThisYearPB:     true,
		PosAllTimePB:       true,
		PosThisYearPB:      true,
		AgeGradeAllTimePB:  true,
		AgeGradeThisYearPB: true,
		TotalAtLocation:    3,
		TotalAllTime:       42,
		FirstAtLocation:    true,
	}
	out := FormatResultsDescription(res, "Newark")

	for _, want := range []string{
		"Parkrun Results",
		"Position: 15th",
		"Time: 24:30",
		"Age Grade: 55.00%",
		"Location: Newark",
		"3rd",
		"42 total",
		"New all-time PB!",
		"New this-year PB!",
		"First time at this location!",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("description missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestFormatManualResultsDescription_FullFields(t *testing.T) {
	out := FormatManualResultsDescription("Newark Parkrun", 42, "25:30", "55.5%", 137, true, true)
	for _, want := range []string{
		"🏃 Parkrun Results:",
		"Position: 42nd",
		"Time: 25:30 · 🏆 New all-time PB!",
		"Age Grade: 55.5% · 🏆 New all-time PB!",
		"Location: Newark Parkrun (137 total)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("description missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestFormatManualResultsDescription_NoPBs(t *testing.T) {
	out := FormatManualResultsDescription("Newark Parkrun", 42, "25:30", "55.5%", 137, false, false)
	if strings.Contains(out, "PB") {
		t.Errorf("no PB badge expected when flags false, got:\n%s", out)
	}
}

// A partial manual entry (only the mandatory finish time) must degrade gracefully:
// no position/age-grade lines, and the location line must NOT render "(0 total)".
func TestFormatManualResultsDescription_GracefulDegradation(t *testing.T) {
	out := FormatManualResultsDescription("Newark Parkrun", 0, "25:30", "", 0, false, false)
	if !strings.Contains(out, "Time: 25:30") {
		t.Errorf("expected time line, got:\n%s", out)
	}
	if strings.Contains(out, "Position") {
		t.Errorf("position line should be omitted when zero, got:\n%s", out)
	}
	if strings.Contains(out, "Age Grade") {
		t.Errorf("age grade line should be omitted when blank, got:\n%s", out)
	}
	if strings.Contains(out, "total") {
		t.Errorf("blank total must not render a bogus count, got:\n%s", out)
	}
	if !strings.Contains(out, "Location: Newark Parkrun") {
		t.Errorf("event name should still anchor the location line, got:\n%s", out)
	}
}

// No event name and no total: the location line drops entirely rather than printing
// an empty or zero-count line.
func TestFormatManualResultsDescription_NoLocation(t *testing.T) {
	out := FormatManualResultsDescription("", 42, "25:30", "", 0, false, false)
	if strings.Contains(out, "Location") || strings.Contains(out, "Total parkruns") {
		t.Errorf("location line should be absent with no event name / total, got:\n%s", out)
	}
	// Total supplied but no event name → surface the count on its own line.
	out2 := FormatManualResultsDescription("", 42, "25:30", "", 250, false, false)
	if !strings.Contains(out2, "Total parkruns: 250") {
		t.Errorf("expected standalone total line, got:\n%s", out2)
	}
}

func TestFormatResultsDescription_NoAgeGradeNoBadges(t *testing.T) {
	res := &Result{
		Time:            "25:00",
		Position:        20,
		AgeGrade:        "", // omitted line
		TotalAtLocation: 1,
		TotalAllTime:    1,
		FirstAtLocation: false,
	}
	out := FormatResultsDescription(res, "Bushy")
	if strings.Contains(out, "Age Grade") {
		t.Errorf("age grade line should be omitted when empty: %s", out)
	}
	if strings.Contains(out, "First time at this location") {
		t.Errorf("first-time badge should be absent: %s", out)
	}
	if !strings.Contains(out, "Location: Bushy, 1st Parkrun here (1 total)") {
		t.Errorf("unexpected location line: %s", out)
	}
}
