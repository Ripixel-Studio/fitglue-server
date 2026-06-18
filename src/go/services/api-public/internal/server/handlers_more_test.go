package server

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// pubRegistryClient embeds the full interface; only the methods under test are set.
type pubRegistryClient struct {
	registrypb.RegistryServiceClient
	err error
}

func (m *pubRegistryClient) GetPluginRegistry(_ context.Context, _ *registrypb.GetPluginRegistryRequest, _ ...grpc.CallOption) (*pbplugin.PluginRegistryResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &pbplugin.PluginRegistryResponse{}, nil
}
func (m *pubRegistryClient) ListPlugins(_ context.Context, _ *registrypb.ListPluginsRequest, _ ...grpc.CallOption) (*registrypb.ListPluginsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &registrypb.ListPluginsResponse{}, nil
}
func (m *pubRegistryClient) GetPlugin(_ context.Context, _ *registrypb.GetPluginRequest, _ ...grpc.CallOption) (*pbplugin.PluginManifest, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &pbplugin.PluginManifest{}, nil
}
func (m *pubRegistryClient) ListCategories(_ context.Context, _ *registrypb.ListCategoriesRequest, _ ...grpc.CallOption) (*registrypb.ListCategoriesResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &registrypb.ListCategoriesResponse{}, nil
}
func (m *pubRegistryClient) ListSources(_ context.Context, _ *registrypb.ListSourcesRequest, _ ...grpc.CallOption) (*registrypb.ListSourcesResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &registrypb.ListSourcesResponse{}, nil
}

type pubActivityClient struct {
	activitypb.ActivityServiceClient
	err error
}

func (m *pubActivityClient) GetPublicShowcase(_ context.Context, _ *activitypb.GetPublicShowcaseRequest, _ ...grpc.CallOption) (*pbactivity.ShowcasedActivity, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &pbactivity.ShowcasedActivity{}, nil
}
func (m *pubActivityClient) GetPublicShowcaseProfile(_ context.Context, _ *activitypb.GetPublicShowcaseProfileRequest, _ ...grpc.CallOption) (*activitypb.GetPublicShowcaseProfileResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &activitypb.GetPublicShowcaseProfileResponse{}, nil
}
func (m *pubActivityClient) GetPublicRoundup(_ context.Context, _ *activitypb.GetPublicRoundupRequest, _ ...grpc.CallOption) (*pbactivity.ShowcaseRoundup, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &pbactivity.ShowcaseRoundup{}, nil
}
func (m *pubActivityClient) GetRecentPublicRoundups(_ context.Context, _ *activitypb.GetRecentPublicRoundupsRequest, _ ...grpc.CallOption) (*activitypb.GetRecentPublicRoundupsResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return &activitypb.GetRecentPublicRoundupsResponse{}, nil
}

func pubServer(reg *pubRegistryClient, act *pubActivityClient) *APIServer {
	return &APIServer{logger: infra.NewLogger(), registrySvc: reg, activitySvc: act}
}

func TestPublicRegistryHandlers(t *testing.T) {
	okReg := &pubRegistryClient{}
	errReg := &pubRegistryClient{err: status.Error(codes.Internal, "boom")}

	type tc struct {
		name string
		call func(s *APIServer, w http.ResponseWriter, r *http.Request)
	}
	cases := []tc{
		{"registry", (*APIServer).handleGetPluginRegistry},
		{"listPlugins", (*APIServer).handleListPlugins},
		{"getPlugin", (*APIServer).handleGetPlugin},
		{"listCategories", (*APIServer).handleListCategories},
		{"listSources", (*APIServer).handleListSources},
	}
	for _, c := range cases {
		t.Run(c.name+"_ok", func(t *testing.T) {
			s := pubServer(okReg, nil)
			w := httptest.NewRecorder()
			c.call(s, w, httptest.NewRequest(http.MethodGet, "/x", nil))
			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}
		})
		t.Run(c.name+"_err", func(t *testing.T) {
			s := pubServer(errReg, nil)
			w := httptest.NewRecorder()
			c.call(s, w, httptest.NewRequest(http.MethodGet, "/x", nil))
			if w.Code != http.StatusInternalServerError {
				t.Errorf("expected 500, got %d", w.Code)
			}
		})
	}
}

func TestPublicShowcaseHandlers(t *testing.T) {
	okAct := &pubActivityClient{}
	errAct := &pubActivityClient{err: status.Error(codes.NotFound, "nope")}

	cases := []struct {
		name string
		call func(s *APIServer, w http.ResponseWriter, r *http.Request)
	}{
		{"getShowcase", (*APIServer).handleGetPublicShowcase},
		{"getProfile", (*APIServer).handleGetPublicShowcaseProfile},
		{"getRoundup", (*APIServer).handleGetPublicRoundup},
		{"recentRoundups", (*APIServer).handleGetRecentPublicRoundups},
	}
	for _, c := range cases {
		t.Run(c.name+"_ok", func(t *testing.T) {
			s := pubServer(nil, okAct)
			w := httptest.NewRecorder()
			c.call(s, w, httptest.NewRequest(http.MethodGet, "/x", nil))
			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}
		})
		t.Run(c.name+"_err", func(t *testing.T) {
			s := pubServer(nil, errAct)
			w := httptest.NewRecorder()
			c.call(s, w, httptest.NewRequest(http.MethodGet, "/x", nil))
			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404, got %d", w.Code)
			}
		})
	}
}
