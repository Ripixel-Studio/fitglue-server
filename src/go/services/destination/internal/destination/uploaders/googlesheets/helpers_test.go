package googlesheets

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestGetHeaderRow(t *testing.T) {
	u := New(&bootstrap.Service{})
	row := u.getHeaderRow()
	require.Len(t, row, 13)
	assert.Equal(t, "Synced At", row[0])
	assert.Equal(t, "Date", row[1])
	assert.Equal(t, "Source", row[2])
	assert.Equal(t, "Description", row[12])
}

func TestBuildSheetRow_FromMetadata(t *testing.T) {
	u := New(&bootstrap.Service{})
	payload := &pbevents.ActivityPayload{
		Source:    pbactivity.ActivitySource_SOURCE_STRAVA,
		Timestamp: timestamppb.New(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)),
		Metadata: map[string]string{
			"activity_type":         "ACTIVITY_TYPE_TRAIL_RUN",
			"activity_name":         "Hill Repeats",
			"calories":              "450",
			"hr_avg":                "150",
			"hr_max":                "180",
			"elevation_gain":        "320",
			"description":           "Tough one",
			"asset_muscle_heatmap":  "http://img/heatmap.png",
			"asset_route_thumbnail": "http://img/route.png",
		},
	}
	row := u.buildSheetRow(payload, true)
	require.Len(t, row, 13)

	assert.Equal(t, "2026-05-01", row[1])
	// Source: SOURCE_ prefix stripped, underscores -> spaces
	assert.Equal(t, "STRAVA", row[2])
	// Activity type: ACTIVITY_TYPE_ prefix stripped, underscores -> spaces
	assert.Equal(t, "TRAIL RUN", row[3])
	assert.Equal(t, "Hill Repeats", row[4])
	assert.Equal(t, "", row[5]) // no sessions -> empty duration
	assert.Equal(t, "", row[6]) // no sessions -> empty distance
	assert.Equal(t, "450", row[7])
	assert.Equal(t, "150", row[8])
	assert.Equal(t, "180", row[9])
	assert.Equal(t, "320", row[10])
	// images joined with newline
	assert.Equal(t, "http://img/heatmap.png\nhttp://img/route.png", row[11])
	assert.Equal(t, "Tough one", row[12])
}

func TestBuildSheetRow_VisualsExcluded(t *testing.T) {
	u := New(&bootstrap.Service{})
	payload := &pbevents.ActivityPayload{
		Metadata: map[string]string{
			"asset_muscle_heatmap":  "http://img/heatmap.png",
			"asset_route_thumbnail": "http://img/route.png",
		},
	}
	row := u.buildSheetRow(payload, false)
	assert.Equal(t, "", row[11]) // visuals suppressed
}

func TestBuildSheetRow_UnspecifiedType(t *testing.T) {
	u := New(&bootstrap.Service{})
	row := u.buildSheetRow(&pbevents.ActivityPayload{Metadata: map[string]string{}}, true)
	assert.Equal(t, "UNSPECIFIED", row[3])
	assert.Equal(t, "", row[1]) // no timestamp
}

func TestBuildSheetRow_FromSession(t *testing.T) {
	u := New(&bootstrap.Service{})
	cal := 600.0
	payload := &pbevents.ActivityPayload{
		Metadata: map[string]string{},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{
					TotalElapsedTime: 3661, // 1h 1m 1s
					TotalDistance:    5000, // 5 km
					TotalCalories:    &cal,
					Laps: []*pbactivity.Lap{
						{Records: []*pbactivity.Record{
							{HeartRate: 100},
							{HeartRate: 200},
							{HeartRate: 0}, // ignored
						}},
					},
				},
			},
		},
	}
	row := u.buildSheetRow(payload, true)
	assert.Equal(t, "01:01:01", row[5]) // duration
	assert.Equal(t, "5.00", row[6])     // distance km
	assert.Equal(t, "600", row[7])      // calories
	assert.Equal(t, "150", row[8])      // avg HR (100+200)/2
	assert.Equal(t, "200", row[9])      // max HR
}

func TestBuildSheetRow_MaxHRFallsBackToSessionMax(t *testing.T) {
	u := New(&bootstrap.Service{})
	mhr := int32(195)
	payload := &pbevents.ActivityPayload{
		Metadata: map[string]string{},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{MaxHeartRate: &mhr}, // no records, falls back to session max
			},
		},
	}
	row := u.buildSheetRow(payload, true)
	assert.Equal(t, "195", row[9])
}

func TestBuildSheetRow_DescriptionTruncated(t *testing.T) {
	u := New(&bootstrap.Service{})
	long := strings.Repeat("x", 6000)
	row := u.buildSheetRow(&pbevents.ActivityPayload{
		Metadata: map[string]string{"description": long},
	}, true)
	desc, ok := row[12].(string)
	require.True(t, ok)
	assert.Equal(t, 5003, len(desc)) // 5000 + "..."
	assert.True(t, strings.HasSuffix(desc, "..."))
}

func TestGoogleSheetsUploader_Create_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	ctx := context.Background()

	_, err := u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, &user.Record{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Google integration")

	rec := &user.Record{Integrations: &pbuser.UserIntegrations{Google: &pbuser.GoogleIntegration{Enabled: false}}}
	_, err = u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, rec)
	require.Error(t, err)
}

func TestGoogleSheetsUploader_Create_MissingSpreadsheetID(t *testing.T) {
	u := New(&bootstrap.Service{})
	rec := &user.Record{Integrations: &pbuser.UserIntegrations{Google: &pbuser.GoogleIntegration{Enabled: true}}}
	_, err := u.Create(context.Background(), &pbevents.ActivityPayload{UserId: "u1", Metadata: map[string]string{}}, rec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "spreadsheet_id not configured")
}
