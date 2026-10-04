// nolint:proto-json
package pipeline

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/internal/pipeline/enricher"
	activitydomain "github.com/fitglue/server/src/go/pkg/domain/activity"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	"github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

type fakeInvoker struct {
	layer       *pbactivity.EnricherRunLayer
	err         error
	gotProvider string
	gotActivity *pbactivity.StandardizedActivity
}

func (f *fakeInvoker) InvokeSingle(ctx context.Context, providerName string, activity *pbactivity.StandardizedActivity, userRec *user.Record) (*pbactivity.EnricherRunLayer, error) {
	f.gotProvider = providerName
	f.gotActivity = activity
	return f.layer, f.err
}

type fakeUserDB struct {
	rec *user.Record
	err error
}

func (f *fakeUserDB) GetUser(ctx context.Context, id string) (*user.Record, error) {
	return f.rec, f.err
}

const (
	testBucket = "test-artifacts"
	testUser   = "u1"
	testAct    = "a1"
)

// newRecordFixture builds a persisted record fixture: a derived activity with a base name,
// description and tags, plus a user-edit overlay that renames the activity. Returns the
// service (with a fake invoker/userDB), the blob store, and the record URI.
func newRecordFixture(t *testing.T, invoker EnricherInvoker) (*Service, *MockBlobStore, string) {
	t.Helper()

	rec := &pbactivity.ActivityRecord{
		ActivityId: testAct,
		UserId:     testUser,
		DerivedActivity: &pbactivity.StandardizedActivity{
			Name:        "Morning Run",
			Description: "base",
			Type:        pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
			Tags:        []string{"base-tag"},
		},
		UserEditOverlay: &pbactivity.ActivityUserEditOverlay{
			Name: proto.String("My Custom Title"),
		},
	}
	data, err := protojson.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal fixture: %v", err)
	}

	recordURI := activitydomain.ActivityRecordURI(testBucket, testUser, testAct)
	blob := &MockBlobStore{Blobs: map[string][]byte{recordURI: data}}

	store := NewMockStore()
	store.Runs[store.key(testUser, "run1")] = &pipeline.PipelineRun{
		Id:                "run1",
		ActivityId:        testAct,
		ActivityRecordUri: recordURI,
	}

	svc := NewService(store, &MockPublisher{}, blob, mockLogger{}, nil)
	svc.SetEnricherInvocation(&fakeUserDB{rec: &user.Record{}}, invoker)
	return svc, blob, recordURI
}

func loadRec(t *testing.T, blob *MockBlobStore, uri string) *pbactivity.ActivityRecord {
	t.Helper()
	got := &pbactivity.ActivityRecord{}
	if err := protojson.Unmarshal(blob.Blobs[uri], got); err != nil {
		t.Fatalf("reload record: %v", err)
	}
	return got
}

func weatherProposal() *pbactivity.EnricherRunLayer {
	return &pbactivity.EnricherRunLayer{
		ProviderName: "weather",
		ExecutionId:  "exec-1",
		Status:       "SUCCESS",
		Contribution: &pbactivity.EnricherContribution{
			Description: proto.String("Weather: 20°C"),
			Tags:        []string{"sunny"},
		},
	}
}

