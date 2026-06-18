package destination

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/stretchr/testify/assert"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/encoding/protojson"
)

func strptr(s string) *string { return &s }

func TestDestinationCreateClaimKey(t *testing.T) {
	cases := []struct {
		name    string
		dest    pbplugin.DestinationType
		payload *pbevents.ActivityPayload
		want    string
	}{
		{
			name:    "uses external id",
			dest:    pbplugin.DestinationType_DESTINATION_STRAVA,
			payload: &pbevents.ActivityPayload{StandardizedActivity: &pbactivity.StandardizedActivity{ExternalId: "ext-1"}},
			want:    "strava:ext-1",
		},
		{
			name:    "falls back to activity id",
			dest:    pbplugin.DestinationType_DESTINATION_HEVY,
			payload: &pbevents.ActivityPayload{ActivityId: strptr("act-9")},
			want:    "hevy:act-9",
		},
		{
			name:    "empty when no identifiers",
			dest:    pbplugin.DestinationType_DESTINATION_STRAVA,
			payload: &pbevents.ActivityPayload{},
			want:    "",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := destinationCreateClaimKey(c.dest, c.payload); got != c.want {
				t.Errorf("destinationCreateClaimKey = %q, want %q", got, c.want)
			}
		})
	}
}

func TestExistingExternalID(t *testing.T) {
	dest := pbplugin.DestinationType_DESTINATION_STRAVA

	t.Run("from pipeline run destination outcome", func(t *testing.T) {
		pr := &pbpipeline.PipelineRun{
			Destinations: []*pbpipeline.DestinationOutcome{
				{Destination: dest, ExternalId: strptr("strava-123")},
			},
		}
		got := existingExternalID(dest, pr, nil, &pbevents.ActivityPayload{})
		if got != "strava-123" {
			t.Errorf("expected strava-123, got %q", got)
		}
	})

	t.Run("same source flow uses standardized external id", func(t *testing.T) {
		meta := map[string]string{"same_source_destination_strava": "true"}
		payload := &pbevents.ActivityPayload{StandardizedActivity: &pbactivity.StandardizedActivity{ExternalId: "src-ext"}}
		got := existingExternalID(dest, nil, meta, payload)
		if got != "src-ext" {
			t.Errorf("expected src-ext, got %q", got)
		}
	})

	t.Run("none found", func(t *testing.T) {
		got := existingExternalID(dest, nil, map[string]string{}, &pbevents.ActivityPayload{})
		if got != "" {
			t.Errorf("expected empty, got %q", got)
		}
	})

	t.Run("pipeline run outcome for other destination ignored", func(t *testing.T) {
		pr := &pbpipeline.PipelineRun{
			Destinations: []*pbpipeline.DestinationOutcome{
				{Destination: pbplugin.DestinationType_DESTINATION_HEVY, ExternalId: strptr("hevy-1")},
			},
		}
		got := existingExternalID(dest, pr, nil, &pbevents.ActivityPayload{})
		if got != "" {
			t.Errorf("expected empty (other dest), got %q", got)
		}
	})
}

// newTestExecutor builds an executor with permissive mock clients.
func newTestExecutor(reg *Registry, db *mocks.MockDatabase) *UploadExecutor {
	userClient := &mockUserServiceClient{
		GetProfileFunc: func(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return &pbuser.UserProfile{UserId: in.UserId}, nil
		},
	}
	if db == nil {
		db = &mocks.MockDatabase{}
	}
	return NewUploadExecutor(reg, userClient, &mockActivityServiceClient{}, db, nil, &mocks.MockPublisher{}, infra.NewLogger())
}

func makeCE(t *testing.T, typ string, payload *pbevents.EnrichedActivityEvent) *event.Event {
	t.Helper()
	b, err := protojson.Marshal(payload)
	assert.NoError(t, err)
	ce := event.New()
	ce.SetID("id")
	ce.SetType(typ)
	ce.SetSource("test")
	ce.SetData("application/json", b)
	return &ce
}

func TestProcess_NoDestinations_NoOp(t *testing.T) {
	exec := newTestExecutor(NewRegistry(), nil)
	ce := makeCE(t, "com.fitglue.event.enriched", &pbevents.EnrichedActivityEvent{
		UserId:       "u1",
		ActivityId:   "a1",
		Destinations: nil,
	})
	// No destinations -> returns nil before any user lookup.
	assert.NoError(t, exec.Process(context.Background(), ce))
}

func TestProcess_BadPayload_AcksWithNil(t *testing.T) {
	exec := newTestExecutor(NewRegistry(), nil)
	ce := event.New()
	ce.SetID("id")
	ce.SetType("com.fitglue.event.enriched")
	ce.SetSource("test")
	ce.SetData("application/json", []byte("{not valid protojson"))
	// Unmarshal failure is swallowed (returns nil so Pub/Sub acks).
	assert.NoError(t, exec.Process(context.Background(), &ce))
}

