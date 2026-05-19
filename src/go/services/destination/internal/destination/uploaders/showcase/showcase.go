package showcase

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/tier"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	activitypb "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	HobbyistRetentionDays = 30
)

// Uploader implements destination.Destination for Showcase
type Uploader struct {
	svc            *bootstrap.Service
	activityClient activitypb.ActivityServiceClient
}

// New returns a new Showcase Uploader initialized with dependencies.
func New(svc *bootstrap.Service, activityClient activitypb.ActivityServiceClient) *Uploader {
	return &Uploader{
		svc:            svc,
		activityClient: activityClient,
	}
}

// Name returns the identifier for this uploader
func (u *Uploader) Name() string {
	return "showcase"
}

func slugify(s string) string {
	s = strings.ToLower(s)
	s = strings.ReplaceAll(s, " ", "-")
	s = strings.ReplaceAll(s, "_", "-")
	reg := regexp.MustCompile(`[^a-z0-9-]`)
	s = reg.ReplaceAllString(s, "")
	reg = regexp.MustCompile(`-+`)
	s = reg.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 30 {
		s = s[:30]
		s = strings.TrimRight(s, "-")
	}
	return s
}

func generateRandomSuffix() string {
	b := make([]byte, 3)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func (u *Uploader) generateShowcaseID(ctx context.Context, title string, startTime time.Time) (string, error) {
	slug := slugify(title)
	if slug == "" {
		slug = "activity"
	}

	dateStr := startTime.Format("2006-01-02")
	baseID := fmt.Sprintf("%s-%s", slug, dateStr)

	exists, err := u.svc.DB.ShowcaseActivityExists(ctx, baseID)
	if err != nil {
		return "", fmt.Errorf("failed to check showcase ID existence: %w", err)
	}

	if !exists {
		return baseID, nil
	}

	for i := 0; i < 5; i++ {
		suffix := generateRandomSuffix()
		candidateID := fmt.Sprintf("%s-%s", baseID, suffix)

		exists, err = u.svc.DB.ShowcaseActivityExists(ctx, candidateID)
		if err != nil {
			return "", fmt.Errorf("failed to check showcase ID existence: %w", err)
		}

		if !exists {
			return candidateID, nil
		}
	}

	return "", fmt.Errorf("failed to generate unique showcase ID after 5 attempts")
}

func calculateExpiration(userRec *user.Record, createdAt time.Time) *time.Time {
	if tier.GetEffectiveTier(userRec) == tier.TierAthlete {
		return nil
	}
	expiry := createdAt.AddDate(0, 0, HobbyistRetentionDays)
	return &expiry
}

// resolveOwnerDisplayName determines the display name for the showcase activity owner.
// Fallback chain: User Profile name → Firebase Auth display name → email prefix → showcase profile → "FitGlue Athlete".
func (u *Uploader) resolveOwnerDisplayName(ctx context.Context, userID string, userRec *user.Record, logger *slog.Logger) string {
	// 1. User's FitGlue profile display name (explicitly set by the user)
	if userRec != nil && userRec.UserProfile != nil && userRec.UserProfile.DisplayName != "" {
		return userRec.UserProfile.DisplayName
	}

	// 2. Firebase Auth display name / email prefix
	if u.svc.Auth != nil {
		authUser, err := u.svc.Auth.GetUser(ctx, userID)
		if err != nil {
			logger.Warn("Failed to fetch user from Firebase Auth for display name", "error", err, "userId", userID)
		} else if authUser != nil {
			if authUser.DisplayName != "" {
				return authUser.DisplayName
			}
			if authUser.Email != "" {
				if atIdx := strings.Index(authUser.Email, "@"); atIdx > 0 {
					return authUser.Email[:atIdx]
				}
				return authUser.Email
			}
		}
	}

	// 3. Showcase Profile display name
	if profile, err := u.svc.DB.GetShowcaseProfileByUserId(ctx, userID); err == nil && profile != nil && profile.DisplayName != "" {
		return profile.DisplayName
	}

	return "FitGlue Athlete"
}

// Create uploads a new activity to Showcase
func (u *Uploader) Create(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record) (string, error) {
	logger := slog.Default()

	var startTime time.Time
	if payload.Timestamp != nil {
		startTime = payload.Timestamp.AsTime()
	} else {
		startTime = time.Now()
	}

	activityName := payload.Metadata["activity_name"]
	if activityName == "" {
		activityName = "Activity"
	}

	showcaseID, err := u.generateShowcaseID(ctx, activityName, startTime)
	if err != nil {
		return "", fmt.Errorf("failed to generate showcase ID: %w", err)
	}

	createdAt := time.Now()
	expiresAt := calculateExpiration(userRec, createdAt)

	activityTypeVal, ok := pbactivity.ActivityType_value[payload.Metadata["activity_type"]]
	activityType := pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED
	if ok {
		activityType = pbactivity.ActivityType(activityTypeVal)
	}

	appliedEnrichmentsStr := payload.Metadata["applied_enrichments"]
	var appliedEnrichments []string
	if appliedEnrichmentsStr != "" {
		appliedEnrichments = strings.Split(appliedEnrichmentsStr, ",")
	}

	tagsStr := payload.Metadata["tags"]
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}

	var photoURLs []string
	if photoURLsStr := payload.Metadata["photo_urls"]; photoURLsStr != "" {
		photoURLs = strings.Split(photoURLsStr, ",")
	}

	showcasedActivity := &pbactivity.ShowcasedActivity{
		ShowcaseId:          showcaseID,
		ActivityId:          payload.GetActivityId(),
		UserId:              payload.UserId,
		Title:               activityName,
		Description:         payload.Metadata["description"],
		ActivityType:        activityType,
		Source:              payload.Source,
		StartTime:           timestamppb.New(startTime),
		ActivityData:        nil,
		FitFileUri:          payload.Metadata["fit_file_uri"],
		AppliedEnrichments:  appliedEnrichments,
		Tags:                tags,
		PipelineExecutionId: payload.PipelineExecutionId,
		CreatedAt:           timestamppb.New(createdAt),
		ExpiresAt:           nil,
		OwnerDisplayName:    u.resolveOwnerDisplayName(ctx, payload.UserId, userRec, logger),
		PhotoUrls:           photoURLs,
	}

	if uri, ok := payload.Metadata["activity_data_uri"]; ok && uri != "" {
		showcasedActivity.ActivityDataUri = uri
	} else {
		logger.Warn("No ActivityDataUri available - showcase will be missing activity data",
			"showcase_id", showcaseID,
			"activity_id", payload.ActivityId,
		)
	}

	if expiresAt != nil {
		showcasedActivity.ExpiresAt = timestamppb.New(*expiresAt)
	}

	if err := u.svc.DB.SetShowcasedActivity(ctx, showcasedActivity); err != nil {
		return "", fmt.Errorf("failed to persist showcased activity: %w", err)
	}

	// Delegate profile entry + stats management to the activity service.
	// AddShowcaseEntry handles: GCS hydration, profile entry creation, and stats aggregation.
	if _, err := u.activityClient.AddShowcaseEntry(ctx, &activitypb.AddShowcaseEntryRequest{
		UserId:     payload.UserId,
		ShowcaseId: showcaseID,
		// destination_count is not available from ActivityPayload (no Destinations field);
		// the Update path will set it correctly from the pipeline run.
	}); err != nil {
		logger.Warn("Failed to add showcase profile entry via activity service", "error", err,
			"showcase_id", showcaseID, "user_id", payload.UserId)
	}

	return showcaseID, nil
}

