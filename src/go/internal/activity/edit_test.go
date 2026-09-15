package activity

import (
	"context"
	"sync"
	"testing"

	cloudevents "github.com/cloudevents/sdk-go/v2"
	"github.com/fitglue/server/src/go/internal/infra"
	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

const (
	testRecordURI = "gs://test-bucket/activity_records/u1/a1.json"
)

// capturingPublisher records the last CloudEvent published, for re-send assertions.
type capturingPublisher struct {
	cloudEventsPublisher
	mu    sync.Mutex
	topic string
	event cloudevents.Event
	calls int
}

func (p *capturingPublisher) PublishCloudEvent(_ context.Context, topic string, e cloudevents.Event) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.topic = topic
	p.event = e
	p.calls++
	return "test-id", nil
}

// newRecordBlobStore returns a MockBlobStore backed by an in-memory object map seeded with rec
// (marshalled as protojson at the record's object path). Writes update the same map, so a
// round-trip through UpdateActivity is observable.
func newRecordBlobStore(t *testing.T, rec *pbactivity.ActivityRecord) (*MockBlobStore, map[string][]byte) {
	t.Helper()
	objects := map[string][]byte{}
	if rec != nil {
		data, err := protojson.Marshal(rec)
		if err != nil {
			t.Fatalf("marshal record: %v", err)
		}
		objects["activity_records/u1/a1.json"] = data
	}
	return &MockBlobStore{
		GetFunc: func(_ context.Context, _, object string) ([]byte, error) {
			if d, ok := objects[object]; ok {
				return d, nil
			}
			return nil, nil
		},
		WriteFunc: func(_ context.Context, _, object string, data []byte) error {
			objects[object] = data
			return nil
		},
	}, objects
}

func testRecord() *pbactivity.ActivityRecord {
	return &pbactivity.ActivityRecord{
		ActivityId: "a1",
		UserId:     "u1",
		Source:     pbactivity.ActivitySource_SOURCE_STRAVA,
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{Name: "Source Name"},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name:        "Derived Name",
			Description: "Derived description",
			Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{},
	}
}

func newEditTestSvc(store ActivityStore, blob BlobStore, pub Publisher) *Service {
	return NewService(store, blob, pub, "test-bucket", "test-showcase-bucket", infra.NewLogger())
}

func storeWithRecordRun(dests ...pbplugin.DestinationType) *MockActivityStore {
	var outcomes []*pbpipeline.DestinationOutcome
	for _, d := range dests {
		outcomes = append(outcomes, &pbpipeline.DestinationOutcome{
			Destination: d,
			Status:      pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS,
		})
	}
	return &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{
				Id:                "run-1",
				PipelineId:        "pl-1",
				ActivityRecordUri: testRecordURI,
				Destinations:      outcomes,
			}, nil
		},
	}
}

