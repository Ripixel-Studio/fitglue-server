package main

import (
	"context"
	"log"
	"net/http"
	"os"

	"cloud.google.com/go/pubsub"
	gcstorage "cloud.google.com/go/storage"
	"github.com/fitglue/server/src/go/internal/infra"
	infraps "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	infrastorage "github.com/fitglue/server/src/go/pkg/infrastructure/storage"
	"github.com/fitglue/server/src/go/services/api-client/internal/server"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
	"google.golang.org/grpc"

	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger := infra.NewLoggerWithComponent("api-client")
	infra.InitSentry()
	ctx := context.Background()

	// Firebase Auth Setup — use credentials file if set (local dev), otherwise ADC (Cloud Run)
	var app *firebase.App
	var err error
	if creds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); creds != "" {
		app, err = firebase.NewApp(ctx, nil, option.WithCredentialsFile(creds))
	} else {
		app, err = firebase.NewApp(ctx, nil)
	}
	if err != nil {
		logger.Error(ctx, "failed to initialize firebase app", "err", err)
		os.Exit(1)
	}

	authClient, err := app.Auth(ctx)
	if err != nil {
		logger.Error(ctx, "failed to initialize firebase auth client", "err", err)
		os.Exit(1)
	}

	// Helper to dial a service
	connect := func(targetEnv string, defaultTarget string) *grpc.ClientConn {
		target := os.Getenv(targetEnv)
		if target == "" {
			target = defaultTarget
		}
		conn, err := infra.GRPCDial(target)
		if err != nil {
			logger.Error(ctx, "failed to configure grpc client", "target", target, "error", err)
			os.Exit(1)
		}
		return conn
	}

	userConn := connect("USER_SERVICE_URL", "localhost:50051")
	defer userConn.Close()
	userClient := userpb.NewUserServiceClient(userConn)

	billingConn := connect("BILLING_SERVICE_URL", "localhost:50052")
	defer billingConn.Close()
	billingClient := billingpb.NewBillingServiceClient(billingConn)

	pipelineConn := connect("PIPELINE_SERVICE_URL", "localhost:50053")
	defer pipelineConn.Close()
	pipelineClient := pipelinepb.NewPipelineServiceClient(pipelineConn)

	activityConn := connect("ACTIVITY_SERVICE_URL", "localhost:50054")
	defer activityConn.Close()
	activityClient := activitypb.NewActivityServiceClient(activityConn)

	registryConn := connect("REGISTRY_SERVICE_URL", "localhost:50055")
	defer registryConn.Close()
	registryClient := registrypb.NewRegistryServiceClient(registryConn)

	// Setup Pub/Sub Client
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = "fitglue" // Fallback or development default
	}
	pubsubClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		logger.Error(ctx, "Failed to initialize Pub/Sub client", "error", err)
		os.Exit(1)
	}
	defer pubsubClient.Close()
	publisher := &infraps.PubSubAdapter{Client: pubsubClient, Logger: logger}

	// Setup Firestore Client for API key storage
	firestoreClient, err := app.Firestore(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to initialize Firestore client", "error", err)
		os.Exit(1)
	}
	defer firestoreClient.Close()
	apiKeyStore := server.NewFirestoreApiKeyStore(firestoreClient)

	// Setup GCS client for signed payload download URLs
	gcsClient, err := gcstorage.NewClient(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to initialize GCS client", "error", err)
		os.Exit(1)
	}
	defer gcsClient.Close()
	gcsSigner := &infrastorage.StorageAdapter{Client: gcsClient}

	// Build API Gateway router
	apiServer := server.NewAPIServer(
		logger,
		authClient,
		publisher,
		apiKeyStore,
		gcsSigner,
		userClient,
		billingClient,
		pipelineClient,
		activityClient,
		registryClient,
	)

	logger.Info(ctx, "Starting service.api.client", "port", port)

	if err := http.ListenAndServe(":"+port, infra.LoggingMiddleware(logger, apiServer)); err != nil {
		log.Fatalf("failed to serve HTTP: %v", err)
	}
}
