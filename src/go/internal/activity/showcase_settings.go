package activity

import (
	"context"
	"fmt"
	"regexp"
	"sort"
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
	"google.golang.org/protobuf/types/known/timestamppb"
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

// formatPRLabel builds a short display label from an enrichment PersonalRecord,
// e.g. "★ BENCH PRESS 1RM 180KG · +5KG".
func formatPRLabel(r *pbactivity.PersonalRecord) string {
	label := strings.ToUpper(strings.ReplaceAll(r.RecordType, "_", " "))
	unit := strings.ToUpper(r.Unit)
	if unit == "SECONDS" {
		mins := int(r.NewValue) / 60
		secs := int(r.NewValue) % 60
		val := fmt.Sprintf("%d:%02d", mins, secs)
		result := fmt.Sprintf("★ %s %s", label, val)
		if r.PreviousValue != nil && *r.PreviousValue > r.NewValue {
			delta := *r.PreviousValue - r.NewValue
			dm := int(delta) / 60
			ds := int(delta) % 60
			if dm > 0 {
				result += fmt.Sprintf(" · −%d:%02d", dm, ds)
			} else {
				result += fmt.Sprintf(" · −%ds", ds)
			}
		}
		return result
	}
	result := fmt.Sprintf("★ %s %.0f%s", label, r.NewValue, unit)
	if r.PreviousValue != nil && r.NewValue > *r.PreviousValue {
		delta := r.NewValue - *r.PreviousValue
		result += fmt.Sprintf(" · +%.0f%s", delta, unit)
	}
	return result
}

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
		var startTime, createdAt string
		if sc.StartTime != nil {
			startTime = sc.StartTime.AsTime().UTC().Format(time.RFC3339)
		}
		if sc.CreatedAt != nil {
			createdAt = sc.CreatedAt.AsTime().UTC().Format(time.RFC3339)
		}
		activities = append(activities, &pbsvc.ShowcaseActivityEntry{
			ShowcaseId:   sc.ShowcaseId,
			Title:        sc.Title,
			ActivityType: sc.ActivityType.String(),
			Source:       sc.Source.String(),
			InProfile:    profileEntryIDs[sc.ShowcaseId],
			StartTime:    startTime,
			CreatedAt:    createdAt,
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
	if showcase == nil {
		return nil, status.Error(codes.NotFound, "showcase activity not found or does not belong to user")
	}

	// Hydrate ActivityData from GCS if not inline.
	// The GCS blob is a full EnrichedActivityEvent (stored by PrepareForPublish).
	var hydratedEnrichments *pbactivity.ActivityEnrichments
	if showcase.ActivityData == nil && showcase.ActivityDataUri != "" {
		data, err := s.blobStore.Get(ctx, "", showcase.ActivityDataUri)
		if err == nil && len(data) > 0 {
			var fullEvent pbevents.EnrichedActivityEvent
			unmarshalOpts := protojson.UnmarshalOptions{DiscardUnknown: true}
			if err := unmarshalOpts.Unmarshal(data, &fullEvent); err == nil {
				showcase.ActivityData = fullEvent.ActivityData
				hydratedEnrichments = fullEvent.Enrichments
			}
		}
	}

	// Enrichments are hydrated at read time from the GCS blob in GetPublicShowcase/GetShowcase;
	// no need to persist them separately to Firestore.

	// Build the entry from showcase metadata
	newEntry := &pbactivity.ShowcaseProfileEntry{
		ShowcaseId:        req.ShowcaseId,
		Title:             showcase.Title,
		ActivityType:      showcase.ActivityType,
		Source:            showcase.Source,
		StartTime:         showcase.StartTime,
		BoosterCount:      int32(len(showcase.AppliedEnrichments)),
		DestinationCount:  req.DestinationCount,
		RouteThumbnailUrl: req.RouteThumbnailUrl,
		PhotoUrls:         showcase.PhotoUrls,
	}

	// Populate metrics from ActivityData if available
	if showcase.ActivityData != nil && len(showcase.ActivityData.Sessions) > 0 {
		sess := showcase.ActivityData.Sessions[0]
		if activityTypeHasDistance(showcase.ActivityType) {
			newEntry.DistanceMeters = sess.TotalDistance
		}
		newEntry.DurationSeconds = sess.TotalElapsedTime
		newEntry.TotalSets = int32(len(sess.StrengthSets))

		// Sum reps and volume (reps × weight per set) across all strength sets.
		var totalReps int32
		var totalVolumeKg float64
		for _, set := range sess.StrengthSets {
			totalReps += set.Reps
			totalVolumeKg += float64(set.Reps) * set.WeightKg
		}
		newEntry.TotalReps = totalReps
		newEntry.TotalWeightKg = totalVolumeKg

		// avg_heart_rate: prefer typed enrichment, fall back to session field
		if hydratedEnrichments != nil && hydratedEnrichments.HeartRate != nil && hydratedEnrichments.HeartRate.AvgBpm > 0 {
			v := hydratedEnrichments.HeartRate.AvgBpm
			newEntry.AvgHeartRate = &v
		} else if sess.AvgHeartRate != nil && *sess.AvgHeartRate > 0 {
			newEntry.AvgHeartRate = sess.AvgHeartRate
		}

		// calories_kcal: from typed enrichment
		if hydratedEnrichments != nil && hydratedEnrichments.Calories != nil && hydratedEnrichments.Calories.Kcal > 0 {
			v := hydratedEnrichments.Calories.Kcal
			newEntry.CaloriesKcal = &v
		}

		// sparkline: downsample HR timeseries from lap records (≤ 24 points)
		if sparkline := buildSparkline(sess); sparkline != nil {
			newEntry.Sparkline = sparkline
		}
	}

	// PR label for profile activity cards (strength activities only)
	if hydratedEnrichments != nil && hydratedEnrichments.PersonalRecords != nil && len(hydratedEnrichments.PersonalRecords.Records) > 0 {
		label := formatPRLabel(hydratedEnrichments.PersonalRecords.Records[0])
		newEntry.PrLabel = &label
	}

	// HR zone minutes for lifetime zone split aggregation on the profile page.
	if hydratedEnrichments != nil && hydratedEnrichments.HeartRateZones != nil {
		zoneMinutes := make([]int32, 6)
		for _, z := range hydratedEnrichments.HeartRateZones.GetZones() {
			if z.ZoneIndex >= 0 && z.ZoneIndex < 6 {
				zoneMinutes[z.ZoneIndex] = z.Minutes
			}
		}
		newEntry.HrZoneMinutes = zoneMinutes
	}

	// Write entry to sub-collection (idempotent via MergeAll)
	if err := s.store.SetShowcaseProfileEntry(ctx, req.UserId, newEntry); err != nil {
		s.logger.Error(ctx, "failed to set showcase profile entry", "error", err)
		return nil, status.Error(codes.Internal, "failed to add entry")
	}

	// Recompute profile stats from all entries so the totals are always consistent
	// regardless of retries or prior data corrections (no delta drift).
	s.recomputeAndSaveProfileStats(ctx, req.UserId)

	return &emptypb.Empty{}, nil
}

// RemoveShowcaseEntry removes a showcased activity from the user's showcase profile.
func (s *Service) RemoveShowcaseEntry(ctx context.Context, req *pbsvc.RemoveShowcaseEntryRequest) (*emptypb.Empty, error) {
	if req.UserId == "" || req.ShowcaseId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and showcase_id are required")
	}

	// Check the entry exists before attempting deletion.
	entries, err := s.store.ListShowcaseProfileEntries(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to list entries for removal", "error", err)
		return nil, status.Error(codes.Internal, "failed to read entries")
	}

	var found bool
	for _, e := range entries {
		if e.ShowcaseId == req.ShowcaseId {
			found = true
			break
		}
	}
	if !found {
		return &emptypb.Empty{}, nil // Not in profile, nothing to remove
	}

	// Delete from sub-collection
	if err := s.store.DeleteShowcaseProfileEntry(ctx, req.UserId, req.ShowcaseId); err != nil {
		s.logger.Error(ctx, "failed to delete showcase profile entry", "error", err)
		return nil, status.Error(codes.Internal, "failed to remove entry")
	}

	// Recompute profile stats from all remaining entries.
	s.recomputeAndSaveProfileStats(ctx, req.UserId)

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

	// Fetch personal records for the medal wall — non-fatal if unavailable
	allPRs, err := s.store.ListUserPersonalRecords(ctx, profile.UserId)
	if err != nil {
		s.logger.Warn(ctx, "failed to fetch personal records for profile", "error", err)
	} else {
		// Separate into time PRs (lower = better) and weight PRs (higher = better).
		// For weight, only include 1RM records — exclude volume/set_volume which are
		// always the largest numbers and not representative of actual weight lifted.
		var timePRs, weightPRs []*pbactivity.ShowcaseTopPR
		for _, pr := range allPRs {
			switch pr.Unit {
			case "seconds":
				timePRs = append(timePRs, pr)
			case "kg":
				if strings.HasSuffix(pr.RecordType, "_1rm") {
					weightPRs = append(weightPRs, pr)
				}
			}
		}
		// Time PRs: most recent first (up to 3)
		sort.Slice(timePRs, func(i, j int) bool {
			return timePRs[j].AchievedAt.AsTime().Before(timePRs[i].AchievedAt.AsTime())
		})
		if len(timePRs) > 3 {
			timePRs = timePRs[:3]
		}
		// Weight PRs: heaviest first (up to 5)
		sort.Slice(weightPRs, func(i, j int) bool {
			return weightPRs[i].Value > weightPRs[j].Value
		})
		if len(weightPRs) > 5 {
			weightPRs = weightPRs[:5]
		}
		profile.TopPrs = append(timePRs, weightPRs...)
	}

	// Lazily backfill streak history for profiles that pre-date the feature.
	if profile.StreakHistory == nil && req.Page == 1 {
		go s.recomputeAndSaveProfileStats(context.Background(), profile.UserId)
	}

	return &pbsvc.GetPublicShowcaseProfileResponse{
		Profile:     profile,
		TotalPages:  totalPages,
		CurrentPage: req.Page,
	}, nil
}

// recomputeAndSaveProfileStats reads every entry in the user's showcase_profile_entries
// sub-collection and derives all profile totals from scratch. Calling this after every
// add/remove ensures the profile is always consistent with its entries — no delta drift,
// no double-counting on retries, no stale values after external data corrections.
func (s *Service) recomputeAndSaveProfileStats(ctx context.Context, userID string) {
	profile, err := s.ensureShowcaseProfile(ctx, userID)
	if err != nil {
		s.logger.Error(ctx, "recompute: failed to load profile", "error", err)
		return
	}

	entries, err := s.store.ListShowcaseProfileEntries(ctx, userID)
	if err != nil {
		s.logger.Error(ctx, "recompute: failed to list entries", "error", err)
		return
	}

	profile.TotalActivities = int32(len(entries))
	profile.TotalDistanceMeters = 0
	profile.TotalDurationSeconds = 0
	profile.TotalSets = 0
	profile.TotalReps = 0
	profile.TotalWeightKg = 0
	profile.LatestActivityAt = nil

	// Heatmap: find current ISO week (Monday) and earliest activity week
	now := time.Now().UTC()
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7
	}
	currentWeekStart := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, time.UTC)

	// First pass: accumulate stats and find earliest activity
	var earliestWeekStart *time.Time
	for _, e := range entries {
		profile.TotalDistanceMeters += e.DistanceMeters
		profile.TotalDurationSeconds += e.DurationSeconds
		profile.TotalSets += e.TotalSets
		profile.TotalReps += e.TotalReps
		profile.TotalWeightKg += e.TotalWeightKg
		if e.StartTime != nil && (profile.LatestActivityAt == nil || e.StartTime.AsTime().After(profile.LatestActivityAt.AsTime())) {
			profile.LatestActivityAt = e.StartTime
		}
		if e.StartTime != nil {
			t := e.StartTime.AsTime().UTC()
			wd := int(t.Weekday())
			if wd == 0 {
				wd = 7
			}
			ws := time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, time.UTC)
			if earliestWeekStart == nil || ws.Before(*earliestWeekStart) {
				earliestWeekStart = &ws
			}
		}
	}

	// Number of weeks from first activity to now (minimum 1)
	numWeeks := 1
	if earliestWeekStart != nil {
		numWeeks = int(currentWeekStart.Sub(*earliestWeekStart).Hours()/(24*7)) + 1
		if numWeeks < 1 {
			numWeeks = 1
		}
	}

	weeklyActive := make([]bool, numWeeks)

	// Second pass: mark active weeks
	for _, e := range entries {
		if e.StartTime == nil {
			continue
		}
		t := e.StartTime.AsTime().UTC()
		wd := int(t.Weekday())
		if wd == 0 {
			wd = 7
		}
		entryWeekStart := time.Date(t.Year(), t.Month(), t.Day()-wd+1, 0, 0, 0, 0, time.UTC)
		weeksAgo := int(currentWeekStart.Sub(entryWeekStart).Hours() / (24 * 7))
		if weeksAgo >= 0 && weeksAgo < numWeeks {
			weeklyActive[numWeeks-1-weeksAgo] = true
		}
	}

	var missedWeeks int32
	for _, a := range weeklyActive {
		if !a {
			missedWeeks++
		}
	}
	profile.StreakHistory = &pbactivity.WeeklyStreakHistory{
		WeeksTracked: int32(numWeeks),
		MissedWeeks:  missedWeeks,
		WeeklyActive: weeklyActive,
	}

	// Aggregate HR zone minutes across all entries and compute LifetimeZoneSplit.
	zoneMinutesTotal := make([]int32, 6)
	for _, e := range entries {
		for i, mins := range e.HrZoneMinutes {
			if i < 6 {
				zoneMinutesTotal[i] += mins
			}
		}
	}
	totalZoneMins := int32(0)
	for _, m := range zoneMinutesTotal {
		totalZoneMins += m
	}
	if totalZoneMins > 0 {
		zoneNames := []string{"Zone 0 (Rest)", "Zone 1 (Recovery)", "Zone 2 (Endurance)", "Zone 3 (Tempo)", "Zone 4 (Threshold)", "Zone 5 (VO2 Max)"}
		zoneBuckets := make([]*pbactivity.HeartRateZoneBucket, 0, 6)
		for i, mins := range zoneMinutesTotal {
			pct := float64(mins) / float64(totalZoneMins) * 100
			zoneBuckets = append(zoneBuckets, &pbactivity.HeartRateZoneBucket{
				ZoneIndex:  int32(i),
				Name:       zoneNames[i],
				Minutes:    mins,
				Percentage: pct,
			})
		}
		// Label based on distribution: Polarized (≥70% in Z0+Z1), Threshold (≥30% in Z3–Z5), else Pyramidal.
		z01Pct := float64(zoneMinutesTotal[0]+zoneMinutesTotal[1]) / float64(totalZoneMins) * 100
		z345Pct := float64(zoneMinutesTotal[3]+zoneMinutesTotal[4]+zoneMinutesTotal[5]) / float64(totalZoneMins) * 100
		label := "Pyramidal"
		switch {
		case z01Pct >= 70:
			label = "Polarized"
		case z345Pct >= 30:
			label = "Threshold"
		}
		profile.ZoneSplit = &pbactivity.LifetimeZoneSplit{
			Zones:      zoneBuckets,
			ComputedAt: timestamppb.Now(),
			Label:      label,
		}
	}

	if _, err := s.store.UpdateShowcasePreferences(ctx, userID, profile); err != nil {
		s.logger.Error(ctx, "recompute: failed to save profile", "error", err)
	}
}