func TestProcess_TargetDestFiltersOthers(t *testing.T) {
	reg := NewRegistry()
	strava := &countingUploader{name: "strava", id: "s1"}
	hevy := &countingUploader{name: "hevy", id: "h1"}
	reg.Register(pbplugin.DestinationType_DESTINATION_STRAVA, strava)
	reg.Register(pbplugin.DestinationType_DESTINATION_HEVY, hevy)
	exec := newTestExecutor(reg, nil)

	// Event type targets only HEVY; STRAVA should be skipped.
	ce := makeCE(t, "com.fitglue.job.DESTINATION_HEVY", &pbevents.EnrichedActivityEvent{
		UserId:     "u1",
		ActivityId: "a1",
		Destinations: []pbplugin.DestinationType{
			pbplugin.DestinationType_DESTINATION_STRAVA,
			pbplugin.DestinationType_DESTINATION_HEVY,
		},
		ActivityData: &pbactivity.StandardizedActivity{ExternalId: "ext-1"},
	})
	assert.NoError(t, exec.Process(context.Background(), ce))

	if strava.createCalls != 0 {
		t.Errorf("strava should be skipped (target=hevy), got %d creates", strava.createCalls)
	}
	if hevy.createCalls != 1 {
		t.Errorf("hevy should be created once, got %d", hevy.createCalls)
	}
}

func TestProcess_UnregisteredDestination_Skipped(t *testing.T) {
	// No uploaders registered: the destination is unregistered and skipped without error.
	exec := newTestExecutor(NewRegistry(), nil)
	runID := "run-1"
	ce := makeCE(t, "com.fitglue.event.enriched", &pbevents.EnrichedActivityEvent{
		UserId:              "u1",
		ActivityId:          "a1",
		PipelineExecutionId: &runID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
		ActivityData:        &pbactivity.StandardizedActivity{ExternalId: "ext-1"},
	})
	assert.NoError(t, exec.Process(context.Background(), ce))
}

func TestProcess_UnspecifiedDestinationSkipped(t *testing.T) {
	reg := NewRegistry()
	strava := &countingUploader{name: "strava", id: "s1"}
	reg.Register(pbplugin.DestinationType_DESTINATION_STRAVA, strava)
	exec := newTestExecutor(reg, nil)

	ce := makeCE(t, "com.fitglue.event.enriched", &pbevents.EnrichedActivityEvent{
		UserId:     "u1",
		ActivityId: "a1",
		Destinations: []pbplugin.DestinationType{
			pbplugin.DestinationType_DESTINATION_UNSPECIFIED,
			pbplugin.DestinationType_DESTINATION_STRAVA,
		},
		ActivityData: &pbactivity.StandardizedActivity{ExternalId: "ext-1"},
	})
	assert.NoError(t, exec.Process(context.Background(), ce))
	if strava.createCalls != 1 {
		t.Errorf("expected strava created once (unspecified skipped), got %d", strava.createCalls)
	}
}

func TestProcess_ListIntegrationsError_ReturnsError(t *testing.T) {
	reg := NewRegistry()
	reg.Register(pbplugin.DestinationType_DESTINATION_STRAVA, &mockUploader{name: "strava", id: "s1"})

	userClient := &mockUserServiceClient{
		GetProfileFunc: func(ctx context.Context, in *userpb.GetProfileRequest, opts ...grpc.CallOption) (*pbuser.UserProfile, error) {
			return &pbuser.UserProfile{UserId: in.UserId}, nil
		},
		ListIntegrationsFunc: func(ctx context.Context, in *userpb.ListIntegrationsRequest, opts ...grpc.CallOption) (*pbuser.UserIntegrations, error) {
			return nil, fmt.Errorf("integrations unavailable")
		},
	}
	exec := NewUploadExecutor(reg, userClient, &mockActivityServiceClient{}, &mocks.MockDatabase{}, nil, &mocks.MockPublisher{}, infra.NewLogger())

	runID := "run-1"
	ce := makeCE(t, "com.fitglue.event.enriched", &pbevents.EnrichedActivityEvent{
		UserId:              "u1",
		ActivityId:          "a1",
		PipelineExecutionId: &runID,
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
		ActivityData:        &pbactivity.StandardizedActivity{ExternalId: "ext-1"},
	})
	err := exec.Process(context.Background(), ce)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "getting user integrations")
}

func TestProcess_UploaderError_ContinuesWithoutFatal(t *testing.T) {
	reg := NewRegistry()
	reg.Register(pbplugin.DestinationType_DESTINATION_STRAVA, &mockUploader{name: "strava", err: fmt.Errorf("upstream 500")})
	exec := newTestExecutor(reg, nil)

	ce := makeCE(t, "com.fitglue.event.enriched", &pbevents.EnrichedActivityEvent{
		UserId:       "u1",
		ActivityId:   "a1",
		Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
		ActivityData: &pbactivity.StandardizedActivity{ExternalId: "ext-1"},
	})
	// Per-destination errors are logged and skipped; Process returns nil overall.
	assert.NoError(t, exec.Process(context.Background(), ce))
}

func TestHandlePubSubPush_BadEnvelope(t *testing.T) {
	exec := newTestExecutor(NewRegistry(), nil)
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader("not json"))
	rec := httptest.NewRecorder()
	exec.HandlePubSubPush(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad envelope, got %d", rec.Code)
	}
}

func TestHandlePubSubPush_BadInnerCloudEvent(t *testing.T) {
	exec := newTestExecutor(NewRegistry(), nil)
	// Valid envelope, but Data is not a valid CloudEvent JSON.
	body := `{"message":{"data":"bm90LWEtY2xvdWQtZXZlbnQ=","messageId":"m1"},"subscription":"s"}`
	req := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(body))
	rec := httptest.NewRecorder()
	exec.HandlePubSubPush(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for bad inner CloudEvent, got %d", rec.Code)
	}
}
