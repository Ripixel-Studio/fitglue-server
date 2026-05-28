package ical_title

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	ical "github.com/arran4/golang-ical"
	"github.com/teambition/rrule-go"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher/providers"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/fit_parser"
	"github.com/fitglue/server/src/go/pkg/domain/user"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

const (
	defaultMinOverlapSeconds = 60
	fetchTimeout             = 10 * time.Second
	// rruleSearchBuffer is how far either side of the activity we expand recurring events.
	// 24 h catches any event that started before or ends after the activity on the same day.
	rruleSearchBuffer = 24 * time.Hour
)

type ICalTitle struct {
	Service    *bootstrap.Service
	httpClient *http.Client
}

func init() {
	providers.Register(NewICalTitle())
}

func NewICalTitle() *ICalTitle {
	return &ICalTitle{
		httpClient: &http.Client{Timeout: fetchTimeout},
	}
}

func (p *ICalTitle) SetService(service *bootstrap.Service) {
	p.Service = service
}

func (p *ICalTitle) Name() string {
	return "ical_title"
}

func (p *ICalTitle) ProviderType() pbplugin.EnricherProviderType {
	return pbplugin.EnricherProviderType_ENRICHER_PROVIDER_ICAL_TITLE
}

func (p *ICalTitle) Enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, user *user.Record, inputs map[string]string, doNotRetry bool) (*providers.EnrichmentResult, error) {
	return p.enrich(ctx, logger, activity, inputs, p.httpClient)
}

func (p *ICalTitle) enrich(ctx context.Context, logger *slog.Logger, activity *pbactivity.StandardizedActivity, inputs map[string]string, client *http.Client) (*providers.EnrichmentResult, error) {
	if activity.StartTime == nil {
		return nil, fmt.Errorf("activity has no start time")
	}

	// Respect titles that were explicitly set — either embedded in the source file or provided by
	// the user at upload time. Auto-generated fallback names (e.g. "Morning Run") are NOT considered
	// explicitly set, so the calendar enricher is still allowed to override them.
	if activity.Name != "" && activity.Source == pbactivity.ActivitySource_SOURCE_FILE_UPLOAD {
		autoName := fit_parser.GenerateActivityName(activity.Type, activity.StartTime.AsTime())
		if activity.Name != autoName {
			return &providers.EnrichmentResult{
				Skipped:    true,
				SkipReason: "file-upload title already set; not overriding",
			}, nil
		}
		// Name matches the auto-generated fallback — fall through and let the calendar match override it.
	}

	urls := collectCalendarURLs(inputs)
	if len(urls) == 0 {
		return &providers.EnrichmentResult{
			Skipped:    true,
			SkipReason: "no ical_url configured",
		}, nil
	}

	minOverlap := defaultMinOverlapSeconds
	if v, ok := inputs["min_overlap_seconds"]; ok && v != "" {
		if n, err := parseInt(v); err == nil && n >= 0 {
			minOverlap = n
		}
	}
	actStart := activity.StartTime.AsTime()
	actEnd := activityEndTime(activity, actStart)

	// Try each calendar in priority order; first single-match wins.
	var firstFetchErr error
	for i, url := range urls {
		cal, err := fetchCalendar(ctx, client, url)
		if err != nil {
			if firstFetchErr == nil {
				firstFetchErr = err
			}
			logger.Warn("ical_title: failed to fetch calendar", "url_index", i+1, "error", err)
			continue
		}

		matches := overlappingEvents(cal, actStart, actEnd, minOverlap)

		switch len(matches) {
		case 0:
			logger.Info("ical_title: no match on calendar", "url_index", i+1, "activity_start", actStart)
			// Try next calendar in priority order.
		case 1:
			title := matches[0]
			logger.Info("ical_title: matched event", "url_index", i+1, "title", title)
			return &providers.EnrichmentResult{Name: title}, nil
		default:
			// Multiple matches are ambiguous — stop and skip rather than guessing.
			logger.Info("ical_title: multiple events overlap on calendar, skipping", "url_index", i+1, "count", len(matches))
			return &providers.EnrichmentResult{
				Skipped:    true,
				SkipReason: fmt.Sprintf("%d calendar events overlap this activity; cannot determine a single title", len(matches)),
			}, nil
		}
	}

	logger.Info("ical_title: no overlapping events found in any calendar", "activity_start", actStart)
	if firstFetchErr != nil {
		return nil, firstFetchErr
	}
	return &providers.EnrichmentResult{
		Skipped:    true,
		SkipReason: "no calendar event overlaps this activity",
	}, nil
}

