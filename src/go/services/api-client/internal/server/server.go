package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

// Publisher defines the interface for emitting events
type Publisher interface {
	PublishCloudEvent(ctx context.Context, topicID string, e event.Event) (string, error)
}

// ApiKeyStore provides persistence for ingress API keys generated during integration setup
type ApiKeyStore interface {
	CreateIngressKey(ctx context.Context, keyHash, userID, label string, scopes []string, createdAt time.Time) error
}

// GCSSigner generates short-lived signed download URLs for GCS objects.
type GCSSigner interface {
	SignedURL(ctx context.Context, bucketName, objectName, contentType string, contentLength int64, expiry time.Duration) (string, error)
}

// APIServer implements the HTTP router interfacing with FitGlue domain gRPC services
type APIServer struct {
	router         *chi.Mux
	logger         infra.Logger
	authClient     *auth.Client
	publisher      Publisher
	apiKeyStore    ApiKeyStore
	gcsSigner      GCSSigner // may be nil in test environments
	userService    userpb.UserServiceClient
	billingService billingpb.BillingServiceClient
	pipelineSvc    pipelinepb.PipelineServiceClient
	activitySvc    activitypb.ActivityServiceClient
	registrySvc    registrypb.RegistryServiceClient
}

// NewAPIServer constructs the application routing and API middleware stack
func NewAPIServer(
	logger infra.Logger,
	authClient *auth.Client,
	publisher Publisher,
	apiKeyStore ApiKeyStore,
	gcsSigner GCSSigner,
	userSvc userpb.UserServiceClient,
	billingSvc billingpb.BillingServiceClient,
	pipelineSvc pipelinepb.PipelineServiceClient,
	activitySvc activitypb.ActivityServiceClient,
	registrySvc registrypb.RegistryServiceClient,
) *APIServer {
	s := &APIServer{
		router:         chi.NewRouter(),
		logger:         logger,
		authClient:     authClient,
		publisher:      publisher,
		apiKeyStore:    apiKeyStore,
		gcsSigner:      gcsSigner,
		userService:    userSvc,
		billingService: billingSvc,
		pipelineSvc:    pipelineSvc,
		activitySvc:    activitySvc,
		registrySvc:    registrySvc,
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

	// Create common middleware for route logging and API parsing
	// s.router.Use(APILogger(s.logger))
	// s.router.Use(JSONResponseHeaders)

	// Legacy OAuth callback path — the Fitbit (and other) developer apps have
	// /auth/{provider}/callback registered from the pre-v2 architecture.
	// Firebase routes /auth/** to api-client, so we handle it here as an alias.
	s.router.Get("/auth/{provider}/callback", s.handleOAuthCallback)

	// API v2 block (Authenticated / CORS config / API routing)
	s.router.Route("/api/v2", func(r chi.Router) {
		r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			fmt.Fprint(w, `{"status": "ok"}`)
		})

		// Unauthenticated callback
		r.Get("/oauth/{provider}/callback", s.handleOAuthCallback)

		// Password reset doesn't require authentication
		r.Post("/auth-email/send-password-reset", s.handleSendPasswordResetEmail)

		// Integration request (unauthenticated - contact form)
		r.Post("/integrations/request", s.handleIntegrationRequest)

		// Plugin registry (unauthenticated - used by Skier build and marketing pages)
		r.Get("/registry", s.handleGetPluginRegistry)

		// Registry browse routes (unauthenticated - used by web app plugin browser)
		s.registerRegistryRoutes(r)

		// Register domain routes here
		r.Group(func(r chi.Router) {
			r.Use(AuthMiddleware(s.authClient))

			s.registerUserRoutes(r)
			s.registerBillingRoutes(r)
			s.registerPipelineRoutes(r)
			s.registerActivityRoutes(r)
			s.registerOAuthRoutes(r)
			s.registerRepostRoutes(r)
		})
	})
}
