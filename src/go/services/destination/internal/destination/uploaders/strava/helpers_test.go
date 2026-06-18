package strava

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStravaUploader_Create_MissingFitFileURI(t *testing.T) {
	u := New(&bootstrap.Service{})
	// No fit_file_uri in metadata -> early error before any network call.
	_, err := u.Create(context.Background(), &pbevents.ActivityPayload{
		UserId:   "u1",
		Metadata: map[string]string{},
	}, nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing fit_file_uri")
}

func TestStravaUploader_Update_NoDestinationFound(t *testing.T) {
	u := New(&bootstrap.Service{})
	// Pipeline run has no Strava destination and not same-source -> activity_not_found.
	err := u.Update(context.Background(), &pbevents.ActivityPayload{
		UserId:   "u1",
		Metadata: map[string]string{},
	}, nil, &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "activity_not_found")
}

func TestStravaUploader_Update_SameSourceWithoutExternalIDStillNotFound(t *testing.T) {
	u := New(&bootstrap.Service{})
	// same_source flag set but no external id on the standardized activity -> still not found.
	err := u.Update(context.Background(), &pbevents.ActivityPayload{
		UserId:   "u1",
		Metadata: map[string]string{"same_source_destination_strava": "true"},
	}, nil, &pbpipeline.PipelineRun{})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "activity_not_found")
}

func TestStravaUploader_Update_PicksUpDestinationFromRun(t *testing.T) {
	// When the pipeline run carries a non-Strava destination only, the Strava
	// destination is not matched and the call still reports not found.
	u := New(&bootstrap.Service{})
	other := "123"
	run := &pbpipeline.PipelineRun{Destinations: []*pbpipeline.DestinationOutcome{
		{Destination: pbplugin.DestinationType_DESTINATION_INTERVALS, ExternalId: &other},
	}}
	err := u.Update(context.Background(), &pbevents.ActivityPayload{
		UserId:   "u1",
		Metadata: map[string]string{},
	}, nil, run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "activity_not_found")
}