func TestUpdateActivity_WritesOverlayAndReturnsResolved(t *testing.T) {
	blob, objects := newRecordBlobStore(t, testRecord())
	svc := newEditTestSvc(storeWithRecordRun(pbplugin.DestinationType_DESTINATION_STRAVA), blob, &cloudEventsPublisher{})

	resp, err := svc.UpdateActivity(context.Background(), &pbsvc.UpdateActivityRequest{
		UserId:       "u1",
		ActivityId:   "a1",
		Overlay:      &pbactivity.ActivityUserEditOverlay{Name: proto.String("Edited Title")},
		UpdateFields: []string{"name"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Response is the resolved activity with the overlay applied + provenance.
	if resp.GetActivity().GetName() != "Edited Title" {
		t.Errorf("resolved name = %q, want Edited Title", resp.GetActivity().GetName())
	}
	// The unedited description must still resolve from the derived layer.
	if resp.GetActivity().GetDescription() != "Derived description" {
		t.Errorf("resolved description = %q, want derived", resp.GetActivity().GetDescription())
	}
	var nameProv *pbactivity.FieldProvenance
	for _, p := range resp.GetProvenance() {
		if p.GetField() == activitydomain.FieldName {
			nameProv = p
		}
	}
	if nameProv == nil || nameProv.GetLayer() != pbactivity.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY {
		t.Errorf("name provenance = %v, want USER_OVERLAY", nameProv)
	}

	// The overlay was persisted back to the same object.
	var written pbactivity.ActivityRecord
	if err := protojson.Unmarshal(objects["activity_records/u1/a1.json"], &written); err != nil {
		t.Fatalf("unmarshal written record: %v", err)
	}
	if written.GetUserEditOverlay().GetName() != "Edited Title" {
		t.Errorf("persisted overlay name = %q, want Edited Title", written.GetUserEditOverlay().GetName())
	}
	if written.GetUserEditOverlay().GetEditedAt() == nil {
		t.Error("edited_at not stamped on persisted overlay")
	}
}

func TestUpdateActivity_Validation(t *testing.T) {
	svc := newEditTestSvc(storeWithRecordRun(), &MockBlobStore{}, &cloudEventsPublisher{})
	ctx := context.Background()

	cases := []struct {
		name string
		req  *pbsvc.UpdateActivityRequest
		code codes.Code
	}{
		{"missing user", &pbsvc.UpdateActivityRequest{ActivityId: "a1", UpdateFields: []string{"name"}}, codes.InvalidArgument},
		{"missing activity", &pbsvc.UpdateActivityRequest{UserId: "u1", UpdateFields: []string{"name"}}, codes.InvalidArgument},
		{"empty mask", &pbsvc.UpdateActivityRequest{UserId: "u1", ActivityId: "a1"}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.UpdateActivity(ctx, tc.req); status.Code(err) != tc.code {
				t.Errorf("got %v, want %v", err, tc.code)
			}
		})
	}
}

func TestUpdateActivity_UnknownFieldRejected(t *testing.T) {
	blob, _ := newRecordBlobStore(t, testRecord())
	svc := newEditTestSvc(storeWithRecordRun(), blob, &cloudEventsPublisher{})
	_, err := svc.UpdateActivity(context.Background(), &pbsvc.UpdateActivityRequest{
		UserId: "u1", ActivityId: "a1", UpdateFields: []string{"time_markers"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("got %v, want InvalidArgument", err)
	}
}

func TestUpdateActivity_NoRecordURI_FailedPrecondition(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return &pbpipeline.PipelineRun{Id: "run-1"}, nil // no ActivityRecordUri
		},
	}
	svc := newEditTestSvc(store, &MockBlobStore{}, &cloudEventsPublisher{})
	_, err := svc.UpdateActivity(context.Background(), &pbsvc.UpdateActivityRequest{
		UserId: "u1", ActivityId: "a1", UpdateFields: []string{"name"},
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("got %v, want FailedPrecondition", err)
	}
}

func TestUpdateActivity_NotFound(t *testing.T) {
	store := &MockActivityStore{
		GetPipelineRunFunc: func(_ context.Context, _, _ string) (*pbpipeline.PipelineRun, error) {
			return nil, nil
		},
	}
	svc := newEditTestSvc(store, &MockBlobStore{}, &cloudEventsPublisher{})
	_, err := svc.UpdateActivity(context.Background(), &pbsvc.UpdateActivityRequest{
		UserId: "u1", ActivityId: "a1", UpdateFields: []string{"name"},
	})
	if status.Code(err) != codes.NotFound {
		t.Errorf("got %v, want NotFound", err)
	}
}

func TestReSendActivity_PublishesResolvedEventToAllDestinations(t *testing.T) {
	rec := testRecord()
	rec.UserEditOverlay = &pbactivity.ActivityUserEditOverlay{Name: proto.String("Edited Title")}
	blob, _ := newRecordBlobStore(t, rec)
	pub := &capturingPublisher{}
	svc := newEditTestSvc(
		storeWithRecordRun(pbplugin.DestinationType_DESTINATION_STRAVA, pbplugin.DestinationType_DESTINATION_HEVY),
		blob, pub,
	)

	resp, err := svc.ReSendActivity(context.Background(), &pbsvc.ReSendActivityRequest{UserId: "u1", ActivityId: "a1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetDestinations()) != 2 {
		t.Fatalf("response destinations = %v, want 2", resp.GetDestinations())
	}
	if pub.calls != 1 {
		t.Fatalf("publish calls = %d, want 1", pub.calls)
	}
	if pub.topic != "topic-enriched-activity" {
		t.Errorf("topic = %q, want topic-enriched-activity", pub.topic)
	}

	var ev pbevents.EnrichedActivityEvent
	if err := protojson.Unmarshal(pub.event.Data(), &ev); err != nil {
		t.Fatalf("unmarshal published event: %v", err)
	}
	// The pushed activity carries the resolved (edited) value.
	if ev.GetName() != "Edited Title" {
		t.Errorf("event name = %q, want Edited Title", ev.GetName())
	}
	if ev.GetPipelineExecutionId() != "run-1" {
		t.Errorf("event pipeline_execution_id = %q, want run-1 (reuse original run)", ev.GetPipelineExecutionId())
	}
	if len(ev.GetDestinations()) != 2 {
		t.Errorf("event destinations = %v, want 2", ev.GetDestinations())
	}
	if ev.GetEnrichmentMetadata()["use_update_method"] != "true" {
		t.Error("use_update_method not set — already-synced destinations would be skipped")
	}
	if ev.GetEnrichmentMetadata()["pipeline_resumed"] != "true" {
		t.Error("pipeline_resumed not set — re-send would not refresh succeeded destinations")
	}
	// Strava is both source and a destination here, so same-source overwrite must be flagged.
	if ev.GetEnrichmentMetadata()["same_source_destination_strava"] != "true" {
		t.Error("same_source_destination_strava not set")
	}
}

func TestReSendActivity_DestinationOverrideIntersectsSent(t *testing.T) {
	blob, _ := newRecordBlobStore(t, testRecord())
	pub := &capturingPublisher{}
	svc := newEditTestSvc(
		storeWithRecordRun(pbplugin.DestinationType_DESTINATION_STRAVA, pbplugin.DestinationType_DESTINATION_HEVY),
		blob, pub,
	)

	// Override asks for HEVY (sent) + FITBIT (never sent) — only HEVY should survive.
	resp, err := svc.ReSendActivity(context.Background(), &pbsvc.ReSendActivityRequest{
		UserId:       "u1",
		ActivityId:   "a1",
		Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_HEVY, pbplugin.DestinationType_DESTINATION_INTERVALS},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resp.GetDestinations()) != 1 || resp.GetDestinations()[0] != pbplugin.DestinationType_DESTINATION_HEVY {
		t.Errorf("destinations = %v, want [HEVY]", resp.GetDestinations())
	}
}

func TestReSendActivity_NoDestinations_FailedPrecondition(t *testing.T) {
	blob, _ := newRecordBlobStore(t, testRecord())
	svc := newEditTestSvc(storeWithRecordRun(), blob, &capturingPublisher{}) // run has no destinations
	_, err := svc.ReSendActivity(context.Background(), &pbsvc.ReSendActivityRequest{UserId: "u1", ActivityId: "a1"})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("got %v, want FailedPrecondition", err)
	}
}

func TestReSendActivity_OverrideMissesAllSent_FailedPrecondition(t *testing.T) {
	blob, _ := newRecordBlobStore(t, testRecord())
	svc := newEditTestSvc(storeWithRecordRun(pbplugin.DestinationType_DESTINATION_STRAVA), blob, &capturingPublisher{})
	_, err := svc.ReSendActivity(context.Background(), &pbsvc.ReSendActivityRequest{
		UserId:       "u1",
		ActivityId:   "a1",
		Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_INTERVALS}, // never sent
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("got %v, want FailedPrecondition", err)
	}
}