func TestInvokeEnricher_RecordsProposalWithoutClobbering(t *testing.T) {
	invoker := &fakeInvoker{layer: weatherProposal()}
	svc, blob, uri := newRecordFixture(t, invoker)

	resp, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
		UserId: testUser, ActivityId: testAct, ProviderName: "weather",
	})
	if err != nil {
		t.Fatalf("InvokeEnricher: %v", err)
	}

	// The enricher ran against the DERIVED activity, not the overlay-applied one.
	if invoker.gotActivity.GetName() != "Morning Run" {
		t.Errorf("enricher should run against derived name, got %q", invoker.gotActivity.GetName())
	}
	if !resp.ProposedCreated || resp.GetProposed().GetExecutionId() != "exec-1" {
		t.Fatalf("expected a created proposal, got %+v", resp)
	}

	// Preview: derived + proposal, with the overlay still winning on name.
	if resp.GetPreview().GetName() != "My Custom Title" {
		t.Errorf("overlay must win in preview name: got %q", resp.GetPreview().GetName())
	}
	if resp.GetPreview().GetDescription() != "base\n\nWeather: 20°C" {
		t.Errorf("preview description: got %q", resp.GetPreview().GetDescription())
	}

	// Persisted record: proposal recorded; derived, overlay and applied layers untouched.
	got := loadRec(t, blob, uri)
	if len(got.GetProposedEnricherLayers()) != 1 {
		t.Fatalf("expected 1 proposed layer, got %d", len(got.GetProposedEnricherLayers()))
	}
	if len(got.GetEnricherLayers()) != 0 {
		t.Errorf("applied enricher_layers must be untouched, got %d", len(got.GetEnricherLayers()))
	}
	if got.GetDerivedActivity().GetDescription() != "base" {
		t.Errorf("derived activity must be untouched, got %q", got.GetDerivedActivity().GetDescription())
	}
	if got.GetUserEditOverlay().GetName() != "My Custom Title" {
		t.Errorf("user overlay must be untouched, got %q", got.GetUserEditOverlay().GetName())
	}
}

func TestInvokeEnricher_NoChangeDoesNotPersistProposal(t *testing.T) {
	invoker := &fakeInvoker{layer: nil} // enricher produced nothing worth proposing
	svc, blob, uri := newRecordFixture(t, invoker)

	resp, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
		UserId: testUser, ActivityId: testAct, ProviderName: "weather",
	})
	if err != nil {
		t.Fatalf("InvokeEnricher: %v", err)
	}
	if resp.ProposedCreated {
		t.Errorf("expected no proposal created")
	}
	if got := loadRec(t, blob, uri); len(got.GetProposedEnricherLayers()) != 0 {
		t.Errorf("no proposal should be persisted, got %d", len(got.GetProposedEnricherLayers()))
	}
}

func TestInvokeEnricher_MapsProviderErrors(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want codes.Code
	}{
		{"not found", enricher.ErrProviderNotFound, codes.NotFound},
		{"not invokable", enricher.ErrNotIndividuallyInvokable, codes.FailedPrecondition},
		{"requires input", enricher.ErrRequiresInput, codes.FailedPrecondition},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			svc, _, _ := newRecordFixture(t, &fakeInvoker{err: tc.err})
			_, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
				UserId: testUser, ActivityId: testAct, ProviderName: "weather",
			})
			if status.Code(err) != tc.want {
				t.Errorf("got code %v, want %v (err=%v)", status.Code(err), tc.want, err)
			}
		})
	}
}

func TestInvokeEnricher_Unconfigured(t *testing.T) {
	// A service without SetEnricherInvocation returns Unimplemented.
	store := NewMockStore()
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	_, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
		UserId: testUser, ActivityId: testAct, ProviderName: "weather",
	})
	if status.Code(err) != codes.Unimplemented {
		t.Errorf("got %v, want Unimplemented", status.Code(err))
	}
}

