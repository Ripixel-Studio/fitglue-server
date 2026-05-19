// nolint:proto-json
package sourceplugins

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	fitbitapi "github.com/fitglue/server/src/go/pkg/api/fitbit"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

func init() {
	register("SOURCE_FITBIT", &fitbitProvider{})
}

type fitbitProvider struct{}

func (p *fitbitProvider) newClient(accessToken string) (*fitbitapi.ClientWithResponses, error) {
	return fitbitapi.NewClientWithResponses(
		"https://api.fitbit.com",
		fitbitapi.WithRequestEditorFn(func(_ context.Context, req *http.Request) error {
			req.Header.Set("Authorization", "Bearer "+accessToken)
			return nil
		}),
	)
}

type fitbitActivityLog struct {
	LogID        int64  `json:"logId"`
	ActivityName string `json:"activityName"`
	StartDate    string `json:"startDate"`
	StartTime    string `json:"startTime"`
}

func (p *fitbitProvider) ListActivities(ctx context.Context, integ *userpb.UserIntegrations, pageToken string) ([]*SourceActivity, string, error) {
	if integ.Fitbit == nil || integ.Fitbit.AccessToken == "" {
		return nil, "", fmt.Errorf("fitbit integration not configured")
	}

	client, err := p.newClient(integ.Fitbit.AccessToken)
	if err != nil {
		return nil, "", fmt.Errorf("failed to create fitbit client: %w", err)
	}

	offset := 0
	if pageToken != "" {
		if n, err := strconv.Atoi(pageToken); err == nil && n >= 0 {
			offset = n
		}
	}
	limit := 100
	sort := "desc"

	resp, err := client.GetActivitiesLogListWithResponse(ctx, &fitbitapi.GetActivitiesLogListParams{
		Sort:   sort,
		Offset: offset,
		Limit:  limit,
	})
	if err != nil {
		return nil, "", fmt.Errorf("fitbit api error: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, "", fmt.Errorf("fitbit api error: status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}

	var result struct {
		Activities []fitbitActivityLog `json:"activities"`
		Pagination struct {
			Offset int    `json:"offset"`
			Limit  int    `json:"limit"`
			Total  int    `json:"total"`
			Next   string `json:"next"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(resp.Body, &result); err != nil {
		return nil, "", fmt.Errorf("failed to decode fitbit response: %w", err)
	}

	var activities []*SourceActivity
	for _, a := range result.Activities {
		sa := &SourceActivity{
			ID:    strconv.FormatInt(a.LogID, 10),
			Title: a.ActivityName,
			Type:  a.ActivityName,
		}
		if t, err := time.Parse("2006-01-02 3:04PM", a.StartDate+" "+a.StartTime); err == nil {
			sa.StartTime = t
		}
		activities = append(activities, sa)
	}

	var nextPageToken string
	if result.Pagination.Next != "" && len(result.Activities) == result.Pagination.Limit {
		nextPageToken = strconv.Itoa(offset + result.Pagination.Limit)
	}

	return activities, nextPageToken, nil
}

func (p *fitbitProvider) FetchActivity(ctx context.Context, integ *userpb.UserIntegrations, userID string, sourceActivityID string) (*pbevents.ActivityPayload, error) {
	if integ.Fitbit == nil || integ.Fitbit.AccessToken == "" {
		return nil, fmt.Errorf("fitbit integration not configured")
	}

	client, err := p.newClient(integ.Fitbit.AccessToken)
	if err != nil {
		return nil, fmt.Errorf("failed to create fitbit client: %w", err)
	}

	includePartial := true
	resp, err := client.GetActivitiesTCXWithResponse(ctx, sourceActivityID, &fitbitapi.GetActivitiesTCXParams{
		IncludePartialTCX: &includePartial,
	})
	if err != nil {
		return nil, fmt.Errorf("fitbit api error: %w", err)
	}
	if resp.StatusCode() != http.StatusOK {
		return nil, fmt.Errorf("fitbit api error: status=%d body=%s", resp.StatusCode(), string(resp.Body))
	}

	return &pbevents.ActivityPayload{
		Source:              activitypb.ActivitySource_SOURCE_FITBIT,
		UserId:              userID,
		OriginalPayloadJson: string(resp.Body),
		ActivityId:          &sourceActivityID,
	}, nil
}