// collectCalendarURLs returns iCal feed URLs from inputs in priority order.
// Reads ical_url (primary), then ical_url_2 and ical_url_3.
func collectCalendarURLs(inputs map[string]string) []string {
	var urls []string
	for _, key := range []string{"ical_url", "ical_url_2", "ical_url_3"} {
		if u := strings.TrimSpace(inputs[key]); u != "" {
			urls = append(urls, u)
		}
	}
	return urls
}

func fetchCalendar(ctx context.Context, client *http.Client, url string) (*ical.Calendar, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d fetching iCal", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading iCal body: %w", err)
	}

	cal, err := ical.ParseCalendar(strings.NewReader(string(body)))
	if err != nil {
		return nil, fmt.Errorf("parsing iCal: %w", err)
	}
	return cal, nil
}

// overlappingEvents returns the summaries of VEVENT entries whose time range
// overlaps [actStart, actEnd] by at least minOverlapSeconds.
// All-day events (DATE-only) are excluded.
// Recurring events (RRULE) are expanded; EXDATE-excluded occurrences are skipped.
func overlappingEvents(cal *ical.Calendar, actStart, actEnd time.Time, minOverlapSeconds int) []string {
	var matches []string
	minOverlap := time.Duration(minOverlapSeconds) * time.Second
	searchStart := actStart.Add(-rruleSearchBuffer)
	searchEnd := actEnd.Add(rruleSearchBuffer)

	for _, component := range cal.Components {
		event, ok := component.(*ical.VEvent)
		if !ok {
			continue
		}

		dtstart := event.GetProperty(ical.ComponentPropertyDtStart)
		if dtstart == nil || isDateOnly(dtstart) {
			continue
		}

		evStart, err := parseEventTime(dtstart)
		if err != nil {
			continue
		}
		evEnd := evStart
		if dtend := event.GetProperty(ical.ComponentPropertyDtEnd); dtend != nil && !isDateOnly(dtend) {
			if t, err := parseEventTime(dtend); err == nil {
				evEnd = t
			}
		}
		duration := evEnd.Sub(evStart)

		summary := event.GetProperty(ical.ComponentPropertySummary)
		if summary == nil || strings.TrimSpace(summary.Value) == "" {
			continue
		}
		title := strings.TrimSpace(summary.Value)

		rruleProp := event.GetProperty(ical.ComponentPropertyRrule)
		if rruleProp == nil {
			// Non-recurring: check overlap directly.
			if overlapDuration(actStart, actEnd, evStart, evEnd) >= minOverlap {
				matches = append(matches, title)
			}
			continue
		}

		// Recurring event: collect EXDATE exclusions, then expand and check each occurrence.
		exdates := parseExdates(event)

		option, err := rrule.StrToROption(rruleProp.Value)
		if err != nil {
			// Unparseable RRULE — fall back to checking the base occurrence.
			if overlapDuration(actStart, actEnd, evStart, evEnd) >= minOverlap {
				matches = append(matches, title)
			}
			continue
		}
		option.Dtstart = evStart
		r, err := rrule.NewRRule(*option)
		if err != nil {
			continue
		}
		for _, occ := range r.Between(searchStart, searchEnd, true) {
			if isExdated(occ, exdates) {
				continue
			}
			occEnd := occ.Add(duration)
			if overlapDuration(actStart, actEnd, occ, occEnd) >= minOverlap {
				matches = append(matches, title)
				break
			}
		}
	}
	return matches
}

