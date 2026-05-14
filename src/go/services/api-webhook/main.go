package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	firebase "firebase.google.com/go/v4"
	"github.com/fitglue/server/src/go/internal/infra"
	infradb "github.com/fitglue/server/src/go/pkg/infrastructure/database"
	infraps "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/server"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/fitbit"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/github"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/hevy"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/intervals"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/mobile"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/mock"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/oura"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/parkrun"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/polar"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/strava"
	"github.com/fitglue/server/src/go/services/api-webhook/internal/webhook/sources/wahoo"

	"google.golang.org/api/option"
)

func main() {
	logger := infra.NewLoggerWithComponent("api-webhook")
	infra.InitSentry()
	ctx := context.Background()
	logger.Info(ctx, "Starting FitGlue Webhook API Gateway", "version", "v1")

	var fbApp *firebase.App
	var err error
	if creds := os.Getenv("GOOGLE_APPLICATION_CREDENTIALS"); creds != "" {
		fbApp, err = firebase.NewApp(ctx, nil, option.WithCredentialsFile(creds))
	} else {
		fbApp, err = firebase.NewApp(ctx, nil)
	}

	if err != nil {
		logger.Error(ctx, "Failed to initialize Firebase App", "error", err)
		os.Exit(1)
	}

	authClient, err := fbApp.Auth(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to initialize Firebase Auth client", "error", err)
		os.Exit(1)
	}

	// Setup gRPC Connections to dependent Domain Services
	userServiceURL := os.Getenv("USER_SERVICE_URL")
	if userServiceURL == "" {
		userServiceURL = "localhost:50051"
	}
	userConn, err := infra.GRPCDial(userServiceURL)
	if err != nil {
		logger.Error(ctx, "Failed to connect to User Service", "url", userServiceURL, "error", err)
		os.Exit(1)
	}
	defer userConn.Close()
	userClient := userpb.NewUserServiceClient(userConn)

	pipelineServiceURL := os.Getenv("PIPELINE_SERVICE_URL")
	if pipelineServiceURL == "" {
		pipelineServiceURL = "localhost:50053"
	}
	pipelineConn, err := infra.GRPCDial(pipelineServiceURL)
	if err != nil {
		logger.Error(ctx, "Failed to connect to Pipeline Service", "url", pipelineServiceURL, "error", err)
		os.Exit(1)
	}
	defer pipelineConn.Close()
	pipelineClient := pipelinepb.NewPipelineServiceClient(pipelineConn)

	activityServiceURL := os.Getenv("ACTIVITY_SERVICE_URL")
	if activityServiceURL == "" {
		activityServiceURL = "localhost:50054"
	}
	activityConn, err := infra.GRPCDial(activityServiceURL)
	if err != nil {
		logger.Error(ctx, "Failed to connect to Activity Service", "url", activityServiceURL, "error", err)
		os.Exit(1)
	}
	defer activityConn.Close()
	activityClient := activitypb.NewActivityServiceClient(activityConn)

	billingServiceURL := os.Getenv("BILLING_SERVICE_URL")
	if billingServiceURL == "" {
		billingServiceURL = "localhost:50052"
	}
	billingConn, err := infra.GRPCDial(billingServiceURL)
	if err != nil {
		logger.Error(ctx, "Failed to connect to Billing Service", "url", billingServiceURL, "error", err)
		os.Exit(1)
	}
	defer billingConn.Close()
	billingClient := billingpb.NewBillingServiceClient(billingConn)

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

	// Setup Firestore for bounceback detection
	fsClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		logger.Error(ctx, "Failed to initialize Firestore client", "error", err)
		os.Exit(1)
	}
	defer fsClient.Close()
	db := infradb.NewFirestoreAdapter(fsClient)

	// Instantiate Webhook Processor & Providers
	processor := webhook.NewProcessor(logger, userClient, publisher, db)

	stravaToken := os.Getenv("STRAVA_WEBHOOK_VERIFY_TOKEN")
	processor.Register(strava.NewProvider(stravaToken))
	processor.Register(intervals.NewProvider())

	fitbitToken := os.Getenv("FITBIT_SUBSCRIBER_VERIFICATION_TOKEN")
	fitbitClientSecret := os.Getenv("FITBIT_OAUTH_CLIENT_SECRET")
	processor.Register(fitbit.NewProvider(fitbitToken, fitbitClientSecret))
	processor.Register(hevy.NewProvider())
	processor.Register(oura.NewProvider())
	processor.Register(github.NewProvider())
	processor.Register(wahoo.NewProvider())
	processor.Register(polar.NewProvider())
	processor.Register(mobile.NewProvider())
	if os.Getenv("ENABLE_MOCK_PROVIDER") == "true" {
		processor.Register(mock.NewProvider())
	}
	processor.Register(parkrun.NewProvider())

	// Initialize the HTTP Gateway Server
	apiServer := server.NewAPIServer(
		logger,
		authClient,
		userClient,
		billingClient,
		pipelineClient,
		activityClient,
		processor,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info(ctx, "Webhook Gateway listening", "port", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), infra.LoggingMiddleware(logger, apiServer)); err != nil {
		logger.Error(ctx, "HTTP server failed", "error", err)
		os.Exit(1)
	}
}
