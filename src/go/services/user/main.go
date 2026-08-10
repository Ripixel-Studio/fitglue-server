package main

import (
	"context"
	"log"
	"net"
	"os"
	"strconv"

	cloudFirestore "cloud.google.com/go/firestore"
	gcsstorage "cloud.google.com/go/storage"
	firebase "firebase.google.com/go/v4"
	firebaseAuth "firebase.google.com/go/v4/auth"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/internal/user"
	emailsender "github.com/fitglue/server/src/go/pkg/infrastructure/email"
	"github.com/fitglue/server/src/go/pkg/infrastructure/storage"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/user"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
)

// firebaseAuthWrapper wraps firebase.auth.Client to implement user.AuthClient.
// This is needed because firebase's Users() returns a concrete *firebaseAuth.UserIterator
// which we abstract behind our user.UserIterator interface.
type firebaseAuthWrapper struct {
	client *firebaseAuth.Client
}

func (f *firebaseAuthWrapper) GetUser(ctx context.Context, uid string) (*firebaseAuth.UserRecord, error) {
	return f.client.GetUser(ctx, uid)
}

func (f *firebaseAuthWrapper) EmailVerificationLinkWithSettings(ctx context.Context, email string, settings *firebaseAuth.ActionCodeSettings) (string, error) {
	return f.client.EmailVerificationLinkWithSettings(ctx, email, settings)
}

func (f *firebaseAuthWrapper) PasswordResetLinkWithSettings(ctx context.Context, email string, settings *firebaseAuth.ActionCodeSettings) (string, error) {
	return f.client.PasswordResetLinkWithSettings(ctx, email, settings)
}

func (f *firebaseAuthWrapper) Users(ctx context.Context, nextPageToken string) user.UserIterator {
	return f.client.Users(ctx, nextPageToken)
}

// Compile-time check that *firebaseAuth.UserIterator implements user.UserIterator.
var _ user.UserIterator = (*firebaseAuth.UserIterator)(nil)

// Compile-time check that *iterator.PageInfo is compatible.
var _ *iterator.PageInfo = (*iterator.PageInfo)(nil)

func main() {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	logger := infra.NewLoggerWithComponent("user")
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

	// Firestore Setup — use ADC on Cloud Run
	projectID := os.Getenv("GOOGLE_CLOUD_PROJECT")
	if projectID == "" {
		projectID = os.Getenv("PROJECT_ID")
	}
	fsClient, err := cloudFirestore.NewClient(ctx, projectID)
	if err != nil {
		logger.Error(ctx, "failed to initialize firestore client", "err", err)
		os.Exit(1)
	}

	// Email Sender Setup
	emailPass := os.Getenv("EMAIL_APP_PASSWORD")
	emailUser := os.Getenv("SYSTEM_EMAIL")
	if emailPass == "" || emailUser == "" {
		logger.Error(ctx, "EMAIL_APP_PASSWORD and SYSTEM_EMAIL must be set")
		os.Exit(1)
	}

	smtpPortStr := os.Getenv("EMAIL_SMTP_PORT")
	if smtpPortStr == "" {
		smtpPortStr = "465"
	}
	smtpPort, _ := strconv.Atoi(smtpPortStr)

	smtpHost := os.Getenv("EMAIL_SMTP_HOST")
	if smtpHost == "" {
		smtpHost = "smtp.gmail.com"
	}

	sender := emailsender.NewSMTPSender(smtpHost, smtpPort, emailUser, emailPass)
	store := user.NewFirestoreStore(fsClient)
	authWrapper := &firebaseAuthWrapper{client: authClient}

	baseURL := os.Getenv("BASE_URL")
	if baseURL == "" {
		baseURL = "https://fitglue.tech"
	}

	var purger user.UserArtifactPurger
	artifactBucket := os.Getenv("ARTIFACT_BUCKET")
	showcaseBucket := os.Getenv("SHOWCASE_ASSETS_BUCKET")
	if artifactBucket != "" && showcaseBucket != "" {
		gcsClient, err := gcsstorage.NewClient(ctx)
		if err != nil {
			logger.Error(ctx, "failed to create storage client for artifact purger", "err", err)
			os.Exit(1)
		}
		purger = user.NewArtifactPurger(&storage.StorageAdapter{Client: gcsClient}, artifactBucket, showcaseBucket, logger)
	} else {
		logger.Warn(ctx, "ARTIFACT_BUCKET/SHOWCASE_ASSETS_BUCKET unset — account deletion will not purge GCS artifacts")
	}

	svc := user.NewService(store, logger, sender, authWrapper, baseURL, purger)

	server := grpc.NewServer(grpc.UnaryInterceptor(infra.LoggingUnaryInterceptor(logger)))
	pbsvc.RegisterUserServiceServer(server, svc)

	healthcheck := health.NewServer()
	grpc_health_v1.RegisterHealthServer(server, healthcheck)

	lis, err := net.Listen("tcp", ":"+port)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	logger.Info(context.Background(), "Starting service.user", "port", port)
	if err := server.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
