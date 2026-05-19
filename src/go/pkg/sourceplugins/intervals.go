// nolint:proto-json
package sourceplugins

import (
	"context"
	"fmt"
	"strconv"
	"time"

	fit_parser "github.com/fitglue/server/src/go/pkg/domain/fit_parser"
	intervalsclient "github.com/fitglue/server/src/go/pkg/integrations/intervals"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func init() {
	register("SOURCE_INTERVALS", &intervalsProvider{})
}

type intervalsProvider struct{}

func (p *intervalsProvider) ListActivities(ctx context.Context, integ *userpb.UserIntegrations, pageToken string) ([]*SourceActivity, string, error) {
	if integ.Intervals == nil || integ.Intervals.ApiKey == "" || integ.Intervals.AthleteId == "" {
		return nil, "", fmt.Errorf("intervals integration not configured")
	}

	client := intervalsclient.NewClient(integ.Intervals.ApiKey, integ.Intervals.AthleteId)

	// Intervals returns all activities within the date range; use a wide range for backfill.
	// pageToken encodes an offset index since Intervals has no server-side pagination.
	// We fetch all at once and return a single page.
	activities, err := client.ListActivities(ctx, intervalsclient.ListActivitiesParams{
		Oldest: "2000-01-01",
		Newest: time.Now().Format("2006-01-02"),
	})
	if err != nil {
		return nil, "", fmt.Errorf("failed to list intervals activities: %w", err)
	}

	offset := 0
	if pageToken != "" {
		n, err := strconv.Atoi(pageToken)
		if err == nil && n >= 0 {
			offset = n
		}
	}

	const pageSize = 50
	end := offset + pageSize
	if end > len(activities) {
		end = len(activities)
	}

	var result []*SourceActivity
	for _, a := range activities[offset:end] {
		sa := &SourceActivity{
			ID:    strconv.FormatInt(a.ID, 10),
			Title: a.Name,
			Type:  a.Type,
		}
		if t, err := time.Parse("2006-01-02T15:04:05", a.StartDateLocal); err == nil {
			sa.StartTime = t
		}
		result = append(result, sa)
	}

	var nextPageToken string
	if end < len(activities) {
		nextPageToken = strconv.Itoa(end)
	}

	return result, nextPageToken, nil
}

func (p *intervalsProvider) FetchActivity(ctx context.Context, integ *userpb.UserIntegrations, userID string, sourceActivityID string) (*pbevents.ActivityPayload, error) {
	if integ.Intervals == nil || integ.Intervals.ApiKey == "" || integ.Intervals.AthleteId == "" {
		return nil, fmt.Errorf("intervals integration not configured")
	}

	activityID, err := strconv.ParseInt(sourceActivityID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid activity id %q: %w", sourceActivityID, err)
	}

	client := intervalsclient.NewClient(integ.Intervals.ApiKey, integ.Intervals.AthleteId)
	fitData, err := client.DownloadFITFile(ctx, activityID)
	if err != nil {
		return nil, fmt.Errorf("failed to download FIT file for activity %d: %w", activityID, err)
	}

	stdActivity, err := fit_parser.ParseFitFile(fitData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse FIT file for activity %d: %w", activityID, err)
	}

	stdActivity.Source = activitypb.ActivitySource_SOURCE_INTERVALS
	stdActivity.UserId = userID
	stdActivity.ExternalId = sourceActivityID

	return &pbevents.ActivityPayload{
		Source:               activitypb.ActivitySource_SOURCE_INTERVALS,
		UserId:               userID,
		ActivityId:           &sourceActivityID,
		StandardizedActivity: stdActivity,
	}, nil
}
