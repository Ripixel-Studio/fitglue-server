package server

import (
	"encoding/json"
	"net/http"

	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
	"github.com/go-chi/chi/v5"
	"google.golang.org/protobuf/encoding/protojson"
)

func (s *APIServer) registerRegistryRoutes(r chi.Router) {
	// Note: Registry routes are typically public or require different auth
	// But according to the task list, they are mounted under /api/registry and proxy to RegistryService.
	r.Get("/registry", s.handleGetPluginRegistry)
	r.Get("/registry/plugins", s.handleGetPluginRegistry) // Frontend expects the full split format here
	r.Get("/registry/plugins/{id}", s.handleGetPlugin)
	r.Get("/registry/plugins/{id}/icon", s.handleGetPluginIcon)
	r.Get("/registry/categories", s.handleListCategories)
	r.Get("/registry/sources", s.handleListSources)
}

func (s *APIServer) handleListPlugins(w http.ResponseWriter, r *http.Request) {
	req := &registrypb.ListPluginsRequest{
		Category: r.URL.Query().Get("category"),
	}

	res, err := s.registrySvc.ListPlugins(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPlugin(w http.ResponseWriter, r *http.Request) {
	req := &registrypb.GetPluginRequest{
		PluginId: chi.URLParam(r, "id"),
	}

	res, err := s.registrySvc.GetPlugin(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPluginIcon(w http.ResponseWriter, r *http.Request) {
	req := &registrypb.GetPluginIconRequest{
		PluginId: chi.URLParam(r, "id"),
	}

	res, err := s.registrySvc.GetPluginIcon(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	// For icons, we need to write the raw bytes and set the content type
	w.Header().Set("Content-Type", res.ContentType)
	w.Header().Set("Cache-Control", "public, max-age=86400") // 1 day cache
	_, _ = w.Write(res.IconData)
}

func (s *APIServer) handleListCategories(w http.ResponseWriter, r *http.Request) {
	req := &registrypb.ListCategoriesRequest{}

	res, err := s.registrySvc.ListCategories(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleListSources(w http.ResponseWriter, r *http.Request) {
	req := &registrypb.ListSourcesRequest{}

	res, err := s.registrySvc.ListSources(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPluginRegistry(w http.ResponseWriter, r *http.Request) {
	marketingMode := r.URL.Query().Get("marketingMode") == "true"

	req := &registrypb.GetPluginRegistryRequest{
		MarketingMode: marketingMode,
	}

	res, err := s.registrySvc.GetPluginRegistry(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	if !marketingMode || s.statsStore == nil {
		WriteJSON(w, res)
		return
	}

	stats, err := s.statsStore.GetPlatformStats(r.Context())
	if err != nil || stats == nil {
		s.logger.Warn(r.Context(), "Failed to fetch platform stats, using zeros", "error", err)
		stats = &PlatformStats{}
	}

	marshaler := protojson.MarshalOptions{UseProtoNames: false, EmitUnpopulated: true}
	protoBytes, err := marshaler.Marshal(res)
	if err != nil {
		WriteError(w, err)
		return
	}

	var combined map[string]interface{}
	if err := json.Unmarshal(protoBytes, &combined); err != nil {
		WriteError(w, err)
		return
	}
	combined["stats"] = stats

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(combined)
}
