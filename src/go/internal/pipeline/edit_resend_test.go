// nolint:proto-json
package pipeline

import (
	"context"
	"testing"

	shared "github.com/fitglue/server/src/go/pkg"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
)

const (
	testRecordURI   = "gs://test-bucket/activity_records/user1/act1.json"
	testEnrichedURI = "gs://test-bucket/enriched_events/user1/exec1.json"
)

// seedEditableActivity registers a pipeline run plus a layered ActivityRecord (and,
// optionally, a stored enriched event) so the edit / re-send paths have something to act on.
func seedEditableActivity(t *testing.T, store *MockPipelineStore, blob *MockBlobStore, rec *pbactivity.ActivityRecord, event *pbevents.EnrichedActivityEvent) {
	t.Helper()
	run := &pipeline.PipelineRun{
		Id:                "exec1",
		ActivityId:        "act1",
		ActivityRecordUri: testRecordURI,
	}
	if event != nil {
		run.EnrichedEventUri = testEnrichedURI
		eb, err := protojson.Marshal(event)
		if err != nil {
			t.Fatalf("marshal event: %v", err)
		}
		blob.Blobs[testEnrichedURI] = eb
	}
	store.Runs[store.key("user1", "exec1")] = run

	rb, err := protojson.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal record: %v", err)
	}
	blob.Blobs[testRecordURI] = rb
}

func baseRecord() *pbactivity.ActivityRecord {
	return &pbactivity.ActivityRecord{
		ActivityId: "act1",
		UserId:     "user1",
		SourceLayer: &pbactivity.ActivitySourceLayer{
			Parsed: &pbactivity.StandardizedActivity{Name: "Morning Run", Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN},
		},
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name: "Morning Run",
			Type: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			Tags: []string{"auto"},
		},
	}
}

func TestUpdateActivity_Validation(t *testing.T) {
	svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	ctx := context.Background()

	cases := []struct {
		name string
		req  *pbsvc.UpdateActivityRequest
		code codes.Code
	}{
		{"missing ids", &pbsvc.UpdateActivityRequest{UpdateMask: []string{"name"}}, codes.InvalidArgument},
		{"empty mask", &pbsvc.UpdateActivityRequest{UserId: "user1", ActivityId: "act1"}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := svc.UpdateActivity(ctx, tc.req)
			if status.Code(err) != tc.code {
				t.Fatalf("expected %v, got %v", tc.code, err)
			}
		})
	}
}

func TestUpdateActivity_NotFoundAndNoRecord(t *testing.T) {
	ctx := context.Background()

	// No run at all.
	svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	_, err := svc.UpdateActivity(ctx, &pbsvc.UpdateActivityRequest{UserId: "user1", ActivityId: "act1", UpdateMask: []string{"name"}})
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	// Run exists but predates layered storage (no record uri).
	store := NewMockStore()
	store.Runs[store.key("user1", "exec1")] = &pipeline.PipelineRun{Id: "exec1", ActivityId: "act1"}
	svc = NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	_, err = svc.UpdateActivity(ctx, &pbsvc.UpdateActivityRequest{UserId: "user1", ActivityId: "act1", UpdateMask: []string{"name"}})
	if status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition, got %v", err)
	}
}

func TestUpdateActivity_WritesOverlay(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()
	blob := &MockBlobStore{Blobs: map[string][]byte{}}
	seedEditableActivity(t, store, blob, baseRecord(), nil)
	svc := NewService(store, &MockPublisher{}, blob, mockLogger{}, nil)

	resolved, err := svc.UpdateActivity(ctx, &pbsvc.UpdateActivityRequest{
		UserId:     "user1",
		ActivityId: "act1",
		Name:       "Evening Run",
		Tags:       []string{"pb", "sunset"},
		UpdateMask: []string{"name", "tags"},
	})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	// Returned resolved view reflects the edit; type falls through from derived (untouched).
	if resolved.GetName() != "Evening Run" {
		t.Fatalf("expected resolved name 'Evening Run', got %q", resolved.GetName())
	}
	if got := resolved.GetTags(); len(got) != 2 || got[0] != "pb" || got[1] != "sunset" {
		t.Fatalf("expected overlay tags [pb sunset], got %v", got)
	}
	if resolved.GetType() != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Fatalf("expected type to fall through to RUN, got %v", resolved.GetType())
	}

	// Persisted record carries the overlay; source and derived layers are untouched.
	var persisted pbactivity.ActivityRecord
	if err := protojson.Unmarshal(blob.Blobs[testRecordURI], &persisted); err != nil {
		t.Fatalf("unmarshal persisted record: %v", err)
	}
	if persisted.GetUserEditOverlay().GetName() != "Evening Run" {
		t.Fatalf("overlay name not persisted: %+v", persisted.GetUserEditOverlay())
	}
	if persisted.GetUserEditOverlay().GetEditedAt() == nil {
		t.Fatalf("expected edited_at to be stamped")
	}
	if persisted.GetSourceLayer().GetParsed().GetName() != "Morning Run" {
		t.Fatalf("source layer must stay immutable, got %q", persisted.GetSourceLayer().GetParsed().GetName())
	}
	if persisted.GetUserEditOverlay().Description != nil {
		t.Fatalf("description was not in the mask; it must stay unset")
	}
}

