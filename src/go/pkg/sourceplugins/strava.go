// nolint:proto-json
package sourceplugins

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	stravaapi "github.com/fitglue/server/src/go/pkg/api/strava"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func init() {
	register("SOURCE_STRAVA", &stravaProvider{})
}

type stravaProvider struct{}

func (p *stravaProvider) newClient(accessToken string) (*stravaapi.ClientWithResponses, error) {
	return stravaapi.NewClientWithResponses(
		"https://www.strava.com/api/v3",
		stravaapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			return nil
		}),
	)
}

func (p *stravaProvider) ListActivities(ctx context.Context, integ *userpb.UserIntegrations, pageToken string) ([]*SourceActivity, string, error) {
	if integ.Strava == nil || integ.Strava.AccessToken == "" {
		return nil, "", fmt.Errorf("strava integration not configured")
	}

	client, err := p.newClient(integ.Strava.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create strava client: %w", err)
	}

	page := stravaapi.Page(1)
	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil && n > 0 {
			page = stravaapi.Page(n)
		}
	}
	perPage := stravaapi.PerPage(50)

	resp, err := client.GetLoggedInAthleteActivitiesWithResponse(ctx, &stravaapi.GetLoggedInAthleteActivitiesParams{
		Page:    &page,
		PerPage: &perPage,
	})
	if err != nil {
		return nil, "", fmt.Errorf("strava api error: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, "", fmt.Errorf("strava api error: status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}
	if resp.JSON200 == nil {
		return nil, "", nil
	}

	var activities []*SourceActivity
	for _, s := range *resp.JSON200 {
		a := &SourceActivity{}
		if s.Id != nil {
			a.ID = strconv.FormatInt(*s.Id, 10)
		}
		if s.Name != nil {
			a.Title = *s.Name
		}
		if s.Type != nil {
			a.Type = string(*s.Type)
		}
		if s.StartDateLocal != nil {
			a.StartTime = *s.StartDateLocal
		}
		activities = append(activities, a)
	}

	var nextPageToken string
	if len(*resp.JSON200) == 50 {
		nextPageToken = strconv.Itoa(int(page) + 1)
	}

	return activities, nextPageToken, nil
}

func (p *stravaProvider) FetchActivity(ctx context.Context, integ *userpb.UserIntegrations, userID string, sourceActivityID string) (*pbevents.ActivityPayload, error) {
	if integ.Strava == nil || integ.Strava.AccessToken == "" {
		return nil, fmt.Errorf("strava integration not configured")
	}

	activityID, err := strconv.ParseInt(sourceActivityID, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid strava activity id %q: %w", sourceActivityID, err)
	}

	client, err := p.newClient(integ.Strava.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create strava client: %w", err)
	}

	resp, err := client.GetActivityByIdWithResponse(ctx, activityID, &stravaapi.GetActivityByIdParams{})
	if err != nil {
		return nil, fmt.Errorf("strava api error: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("strava api error: status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}

	return &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_STRAVA,
		UserId:              userID,
		OriginalPayloadJson: string(resp.Body),
		ActivityId:          &sourceActivityID,
	}, nil
}
