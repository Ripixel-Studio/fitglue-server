package ical_title

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// icalFixture returns a minimal iCal string with a single timed event.
func icalFixture(summary string, start, end time.Time) string {
	const layout = "20060102T150405Z"
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//test//test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:test-uid@example.com\r\n" +
		"DTSTART:" + start.UTC().Format(layout) + "\r\n" +
		"DTEND:" + end.UTC().Format(layout) + "\r\n" +
		"SUMMARY:" + summary + "\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func icalMultiFixture(events []struct {
	summary    string
	start, end time.Time
}) string {
	const layout = "20060102T150405Z"
	s := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//test//test//EN\r\n"
	for _, e := range events {
		s += "BEGIN:VEVENT\r\n" +
			"UID:" + e.summary + "@example.com\r\n" +
			"DTSTART:" + e.start.UTC().Format(layout) + "\r\n" +
			"DTEND:" + e.end.UTC().Format(layout) + "\r\n" +
			"SUMMARY:" + e.summary + "\r\n" +
			"END:VEVENT\r\n"
	}
	return s + "END:VCALENDAR\r\n"
}

func icalAllDayFixture(summary string, date time.Time) string {
	return "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//test//test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:allday@example.com\r\n" +
		"DTSTART;VALUE=DATE:" + date.UTC().Format("20060102") + "\r\n" +
		"DTEND;VALUE=DATE:" + date.UTC().AddDate(0, 0, 1).Format("20060102") + "\r\n" +
		"SUMMARY:" + summary + "\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"
}

func newTestActivity(start time.Time, durationSec float64) *pbactivity.StandardizedActivity {
	return &pbactivity.StandardizedActivity{
		StartTime: timestamppb.New(start),
		Sessions: []*pbactivity.Session{
			{TotalElapsedTime: durationSec},
		},
	}
}

func serveICal(t *testing.T, body string) (*httptest.Server, *http.Client) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/calendar")
		w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv, srv.Client()
}

func TestProviderType(t *testing.T) {
	p := NewICalTitle()
	if p.ProviderType() != pbplugin.EnricherProviderType_ENRICHER_PROVIDER_ICAL_TITLE {
		t.Errorf("unexpected provider type %v", p.ProviderType())
	}
}

func TestName(t *testing.T) {
	p := NewICalTitle()
	if p.Name() != "ical_title" {
		t.Errorf("unexpected name %q", p.Name())
	}
}

func TestNoURL(t *testing.T) {
	p := NewICalTitle()
	act := newTestActivity(time.Now(), 3600)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{}, p.httpClient)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Error("expected Skipped when no ical_url configured")
	}
}