func TestUpdateActivity_UnknownMaskField(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()
	blob := &MockBlobStore{Blobs: map[string][]byte{}}
	seedEditableActivity(t, store, blob, baseRecord(), nil)
	svc := NewService(store, &MockPublisher{}, blob, mockLogger{}, nil)

	_, err := svc.UpdateActivity(ctx, &pbsvc.UpdateActivityRequest{
		UserId: "user1", ActivityId: "act1", UpdateMask: []string{"distance"},
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("expected InvalidArgument for unknown field, got %v", err)
	}
}

func TestResendActivity_Preconditions(t *testing.T) {
	ctx := context.Background()

	// No run.
	svc := NewService(NewMockStore(), &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	if _, err := svc.ResendActivity(ctx, &pbsvc.ResendActivityRequest{UserId: "user1", ActivityId: "act1"}); status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}

	// Record present but no enriched event to re-send.
	store := NewMockStore()
	blob := &MockBlobStore{Blobs: map[string][]byte{}}
	seedEditableActivity(t, store, blob, baseRecord(), nil) // event nil => no EnrichedEventUri
	svc = NewService(store, &MockPublisher{}, blob, mockLogger{}, nil)
	if _, err := svc.ResendActivity(ctx, &pbsvc.ResendActivityRequest{UserId: "user1", ActivityId: "act1"}); status.Code(err) != codes.FailedPrecondition {
		t.Fatalf("expected FailedPrecondition (no enriched event), got %v", err)
	}
}

func TestResendActivity_PushesResolvedToDestinations(t *testing.T) {
	ctx := context.Background()
	store := NewMockStore()
	blob := &MockBlobStore{Blobs: map[string][]byte{}}
	publisher := &MockPublisher{}

	// The user previously edited the title; the resolved activity must reflect it.
	rec := baseRecord()
	rec.UserEditOverlay = &pbactivity.ActivityUserEditOverlay{Name: strptr("Corrected Title")}

	execID := "exec1"
	event := &pbevents.EnrichedActivityEvent{
		UserId:              "user1",
		ActivityId:          "act1",
		Name:                "Morning Run",
		Destinations:        []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
		PipelineExecutionId: &execID,
		EnrichmentMetadata:  map[string]string{"strava_foo": "bar"},
		ActivityData:        &pbactivity.StandardizedActivity{Name: "Morning Run"},
	}
	seedEditableActivity(t, store, blob, rec, event)
	svc := NewService(store, publisher, blob, mockLogger{}, nil)

	if _, err := svc.ResendActivity(ctx, &pbsvc.ResendActivityRequest{UserId: "user1", ActivityId: "act1"}); err != nil {
		t.Fatalf("ResendActivity: %v", err)
	}

	if len(publisher.PublishedEvents) != 1 {
		t.Fatalf("expected exactly one published event, got %d", len(publisher.PublishedEvents))
	}

	var published pbevents.EnrichedActivityEvent
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(publisher.PublishedEvents[0].Data(), &published); err != nil {
		t.Fatalf("unmarshal published event: %v", err)
	}

	// The published (slim) event points at an offloaded blob and preserves destinations + config.
	if published.GetActivityDataUri() == "" {
		t.Fatalf("expected slim event to carry an activity_data_uri")
	}
	if published.GetActivityData() != nil {
		t.Fatalf("expected slim event to have inline activity_data cleared")
	}
	if len(published.GetDestinations()) != 1 || published.GetDestinations()[0] != pbplugin.DestinationType_DESTINATION_STRAVA {
		t.Fatalf("destinations not preserved: %v", published.GetDestinations())
	}
	if published.GetEnrichmentMetadata()["strava_foo"] != "bar" {
		t.Fatalf("per-destination config not preserved")
	}
	// Marked as a re-send so the destination executor updates in place instead of skipping.
	if published.GetEnrichmentMetadata()["is_repost"] != "true" || published.GetEnrichmentMetadata()["repost_mode"] != resendRepostMode {
		t.Fatalf("re-send metadata missing: %v", published.GetEnrichmentMetadata())
	}
	// Original pipeline execution id is reused so prior destination outcomes are visible.
	if published.GetPipelineExecutionId() != execID {
		t.Fatalf("expected pipeline execution id %q reused, got %q", execID, published.GetPipelineExecutionId())
	}

	// The offloaded full event carries the RESOLVED (edited) content.
	full, ok := blob.Blobs[published.GetActivityDataUri()]
	if !ok {
		t.Fatalf("offloaded event blob missing at %s", published.GetActivityDataUri())
	}
	var fullEvent pbevents.EnrichedActivityEvent
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(full, &fullEvent); err != nil {
		t.Fatalf("unmarshal offloaded event: %v", err)
	}
	if fullEvent.GetName() != "Corrected Title" || fullEvent.GetActivityData().GetName() != "Corrected Title" {
		t.Fatalf("re-sent event must carry the resolved title, got name=%q data.name=%q", fullEvent.GetName(), fullEvent.GetActivityData().GetName())
	}
}

// verify the published topic constant is the enriched-activity topic (compile-time guard
// that we publish where the router consumes).
var _ = shared.TopicEnrichedActivity

func strptr(s string) *string { return &s }
