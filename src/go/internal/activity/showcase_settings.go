package activity

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/emptypb"
)

const (
	maxLinks      = 10
	maxCallouts   = 10
	maxCalloutLen = 150
	maxLabelLen   = 50
	maxURLLen     = 200
)

// Ensure unused imports are referenced
var _ = time.Minute

var slugRegex = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{2,38}[a-z0-9]$`)

// GetShowcaseSettings returns the user's showcase profile settings along with their showcased activities.
func (s *Service) GetShowcaseSettings(ctx context.Context, req *pbsvc.GetShowcaseSettingsRequest) (*pbsvc.GetShowcaseSettingsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	profile, err := s.ensureShowcaseProfile(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get showcase profile", "error", err)
		return nil, status.Error(codes.Internal, "failed to get showcase settings")
	}

	// Fetch showcased activities to build the entries list
	showcases, err := s.store.ListShowcases(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list showcases for settings", "error", err)
		return nil, status.Error(codes.Internal, "failed to list showcases")
	}

	// Read profile entries from sub-collection
	profileEntries, err := s.store.ListShowcaseProfileEntries(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list showcase profile entries", "error", err)
		return nil, status.Error(codes.Internal, "failed to list profile entries")
	}

	// Build ShowcaseActivityEntry list with in_profile flags
	profileEntryIDs := make(map[string]bool)
	for _, entry := range profileEntries {
		profileEntryIDs[entry.ShowcaseId] = true
	}

	var activities []*pbsvc.ShowcaseActivityEntry
	for _, sc := range showcases {
		activities = append(activities, &pbsvc.ShowcaseActivityEntry{
			ShowcaseId:   sc.ShowcaseId,
			Title:        sc.Title,
			ActivityType: sc.ActivityType.String(),
			Source:       sc.Source.String(),
			InProfile:    profileEntryIDs[sc.ShowcaseId],
		})
	}

	return &pbsvc.GetShowcaseSettingsResponse{
		Profile:    profile,
		Activities: activities,
	}, nil
}

// UpdateShowcaseSettings updates the user's showcase profile settings (display name, bio, theme, etc.)
func (s *Service) UpdateShowcaseSettings(ctx context.Context, req *pbsvc.UpdateShowcaseSettingsRequest) (*pbactivity.ShowcaseProfile, error) {
	if req.UserId == "" || req.Settings == nil {
		return nil, status.Error(codes.InvalidArgument, "user_id and settings are required")
	}

	// Validate links
	if len(req.Settings.Links) > maxLinks {
		return nil, status.Errorf(codes.InvalidArgument, "too many links: maximum is %d", maxLinks)
	}
	for i, link := range req.Settings.Links {
		if len(link.Label) > maxLabelLen {
			return nil, status.Errorf(codes.InvalidArgument, "link %d: label exceeds %d characters", i, maxLabelLen)
		}
		if len(link.Url) > maxURLLen {
			return nil, status.Errorf(codes.InvalidArgument, "link %d: URL exceeds %d characters", i, maxURLLen)
		}
		if !strings.HasPrefix(link.Url, "https://") {
			return nil, status.Errorf(codes.InvalidArgument, "link %d: URL must start with https://", i)
		}
	}

	// Validate callouts
	if len(req.Settings.Callouts) > maxCallouts {
		return nil, status.Errorf(codes.InvalidArgument, "too many callouts: maximum is %d", maxCallouts)
	}
	for i, callout := range req.Settings.Callouts {
		if len(callout.Text) > maxCalloutLen {
			return nil, status.Errorf(codes.InvalidArgument, "callout %d: text exceeds %d characters", i, maxCalloutLen)
		}
	}

	// Ensure the user ID is set on the settings
	req.Settings.UserId = req.UserId

	// When the HTTP handler sends a partial update it includes an "x-showcase-update-fields"
	// metadata key listing the top-level JSON fields that were actually present in the request
	// body. We use that to build a targeted patch map so only the intended fields are written
	// to Firestore — preventing one section's save from blanking out another section's data.
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if values := md.Get("x-showcase-update-fields"); len(values) > 0 && values[0] != "" {
			fieldNames := strings.Split(values[0], ",")
			patch, err := buildShowcasePatch(req.Settings, fieldNames)
			if err != nil {
				s.logger.Error(ctx, "failed to build showcase patch", "error", err)
				return nil, status.Error(codes.Internal, "failed to update showcase settings")
			}
			updated, err := s.store.PatchShowcaseProfile(ctx, req.UserId, patch)
			if err != nil {
				s.logger.Error(ctx, "failed to patch showcase settings", "error", err)
				return nil, status.Error(codes.Internal, "failed to update showcase settings")
			}
			return updated, nil
		}
	}

	updated, err := s.store.UpdateShowcasePreferences(ctx, req.UserId, req.Settings)
	if err != nil {
		s.logger.Error(ctx, "failed to update showcase settings", "error", err)
		return nil, status.Error(codes.Internal, "failed to update showcase settings")
	}

	return updated, nil
}

// buildShowcasePatch builds a Firestore field map containing only the fields named in fieldNames
// (proto JSON snake_case names), plus user_id. Relies on encodeProtoMap's EmitUnpopulated so
// that explicitly-cleared fields (e.g. links:[]) are included when the caller asked for them.
func buildShowcasePatch(prefs *pbactivity.ShowcaseProfile, fieldNames []string) (map[string]interface{}, error) {
	fullData, err := encodeProtoMap(prefs)
	if err != nil {
		return nil, err
	}
	patch := map[string]interface{}{
		"user_id": prefs.UserId,
	}
	for _, name := range fieldNames {
		snake := camelToSnake(name)
		if val, ok := fullData[snake]; ok {
			patch[snake] = val
		}
	}
	return patch, nil
}

// camelToSnake converts a camelCase or PascalCase string to snake_case.
func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if unicode.IsUpper(r) && i > 0 {
			b.WriteByte('_')
		}
		b.WriteRune(unicode.ToLower(r))
	}
	return b.String()
}

// UpdateShowcaseSlug updates the user's showcase profile slug (URL-friendly unique identifier).
func (s *Service) UpdateShowcaseSlug(ctx context.Context, req *pbsvc.UpdateShowcaseSlugRequest) (*pbsvc.UpdateShowcaseSlugResponse, error) {
	if req.UserId == "" || req.Slug == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and slug are required")
	}

	// Normalize slug
	slug := strings.ToLower(strings.TrimSpace(req.Slug))

	// Validate slug format
	if !slugRegex.MatchString(slug) {
		return nil, status.Error(codes.InvalidArgument, "slug must be 4-40 characters, lowercase alphanumeric and hyphens only, cannot start or end with a hyphen")
	}

	if err := s.store.UpdateShowcaseSlug(ctx, req.UserId, slug); err != nil {
		if status.Code(err) == codes.AlreadyExists {
			return nil, err
		}
		s.logger.Error(ctx, "failed to update showcase slug", "error", err)
		return nil, status.Error(codes.Internal, "failed to update showcase slug")
	}

	return &pbsvc.UpdateShowcaseSlugResponse{
		Slug: slug,
	}, nil
}

// AddShowcaseEntry adds a showcased activity to the user's showcase profile.
func (s *Service) AddShowcaseEntry(ctx context.Context, req *pbsvc.AddShowcaseEntryRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase_id are required")
	}

	// Verify the showcase exists and belongs to the user
	showcase, err := s.store.GetShowcase(ctx, req.UserId, req.ShowcaseId)
	if err != nil {
		s.logger.Error(ctx, "failed to verify showcase ownership", "error", err)
		return nil, status.Error(codes.Internal, "failed to verify showcase")
	}
	if showcase == nil || showcase.UserId != req.UserId {
		return nil, status.Error(codes.NotFound, "showcase activity not found or does not belong to user")
	}

	// Hydrate ActivityData from GCS if not inline.
	// The GCS blob is a full EnrichedActivityEvent (stored by PrepareForPublish).
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

	// Build the entry from showcase metadata
	newEntry := &pbactivity.ShowcaseProfileEntry{
		ShowcaseId:        req.ShowcaseId,
		Title:             showcase.Title,
		ActivityType:      showcase.ActivityType,
		Source:            showcase.Source,
		StartTime:         showcase.StartTime,
		RouteThumbnailUrl: showcase.EnrichmentMetadata["asset_route_thumbnail"],
	}

	// Populate metrics from ActivityData if available
	if showcase.ActivityData != nil && len(showcase.ActivityData.Sessions) > 0 {
		newEntry.DistanceMeters = showcase.ActivityData.Sessions[0].TotalDistance
		newEntry.DurationSeconds = showcase.ActivityData.Sessions[0].TotalElapsedTime
		newEntry.TotalSets = int32(len(showcase.ActivityData.Sessions[0].StrengthSets))

		// Sum reps and weight if it's a strength activity
		var totalReps int32
		var totalWeight float64
		for _, set := range showcase.ActivityData.Sessions[0].StrengthSets {
			totalReps += set.Reps
			totalWeight += set.WeightKg
		}
		newEntry.TotalReps = totalReps
		newEntry.TotalWeightKg = totalWeight
	}

	// Write entry to sub-collection (idempotent via MergeAll)
	if err := s.store.SetShowcaseProfileEntry(ctx, req.UserId, newEntry); err != nil {
		s.logger.Error(ctx, "failed to set showcase profile entry", "error", err)
		return nil, status.Error(codes.Internal, "failed to add entry")
	}

	// Delta update: increment profile stats
	profile, err := s.ensureShowcaseProfile(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get showcase profile for stats update", "error", err)
		return &emptypb.Empty{}, nil // Entry saved, stats update is best-effort
	}

	profile.TotalActivities++
	profile.TotalDistanceMeters += newEntry.DistanceMeters
	profile.TotalDurationSeconds += newEntry.DurationSeconds
	profile.TotalSets += newEntry.TotalSets
	profile.TotalReps += newEntry.TotalReps
	profile.TotalWeightKg += newEntry.TotalWeightKg

	if newEntry.StartTime != nil && (profile.LatestActivityAt == nil || newEntry.StartTime.AsTime().After(profile.LatestActivityAt.AsTime())) {
		profile.LatestActivityAt = newEntry.StartTime
	}

	if _, err := s.store.UpdateShowcasePreferences(ctx, req.UserId, profile); err != nil {
		s.logger.Error(ctx, "failed to update profile stats after add", "error", err)
	}

	return &emptypb.Empty{}, nil
}

// RemoveShowcaseEntry removes a showcased activity from the user's showcase profile.
func (s *Service) RemoveShowcaseEntry(ctx context.Context, req *pbsvc.RemoveShowcaseEntryRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase_id are required")
	}

	// Read the entry before deleting so we can apply delta subtraction
	entries, err := s.store.ListShowcaseProfileEntries(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list entries for removal", "error", err)
		return nil, status.Error(codes.Internal, "failed to read entries")
	}

	var removedEntry *pbactivity.ShowcaseProfileEntry
	for _, e := range entries {
		if e.ShowcaseId == req.ShowcaseId {
			removedEntry = e
			break
		}
	}

	if removedEntry == nil {
		return &emptypb.Empty{}, nil // Not in profile, nothing to remove
	}

	// Delete from sub-collection
	if err := s.store.DeleteShowcaseProfileEntry(ctx, req.UserId, req.ShowcaseId); err != nil {
		s.logger.Error(ctx, "failed to delete showcase profile entry", "error", err)
		return nil, status.Error(codes.Internal, "failed to remove entry")
	}

	// Delta update: decrement profile stats
	profile, err := s.ensureShowcaseProfile(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to get showcase profile for stats update", "error", err)
		return &emptypb.Empty{}, nil // Entry removed, stats update is best-effort
	}

	profile.TotalActivities--
	if profile.TotalActivities < 0 {
		profile.TotalActivities = 0
	}
	profile.TotalDistanceMeters -= removedEntry.DistanceMeters
	profile.TotalDurationSeconds -= removedEntry.DurationSeconds
	profile.TotalSets -= removedEntry.TotalSets
	profile.TotalReps -= removedEntry.TotalReps
	profile.TotalWeightKg -= removedEntry.TotalWeightKg

	// If we removed the latest activity, find the new latest from remaining entries
	if removedEntry.StartTime != nil && profile.LatestActivityAt != nil &&
		removedEntry.StartTime.AsTime().Equal(profile.LatestActivityAt.AsTime()) {
		profile.LatestActivityAt = nil
		for _, e := range entries {
			if e.ShowcaseId == req.ShowcaseId {
				continue // skip the removed one
			}
			if e.StartTime != nil && (profile.LatestActivityAt == nil || e.StartTime.AsTime().After(profile.LatestActivityAt.AsTime())) {
				profile.LatestActivityAt = e.StartTime
			}
		}
	}

	if _, err := s.store.UpdateShowcasePreferences(ctx, req.UserId, profile); err != nil {
		s.logger.Error(ctx, "failed to update profile stats after remove", "error", err)
	}

	return &emptypb.Empty{}, nil
}

// GetShowcaseProfilePictureUploadUrl generates a signed URL for uploading a showcase profile picture.
func (s *Service) GetShowcaseProfilePictureUploadUrl(ctx context.Context, req *pbsvc.GetShowcaseProfilePictureUploadUrlRequest) (*pbsvc.GetShowcaseProfilePictureUploadUrlResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	// Validate content type
	validTypes := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
	}
	ext, ok := validTypes[contentType]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "content_type must be image/jpeg, image/png, or image/webp")
	}

	path := fmt.Sprintf("showcase_pictures/%s/profile.%s", req.UserId, ext)
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.showcaseAssetsBucket, path)

	uploadURL, err := s.blobStore.SignedURL(ctx, s.showcaseAssetsBucket, path, contentType, 5*1024*1024, 15*time.Minute)
	if err != nil {
		s.logger.Error(ctx, "failed to generate signed upload URL", "error", err)
		return nil, status.Error(codes.Internal, "failed to generate upload URL")
	}

	return &pbsvc.GetShowcaseProfilePictureUploadUrlResponse{
		UploadUrl:    uploadURL,
		PublicUrl:    publicURL,
		ContentType:  contentType,
		MaxSizeBytes: 5 * 1024 * 1024, // 5MB limit
	}, nil
}

// GetActivityPhotoUploadUrl generates a signed URL for uploading an activity photo directly to GCS.
func (s *Service) GetActivityPhotoUploadUrl(ctx context.Context, req *pbsvc.GetActivityPhotoUploadUrlRequest) (*pbsvc.GetActivityPhotoUploadUrlResponse, error) {
	if req.UserId == "" || req.ActivityId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and activity_id are required")
	}

	contentType := req.ContentType
	if contentType == "" {
		contentType = "image/jpeg"
	}

	validTypes := map[string]string{
		"image/jpeg": "jpg",
		"image/png":  "png",
		"image/webp": "webp",
		"image/heic": "heic",
	}
	ext, ok := validTypes[contentType]
	if !ok {
		return nil, status.Error(codes.InvalidArgument, "content_type must be image/jpeg, image/png, image/webp, or image/heic")
	}

	// Use filename hint extension if provided and valid
	if req.Filename != "" {
		parts := strings.Split(req.Filename, ".")
		if len(parts) > 1 {
			hintExt := strings.ToLower(parts[len(parts)-1])
			if hintExt == "jpg" || hintExt == "jpeg" || hintExt == "png" || hintExt == "webp" || hintExt == "heic" {
				if hintExt == "jpeg" {
					hintExt = "jpg"
				}
				ext = hintExt
			}
		}
	}

	fileUUID := fmt.Sprintf("%d", time.Now().UnixNano())
	path := fmt.Sprintf("activity_photos/%s/%s/%s.%s", req.UserId, req.ActivityId, fileUUID, ext)
	publicURL := fmt.Sprintf("https://storage.googleapis.com/%s/%s", s.showcaseAssetsBucket, path)

	uploadURL, err := s.blobStore.SignedURL(ctx, s.showcaseAssetsBucket, path, contentType, 10*1024*1024, 15*time.Minute)
	if err != nil {
		s.logger.Error(ctx, "failed to generate signed photo upload URL", "error", err)
		return nil, status.Error(codes.Internal, "failed to generate upload URL")
	}

	return &pbsvc.GetActivityPhotoUploadUrlResponse{
		UploadUrl:    uploadURL,
		PublicUrl:    publicURL,
		ContentType:  contentType,
		MaxSizeBytes: 10 * 1024 * 1024,
	}, nil
}

// GetPublicShowcaseProfile returns a public showcase profile page by slug, with paginated activities.
func (s *Service) GetPublicShowcaseProfile(ctx context.Context, req *pbsvc.GetPublicShowcaseProfileRequest) (*pbsvc.GetPublicShowcaseProfileResponse, error) {
	if req.Slug == "" {
		return nil, status.Error(codes.InvalidArgument, "slug is required")
	}

	profile, err := s.store.GetShowcaseProfileBySlug(ctx, req.Slug)
	if err != nil {
		s.logger.Error(ctx, "failed to get showcase profile by slug", "error", err)
		return nil, status.Error(codes.Internal, "failed to read showcase profile")
	}
	if profile == nil {
		return nil, status.Error(codes.NotFound, "showcase profile not found")
	}

	// Check visibility
	if !profile.Visible {
		return nil, status.Error(codes.NotFound, "showcase profile not found")
	}

	// Paginated showcased activities from the user's sub-collection
	// TODO: ListShowcaseProfileEntries currently returns all.
	// In a real high-volume scenario we'd add Limit/Offset to the store method.
	profileEntries, err := s.store.ListShowcaseProfileEntries(ctx, profile.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list showcase profile entries", "error", err)
		return nil, status.Error(codes.Internal, "failed to list profile entries")
	}

	// Mock pagination from the full list for now
	pageSize := 12
	start := (int(req.Page) - 1) * pageSize
	if start < 0 {
		start = 0
	}

	var pageEntries []*pbactivity.ShowcaseProfileEntry
	if start < len(profileEntries) {
		end := start + pageSize
		if end > len(profileEntries) {
			end = len(profileEntries)
		}
		pageEntries = profileEntries[start:end]
	}

	totalShowcases := int32(len(profileEntries))
	totalPages := (totalShowcases + int32(pageSize) - 1) / int32(pageSize)
	if totalPages < 1 {
		totalPages = 1
	}

	// Populate profile.Entries directly with the page entries
	// This preserves all fields (totalSets, totalReps, totalWeightKg, etc.)
	// without lossy conversion to ShowcasedActivity.
	profile.Entries = pageEntries

	// Double-check display name fallback if still empty
	if profile.DisplayName == "" {
		if profile.Slug != "" {
			profile.DisplayName = strings.Title(profile.Slug)
		} else {
			profile.DisplayName = "FitGlue Athlete"
		}
	}

	return &pbsvc.GetPublicShowcaseProfileResponse{
		Profile:     profile,
		TotalPages:  totalPages,
		CurrentPage: req.Page,
	}, nil
}

// GetActivityStats returns aggregated activity statistics for a user (pipeline run counts, showcase counts).
func (s *Service) GetActivityStats(ctx context.Context, req *pbsvc.GetActivityStatsRequest) (*pbsvc.GetActivityStatsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// Count total pipeline runs (all statuses)
	totalActivities, err := s.store.CountPipelineRunsByStatus(ctx, req.UserId, "")
	if err != nil {
		s.logger.Error(ctx, "failed to count total pipeline runs", "error", err)
		totalActivities = 0
	}

	// Count showcased activities
	totalShowcases, err := s.store.CountShowcasedActivities(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to count showcased activities", "error", err)
		totalShowcases = 0
	}

	return &pbsvc.GetActivityStatsResponse{
		TotalActivities: totalActivities,
		TotalShowcases:  totalShowcases,
	}, nil
}
