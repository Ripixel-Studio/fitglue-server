package trainingpeaks

import (
	"context"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/timestamppb"
)

func TestMapToTrainingPeaksType(t *testing.T) {
	tests := []struct {
		in   pbactivity.ActivityType
		want string
	}{
		{pbactivity.ActivityType_ACTIVITY_TYPE_RUN, "Run"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN, "Run"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN, "Run"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_RIDE, "Bike"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE, "Bike"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE, "Bike"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE, "Bike"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE, "Bike"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE, "Bike"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_SWIM, "Swim"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING, "Strength"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_CROSSFIT, "Strength"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_HIGH_INTENSITY_INTERVAL_TRAINING, "Strength"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_YOGA, "Other"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED, "Other"},
		{pbactivity.ActivityType_ACTIVITY_TYPE_WALK, "Other"},
	}
	for _, tc := range tests {
		t.Run(tc.in.String(), func(t *testing.T) {
			assert.Equal(t, tc.want, mapToTrainingPeaksType(tc.in))
		})
	}
}

func TestBuildTrainingPeaksWorkout_Metadata(t *testing.T) {
	payload := &pbevents.ActivityPayload{
		Timestamp: timestamppb.New(time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)),
		Metadata: map[string]string{
			"activity_name": "Tempo Run",
			"description":   "Negative splits",
			"activity_type": "ACTIVITY_TYPE_RUN",
		},
	}
	w := buildTrainingPeaksWorkout(payload)
	assert.Equal(t, "Tempo Run", w.Title)
	assert.Equal(t, "Negative splits", w.Description)
	assert.Equal(t, "Run", w.WorkoutType)
	assert.Equal(t, "2026-02-03T04:05:06Z", w.StartDate)
	// no sessions -> no totals or HR
	assert.Zero(t, w.TotalTimePlanned)
	assert.Zero(t, w.DistancePlanned)
	assert.Nil(t, w.HeartRateAvg)
	assert.Nil(t, w.HeartRateMax)
}

func TestBuildTrainingPeaksWorkout_InvalidActivityType(t *testing.T) {
	w := buildTrainingPeaksWorkout(&pbevents.ActivityPayload{
		Metadata: map[string]string{"activity_type": "GARBAGE"},
	})
	assert.Equal(t, "Other", w.WorkoutType) // falls back to UNSPECIFIED -> Other
	assert.Empty(t, w.StartDate)            // no timestamp
}

func TestBuildTrainingPeaksWorkout_Sessions(t *testing.T) {
	payload := &pbevents.ActivityPayload{
		Metadata: map[string]string{"activity_type": "ACTIVITY_TYPE_RIDE"},
		StandardizedActivity: &pbactivity.StandardizedActivity{
			Sessions: []*pbactivity.Session{
				{
					TotalElapsedTime: 1800,
					TotalDistance:    10000,
					Laps: []*pbactivity.Lap{
						{Records: []*pbactivity.Record{
							{HeartRate: 120},
							{HeartRate: 160},
							{HeartRate: 0}, // ignored
						}},
					},
				},
				{
					TotalElapsedTime: 1200,
					TotalDistance:    5000,
				},
			},
		},
	}
	w := buildTrainingPeaksWorkout(payload)
	assert.Equal(t, "Bike", w.WorkoutType)
	assert.Equal(t, 3000.0, w.TotalTimePlanned) // 1800 + 1200
	assert.Equal(t, 15000.0, w.DistancePlanned) // 10000 + 5000
	require.NotNil(t, w.HeartRateAvg)
	assert.Equal(t, 140, *w.HeartRateAvg) // (120+160)/2
	require.NotNil(t, w.HeartRateMax)
	assert.Equal(t, 160, *w.HeartRateMax)
}

func TestTrainingPeaksUploader_Create_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	ctx := context.Background()

	_, err := u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, &user.Record{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no TrainingPeaks integration")

	rec := &user.Record{Integrations: &pbuser.UserIntegrations{Trainingpeaks: &pbuser.TrainingPeaksIntegration{Enabled: false}}}
	_, err = u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, rec)
	require.Error(t, err)
}

func TestTrainingPeaksUploader_Update_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	err := u.Update(context.Background(), &pbevents.ActivityPayload{UserId: "u1"}, &user.Record{}, &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no TrainingPeaks integration")
}

func TestTrainingPeaksUploader_Update_NoDestinationInRun(t *testing.T) {
	u := New(&bootstrap.Service{})
	rec := &user.Record{Integrations: &pbuser.UserIntegrations{Trainingpeaks: &pbuser.TrainingPeaksIntegration{Enabled: true, AthleteId: "a1"}}}
	err := u.Update(context.Background(), &pbevents.ActivityPayload{UserId: "u1", Metadata: map[string]string{}}, rec, &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no TrainingPeaks destination")
}
