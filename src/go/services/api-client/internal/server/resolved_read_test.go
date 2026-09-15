package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	pbactivitym "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func TestHandleGetResolvedActivity_Success(t *testing.T) {
	svc := &mockActivityServiceClient{
		getResolvedActivity: func(_ context.Context, in *activitypb.GetResolvedActivityRequest, _ ...grpc.CallOption) (*pbactivitym.ResolvedActivity, error) {
			if in.GetUserId() != "user1" {
				t.Errorf("expected user id from token, got %q", in.GetUserId())
			}
			return &pbactivitym.ResolvedActivity{
				Activity: &pbactivitym.StandardizedActivity{Name: "Resolved"},
				Provenance: []*pbactivitym.FieldProvenance{
					{Field: "name", Layer: pbactivitym.ProvenanceLayer_PROVENANCE_LAYER_USER_OVERLAY},
				},
			}, nil
		},
	}
	s := buildActivityServer(svc)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities/act123/resolved", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetResolvedActivity(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleGetResolvedActivity_NoToken(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities/act123/resolved", nil)
	w := httptest.NewRecorder()
	s.handleGetResolvedActivity(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleGetResolvedActivity_NotFound(t *testing.T) {
	svc := &mockActivityServiceClient{
		getResolvedActivity: func(_ context.Context, _ *activitypb.GetResolvedActivityRequest, _ ...grpc.CallOption) (*pbactivitym.ResolvedActivity, error) {
			return nil, status.Error(codes.NotFound, "activity not found")
		},
	}
	s := buildActivityServer(svc)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/activities/missing/resolved", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleGetResolvedActivity(w, r)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleListResolvedActivities_Success(t *testing.T) {
	svc := &mockActivityServiceClient{
		listResolvedActivities: func(_ context.Context, in *activitypb.ListResolvedActivitiesRequest, _ ...grpc.CallOption) (*activitypb.ListResolvedActivitiesResponse, error) {
			if in.GetLimit() != 10 {
				t.Errorf("expected limit 10 from query, got %d", in.GetLimit())
			}
			if in.GetPageToken() != "abc" {
				t.Errorf("expected page_token 'abc', got %q", in.GetPageToken())
			}
			return &activitypb.ListResolvedActivitiesResponse{
				Activities: []*pbactivitym.ResolvedActivity{
					{Activity: &pbactivitym.StandardizedActivity{Name: "One"}},
				},
				NextPageToken: "next",
			}, nil
		},
	}
	s := buildActivityServer(svc)
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/resolved-activities?limit=10&page_token=abc", nil)
	r = withToken(r, "user1")
	w := httptest.NewRecorder()
	s.handleListResolvedActivities(w, r)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestHandleListResolvedActivities_NoToken(t *testing.T) {
	s := buildActivityServer(&mockActivityServiceClient{})
	r := httptest.NewRequest(http.MethodGet, "/api/v2/users/me/resolved-activities", nil)
	w := httptest.NewRecorder()
	s.handleListResolvedActivities(w, r)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}
