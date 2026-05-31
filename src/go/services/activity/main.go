package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strings"

	firebase "firebase.google.com/go/v4"
	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	"github.com/fitglue/server/src/go/internal/activity"
	"github.com/fitglue/server/src/go/internal/infra"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	"github.com/fitglue/server/src/go/pkg/infrastructure/notifications"
	gcsstorage "github.com/fitglue/server/src/go/pkg/infrastructure/storage"
	pb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	grpc_health_v1 "google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8084"
	}

	ctx := context.Background()
	logger := infra.NewLoggerWithComponent("activity")
	infra.InitSentry()

	projectID := os.Getenv("PROJECT_ID")
	if projectID == "" {
		projectID = "fitglue-server-dev"
	}

	fsClient, err := firestore.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to init firestore: %v", err)
	}
	defer fsClient.Close()
	store := activity.NewFirestoreStore(fsClient)

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to init gcs: %v", err)
	}
	defer gcsClient.Close()
	blobStore := &GCSBlobStore{adapter: &gcsstorage.StorageAdapter{Client: gcsClient}}

	pubsubClient, err := pubsub.NewClient(ctx, projectID)
	if err != nil {
		log.Fatalf("failed to init pubsub: %v", err)
	}
	defer pubsubClient.Close()
	pub := &infrapubsub.PubSubAdapter{Client: pubsubClient, Logger: logger}

	bucketName := os.Getenv("ARTIFACT_BUCKET")
	if bucketName == "" {
		bucketName = "fitglue-server-dev-artifacts"
	}
	showcaseAssetsBucket := os.Getenv("SHOWCASE_ASSETS_BUCKET")
	if showcaseAssetsBucket == "" {
		showcaseAssetsBucket = "fitglue-server-dev-showcase-assets"
	}

	// FCM — non-fatal if unavailable; roundup notifications are best-effort
	var notifSvc *notifications.FCMAdapter
	fbApp, err := firebase.NewApp(ctx, &firebase.Config{ProjectID: projectID})
	if err != nil {
		logger.Warn(ctx, "Firebase init failed — roundup notifications disabled", "error", err)
	} else {
		notifSvc, err = notifications.NewFCMAdapter(ctx, fbApp, fsClient, logger)
		if err != nil {
			logger.Warn(ctx, "FCM init failed — roundup notifications disabled", "error", err)
		}
	}

	svc := activity.NewService(store, blobStore, pub, bucketName, showcaseAssetsBucket, logger, notifSvc)

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(infra.LoggingUnaryInterceptor(logger)))
	pb.RegisterActivityServiceServer(grpcServer, svc)
	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthcheck)

	mux := http.NewServeMux()
	mux.HandleFunc("/pubsub/roundup", svc.HandleRoundupTrigger)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
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
	logger.Info(ctx, "Starting service.activity (gRPC + HTTP)", "port", port)
	if err := http.ListenAndServe(":"+port, h2c.NewHandler(handler, h2s)); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