// activityTypeHasDistance returns true for activity types where distance is a meaningful metric.
// Non-distance types (yoga, strength, HIIT, etc.) must not contribute to distance totals,
// as their source data often carries the FIT "invalid" sentinel (0xFFFFFFFF) rather than zero.
func activityTypeHasDistance(t pbactivity.ActivityType) bool {
	switch t {
	case pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_TRAIL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RUN,
		pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_VIRTUAL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_GRAVEL_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_MOUNTAIN_BIKE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_EMOUNTAIN_BIKE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_EBIKE_RIDE,
		pbactivity.ActivityType_ACTIVITY_TYPE_SWIM,
		pbactivity.ActivityType_ACTIVITY_TYPE_WALK,
		pbactivity.ActivityType_ACTIVITY_TYPE_HIKE,
		pbactivity.ActivityType_ACTIVITY_TYPE_ROWING,
		pbactivity.ActivityType_ACTIVITY_TYPE_ALPINE_SKI,
		pbactivity.ActivityType_ACTIVITY_TYPE_NORDIC_SKI,
		pbactivity.ActivityType_ACTIVITY_TYPE_SNOWBOARD,
		pbactivity.ActivityType_ACTIVITY_TYPE_SOCCER,
		pbactivity.ActivityType_ACTIVITY_TYPE_TENNIS,
		pbactivity.ActivityType_ACTIVITY_TYPE_GOLF,
		pbactivity.ActivityType_ACTIVITY_TYPE_KAYAKING,
		pbactivity.ActivityType_ACTIVITY_TYPE_STAND_UP_PADDLING,
		pbactivity.ActivityType_ACTIVITY_TYPE_SURFING,
		pbactivity.ActivityType_ACTIVITY_TYPE_SAIL,
		pbactivity.ActivityType_ACTIVITY_TYPE_ICE_SKATE,
		pbactivity.ActivityType_ACTIVITY_TYPE_INLINE_SKATE,
		pbactivity.ActivityType_ACTIVITY_TYPE_ROCK_CLIMBING:
		return true
	default:
		return false
	}
}

