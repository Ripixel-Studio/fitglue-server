package main

import (
	"context"
	"log"
	"net"
	"os"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	"github.com/fitglue/server/src/go/internal/billing"
	"github.com/fitglue/server/src/go/internal/infra"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081" // Different port to avoid conflict
	}

	logger := infra.NewLoggerWithComponent("billing")
	infra.InitSentry()
	ctx := context.Background()

	// Firestore Setup
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
	store := billing.NewFirestoreStore(fsClient)

	stripeSecret := os.Getenv("STRIPE_SECRET_KEY")
	webhookSecret := os.Getenv("STRIPE_WEBHOOK_SECRET")
	priceID := os.Getenv("STRIPE_PRICE_ID")

	if stripeSecret == "" || webhookSecret == "" || priceID == "" {
		logger.Error(context.Background(), "STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET, and STRIPE_PRICE_ID must be set")
		os.Exit(1)
	}

	stripeClient := billing.NewLiveStripeClient(stripeSecret)

	rawPubClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		logger.Error(ctx, "Failed to initialize Pub/Sub client", "error", err)
		os.Exit(1)
	}
	defer rawPubClient.Close()
	publisher := &infrapubsub.PubSubAdapter{Client: rawPubClient, Logger: logger}

	svc := billing.NewService(store, logger, stripeClient, publisher, priceID, webhookSecret)

	server := grpc.NewServer(grpc.UnaryInterceptor(infra.LoggingUnaryInterceptor(logger)))
	pbsvc.RegisterBillingServiceServer(server, svc)

	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthcheck)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	logger.Info(context.Background(), "Starting service.billing", "port", port)
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
