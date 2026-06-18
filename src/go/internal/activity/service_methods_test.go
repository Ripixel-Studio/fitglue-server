package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbsvc "github.com/fitglue/server/src/go/pkg/types/pb/services/activity"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// ---------------- GetShowcase ----------------

func TestGetShowcase_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetShowcase(ctx, &pbsvc.GetShowcaseRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetShowcase(ctx, &pbsvc.GetShowcaseRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				return nil, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetShowcase(ctx, &pbsvc.GetShowcaseRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("SuccessHydratesFromGCS", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				return &pbactivity.ShowcasedActivity{
					ShowcaseId:      "s1",
					ActivityDataUri: "gs://b/x.json",
				}, nil
			},
		}
		blob := &MockBlobStore{
			GetFunc: func(_ context.Context, _, _ string) ([]byte, error) {
				return []byte(`{"activityData":{"name":"Hydrated Run"}}`), nil
			},
		}
		svc := newTestSvc(store, blob)
		res, err := svc.GetShowcase(ctx, &pbsvc.GetShowcaseRequest{UserId: "u1", ShowcaseId: "s1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.ActivityData == nil || res.ActivityData.Name != "Hydrated Run" {
			t.Errorf("expected ActivityData hydrated from GCS, got %+v", res.ActivityData)
		}
	})
}

// ---------------- ListShowcases ----------------

func TestListShowcases_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.ListShowcases(ctx, &pbsvc.ListShowcasesRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			ListShowcasesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.ListShowcases(ctx, &pbsvc.ListShowcasesRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{
			ListShowcasesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{{ShowcaseId: "s1"}, {ShowcaseId: "s2"}}, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.ListShowcases(ctx, &pbsvc.ListShowcasesRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Showcases) != 2 {
			t.Errorf("expected 2 showcases, got %d", len(res.Showcases))
		}
	})
}

// ---------------- CreateShowcase / UpdateShowcase ----------------