// GetActivityStats returns aggregated activity statistics for a user.
func (s *Service) GetActivityStats(ctx context.Context, req *pbsvc.GetActivityStatsRequest) (*pbsvc.GetActivityStatsResponse, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	// All-time count from billing_events (durable; not subject to pipeline_runs TTL).
	totalActivities, err := s.store.CountBillingEvents(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to count billing events", "error", err)
		totalActivities = 0
	}

	// Current-month counts from billing_events.
	now := time.Now()
	period := now.Format("2006-01")
	uploadsThisMonth, err := s.store.CountBillingEventsForPeriod(ctx, req.UserId, period)
	if err != nil {
		s.logger.Error(ctx, "failed to count billing events for period", "error", err, "period", period)
		uploadsThisMonth = 0
	}
	activitiesThisMonth, err := s.store.CountDistinctActivitiesForPeriod(ctx, req.UserId, period)
	if err != nil {
		s.logger.Error(ctx, "failed to count distinct activities for period", "error", err, "period", period)
		activitiesThisMonth = 0
	}

	// Current-week counts: Monday 00:00 UTC to now.
	weekday := int(now.Weekday())
	if weekday == 0 {
		weekday = 7 // Sunday → 7 so Monday=1
	}
	startOfWeek := time.Date(now.Year(), now.Month(), now.Day()-weekday+1, 0, 0, 0, 0, time.UTC)
	uploadsThisWeek, err := s.store.CountBillingEventsSince(ctx, req.UserId, startOfWeek)
	if err != nil {
		s.logger.Error(ctx, "failed to count billing events this week", "error", err)
		uploadsThisWeek = 0
	}
	activitiesThisWeek, err := s.store.CountDistinctActivitiesSince(ctx, req.UserId, startOfWeek)
	if err != nil {
		s.logger.Error(ctx, "failed to count distinct activities this week", "error", err)
		activitiesThisWeek = 0
	}

	// Count showcased activities.
	totalShowcases, err := s.store.CountShowcasedActivities(ctx, req.UserId)
	if err != nil {
		s.logger.Error(ctx, "failed to count showcased activities", "error", err)
		totalShowcases = 0
	}

	return &pbsvc.GetActivityStatsResponse{
		TotalActivities:     totalActivities,
		TotalShowcases:      totalShowcases,
		UploadsThisMonth:    uploadsThisMonth,
		ActivitiesThisMonth: activitiesThisMonth,
		UploadsThisWeek:     uploadsThisWeek,
		ActivitiesThisWeek:  activitiesThisWeek,
	}, nil
}

