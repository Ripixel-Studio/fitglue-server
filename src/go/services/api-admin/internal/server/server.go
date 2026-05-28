package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go/v4/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

// APIServer implements the HTTP router interfacing with FitGlue domain gRPC services
type APIServer struct {
	router          *chi.Mux
	logger          infra.Logger
	authClient      *auth.Client
	userService     userpb.UserServiceClient
	pipelineSvc     pipelinepb.PipelineServiceClient
	activitySvc     activitypb.ActivityServiceClient
	firestoreClient *firestore.Client
}

// NewAPIServer constructs the application routing and API middleware stack
func NewAPIServer(
	logger infra.Logger,
	authClient *auth.Client,
	userSvc userpb.UserServiceClient,
	pipelineSvc pipelinepb.PipelineServiceClient,
	activitySvc activitypb.ActivityServiceClient,
	fsClient *firestore.Client,
) *APIServer {
	s := &APIServer{
		router:          chi.NewRouter(),
		logger:          logger,
		authClient:      authClient,
		userService:     userSvc,
		pipelineSvc:     pipelineSvc,
		activitySvc:     activitySvc,
		firestoreClient: fsClient,
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

	// Health check (no auth required)
	s.router.Get("/api/admin/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		fmt.Fprint(w, `{"status": "ok", "role": "admin"}`)
	})

	// API Admin block (Admin Auth / API routing)
	s.router.Route("/api/admin", func(r chi.Router) {
		r.Use(AdminMiddleware(s.authClient, s.firestoreClient))
		s.registerAdminRoutes(r)
	})
}

func (s *APIServer) registerAdminRoutes(r chi.Router) {
	r.Get("/stats", s.handleGetStats)

	r.Get("/users", s.handleListUsers)
	r.Get("/users/{id}", s.handleGetUser)
	r.Put("/users/{id}", s.handleUpdateUser)
	r.Delete("/users/{id}", s.handleDeleteUser)
	r.Delete("/users/{id}/{dataType}", s.handleDeleteUserData)

	r.Get("/pipelines", s.handleListAllPipelines)
	r.Get("/pipeline-runs", s.handleAdminPipelineRuns)
}

func (s *APIServer) handleListUsers(w http.ResponseWriter, r *http.Request) {
	limitStr := r.URL.Query().Get("limit")
	limit := 50
	if limitStr != "" {
		if l, err := strconv.Atoi(limitStr); err == nil && l > 0 && l <= 1000 {
			limit = l
		}
	}
	pageToken := r.URL.Query().Get("page_token")

	res, err := s.userService.ListUsers(r.Context(), &userpb.ListUsersRequest{
		Limit:     int32(limit),
		PageToken: pageToken,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleGetUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}

	res, err := s.userService.GetProfile(r.Context(), &userpb.GetProfileRequest{
		UserId: userID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleUpdateUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}

	var req struct {
		AccessEnabled *bool `json:"accessEnabled"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		WriteError(w, statusError(http.StatusBadRequest, "invalid request body"))
		return
	}

	profile, err := s.userService.GetProfile(r.Context(), &userpb.GetProfileRequest{UserId: userID})
	if err != nil {
		WriteError(w, err)
		return
	}

	if req.AccessEnabled != nil {
		profile.AccessEnabled = *req.AccessEnabled
	}

	res, err := s.userService.UpdateProfile(r.Context(), &userpb.UpdateProfileRequest{
		UserId:  userID,
		Profile: profile,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}

func (s *APIServer) handleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID := chi.URLParam(r, "id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "missing user id"))
		return
	}

	_, err := s.userService.DeleteUser(r.Context(), &userpb.DeleteUserRequest{
		UserId: userID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *APIServer) handleListAllPipelines(w http.ResponseWriter, r *http.Request) {
	userID := r.URL.Query().Get("user_id")
	if userID == "" {
		WriteError(w, statusError(http.StatusBadRequest, "user_id query parameter is required"))
		return
	}

	res, err := s.pipelineSvc.ListPipelines(r.Context(), &pipelinepb.ListPipelinesRequest{
		UserId: userID,
	})
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, res)
}
