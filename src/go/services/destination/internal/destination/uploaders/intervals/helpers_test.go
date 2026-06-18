package intervals

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enabledRec(apiKey, athleteID string) *user.Record {
	return &user.Record{Integrations: &pbuser.UserIntegrations{
		Intervals: &pbuser.IntervalsIntegration{Enabled: true, ApiKey: apiKey, AthleteId: athleteID},
	}}
}

func TestIntervalsUploader_Create_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	ctx := context.Background()

	_, err := u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, &user.Record{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Intervals integration")

	rec := &user.Record{Integrations: &pbuser.UserIntegrations{Intervals: &pbuser.IntervalsIntegration{Enabled: false}}}
	_, err = u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, rec)
	require.Error(t, err)
}

func TestIntervalsUploader_Create_IncompleteCredentials(t *testing.T) {
	u := New(&bootstrap.Service{})
	ctx := context.Background()

	// Missing athlete ID
	_, err := u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, enabledRec("key", ""))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials incomplete")

	// Missing API key
	_, err = u.Create(ctx, &pbevents.ActivityPayload{UserId: "u1"}, enabledRec("", "athlete"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials incomplete")
}

func TestIntervalsUploader_Create_MissingFitFileURI(t *testing.T) {
	u := New(&bootstrap.Service{})
	rec := enabledRec("key", "athlete")
	_, err := u.Create(context.Background(), &pbevents.ActivityPayload{UserId: "u1", Metadata: map[string]string{}}, rec)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing fit_file_uri")
}

func TestIntervalsUploader_Update_NoIntegration(t *testing.T) {
	u := New(&bootstrap.Service{})
	err := u.Update(context.Background(), &pbevents.ActivityPayload{UserId: "u1"}, &user.Record{}, &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Intervals integration")
}

func TestIntervalsUploader_Update_IncompleteCredentials(t *testing.T) {
	u := New(&bootstrap.Service{})
	err := u.Update(context.Background(), &pbevents.ActivityPayload{UserId: "u1"}, enabledRec("", ""), &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "credentials incomplete")
}

func TestIntervalsUploader_Update_NoDestinationInRun(t *testing.T) {
	u := New(&bootstrap.Service{})
	rec := enabledRec("key", "athlete")
	// pipeline run with a non-Intervals destination only
	otherID := "x"
	run := &pbpipeline.PipelineRun{Destinations: []*pbpipeline.DestinationOutcome{
		{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, ExternalId: &otherID},
	}}
	err := u.Update(context.Background(), &pbevents.ActivityPayload{UserId: "u1", Metadata: map[string]string{}}, rec, run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no Intervals destination")
}
