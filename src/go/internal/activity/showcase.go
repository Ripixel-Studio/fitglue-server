package activity

import (
	"context"
	"fmt"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

func (s *Service) GetShowcase(ctx context.Context, req *pbsvc.GetShowcaseRequest) (*pbactivity.ShowcasedActivity, error) {
	if req.UserId == "" || req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase_id are required")
	}

	showcase, err := s.store.GetShowcase(ctx, req.UserId, req.ShowcaseId)
	if err != nil {
		s.logger.Error(ctx, "failed to get showcase", "error", err)
		return nil, status.Error(codes.Internal, "failed to read showcase")
	}
	if showcase == nil {
		return nil, status.Error(codes.NotFound, "showcase not found")
	}

	// Fetch data from GCS if unloaded
	// The GCS blob is a full EnrichedActivityEvent (stored by PrepareForPublish),
	// so we unmarshal it as such and extract just the ActivityData field.
	if showcase.ActivityData == nil && showcase.ActivityDataUri != "" {
		data, err := s.blobStore.Get(ctx, "", showcase.ActivityDataUri)
		if err == nil && len(data) > 0 {
			var fullEvent pbevents.EnrichedActivityEvent
			unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := unmarshalOpts.Unmarshal(data, &fullEvent); err == nil {
				showcase.ActivityData = fullEvent.ActivityData
			}
		}
	}

	return showcase, nil
}

func (s *Service) ListShowcases(ctx context.Context, req *pbsvc.ListShowcasesRequest) (*pbsvc.ListShowcasesResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	showcases, err := s.store.ListShowcases(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list showcases", "error", err)
		return nil, status.Error(codes.Internal, "failed to list showcases")
	}

	return &pbsvc.ListShowcasesResponse{
		Showcases: showcases,
	}, nil
}

func (s *Service) offloadShowcaseData(ctx context.Context, showcase *pbactivity.ShowcasedActivity) error {
	if showcase.ActivityData != nil {
		data, err := protojson.Marshal(showcase.ActivityData)
		if err != nil {
			return err
		}

		fileName := fmt.Sprintf("showcase_data/%s/%s_data.json", showcase.UserId, showcase.ShowcaseId)
		if err := s.blobStore.Write(ctx, s.bucketName, fileName, data); err != nil {
			return err
		}

		showcase.ActivityDataUri = fmt.Sprintf("gs://%s/%s", s.bucketName, fileName)
		showcase.ActivityData = nil // Prevent writing to Firestore
	}
	return nil
}

func (s *Service) CreateShowcase(ctx context.Context, req *pbsvc.CreateShowcaseRequest) (*pbactivity.ShowcasedActivity, error) {
	if req.UserId == "" || req.Showcase == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase data are required")
	}

	if err := s.offloadShowcaseData(ctx, req.Showcase); err != nil {
		s.logger.Error(ctx, "failed to offload showcase data to GCS", "error", err)
		return nil, status.Error(codes.Internal, "failed to process showcase data")
	}

	created, err := s.store.CreateShowcase(ctx, req.UserId, req.Showcase)
	if err != nil {
		s.logger.Error(ctx, "failed to create showcase", "error", err)
		return nil, status.Error(codes.Internal, "failed to create showcase")
	}

	return created, nil
}

func (s *Service) UpdateShowcase(ctx context.Context, req *pbsvc.UpdateShowcaseRequest) (*pbactivity.ShowcasedActivity, error) {
	if req.UserId == "" || req.ShowcaseId == "" || req.Showcase == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id, showcase_id, and showcase data are required")
	}

	if err := s.offloadShowcaseData(ctx, req.Showcase); err != nil {
		s.logger.Error(ctx, "failed to offload showcase data to GCS", "error", err)
		return nil, status.Error(codes.Internal, "failed to process showcase data")
	}

	updated, err := s.store.UpdateShowcase(ctx, req.UserId, req.Showcase)
	if err != nil {
		s.logger.Error(ctx, "failed to update showcase", "error", err)
		return nil, status.Error(codes.Internal, "failed to update showcase")
	}

	return updated, nil
}

func (s *Service) DeleteShowcase(ctx context.Context, req *pbsvc.DeleteShowcaseRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase_id are required")
	}

	// Delete from Store (Firestore)
	if err := s.store.DeleteShowcase(ctx, req.UserId, req.ShowcaseId); err != nil {
		s.logger.Error(ctx, "failed to delete showcase", "error", err)
		return nil, status.Error(codes.Internal, "failed to delete showcase")
	}

	return &emptypb.Empty{}, nil
}

