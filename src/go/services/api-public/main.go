package main

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/fitglue/server/src/go/internal/infra"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	registrypb "github.com/fitglue/server/src/go/pkg/types/pb/services/registry"
	"github.com/fitglue/server/src/go/services/api-public/internal/server"
)

func main() {
	logger := infra.NewLoggerWithComponent("api-public")
	infra.InitSentry()
	ctx := context.Background()
	logger.Info(ctx, "Starting FitGlue Public API Gateway", "version", "v1")

	// 1. Setup gRPC Connections to dependent Domain Services
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

	registryServiceURL := os.Getenv("REGISTRY_SERVICE_URL")
	if registryServiceURL == "" {
		registryServiceURL = "localhost:50055"
	}
	registryConn, err := infra.GRPCDial(registryServiceURL)
	if err != nil {
		logger.Error(ctx, "Failed to connect to Registry Service", "url", registryServiceURL, "error", err)
		os.Exit(1)
	}
	defer registryConn.Close()
	registryClient := registrypb.NewRegistryServiceClient(registryConn)

	// 2. Initialize the HTTP Gateway Server
	viewSalt := os.Getenv("VIEW_SALT_SECRET")
	if viewSalt == "" {
		logger.Warn(ctx, "VIEW_SALT_SECRET is not set — showcase view de-duplication is disabled")
	}
	apiServer := server.NewAPIServer(
		logger,
		activityClient,
		registryClient,
		viewSalt,
	)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger.Info(ctx, "Public API Gateway listening", "port", port)
	if err := http.ListenAndServe(fmt.Sprintf(":%s", port), infra.LoggingMiddleware(logger, apiServer)); err != nil {
		logger.Error(ctx, "HTTP server failed", "error", err)
		os.Exit(1)
	}
}