// buildSparkline builds a downsampled timeseries for a showcase card from lap records.
// It picks the best available metric (heart_rate > power > speed) and returns ≤ 24 samples.
// Returns nil if no non-zero values are found.
func buildSparkline(sess *pbactivity.Session) *pbactivity.EntrySparkline {
	const maxPoints = 24

	// Collect all non-zero values from all laps for each candidate metric.
	var hrVals, powerVals, speedVals []float64
	for _, lap := range sess.Laps {
		for _, rec := range lap.Records {
			if rec.HeartRate > 0 {
				hrVals = append(hrVals, float64(rec.HeartRate))
			}
			if rec.Power > 0 {
				powerVals = append(powerVals, float64(rec.Power))
			}
			if rec.Speed > 0 {
				speedVals = append(speedVals, rec.Speed)
			}
		}
	}

	var metric string
	var raw []float64
	switch {
	case len(hrVals) > 0:
		metric, raw = "heart_rate", hrVals
	case len(powerVals) > 0:
		metric, raw = "power", powerVals
	case len(speedVals) > 0:
		metric, raw = "speed", speedVals
	default:
		return nil
	}

	values := downsample(raw, maxPoints)
	return &pbactivity.EntrySparkline{Metric: metric, Values: values}
}

// downsample reduces a slice to at most n evenly-spaced samples using bucket averaging.
func downsample(in []float64, n int) []float64 {
	if len(in) == 0 {
		return nil
	}
	if len(in) <= n {
		return in
	}
	out := make([]float64, n)
	step := float64(len(in)) / float64(n)
	for i := range out {
		start := int(float64(i) * step)
		end := int(float64(i+1) * step)
		if end > len(in) {
			end = len(in)
		}
		var sum float64
		for _, v := range in[start:end] {
			sum += v
		}
		out[i] = sum / float64(end-start)
	}
	return out
}
