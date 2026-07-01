package server

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
)

// APIServer implements the HTTP router interfacing with FitGlue domain gRPC services
type APIServer struct {
	router      *chi.Mux
	logger      infra.Logger
	activitySvc activitypb.ActivityServiceClient
	registrySvc registrypb.RegistryServiceClient
	// viewSalt is the secret used to derive the daily-rotating visitor hash for
	// showcase view de-duplication. Empty disables visitor de-dup (views still count).
	viewSalt string
}

// NewAPIServer constructs the application routing and API middleware stack
func NewAPIServer(
	logger infra.Logger,
	activitySvc activitypb.ActivityServiceClient,
	registrySvc registrypb.RegistryServiceClient,
	viewSalt string,
) *APIServer {
	s := &APIServer{
		router:      chi.NewRouter(),
		logger:      logger,
		activitySvc: activitySvc,
		registrySvc: registrySvc,
		viewSalt:    viewSalt,
	}

	s.setupRoutes()
	return s
}

// ServeHTTP implements http.Handler automatically so the APIServer can be bound to net/http
func (s *APIServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.router.ServeHTTP(w, r)
}

func (s *APIServer) setupRoutes() {
	// Root level middleware (shared by all endpoints)
	s.router.Use(middleware.RequestID)
	s.router.Use(middleware.RealIP)
	s.router.Use(middleware.Recoverer)

	// API Public block (No Auth / API routing)
	s.router.Route("/api/public", func(r chi.Router) {
		// Public, unauthenticated, read-only data — allow any browser origin so
		// third-party tools (e.g. a user's own post maker) can read a public
		// showcase. No credentials are involved, so "*" is safe here.
		r.Use(publicCORS)

		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		s.registerRegistryRoutes(r)
		s.registerShowcaseRoutes(r)
	})
}

// publicCORS adds permissive CORS headers (and answers preflight) for the
// public API block.
func publicCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Max-Age", "86400")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *APIServer) registerRegistryRoutes(r chi.Router) {
	r.Get("/registry", s.handleGetPluginRegistry)
	r.Get("/registry/plugins", s.handleListPlugins)
	r.Get("/registry/plugins/{id}", s.handleGetPlugin)
	r.Get("/registry/categories", s.handleListCategories)
	r.Get("/registry/sources", s.handleListSources)

	// Wait, is endpoints mapping to API Public list Destinations requested?
	// Let's implement handles.
}

func (s *APIServer) handleGetPluginRegistry(w http.ResponseWriter, r *http.Request) {
	req := &registrypb.GetPluginRegistryRequest{}

	res, err := s.registrySvc.GetPluginRegistry(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) registerShowcaseRoutes(r chi.Router) {
	r.Get("/showcase/{id}", s.handleGetPublicShowcase)
	r.Get("/showcase/profile/{slug}", s.handleGetPublicShowcaseProfile)
	r.Get("/showcase/{slug}/roundup/{periodKey}", s.handleGetPublicRoundup)
	r.Get("/showcase/{slug}/roundups/recent", s.handleGetRecentPublicRoundups)

	// View beacons (fire-and-forget; always respond 204)
	r.Post("/showcase/{id}/view", s.handleRecordShowcaseActivityView)
	r.Post("/showcase/profile/{slug}/view", s.handleRecordShowcaseProfileView)
	r.Post("/showcase/{slug}/roundup/{periodKey}/view", s.handleRecordShowcaseRoundupView)
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

func (s *APIServer) handleGetPublicShowcase(w http.ResponseWriter, r *http.Request) {
	req := &activitypb.GetPublicShowcaseRequest{
		ShowcaseId: chi.URLParam(r, "id"),
	}

	res, err := s.activitySvc.GetPublicShowcase(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPublicShowcaseProfile(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page := int32(1)
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = int32(p)
		}
	}

	req := &activitypb.GetPublicShowcaseProfileRequest{
		Slug: chi.URLParam(r, "slug"),
		Page: page,
	}

	res, err := s.activitySvc.GetPublicShowcaseProfile(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetPublicRoundup(w http.ResponseWriter, r *http.Request) {
	req := &activitypb.GetPublicRoundupRequest{
		Slug:      chi.URLParam(r, "slug"),
		PeriodKey: chi.URLParam(r, "periodKey"),
	}

	res, err := s.activitySvc.GetPublicRoundup(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetRecentPublicRoundups(w http.ResponseWriter, r *http.Request) {
	pageStr := r.URL.Query().Get("page")
	page := int32(1)
	if pageStr != "" {
		if p, err := strconv.Atoi(pageStr); err == nil && p > 0 {
			page = int32(p)
		}
	}

	req := &activitypb.GetRecentPublicRoundupsRequest{
		Slug: chi.URLParam(r, "slug"),
		Page: page,
	}

	res, err := s.activitySvc.GetRecentPublicRoundups(r.Context(), req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

// statusError is a helper for manually generating an error satisfying gRPC status layout
func statusError(code int, msg string) error {
	// Simple wrapper for non-gRPC errors to use WriteError
	return &CustomError{HTTPCode: code, Msg: msg}
}

// CustomError helps map generic HTTP errors into our WriteError handler
type CustomError struct {
	HTTPCode int
	Msg      string
}

func (e *CustomError) Error() string {
	return e.Msg
}