func TestSingleMatch(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	evStart := now.Add(-30 * time.Minute)
	evEnd := now.Add(30 * time.Minute)

	srv, client := serveICal(t, icalFixture("Morning Run", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Fatalf("expected match but got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Morning Run" {
		t.Errorf("expected %q, got %q", "Morning Run", result.Name)
	}
}

func TestNoOverlap(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	evStart := now.Add(2 * time.Hour)
	evEnd := now.Add(3 * time.Hour)

	srv, client := serveICal(t, icalFixture("Later Meeting", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Error("expected Skipped when event does not overlap")
	}
}

func TestMultipleMatches(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	events := []struct {
		summary    string
		start, end time.Time
	}{
		{"Event A", now.Add(-10 * time.Minute), now.Add(20 * time.Minute)},
		{"Event B", now.Add(-5 * time.Minute), now.Add(25 * time.Minute)},
	}

	srv, client := serveICal(t, icalMultiFixture(events))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Error("expected Skipped when multiple events overlap")
	}
}

func TestAllDayEventSkipped(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)

	srv, client := serveICal(t, icalAllDayFixture("Public Holiday", now))

	p := NewICalTitle()
	act := newTestActivity(now, 3600)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Error("expected Skipped for all-day event")
	}
}

func TestMinOverlapRespected(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	// Event overlaps by only 30 seconds; default min is 60s
	evStart := now.Add(-1 * time.Hour)
	evEnd := now.Add(30 * time.Second)

	srv, client := serveICal(t, icalFixture("Short Overlap", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 3600)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Error("expected Skipped when overlap is below min threshold")
	}
}

func TestCustomMinOverlap(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	// Event overlaps by 30 seconds; custom min set to 10s
	evStart := now.Add(-1 * time.Hour)
	evEnd := now.Add(30 * time.Second)

	srv, client := serveICal(t, icalFixture("Short Overlap OK", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 3600)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{
		"ical_url":            srv.URL,
		"min_overlap_seconds": "10",
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected match with custom min_overlap_seconds=10, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Short Overlap OK" {
		t.Errorf("expected %q, got %q", "Short Overlap OK", result.Name)
	}
}

func TestTZIDMatch(t *testing.T) {
	// Reproduces: Bootcamp Class 6:30–7:00 BST, activity starts 6:31 BST (5:31 UTC).
	// Without TZID-aware parsing the event is treated as 6:30 UTC and no overlap is found.
	icalBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Google Inc//GoogleCalendarV1//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp@example.com\r\n" +
		"DTSTART;TZID=Europe/London:20260521T063000\r\n" +
		"DTEND;TZID=Europe/London:20260521T070000\r\n" +
		"SUMMARY:Bootcamp Class\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	srv, client := serveICal(t, icalBody)

	p := NewICalTitle()
	// Activity 5:31–6:08 UTC (= 6:31–7:08 BST), 37 minutes
	actStart := time.Date(2026, 5, 21, 5, 31, 0, 0, time.UTC)
	act := newTestActivity(actStart, 37*60)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected match for TZID-based BST event, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Bootcamp Class" {
		t.Errorf("expected %q, got %q", "Bootcamp Class", result.Name)
	}
}

func TestRRuleExpansion(t *testing.T) {
	// Recurring weekly Thursday Bootcamp that started in January.
	// The activity is on May 21, 2026 (Thursday) at 5:31 UTC (= 6:31 BST).
	// Without RRULE expansion, DTSTART points to January and no overlap is found.
	icalBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Google Inc//GoogleCalendarV1//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-recurring@example.com\r\n" +
		"DTSTART;TZID=Europe/London:20260115T063000\r\n" +
		"DTEND;TZID=Europe/London:20260115T070000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TH\r\n" +
		"SUMMARY:Bootcamp Class\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	srv, client := serveICal(t, icalBody)

	p := NewICalTitle()
	actStart := time.Date(2026, 5, 21, 5, 31, 0, 0, time.UTC)
	act := newTestActivity(actStart, 37*60)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected RRULE occurrence to match, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Bootcamp Class" {
		t.Errorf("expected %q, got %q", "Bootcamp Class", result.Name)
	}
}

// --- RECURRENCE-ID: edited single occurrences of a recurring event ---

