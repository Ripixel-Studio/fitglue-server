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