// ensureShowcaseProfile retrieves the user's showcase profile, creating and
// persisting a default one if none exists. This guarantees the Firestore
// document is present so subsequent MergeAll updates are always safe.
func (s *Service) ensureShowcaseProfile(ctx context.Context, userID string) (*pbactivity.ShowcaseProfile, error) {
	profile, err := s.store.GetShowcasePreferences(ctx, userID)
	if err != nil {
		return nil, err
	}

	if profile == nil {
		profile = &pbactivity.ShowcaseProfile{
			UserId:   userID,
			Subtitle: fmt.Sprintf("Joined %d", time.Now().Year()),
			Bio:      "Check out my FitGlue activities",
			Visible:  false,
		}
		if _, err := s.store.UpdateShowcasePreferences(ctx, userID, profile); err != nil {
			s.logger.Error(ctx, "failed to create default showcase profile", "error", err)
			// Non-fatal: return the in-memory default even if persist fails
		}
	}

	return profile, nil
}

// GetShowcasePreferences retrieves a user's showcase profile settings and historical aggregates
func (s *Service) GetShowcasePreferences(ctx context.Context, req *pbsvc.GetShowcasePreferencesRequest) (*pbactivity.ShowcaseProfile, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	profile, err := s.ensureShowcaseProfile(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get showcase preferences", "error", err)
		return nil, status.Error(codes.Internal, "failed to get showcase preferences")
	}

	return profile, nil
}

// UpdateShowcasePreferences updates a user's showcase profile settings.
// Delegates to UpdateShowcaseSettings to avoid code duplication.
func (s *Service) UpdateShowcasePreferences(ctx context.Context, req *pbsvc.UpdateShowcasePreferencesRequest) (*pbactivity.ShowcaseProfile, error) {
	if req.UserId == "" || req.Preferences == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and preferences are required")
	}

	return s.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
		UserId:   req.UserId,
		Settings: req.Preferences,
	})
}

// GenerateShowcaseImages sends a generation request (e.g., via Pub/Sub or synchronous generation)
func (s *Service) GenerateShowcaseImages(ctx context.Context, req *pbsvc.GenerateShowcaseImagesRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase_id are required")
	}

	// In the real system, this likely published a message to trigger an offline generator.
	// For now, we will log it and return OK.
	s.logger.Info(ctx, "GenerateShowcaseImages requested", "user_id", req.UserId, "showcase_id", req.ShowcaseId)

	return &emptypb.Empty{}, nil
}

// GetPublicShowcase acts identically to GetShowcase but bypasses the UserId ownership check by looking up globally or within a specific index
func (s *Service) GetPublicShowcase(ctx context.Context, req *pbsvc.GetPublicShowcaseRequest) (*pbactivity.ShowcasedActivity, error) {
	if req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "showcase_id is required")
	}

	showcase, err := s.store.GetPublicShowcase(ctx, req.ShowcaseId)
	if err != nil {
		s.logger.Error(ctx, "failed to get public showcase", "error", err)
		return nil, status.Error(codes.Internal, "failed to read public showcase")
	}
	if showcase == nil {
		return nil, status.Error(codes.NotFound, "showcase not found")
	}

	// Hydrate owner metadata from showcase profile
	if profile, err := s.store.GetShowcasePreferences(ctx, showcase.UserId); err == nil && profile != nil {
		if profile.DisplayName != "" {
			showcase.OwnerDisplayName = profile.DisplayName
		}
		if profile.ProfilePictureUrl != "" {
			showcase.OwnerProfilePictureUrl = profile.ProfilePictureUrl
		}
		if profile.Slug != "" {
			showcase.OwnerProfileSlug = profile.Slug
		}
	}

	// Fetch data from GCS if unloaded
	// The GCS blob is a full EnrichedActivityEvent (stored by PrepareForPublish),
	// so we unmarshal it as such and extract just the ActivityData field.
	if showcase.ActivityData == nil && showcase.ActivityDataUri != "" {
		data, err := s.blobStore.Get(ctx, "", showcase.ActivityDataUri)
		if err == nil && len(data) > 0 {
			var fullEvent pbevents.EnrichedActivityEvent
			unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := unmarshalOpts.Unmarshal(data, &fullEvent); err == nil {
				showcase.ActivityData = fullEvent.ActivityData
			} else {
				s.logger.Warn(ctx, "Failed to unmarshal enriched event from GCS", "error", err, "uri", showcase.ActivityDataUri)
			}
		} else if err != nil {
			s.logger.Warn(ctx, "Failed to fetch activity data from GCS", "error", err, "uri", showcase.ActivityDataUri)
		}
	}

	// Final fallback for display name
	if showcase.OwnerDisplayName == "" {
		showcase.OwnerDisplayName = "FitGlue Athlete"
	}

	return showcase, nil
}
