package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"

	"github.com/fitglue/server/src/go/services/destination/internal/destination"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/github"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/googlesheets"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/hevy"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/intervals"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/mock"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/showcase"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/strava"
	"github.com/fitglue/server/src/go/services/destination/internal/destination/uploaders/trainingpeaks"
)

func main() {
	logger := infra.NewLoggerWithComponent("destination")
	infra.InitSentry()
	ctx := context.Background()
	logger.Info(ctx, "Starting FitGlue Destination Service", "version", "v1")

	svc, err := bootstrap.NewService(ctx)
	if err != nil {
		logger.Error(ctx, "Failed to initialize bootstrap service", "error", err)
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

	showcaseAssetsBucket := os.Getenv("SHOWCASE_ASSETS_BUCKET")
	if showcaseAssetsBucket == "" {
		showcaseAssetsBucket = fmt.Sprintf("%s-showcase-assets", svc.Config.ProjectID)
	}

	// Setup Destination Registry & Executor
	registry := destination.NewRegistry()

	registry.Register(pbplugin.DestinationType_DESTINATION_STRAVA, strava.New(svc))
	registry.Register(pbplugin.DestinationType_DESTINATION_HEVY, hevy.New(svc))
	registry.Register(pbplugin.DestinationType_DESTINATION_TRAININGPEAKS, trainingpeaks.New(svc))
	registry.Register(pbplugin.DestinationType_DESTINATION_INTERVALS, intervals.New(svc))
	registry.Register(pbplugin.DestinationType_DESTINATION_GOOGLESHEETS, googlesheets.New(svc))
	registry.Register(pbplugin.DestinationType_DESTINATION_GITHUB, github.New(svc))
	registry.Register(pbplugin.DestinationType_DESTINATION_SHOWCASE, showcase.New(svc, activityClient, showcaseAssetsBucket))
	registry.Register(pbplugin.DestinationType_DESTINATION_MOCK, mock.New())

	executor := destination.NewUploadExecutor(registry, userClient, activityClient, svc.DB, svc.Store, svc.Pub, logger)

	// Create an HTTP handler to receive Pub/Sub pushes
	mux := http.NewServeMux()
	mux.HandleFunc("/", executor.HandlePubSubPush)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info(ctx, "Destination Service listening", "port", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), infra.LoggingMiddleware(logger, mux)); err != nil {
		logger.Error(ctx, "HTTP server failed", "error", err)
		os.Exit(1)
	}
}