func TestRecurrenceIdOverrideNoFalseClash(t *testing.T) {
	// Weekly Thursday bootcamp. The 2026-05-21 occurrence was edited (moved 15 min
	// later and retitled) via a RECURRENCE-ID override. Google Calendar does NOT add
	// an EXDATE for edits, so the master still yields an occurrence at 06:30 and the
	// override yields one at 06:45 — both overlap the activity. Without handling the
	// override this looks like two clashing events; it must resolve to the override.
	icalBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Google Inc//GoogleCalendarV1//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-recur@example.com\r\n" +
		"DTSTART;TZID=Europe/London:20260115T063000\r\n" +
		"DTEND;TZID=Europe/London:20260115T070000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TH\r\n" +
		"SUMMARY:Bootcamp Class\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-recur@example.com\r\n" +
		"RECURRENCE-ID;TZID=Europe/London:20260521T063000\r\n" +
		"DTSTART;TZID=Europe/London:20260521T064500\r\n" +
		"DTEND;TZID=Europe/London:20260521T071500\r\n" +
		"SUMMARY:Bootcamp Class (Special)\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	srv, client := serveICal(t, icalBody)

	p := NewICalTitle()
	actStart := time.Date(2026, 5, 21, 5, 46, 0, 0, time.UTC) // 6:46 BST
	act := newTestActivity(actStart, 37*60)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Fatalf("expected the edited occurrence to match, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Bootcamp Class (Special)" {
		t.Errorf("expected %q, got %q", "Bootcamp Class (Special)", result.Name)
	}
}

func TestRecurrenceIdOverrideOtherOccurrencesUnaffected(t *testing.T) {
	// Same calendar; the override only suppresses the 2026-05-21 master occurrence.
	// The following Thursday (2026-05-28) must still match the master normally.
	icalBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//Google Inc//GoogleCalendarV1//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-recur2@example.com\r\n" +
		"DTSTART;TZID=Europe/London:20260115T063000\r\n" +
		"DTEND;TZID=Europe/London:20260115T070000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TH\r\n" +
		"SUMMARY:Bootcamp Class\r\n" +
		"END:VEVENT\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-recur2@example.com\r\n" +
		"RECURRENCE-ID;TZID=Europe/London:20260521T063000\r\n" +
		"DTSTART;TZID=Europe/London:20260521T064500\r\n" +
		"DTEND;TZID=Europe/London:20260521T071500\r\n" +
		"SUMMARY:Bootcamp Class (Special)\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	srv, client := serveICal(t, icalBody)

	p := NewICalTitle()
	actStart := time.Date(2026, 5, 28, 5, 31, 0, 0, time.UTC) // 6:31 BST, next Thursday
	act := newTestActivity(actStart, 37*60)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Fatalf("expected the following-week occurrence to match, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Bootcamp Class" {
		t.Errorf("expected %q, got %q", "Bootcamp Class", result.Name)
	}
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	p := NewICalTitle()
	act := newTestActivity(time.Now(), 3600)
	_, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, srv.Client())
	if err == nil {
		t.Error("expected error on HTTP 500")
	}
}

// --- File-upload title protection ---

func TestFileUploadTitleNotOverridden(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	evStart := now.Add(-30 * time.Minute)
	evEnd := now.Add(30 * time.Minute)

	srv, client := serveICal(t, icalFixture("Calendar Event", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	act.Source = pbactivity.ActivitySource_SOURCE_FILE_UPLOAD
	act.Name = "My FIT File Title"

	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Error("expected Skipped when activity is a file upload with an existing name")
	}
}

func TestFileUploadWithAutoGeneratedNameGetsCalendarTitle(t *testing.T) {
	// Simulates the common case: FIT file had no embedded name so the parser generated
	// "Morning Run". The iCal enricher should still match the calendar event.
	now := time.Date(2026, 5, 28, 8, 0, 0, 0, time.UTC) // 08:00 → "Morning"
	evStart := now.Add(-30 * time.Minute)
	evEnd := now.Add(30 * time.Minute)

	srv, client := serveICal(t, icalFixture("Bootcamp", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	act.Source = pbactivity.ActivitySource_SOURCE_FILE_UPLOAD
	act.Type = pbactivity.ActivityType_ACTIVITY_TYPE_RUN
	act.Name = "Morning Run" // auto-generated by fit_parser.GenerateActivityName

	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected calendar match to override auto-generated name, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Bootcamp" {
		t.Errorf("expected %q, got %q", "Bootcamp", result.Name)
	}
}

func TestFileUploadWithNoNameGetsCalendarTitle(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	evStart := now.Add(-30 * time.Minute)
	evEnd := now.Add(30 * time.Minute)

	srv, client := serveICal(t, icalFixture("Calendar Event", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	act.Source = pbactivity.ActivitySource_SOURCE_FILE_UPLOAD
	act.Name = "" // no name in the file

	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected match for file-upload with no name, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Calendar Event" {
		t.Errorf("expected %q, got %q", "Calendar Event", result.Name)
	}
}

// --- Multi-calendar priority ---

func TestSecondCalendarUsedWhenPrimaryHasNoMatch(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	evStart := now.Add(-30 * time.Minute)
	evEnd := now.Add(30 * time.Minute)

	// Primary calendar has an event AFTER the activity — no overlap.
	primary := icalFixture("Primary Event", now.Add(2*time.Hour), now.Add(3*time.Hour))
	// Secondary calendar has a matching event.
	secondary := icalFixture("Secondary Event", evStart, evEnd)

	srv1, client := serveICal(t, primary)
	srv2, _ := serveICal(t, secondary)

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{
		"ical_url":   srv1.URL,
		"ical_url_2": srv2.URL,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected match from secondary calendar, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Secondary Event" {
		t.Errorf("expected %q, got %q", "Secondary Event", result.Name)
	}
}

func TestPrimaryCalendarWinsOverSecondary(t *testing.T) {
	now := time.Now().UTC().Truncate(time.Second)
	evStart := now.Add(-30 * time.Minute)
	evEnd := now.Add(30 * time.Minute)

	// Both calendars have exactly-one matching event.
	srv1, client := serveICal(t, icalFixture("Primary Event", evStart, evEnd))
	srv2, _ := serveICal(t, icalFixture("Secondary Event", evStart, evEnd))

	p := NewICalTitle()
	act := newTestActivity(now, 1800)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{
		"ical_url":   srv1.URL,
		"ical_url_2": srv2.URL,
	}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected match, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Primary Event" {
		t.Errorf("expected primary calendar to win, got %q", result.Name)
	}
}

// --- EXDATE: deleted recurring event occurrences ---

func TestExdateSkipsDeletedOccurrence(t *testing.T) {
	// Weekly Thursday bootcamp, but the occurrence on 2026-05-21 was deleted via EXDATE.
	// The activity is on 2026-05-21 — it should NOT match.
	icalBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//test//test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-exdate@example.com\r\n" +
		"DTSTART;TZID=Europe/London:20260115T063000\r\n" +
		"DTEND;TZID=Europe/London:20260115T070000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TH\r\n" +
		"EXDATE;TZID=Europe/London:20260521T063000\r\n" +
		"SUMMARY:Bootcamp Class\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	srv, client := serveICal(t, icalBody)

	p := NewICalTitle()
	actStart := time.Date(2026, 5, 21, 5, 31, 0, 0, time.UTC) // 6:31 BST
	act := newTestActivity(actStart, 37*60)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Skipped {
		t.Errorf("expected Skipped for EXDATE-deleted occurrence, got title %q", result.Name)
	}
}

func TestExdateDoesNotBlockOtherOccurrences(t *testing.T) {
	// Same weekly bootcamp; 2026-05-21 is deleted but 2026-05-28 is not.
	icalBody := "BEGIN:VCALENDAR\r\n" +
		"VERSION:2.0\r\n" +
		"PRODID:-//test//test//EN\r\n" +
		"BEGIN:VEVENT\r\n" +
		"UID:bootcamp-exdate2@example.com\r\n" +
		"DTSTART;TZID=Europe/London:20260115T063000\r\n" +
		"DTEND;TZID=Europe/London:20260115T070000\r\n" +
		"RRULE:FREQ=WEEKLY;BYDAY=TH\r\n" +
		"EXDATE;TZID=Europe/London:20260521T063000\r\n" +
		"SUMMARY:Bootcamp Class\r\n" +
		"END:VEVENT\r\n" +
		"END:VCALENDAR\r\n"

	srv, client := serveICal(t, icalBody)

	p := NewICalTitle()
	actStart := time.Date(2026, 5, 28, 5, 31, 0, 0, time.UTC) // 6:31 BST, next Thursday
	act := newTestActivity(actStart, 37*60)
	result, err := p.enrich(context.Background(), slog.Default(), act, map[string]string{"ical_url": srv.URL}, client)
	if err != nil {
		t.Fatal(err)
	}
	if result.Skipped {
		t.Errorf("expected match for non-deleted occurrence on 2026-05-28, got Skipped: %s", result.SkipReason)
	}
	if result.Name != "Bootcamp Class" {
		t.Errorf("expected %q, got %q", "Bootcamp Class", result.Name)
	}
}
