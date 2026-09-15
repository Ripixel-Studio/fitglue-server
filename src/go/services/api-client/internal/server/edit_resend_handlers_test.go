package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"

	pbactivitym "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
)

func TestHandleUpdateActivity_DerivesMaskAndParsesType(t *testing.T) {
	var captured *pipelinepb.UpdateActivityRequest
	svc := &mockPipelineServiceClient{
		updateActivity: func(_ context.Context, in *pipelinepb.UpdateActivityRequest, _ ...grpc.CallOption) (*pbactivitym.StandardizedActivity, error) {
			captured = in
			return &pbactivitym.StandardizedActivity{Name: in.Name}, nil
		},
	}
	s := serverWithDeps(&APIServer{pipelineSvc: svc})

	body := `{"name":"Evening Run","type":"Ride"}`
	w := httptest.NewRecorder()
	s.handleUpdateActivity(w, withToken(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(body)), "u1"))

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if captured == nil {
		t.Fatal("expected UpdateActivity to be called")
	}
	if captured.UserId != "u1" {
		t.Errorf("expected user id from token, got %q", captured.UserId)
	}
	// Mask derived from the fields present in the body (name + type, not description/tags).
	if len(captured.UpdateMask) != 2 {
		t.Fatalf("expected 2 masked fields, got %v", captured.UpdateMask)
	}
	gotMask := strings.Join(captured.UpdateMask, ",")
	if !strings.Contains(gotMask, "name") || !strings.Contains(gotMask, "type") {
		t.Errorf("expected mask to contain name and type, got %v", captured.UpdateMask)
	}
	if captured.Type != pbactivitym.ActivityType_ACTIVITY_TYPE_RIDE {
		t.Errorf("expected type parsed to RIDE, got %v", captured.Type)
	}
}

func TestHandleUpdateActivity_NoFields(t *testing.T) {
	s := serverWithDeps(&APIServer{pipelineSvc: &mockPipelineServiceClient{}})
	w := httptest.NewRecorder()
	s.handleUpdateActivity(w, withToken(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{}`)), "u1"))
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for empty patch, got %d", w.Code)
	}
}

func TestHandleUpdateActivity_ExplicitMaskClears(t *testing.T) {
	var captured *pipelinepb.UpdateActivityRequest
	svc := &mockPipelineServiceClient{
		updateActivity: func(_ context.Context, in *pipelinepb.UpdateActivityRequest, _ ...grpc.CallOption) (*pbactivitym.StandardizedActivity, error) {
			captured = in
			return &pbactivitym.StandardizedActivity{}, nil
		},
	}
	s := serverWithDeps(&APIServer{pipelineSvc: svc})
	// Explicit mask naming a field with no value present => clear-to-empty intent.
	w := httptest.NewRecorder()
	s.handleUpdateActivity(w, withToken(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"update_mask":["description"]}`)), "u1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if len(captured.UpdateMask) != 1 || captured.UpdateMask[0] != "description" {
		t.Fatalf("expected explicit mask [description], got %v", captured.UpdateMask)
	}
}

func TestHandleUpdateActivity_ServiceError(t *testing.T) {
	svc := &mockPipelineServiceClient{
		updateActivity: func(_ context.Context, _ *pipelinepb.UpdateActivityRequest, _ ...grpc.CallOption) (*pbactivitym.StandardizedActivity, error) {
			return nil, status.Error(codes.FailedPrecondition, "not editable")
		},
	}
	s := serverWithDeps(&APIServer{pipelineSvc: svc})
	w := httptest.NewRecorder()
	s.handleUpdateActivity(w, withToken(httptest.NewRequest(http.MethodPatch, "/", strings.NewReader(`{"name":"x"}`)), "u1"))
	if w.Code != http.StatusPreconditionFailed { // FailedPrecondition maps to 412 in this gateway
		t.Fatalf("expected 412 from FailedPrecondition, got %d", w.Code)
	}
}

func TestHandleResendActivity(t *testing.T) {
	var called bool
	svc := &mockPipelineServiceClient{
		resendActivity: func(_ context.Context, in *pipelinepb.ResendActivityRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			called = true
			if in.UserId != "u1" {
				t.Errorf("expected user id from token, got %q", in.UserId)
			}
			return &emptypb.Empty{}, nil
		},
	}
	s := serverWithDeps(&APIServer{pipelineSvc: svc})
	w := httptest.NewRecorder()
	s.handleResendActivity(w, withToken(httptest.NewRequest(http.MethodPost, "/", nil), "u1"))
	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d", w.Code)
	}
	if !called {
		t.Fatal("expected ResendActivity to be called")
	}
}

func TestHandleResendActivity_Error(t *testing.T) {
	svc := &mockPipelineServiceClient{
		resendActivity: func(_ context.Context, _ *pipelinepb.ResendActivityRequest, _ ...grpc.CallOption) (*emptypb.Empty, error) {
			return nil, status.Error(codes.NotFound, "gone")
		},
	}
	s := serverWithDeps(&APIServer{pipelineSvc: svc})
	w := httptest.NewRecorder()
	s.handleResendActivity(w, withToken(httptest.NewRequest(http.MethodPost, "/", nil), "u1"))
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}