// parseExdates collects all excluded datetime occurrences from an event's EXDATE properties.
// EXDATE values may be comma-separated within a single property, or spread across multiple
// EXDATE lines (both forms are valid per RFC 5545).
func parseExdates(event *ical.VEvent) map[time.Time]struct{} {
	excluded := make(map[time.Time]struct{})
	for _, prop := range event.Properties {
		if !strings.EqualFold(prop.IANAToken, "EXDATE") {
			continue
		}
		// Value may be a comma-separated list of datetime strings sharing the same TZID.
		for _, dateStr := range strings.Split(prop.Value, ",") {
			dateStr = strings.TrimSpace(dateStr)
			if dateStr == "" {
				continue
			}
			// Reuse parseEventTime with a copy of the property set to this single date.
			tmp := prop
			tmp.Value = dateStr
			if t, err := parseEventTime(&tmp); err == nil {
				// Truncate to minute so EXDATE at e.g. T063000Z matches occurrence at T063000Z
				// even when there is sub-minute clock skew in the ICS feed.
				excluded[t.UTC().Truncate(time.Minute)] = struct{}{}
			}
		}
	}
	return excluded
}

// isExdated reports whether an occurrence is excluded by an EXDATE.
func isExdated(occ time.Time, exdates map[time.Time]struct{}) bool {
	_, excluded := exdates[occ.UTC().Truncate(time.Minute)]
	return excluded
}

func overlapDuration(aStart, aEnd, bStart, bEnd time.Time) time.Duration {
	start := aStart
	if bStart.After(start) {
		start = bStart
	}
	end := aEnd
	if bEnd.Before(end) {
		end = bEnd
	}
	if end.Before(start) {
		return 0
	}
	return end.Sub(start)
}

func isDateOnly(prop *ical.IANAProperty) bool {
	if prop == nil {
		return false
	}
	for _, vals := range prop.ICalParameters {
		for _, v := range vals {
			if strings.EqualFold(v, "DATE") {
				return true
			}
		}
	}
	// Heuristic: date-only values are exactly 8 chars (YYYYMMDD)
	return len(strings.TrimSpace(prop.Value)) == 8
}

// parseEventTime parses a DTSTART/DTEND property, correctly applying the TZID
// parameter when present. The returned time preserves the original Location so
// that rrule-go can handle DST transitions correctly when expanding recurring
// events — without this, a weekly event at "6:30 AM London" would be generated
// at the wrong UTC offset once clocks change.
func parseEventTime(prop *ical.IANAProperty) (time.Time, error) {
	value := strings.TrimSpace(prop.Value)

	for paramName, paramVals := range prop.ICalParameters {
		if strings.EqualFold(paramName, "TZID") && len(paramVals) > 0 {
			loc, err := time.LoadLocation(paramVals[0])
			if err != nil {
				return time.Time{}, fmt.Errorf("unknown timezone %q: %w", paramVals[0], err)
			}
			for _, layout := range []string{"20060102T150405", "20060102T150405Z"} {
				if t, err := time.ParseInLocation(layout, value, loc); err == nil {
					return t, nil // keep original Location; Go time comparisons are TZ-aware
				}
			}
			return time.Time{}, fmt.Errorf("cannot parse %q with TZID %q", value, paramVals[0])
		}
	}

	// UTC (trailing Z) or floating time
	for _, layout := range []string{"20060102T150405Z", "20060102T150405"} {
		if t, err := time.Parse(layout, value); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("cannot parse time %q", value)
}

func activityEndTime(activity *pbactivity.StandardizedActivity, start time.Time) time.Time {
	if len(activity.Sessions) > 0 && activity.Sessions[0].TotalElapsedTime > 0 {
		return start.Add(time.Duration(activity.Sessions[0].TotalElapsedTime) * time.Second)
	}
	return start.Add(time.Hour)
}

func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}
