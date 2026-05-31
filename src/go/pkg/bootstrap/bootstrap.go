package bootstrap

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"

	firebase "firebase.google.com/go/v4"
	"firebase.google.com/go/v4/auth"

	"github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/infrastructure/database"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	sentryPkg "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
	infrastorage "github.com/fitglue/server/src/go/pkg/infrastructure/storage"
)

// Config holds standard configuration for all services
type Config struct {
	ProjectID         string
	GCSArtifactBucket string
}

// Service holds initialized dependencies
type Service struct {
	DB     shared.Database
	Store  shared.BlobStore
	Pub    shared.Publisher
	Auth   *auth.Client
	Config *Config
}

// LoadConfig reads configuration from environment variables
func LoadConfig() *Config {
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = shared.ProjectID // Fallback
	}

	return &Config{
		ProjectID:         projectID,
		GCSArtifactBucket: os.Getenv("GCS_ARTIFACT_BUCKET"),
	}
}

// GetSlogHandlerOptions returns standard handler options for GCP.
// Delegates to infra.GCPHandlerOptions for the ReplaceAttr mapping.
func GetSlogHandlerOptions(level slog.Level) *slog.HandlerOptions {
	return infra.GCPHandlerOptions(level)
}

// InitLogger configures structured logging with Cloud Logging compatible keys
// Logger chain: JSONHandler -> ComponentHandler -> SentryHandler
func InitLogger() {
	opts := GetSlogHandlerOptions(slog.LevelInfo)
	jsonHandler := slog.NewJSONHandler(os.Stdout, opts)
	compHandler := &infra.ComponentHandler{Handler: jsonHandler}
	sentryHandler := sentryPkg.NewSentryHandler(compHandler)
	logger := slog.New(sentryHandler)
	slog.SetDefault(logger)
}

// NewLogger creates a configured logger instance
// Logger chain: JSONHandler -> ComponentHandler -> SentryHandler
func NewLogger(serviceName string, isDev bool) *slog.Logger {
	logLevelStr := os.Getenv("LOG_LEVEL")
	var level slog.Level
	switch strings.ToLower(logLevelStr) {
	case "debug":
		level = slog.LevelDebug
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}

	opts := GetSlogHandlerOptions(level)
	jsonHandler := slog.NewJSONHandler(os.Stdout, opts)
	compHandler := &infra.ComponentHandler{Handler: jsonHandler}
	sentryHandler := sentryPkg.NewSentryHandler(compHandler)
	return slog.New(sentryHandler).With("service", serviceName)
}

// NewService initializes all standard dependencies
func NewService(ctx context.Context) (*Service, error) {
	InitLogger()
	cfg := LoadConfig()
	logger := infra.NewLoggerWithComponent("bootstrap")

	logger.Info(ctx, "Initializing service", "project_id", cfg.ProjectID)

	// Firestore
	fsClient, err := firestore.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Error(ctx, "Firestore init failed", "error", err)
		return nil, fmt.Errorf("firestore init: %w", err)
	}

	// Pub/Sub - always use real publisher
	psClient, err := pubsub.NewClient(ctx, cfg.ProjectID)
	if err != nil {
		logger.Error(ctx, "PubSub init failed", "error", err)
		return nil, fmt.Errorf("pubsub init: %w", err)
	}
	pubAdapter := &infrapubsub.PubSubAdapter{Client: psClient, Logger: logger}
	logger.Info(ctx, "Pub/Sub initialized")

	// Storage
	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		logger.Error(ctx, "Storage init failed", "error", err)
		return nil, fmt.Errorf("storage init: %w", err)
	}

	// Firebase (for FCM and potentially other admin features)
	fbApp, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: cfg.ProjectID})
	if err != nil {
		logger.Error(ctx, "Firebase App init failed", "error", err)
		return nil, fmt.Errorf("firebase app init: %w", err)
	}

	// Firebase Auth (for user display name lookup)
	authClient, err := fbApp.Auth(ctx)
	if err != nil {
		logger.Warn(ctx, "Firebase Auth initialization failed", "error", err)
	}

	// Initialize Sentry
	sentryDSN := os.Getenv("SENTRY_DSN")
	environment := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if environment == "" {
		environment = "fitglue-server-dev"
	}
	release := os.Getenv("SENTRY_RELEASE")
	if release == "" {
		release = os.Getenv("K_REVISION")
		if release == "" {
			release = "unknown"
		}
	}
	serverName := os.Getenv("K_SERVICE")

	tracesSampleRate := 0.1
	if environment == "fitglue-server-dev" {
		tracesSampleRate = 1.0
	}

	if err := sentryPkg.Init(sentryPkg.Config{
		DSN:                sentryDSN,
		Environment:        environment,
		Release:            release,
		ServerName:         serverName,
		TracesSampleRate:   tracesSampleRate,
		ProfilesSampleRate: tracesSampleRate,
	}, slog.Default()); err != nil {
		// Log but don't fail - Sentry is optional
		logger.Warn(ctx, "Sentry initialization failed", "error", err)
	}

	return &Service{
		DB:    database.NewFirestoreAdapter(fsClient),
		Pub:   pubAdapter,
		Store: &infrastorage.StorageAdapter{Client: gcsClient},

		Auth:   authClient,
		Config: cfg,
	}, nil
}
