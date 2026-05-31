package main

import (
	"context"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	"github.com/fitglue/server/src/go/internal/billing"
	"github.com/fitglue/server/src/go/internal/infra"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/billing"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}

	logger := infra.NewLoggerWithComponent("billing")
	infra.InitSentry()
	ctx := context.Background()

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
		logger.Error(ctx, "STRIPE_SECRET_KEY, STRIPE_WEBHOOK_SECRET, and STRIPE_PRICE_ID must be set")
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
	trialChecker := billing.NewTrialChecker(store, publisher, logger)

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(infra.LoggingUnaryInterceptor(logger)))
	pbsvc.RegisterBillingServiceServer(grpcServer, svc)
	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthcheck)

	mux := http.NewServeMux()
	mux.HandleFunc("/pubsub/trial-check", trialChecker.HandleTrialCheck)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	loggingMux := infra.LoggingMiddleware(logger, mux)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			loggingMux.ServeHTTP(w, r)
		}
	})

	h2s := &http2.Server{}
	logger.Info(ctx, "Starting service.billing (gRPC + HTTP)", "port", port)
	if err := http.ListenAndServe(":"+port, h2c.NewHandler(handler, h2s)); err != nil {
		logger.Error(ctx, "serve failed", "error", err)
		os.Exit(1)
	}
}
