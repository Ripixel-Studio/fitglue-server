// nolint:proto-json
package main

import (
	"context"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"os"
	"strings"

	"cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub"
	"cloud.google.com/go/storage"
	cloudevents "github.com/cloudevents/sdk-go/v2"
	cehttp "github.com/cloudevents/sdk-go/v2/protocol/http"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/internal/pipeline"
	"github.com/fitglue/server/src/go/internal/pipeline/enricher"
	"github.com/fitglue/server/src/go/internal/pipeline/router"
	"github.com/fitglue/server/src/go/internal/pipeline/splitter"
	infradb "github.com/fitglue/server/src/go/pkg/infrastructure/database"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	pb "github.com/fitglue/server/src/go/pkg/types/pb/services/pipeline"
	userpb "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"golang.org/x/net/http2"
	"golang.org/x/net/http2/h2c"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

func main() {
	logger := infra.NewLoggerWithComponent("pipeline")
	infra.InitSentry()
	ctx := context.Background()

	// Initialize dependencies
	fsClient, err := firestore.NewClient(ctx, os.Getenv("PROJECT_ID"))
	if err != nil {
		log.Fatalf("failed to init firestore: %v", err)
	}
	defer fsClient.Close()

	store := pipeline.NewFirestoreStore(fsClient)

	// In the new architecture, we use a real publisher and blob store
	rawPubClient, err := pubsub.NewClient(ctx, os.Getenv("PROJECT_ID"))
	if err != nil {
		log.Fatalf("failed to init pubsub: %v", err)
	}
	pubClient := &infrapubsub.PubSubAdapter{Client: rawPubClient, Logger: logger}

	gcsClient, err := storage.NewClient(ctx)
	if err != nil {
		log.Fatalf("failed to init gcs: %v", err)
	}

	blobStore := NewGCSBlobStore(gcsClient)
	bucketName := os.Getenv("ARTIFACT_BUCKET")
	if bucketName == "" {
		bucketName = "fitglue-server-dev-artifacts"
	}

	// Connect to user service for source credential lookups (backfill feature)
	userTarget := os.Getenv("USER_SERVICE_URL")
	if userTarget == "" {
		userTarget = "localhost:50051"
	}
	userConn, err := infra.GRPCDial(userTarget)
	if err != nil {
		log.Fatalf("failed to dial user service: %v", err)
	}
	defer userConn.Close()
	userClient := userpb.NewUserServiceClient(userConn)

	// Full database adapter (for cross-user queries used by the parkrun checker)
	db := infradb.NewFirestoreAdapter(fsClient)

	// 1. gRPC Service (CRUD) — serves on the same port as HTTP (required for Cloud Run single-port)
	svc := pipeline.NewService(store, pubClient, blobStore, logger, userClient)

	grpcServer := grpc.NewServer(grpc.UnaryInterceptor(infra.LoggingUnaryInterceptor(logger)))
	pb.RegisterPipelineServiceServer(grpcServer, svc)

	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthcheck)

	// 2. HTTP Server for Pub/Sub Pushes
	splitterSvc := splitter.NewSplitter(store, pubClient, logger)
	routerSvc := router.NewRouter(store, pubClient, blobStore, bucketName, logger)

	mux := http.NewServeMux()
	parkrunChecker := pipeline.NewParkrunChecker(db, svc, logger, pubClient)

	mux.HandleFunc("/pubsub/raw", handlePubSubPush(logger, splitterSvc.SplitByPipeline))
	mux.HandleFunc("/pubsub/run", enricher.EnrichActivityHTTP)
	mux.HandleFunc("/pubsub/enriched", handlePubSubPush(logger, routerSvc.RouteActivity))
	mux.HandleFunc("/pubsub/parkrun-check", parkrunChecker.HandleCheck)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	loggingMux := infra.LoggingMiddleware(logger, mux)

	// 3. Unified handler: route gRPC vs HTTP based on content-type
	// Cloud Run only exposes a single port, so both must share it.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.ProtoMajor == 2 && strings.HasPrefix(r.Header.Get("Content-Type"), "application/grpc") {
			grpcServer.ServeHTTP(w, r)
		} else {
			loggingMux.ServeHTTP(w, r)
		}
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	// Use h2c so gRPC works without TLS (Cloud Run terminates TLS at the load balancer)
	h2s := &http2.Server{}

	logger.Info(ctx, "Starting service.pipeline (gRPC + HTTP)", "port", port)
	if err := http.ListenAndServe(":"+port, h2c.NewHandler(handler, h2s)); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}

// handlePubSubPush wraps a domain function taking (ctx, cloudevents.Event) with HTTP Pub/Sub Push unmarshalling
func handlePubSubPush(logger infra.Logger, handler func(context.Context, cloudevents.Event) error) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx := r.Context()

		// Try standard CloudEvents HTTP first
		event, err := cehttp.NewEventFromHTTPRequest(r)
		if err != nil {
			// Fallback: This might be wrapped in a Pub/Sub push payload {"message": {"data": "...", ...}}
			body, ioErr := io.ReadAll(r.Body)
			if ioErr != nil {
				logger.Error(ctx, "Failed to read request body", "error", ioErr)
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}
			defer r.Body.Close()

			var pushMsg struct {
				Message struct {
					Data       []byte            `json:"data"`
					MessageID  string            `json:"messageId"`
					Attributes map[string]string `json:"attributes"`
				} `json:"message"`
			}
			if unmarshalErr := json.Unmarshal(body, &pushMsg); unmarshalErr != nil || len(pushMsg.Message.Data) == 0 {
				logger.Error(ctx, "Failed to parse Pub/Sub push payload", "error", unmarshalErr)
				http.Error(w, "Bad Request", http.StatusBadRequest)
				return
			}

			// The inner data should be our serialized CloudEvent
			parsedEvent := cloudevents.NewEvent()
			if jsonErr := json.Unmarshal(pushMsg.Message.Data, &parsedEvent); jsonErr != nil || parsedEvent.Type() == "" {
				// Very raw payload, encapsulate it
				parsedEvent.SetID(pushMsg.Message.MessageID)
				parsedEvent.SetSource("pubsub-push")
				parsedEvent.SetType("com.fitglue.unknown")
				parsedEvent.SetData(cloudevents.ApplicationJSON, pushMsg.Message.Data)
			}
			event = &parsedEvent
		}

		if err := handler(ctx, *event); err != nil {
			logger.Error(ctx, "Handler failed processing event", "error", err, "eventId", event.ID())
			// Return 500 so Pub/Sub retries NACKs
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"status":"ok"}`))
	}
}