func TestCreateShowcase_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.CreateShowcase(ctx, &pbsvc.CreateShowcaseRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("OffloadError", func(t *testing.T) {
		blob := &MockBlobStore{
			WriteFunc: func(_ context.Context, _, _ string, _ []byte) error {
				return errors.New("gcs write failed")
			},
		}
		svc := newTestSvc(&MockActivityStore{}, blob)
		_, err := svc.CreateShowcase(ctx, &pbsvc.CreateShowcaseRequest{
			UserId:   "u1",
			Showcase: &pbactivity.ShowcasedActivity{ShowcaseId: "s1", ActivityData: &pbactivity.StandardizedActivity{Name: "x"}},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			CreateShowcaseFunc: func(_ context.Context, _ string, _ *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.CreateShowcase(ctx, &pbsvc.CreateShowcaseRequest{
			UserId:   "u1",
			Showcase: &pbactivity.ShowcasedActivity{ShowcaseId: "s1"},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})
}

func TestUpdateShowcase_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcase(ctx, &pbsvc.UpdateShowcaseRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			UpdateShowcaseFunc: func(_ context.Context, _ string, _ *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateShowcase(ctx, &pbsvc.UpdateShowcaseRequest{
			UserId:     "u1",
			ShowcaseId: "s1",
			Showcase:   &pbactivity.ShowcasedActivity{ShowcaseId: "s1"},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{
			UpdateShowcaseFunc: func(_ context.Context, _ string, sc *pbactivity.ShowcasedActivity) (*pbactivity.ShowcasedActivity, error) {
				return sc, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.UpdateShowcase(ctx, &pbsvc.UpdateShowcaseRequest{
			UserId:     "u1",
			ShowcaseId: "s1",
			Showcase:   &pbactivity.ShowcasedActivity{ShowcaseId: "s1", Title: "T"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Title != "T" {
			t.Errorf("expected title preserved, got %q", res.Title)
		}
	})
}

// ---------------- GetShowcaseSettings ----------------

func TestGetShowcaseSettings_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetShowcaseSettings(ctx, &pbsvc.GetShowcaseSettingsRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("ListShowcasesError", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
				return &pbactivity.ShowcaseProfile{UserId: "u1"}, nil
			},
			ListShowcasesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetShowcaseSettings(ctx, &pbsvc.GetShowcaseSettingsRequest{UserId: "u1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcasePreferencesFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
				return &pbactivity.ShowcaseProfile{UserId: "u1"}, nil
			},
			ListShowcasesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{
					{ShowcaseId: "s1", Title: "Run", StartTime: timestamppb.Now()},
				}, nil
			},
			ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{{ShowcaseId: "s1"}}, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.GetShowcaseSettings(ctx, &pbsvc.GetShowcaseSettingsRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(res.Activities) != 1 || !res.Activities[0].InProfile {
			t.Errorf("expected one in-profile activity, got %+v", res.Activities)
		}
	})
}

// ---------------- UpdateShowcaseSettings ----------------

func TestUpdateShowcaseSettings_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("TooManyLinks", func(t *testing.T) {
		links := make([]*pbactivity.ShowcaseLink, maxLinks+1)
		for i := range links {
			links[i] = &pbactivity.ShowcaseLink{Url: "https://x.com"}
		}
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.ShowcaseProfile{Links: links},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for too many links, got %v", err)
		}
	})

	t.Run("LinkNotHTTPS", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.ShowcaseProfile{Links: []*pbactivity.ShowcaseLink{{Label: "x", Url: "http://insecure.com"}}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for non-https link, got %v", err)
		}
	})

	t.Run("CalloutTooLong", func(t *testing.T) {
		long := make([]byte, maxCalloutLen+1)
		for i := range long {
			long[i] = 'a'
		}
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.ShowcaseProfile{Callouts: []*pbactivity.ShowcaseBioCallout{{Text: string(long)}}},
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for long callout, got %v", err)
		}
	})

	t.Run("FullUpdateSuccess", func(t *testing.T) {
		var captured string
		store := &MockActivityStore{
			UpdateShowcasePreferencesFunc: func(_ context.Context, _ string, prefs *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
				captured = prefs.UserId
				return prefs, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.ShowcaseProfile{Bio: "hi"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if captured != "u1" {
			t.Errorf("expected user id injected, got %q", captured)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			UpdateShowcasePreferencesFunc: func(_ context.Context, _ string, _ *pbactivity.ShowcaseProfile) (*pbactivity.ShowcaseProfile, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSettings(ctx, &pbsvc.UpdateShowcaseSettingsRequest{
			UserId:   "u1",
			Settings: &pbactivity.ShowcaseProfile{},
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})
}

// ---------------- UpdateShowcaseSlug ----------------

func TestUpdateShowcaseSlug_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSlug(ctx, &pbsvc.UpdateShowcaseSlugRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("InvalidFormat", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSlug(ctx, &pbsvc.UpdateShowcaseSlugRequest{UserId: "u1", Slug: "-bad-"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument for bad slug, got %v", err)
		}
	})

	t.Run("AlreadyExistsPassthrough", func(t *testing.T) {
		store := &MockActivityStore{
			UpdateShowcaseSlugFunc: func(_ context.Context, _, _ string) error {
				return status.Error(codes.AlreadyExists, "slug taken")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSlug(ctx, &pbsvc.UpdateShowcaseSlugRequest{UserId: "u1", Slug: "valid-slug"})
		if status.Code(err) != codes.AlreadyExists {
			t.Errorf("expected AlreadyExists, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			UpdateShowcaseSlugFunc: func(_ context.Context, _, _ string) error {
				return errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.UpdateShowcaseSlug(ctx, &pbsvc.UpdateShowcaseSlugRequest{UserId: "u1", Slug: "valid-slug"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("SuccessNormalizes", func(t *testing.T) {
		var stored string
		store := &MockActivityStore{
			UpdateShowcaseSlugFunc: func(_ context.Context, _, slug string) error {
				stored = slug
				return nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.UpdateShowcaseSlug(ctx, &pbsvc.UpdateShowcaseSlugRequest{UserId: "u1", Slug: "  Valid-Slug  "})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if stored != "valid-slug" || res.Slug != "valid-slug" {
			t.Errorf("expected normalized slug, stored=%q resp=%q", stored, res.Slug)
		}
	})
}

// ---------------- AddShowcaseEntry / RemoveShowcaseEntry ----------------

func TestAddShowcaseEntry_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.AddShowcaseEntry(ctx, &pbsvc.AddShowcaseEntryRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("ShowcaseNotFound", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				return nil, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.AddShowcaseEntry(ctx, &pbsvc.AddShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("GetShowcaseError", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.AddShowcaseEntry(ctx, &pbsvc.AddShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("SuccessWithMetrics", func(t *testing.T) {
		var setEntry *pbactivity.ShowcaseProfileEntry
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				avg := int32(150)
				return &pbactivity.ShowcasedActivity{
					ShowcaseId:   "s1",
					Title:        "Run",
					ActivityType: pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
					ActivityData: &pbactivity.StandardizedActivity{
						Sessions: []*pbactivity.Session{
							{TotalDistance: 5000, TotalElapsedTime: 1800, AvgHeartRate: &avg},
						},
					},
				}, nil
			},
			SetShowcaseProfileEntryFunc: func(_ context.Context, _ string, entry *pbactivity.ShowcaseProfileEntry) error {
				setEntry = entry
				return nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.AddShowcaseEntry(ctx, &pbsvc.AddShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if setEntry == nil || setEntry.DistanceMeters != 5000 {
			t.Errorf("expected distance populated, got %+v", setEntry)
		}
	})

	t.Run("SetEntryError", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseFunc: func(_ context.Context, _, _ string) (*pbactivity.ShowcasedActivity, error) {
				return &pbactivity.ShowcasedActivity{ShowcaseId: "s1"}, nil
			},
			SetShowcaseProfileEntryFunc: func(_ context.Context, _ string, _ *pbactivity.ShowcaseProfileEntry) error {
				return errors.New("set failed")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.AddShowcaseEntry(ctx, &pbsvc.AddShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})
}

func TestRemoveShowcaseEntry_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.RemoveShowcaseEntry(ctx, &pbsvc.RemoveShowcaseEntryRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("ListError", func(t *testing.T) {
		store := &MockActivityStore{
			ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.RemoveShowcaseEntry(ctx, &pbsvc.RemoveShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("NotInProfileNoop", func(t *testing.T) {
		store := &MockActivityStore{
			ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{{ShowcaseId: "other"}}, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.RemoveShowcaseEntry(ctx, &pbsvc.RemoveShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if err != nil {
			t.Errorf("expected no error for not-in-profile, got %v", err)
		}
	})

	t.Run("DeleteError", func(t *testing.T) {
		store := &MockActivityStore{
			ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{{ShowcaseId: "s1"}}, nil
			},
			DeleteShowcaseProfileEntryFunc: func(_ context.Context, _, _ string) error {
				return errors.New("delete failed")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.RemoveShowcaseEntry(ctx, &pbsvc.RemoveShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{
			ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{{ShowcaseId: "s1"}}, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.RemoveShowcaseEntry(ctx, &pbsvc.RemoveShowcaseEntryRequest{UserId: "u1", ShowcaseId: "s1"})
		if err != nil {
			t.Errorf("expected success, got %v", err)
		}
	})
}

// ---------------- Upload URL methods ----------------

func TestGetShowcaseProfilePictureUploadUrl_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetShowcaseProfilePictureUploadUrl(ctx, &pbsvc.GetShowcaseProfilePictureUploadUrlRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("InvalidContentType", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetShowcaseProfilePictureUploadUrl(ctx, &pbsvc.GetShowcaseProfilePictureUploadUrlRequest{
			UserId: "u1", ContentType: "application/pdf",
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("SignError", func(t *testing.T) {
		blob := &MockBlobStore{
			SignedURLFunc: func(_ context.Context, _, _, _ string, _ int64, _ time.Duration) (string, error) {
				return "", errors.New("sign error")
			},
		}
		svc := newTestSvc(&MockActivityStore{}, blob)
		_, err := svc.GetShowcaseProfilePictureUploadUrl(ctx, &pbsvc.GetShowcaseProfilePictureUploadUrlRequest{
			UserId: "u1", ContentType: "image/png",
		})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("DefaultContentTypeSuccess", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		res, err := svc.GetShowcaseProfilePictureUploadUrl(ctx, &pbsvc.GetShowcaseProfilePictureUploadUrlRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.UploadUrl == "" || res.ContentType != "image/jpeg" {
			t.Errorf("expected jpeg default with upload url, got %+v", res)
		}
	})
}

func TestGetActivityPhotoUploadUrl_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetActivityPhotoUploadUrl(ctx, &pbsvc.GetActivityPhotoUploadUrlRequest{UserId: "u1"})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("InvalidContentType", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetActivityPhotoUploadUrl(ctx, &pbsvc.GetActivityPhotoUploadUrlRequest{
			UserId: "u1", ActivityId: "a1", ContentType: "text/plain",
		})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("SuccessWithFilenameHint", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		res, err := svc.GetActivityPhotoUploadUrl(ctx, &pbsvc.GetActivityPhotoUploadUrlRequest{
			UserId: "u1", ActivityId: "a1", Filename: "photo.png",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.UploadUrl == "" {
			t.Error("expected an upload url")
		}
	})
}

// ---------------- GetPublicShowcaseProfile ----------------

func TestGetPublicShowcaseProfile_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetPublicShowcaseProfile(ctx, &pbsvc.GetPublicShowcaseProfileRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("StoreError", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseProfileBySlugFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
				return nil, errors.New("db error")
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetPublicShowcaseProfile(ctx, &pbsvc.GetPublicShowcaseProfileRequest{Slug: "x"})
		if status.Code(err) != codes.Internal {
			t.Errorf("expected Internal, got %v", err)
		}
	})

	t.Run("NotFound", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseProfileBySlugFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
				return nil, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetPublicShowcaseProfile(ctx, &pbsvc.GetPublicShowcaseProfileRequest{Slug: "x"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound, got %v", err)
		}
	})

	t.Run("InvisibleNotFound", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseProfileBySlugFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
				return &pbactivity.ShowcaseProfile{UserId: "u1", Slug: "x", Visible: false}, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		_, err := svc.GetPublicShowcaseProfile(ctx, &pbsvc.GetPublicShowcaseProfileRequest{Slug: "x"})
		if status.Code(err) != codes.NotFound {
			t.Errorf("expected NotFound for invisible, got %v", err)
		}
	})

	t.Run("VisibleSuccessPaginates", func(t *testing.T) {
		store := &MockActivityStore{
			GetShowcaseProfileBySlugFunc: func(_ context.Context, _ string) (*pbactivity.ShowcaseProfile, error) {
				return &pbactivity.ShowcaseProfile{UserId: "u1", Slug: "athlete", Visible: true, StreakHistory: &pbactivity.WeeklyStreakHistory{}}, nil
			},
			ListShowcaseProfileEntriesFunc: func(_ context.Context, _ string) ([]*pbactivity.ShowcaseProfileEntry, error) {
				return []*pbactivity.ShowcaseProfileEntry{{ShowcaseId: "s1"}}, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.GetPublicShowcaseProfile(ctx, &pbsvc.GetPublicShowcaseProfileRequest{Slug: "athlete", Page: 1})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.TotalPages != 1 || len(res.Profile.Entries) != 1 {
			t.Errorf("expected 1 page and 1 entry, got pages=%d entries=%d", res.TotalPages, len(res.Profile.Entries))
		}
	})
}

// ---------------- GetActivityStats ----------------

func TestGetActivityStats_Methods(t *testing.T) {
	ctx := context.Background()

	t.Run("Validation", func(t *testing.T) {
		svc := newTestSvc(&MockActivityStore{}, &MockBlobStore{})
		_, err := svc.GetActivityStats(ctx, &pbsvc.GetActivityStatsRequest{})
		if status.Code(err) != codes.InvalidArgument {
			t.Errorf("expected InvalidArgument, got %v", err)
		}
	})

	t.Run("Success", func(t *testing.T) {
		store := &MockActivityStore{
			CountBillingEventsFunc: func(_ context.Context, _ string) (int32, error) { return 42, nil },
			CountShowcasedActivitiesFunc: func(_ context.Context, _ string) (int32, error) {
				return 3, nil
			},
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.GetActivityStats(ctx, &pbsvc.GetActivityStatsRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.TotalActivities != 42 || res.TotalShowcases != 3 {
			t.Errorf("expected totals 42/3, got %d/%d", res.TotalActivities, res.TotalShowcases)
		}
	})

	t.Run("StoreErrorsTolerated", func(t *testing.T) {
		// All count funcs error → service logs and zero-fills rather than failing.
		store := &MockActivityStore{
			CountBillingEventsFunc:               func(_ context.Context, _ string) (int32, error) { return 0, errors.New("e") },
			CountBillingEventsForPeriodFunc:      func(_ context.Context, _, _ string) (int32, error) { return 0, errors.New("e") },
			CountDistinctActivitiesForPeriodFunc: func(_ context.Context, _, _ string) (int32, error) { return 0, errors.New("e") },
			CountBillingEventsSinceFunc:          nil,
			CountShowcasedActivitiesFunc:         func(_ context.Context, _ string) (int32, error) { return 0, errors.New("e") },
		}
		svc := newTestSvc(store, &MockBlobStore{})
		res, err := svc.GetActivityStats(ctx, &pbsvc.GetActivityStatsRequest{UserId: "u1"})
		if err != nil {
			t.Fatalf("expected no error despite store failures, got %v", err)
		}
		if res.TotalActivities != 0 {
			t.Errorf("expected zero-fill, got %d", res.TotalActivities)
		}
	})
}
