package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"cloud.google.com/go/firestore"
	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	billingpb "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	pipelinepb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"github.com/fitglue/server/src/go/services/api-admin/internal/server"

	firebase "firebase.google.com/go/v4"
	"google.golang.org/api/option"
)

func main() {
	logger := infra.NewLoggerWithComponent("api-admin")
	infra.InitSentry()
	ctx := context.Background()

	logger.Info(ctx, "Starting FitGlue Admin API Gateway", "version", "v1")

	// 1. Initialize Firebase Auth for verifying Admin JWTs
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

	// 2. Setup gRPC Connections to dependent Domain Services
	userServiceURL := os.Getenv("USER_SERVICE_URL")
	if userServiceURL == "" {
		userServiceURL = "localhost:50051" // Default for local dev
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

	// 3. Initialize Firestore for admin stats queries
	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		projectID = os.Getenv("GOOGLE_CLOUD_PROJECT")
	}
	fsClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		logger.Error(ctx, "Failed to initialize Firestore client", "error", err)
		os.Exit(1)
	}
	defer fsClient.Close()

	// 4. Initialize the HTTP Gateway Server
	apiServer := server.NewAPIServer(
		logger,
		authClient,
		userClient,
		pipelineClient,
		activityClient,
		billingClient,
		fsClient,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info(ctx, "Admin API Gateway listening", "port", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), infra.LoggingMiddleware(logger, apiServer)); err != nil {
		logger.Error(ctx, "HTTP server failed", "error", err)
		os.Exit(1)
	}
}
