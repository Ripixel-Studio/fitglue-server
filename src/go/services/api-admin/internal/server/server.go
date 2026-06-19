package server

import (
	"fmt"
	"net/http"

	"cloud.google.com/go/firestore"
	"firebase.google.com/go/v4/auth"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
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
	billingSvc      billingpb.BillingServiceClient
	firestoreClient *firestore.Client
}

// NewAPIServer constructs the application routing and API middleware stack
func NewAPIServer(
	logger infra.Logger,
	authClient *auth.Client,
	userSvc userpb.UserServiceClient,
	pipelineSvc pipelinepb.PipelineServiceClient,
	activitySvc activitypb.ActivityServiceClient,
	billingSvc billingpb.BillingServiceClient,
	fsClient *firestore.Client,
) *APIServer {
	s := &APIServer{
		router:          chi.NewRouter(),
		logger:          logger,
		authClient:      authClient,
		userService:     userSvc,
		pipelineSvc:     pipelineSvc,
		activitySvc:     activitySvc,
		billingSvc:      billingSvc,
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
	s.router.Use(SentryRecoveryMiddleware(s.logger))

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

	// User management
	r.Get("/users", s.handleListUsers)
	r.Get("/users/{id}", s.handleGetUser)
	r.Put("/users/{id}", s.handleUpdateUser)
	r.Delete("/users/{id}", s.handleDeleteUser)
	r.Delete("/users/{id}/{dataType}", s.handleDeleteUserData)

	// User actions
	r.Post("/users/{id}/send-password-reset", s.handleSendPasswordReset)
	r.Post("/users/{id}/send-verification-email", s.handleSendVerificationEmail)
	r.Post("/users/{id}/integrations/{provider}/enabled", s.handleSetIntegrationEnabled)
	r.Delete("/users/{id}/integrations/{provider}", s.handleDeleteIntegration)

	// Pipeline management
	r.Get("/pipelines", s.handleListAllPipelines)
	r.Get("/pipeline-runs", s.handleAdminPipelineRuns)
	r.Get("/users/{id}/pipeline-runs/{runId}", s.handleGetPipelineRun)
	r.Post("/users/{id}/activities/{activityId}/repost", s.handleRepostActivity)
	r.Post("/users/{id}/pipeline-runs/{runId}/cancel", s.handleCancelPipelineRun)
	r.Get("/users/{id}/pending-inputs", s.handleListPendingInputs)
	r.Post("/users/{id}/pending-inputs/{inputId}/resolve", s.handleResolvePendingInput)

	// Billing
	r.Get("/users/{id}/billing", s.handleGetUserBilling)
	r.Post("/users/{id}/billing/trial", s.handleStartTrial)
	r.Post("/users/{id}/billing/cancel", s.handleCancelSubscription)
	r.Post("/users/{id}/billing/portal", s.handleCreateBillingPortal)

	// Audit log
	r.Get("/audit-log", s.handleListAuditLog)
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