func TestAcceptProposedEnricherRun_AppliesAndPreservesOverlay(t *testing.T) {
	invoker := &fakeInvoker{layer: weatherProposal()}
	svc, blob, uri := newRecordFixture(t, invoker)

	if _, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
		UserId: testUser, ActivityId: testAct, ProviderName: "weather",
	}); err != nil {
		t.Fatalf("InvokeEnricher: %v", err)
	}

	resolved, err := svc.AcceptProposedEnricherRun(context.Background(), &pbsvc.AcceptProposedEnricherRunRequest{
		UserId: testUser, ActivityId: testAct, ExecutionId: "exec-1",
	})
	if err != nil {
		t.Fatalf("Accept: %v", err)
	}

	// Resolved: derived now folds the enricher; overlay name still wins.
	if resolved.GetName() != "My Custom Title" {
		t.Errorf("overlay must still win after accept: got %q", resolved.GetName())
	}
	if resolved.GetDescription() != "base\n\nWeather: 20°C" {
		t.Errorf("accepted description: got %q", resolved.GetDescription())
	}

	got := loadRec(t, blob, uri)
	if len(got.GetProposedEnricherLayers()) != 0 {
		t.Errorf("proposal should be consumed, got %d", len(got.GetProposedEnricherLayers()))
	}
	if len(got.GetEnricherLayers()) != 1 || got.GetEnricherLayers()[0].GetExecutionId() != "exec-1" {
		t.Errorf("proposal should be promoted to applied history: got %+v", got.GetEnricherLayers())
	}
	if got.GetDerivedActivity().GetDescription() != "base\n\nWeather: 20°C" {
		t.Errorf("derived should include accepted enricher: got %q", got.GetDerivedActivity().GetDescription())
	}
	if got.GetUserEditOverlay().GetName() != "My Custom Title" {
		t.Errorf("overlay must be untouched by accept: got %q", got.GetUserEditOverlay().GetName())
	}
}

func TestAcceptProposedEnricherRun_NotFound(t *testing.T) {
	svc, _, _ := newRecordFixture(t, &fakeInvoker{})
	_, err := svc.AcceptProposedEnricherRun(context.Background(), &pbsvc.AcceptProposedEnricherRunRequest{
		UserId: testUser, ActivityId: testAct, ExecutionId: "nope",
	})
	if status.Code(err) != codes.NotFound {
		t.Errorf("got %v, want NotFound", status.Code(err))
	}
}

func TestDismissProposedEnricherRun_RemovesProposal(t *testing.T) {
	invoker := &fakeInvoker{layer: weatherProposal()}
	svc, blob, uri := newRecordFixture(t, invoker)

	if _, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
		UserId: testUser, ActivityId: testAct, ProviderName: "weather",
	}); err != nil {
		t.Fatalf("InvokeEnricher: %v", err)
	}

	if _, err := svc.DismissProposedEnricherRun(context.Background(), &pbsvc.DismissProposedEnricherRunRequest{
		UserId: testUser, ActivityId: testAct, ExecutionId: "exec-1",
	}); err != nil {
		t.Fatalf("Dismiss: %v", err)
	}

	got := loadRec(t, blob, uri)
	if len(got.GetProposedEnricherLayers()) != 0 {
		t.Errorf("proposal should be dismissed, got %d", len(got.GetProposedEnricherLayers()))
	}
	if len(got.GetEnricherLayers()) != 0 {
		t.Errorf("dismiss must not apply the enricher, got %d applied layers", len(got.GetEnricherLayers()))
	}
	if got.GetDerivedActivity().GetDescription() != "base" {
		t.Errorf("derived must be untouched by dismiss: got %q", got.GetDerivedActivity().GetDescription())
	}
}

func TestInvokeEnricher_NoRecord(t *testing.T) {
	// A run without a layered record URI is not re-runnable.
	store := NewMockStore()
	store.Runs[store.key(testUser, "run1")] = &pipeline.PipelineRun{Id: "run1", ActivityId: testAct}
	svc := NewService(store, &MockPublisher{}, &MockBlobStore{Blobs: map[string][]byte{}}, mockLogger{}, nil)
	svc.SetEnricherInvocation(&fakeUserDB{rec: &user.Record{}}, &fakeInvoker{layer: weatherProposal()})

	_, err := svc.InvokeEnricher(context.Background(), &pbsvc.InvokeEnricherRequest{
		UserId: testUser, ActivityId: testAct, ProviderName: "weather",
	})
	if status.Code(err) != codes.FailedPrecondition {
		t.Errorf("got %v, want FailedPrecondition", status.Code(err))
	}
}