// Update modifies an existing Showcase activity and profile entry
func (u *Uploader) Update(ctx context.Context, payload *pbevents.ActivityPayload, userRec *user.Record, pipelineRun *pbpipeline.PipelineRun) error {
	logger := slog.Default()

	var showcaseID string
	if pipelineRun != nil {
		for _, dest := range pipelineRun.Destinations {
			if dest.Destination == pbplugin.DestinationType_DESTINATION_SHOWCASE && dest.ExternalId != nil && *dest.ExternalId != "" {
				showcaseID = *dest.ExternalId
				break
			}
		}
	}

	if showcaseID == "" {
		return fmt.Errorf("no Showcase destination found in pipeline run")
	}

	createdAt := time.Now()
	expiresAt := calculateExpiration(userRec, createdAt)

	var startTime time.Time
	if payload.Timestamp != nil {
		startTime = payload.Timestamp.AsTime()
	} else {
		startTime = time.Now()
	}

	activityName := payload.Metadata["activity_name"]
	if activityName == "" {
		activityName = "Activity"
	}

	activityTypeVal, ok := pbactivity.ActivityType_value[payload.Metadata["activity_type"]]
	activityType := pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED
	if ok {
		activityType = pbactivity.ActivityType(activityTypeVal)
	}

	appliedEnrichmentsStr := payload.Metadata["applied_enrichments"]
	var appliedEnrichments []string
	if appliedEnrichmentsStr != "" {
		appliedEnrichments = strings.Split(appliedEnrichmentsStr, ",")
	}

	tagsStr := payload.Metadata["tags"]
	var tags []string
	if tagsStr != "" {
		tags = strings.Split(tagsStr, ",")
	}

	var updatePhotoURLs []string
	if photoURLsStr := payload.Metadata["photo_urls"]; photoURLsStr != "" {
		updatePhotoURLs = strings.Split(photoURLsStr, ",")
	}

	showcasedActivity := &pbactivity.ShowcasedActivity{
		ShowcaseId:          showcaseID,
		ActivityId:          payload.GetActivityId(),
		UserId:              payload.UserId,
		Title:               activityName,
		Description:         payload.Metadata["description"],
		ActivityType:        activityType,
		Source:              payload.Source,
		StartTime:           timestamppb.New(startTime),
		ActivityData:        nil,
		FitFileUri:          payload.Metadata["fit_file_uri"],
		AppliedEnrichments:  appliedEnrichments,
		Tags:                tags,
		PipelineExecutionId: payload.PipelineExecutionId,
		CreatedAt:           timestamppb.New(createdAt),
		OwnerDisplayName:    u.resolveOwnerDisplayName(ctx, payload.UserId, userRec, logger),
		PhotoUrls:           updatePhotoURLs,
	}

	if uri, ok := payload.Metadata["activity_data_uri"]; ok && uri != "" {
		showcasedActivity.ActivityDataUri = uri
	}

	if expiresAt != nil {
		showcasedActivity.ExpiresAt = timestamppb.New(*expiresAt)
	}

	if err := u.svc.DB.SetShowcasedActivity(ctx, showcasedActivity); err != nil {
		return fmt.Errorf("failed to persist updated showcased activity: %w", err)
	}

	// Delegate profile entry + stats management to the activity service.
	// AddShowcaseEntry handles: GCS hydration, profile entry creation, and stats aggregation.
	destCount := int32(0)
	if pipelineRun != nil {
		destCount = int32(len(pipelineRun.Destinations))
	}
	if _, err := u.activityClient.AddShowcaseEntry(ctx, &activitypb.AddShowcaseEntryRequest{
		UserId:           payload.UserId,
		ShowcaseId:       showcaseID,
		DestinationCount: destCount,
	}); err != nil {
		logger.Warn("Failed to update showcase profile entry via activity service", "error", err,
			"showcase_id", showcaseID, "user_id", payload.UserId)
	}

	return nil
}
