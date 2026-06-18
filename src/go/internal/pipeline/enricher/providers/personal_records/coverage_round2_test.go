package personal_records

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/fitglue/server/src/go/pkg/bootstrap"
	"github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
)

// prDB embeds MockDatabase but overrides the personal-record methods so tests can
// control existing-record values, lookup errors, and persist errors. The base
// MockDatabase returns (nil,nil) / nil for these, which can't exercise the
// "already a PR" / "lookup error" / "save error" branches.
type prDB struct {
	*mocks.MockDatabase
	getRecord func(recordType string) (*pbuser.PersonalRecord, error)
	setErr    error
	setCalls  int
}

func (d *prDB) GetPersonalRecord(_ context.Context, _ string, recordType string) (*pbuser.PersonalRecord, error) {
	if d.getRecord != nil {
		return d.getRecord(recordType)
	}
	return nil, nil
}

func (d *prDB) SetPersonalRecord(_ context.Context, _ string, _ *pbuser.PersonalRecord) error {
	d.setCalls++
	return d.setErr
}

func newProviderWithDB(db *prDB) *PersonalRecordsProvider {
	p := NewPersonalRecordsProvider()
	p.SetService(&bootstrap.Service{DB: db})
	return p
}

func prUser() *user.Record {
	return &user.Record{UserProfile: &pbuser.UserProfile{UserId: "u1"}}
}

// TestCheckAndUpdateRecord_NotABetterRecord covers the path where an existing
// record beats the new value (no PR) and the existing record is from a different
// activity → returns (nil, nil).
func TestCheckAndUpdateRecord_NotABetterRecord(t *testing.T) {
	db := &prDB{
		MockDatabase: &mocks.MockDatabase{},
		getRecord: func(string) (*pbuser.PersonalRecord, error) {
			// Existing faster 5k (lowerIsBetter) from another activity.
			return &pbuser.PersonalRecord{Value: 1000, Unit: "seconds", ActivityId: "other-act"}, nil
		},
	}
	p := newProviderWithDB(db)
	activity := &pbactivity.StandardizedActivity{ExternalId: "this-act"}
	res, err := p.checkAndUpdateRecord(context.Background(), "u1", "fastest_5km", 1200, "seconds", activity, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res != nil {
		t.Errorf("expected nil result (not a PR), got %+v", res)
	}
	if db.setCalls != 0 {
		t.Error("SetPersonalRecord should not be called when there is no PR")
	}
}

// TestCheckAndUpdateRecord_IdempotentSameActivity covers reconstruction of the
// result when the existing record was set by the same activity.
func TestCheckAndUpdateRecord_IdempotentSameActivity(t *testing.T) {
	prev := 1300.0
	imp := 100.0
	db := &prDB{
		MockDatabase: &mocks.MockDatabase{},
		getRecord: func(string) (*pbuser.PersonalRecord, error) {
			return &pbuser.PersonalRecord{
				Value: 1200, Unit: "seconds", ActivityId: "same-act",
				PreviousValue: &prev, Improvement: &imp,
			}, nil
		},
	}
	p := newProviderWithDB(db)
	activity := &pbactivity.StandardizedActivity{ExternalId: "same-act"}
	res, err := p.checkAndUpdateRecord(context.Background(), "u1", "fastest_5km", 1200, "seconds", activity, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.NewValue != 1200 {
		t.Fatalf("expected reconstructed result, got %+v", res)
	}
	if res.DisplayMessage == "" {
		t.Error("expected a display message on reconstructed result")
	}
}

// TestCheckAndUpdateRecord_LookupErrorNonNotFound covers the real-error path of
// GetPersonalRecord (not a not-found error).
func TestCheckAndUpdateRecord_LookupErrorNonNotFound(t *testing.T) {
	db := &prDB{
		MockDatabase: &mocks.MockDatabase{},
		getRecord: func(string) (*pbuser.PersonalRecord, error) {
			return nil, errors.New("firestore unavailable")
		},
	}
	p := newProviderWithDB(db)
	activity := &pbactivity.StandardizedActivity{ExternalId: "a"}
	_, err := p.checkAndUpdateRecord(context.Background(), "u1", "fastest_5km", 1200, "seconds", activity, true)
	if err == nil {
		t.Error("expected error to propagate for non-not-found lookup failure")
	}
}

// TestCheckAndUpdateRecord_NotFoundTreatedAsFirstPR covers the not-found path
// (treated as first record) plus the SetPersonalRecord error branch.
func TestCheckAndUpdateRecord_SaveError(t *testing.T) {
	db := &prDB{
		MockDatabase: &mocks.MockDatabase{},
		getRecord: func(string) (*pbuser.PersonalRecord, error) {
			return nil, errors.New("document not found")
		},
		setErr: errors.New("write failed"),
	}
	p := newProviderWithDB(db)
	activity := &pbactivity.StandardizedActivity{ExternalId: "a"}
	_, err := p.checkAndUpdateRecord(context.Background(), "u1", "longest_run", 5000, "meters", activity, false)
	if err == nil {
		t.Error("expected error from SetPersonalRecord failure")
	}
}

// TestCheckAndUpdateRecord_ImprovementOverExisting covers the PR-with-previous
// branch where improvement is computed and the record is persisted.
func TestCheckAndUpdateRecord_ImprovementOverExisting(t *testing.T) {
	db := &prDB{
		MockDatabase: &mocks.MockDatabase{},
		getRecord: func(string) (*pbuser.PersonalRecord, error) {
			return &pbuser.PersonalRecord{Value: 4000, Unit: "meters", ActivityId: "old"}, nil
		},
	}
	p := newProviderWithDB(db)
	activity := &pbactivity.StandardizedActivity{
		ExternalId: "new",
		Type:       pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions:   []*pbactivity.Session{{TotalDistance: 5000}},
	}
	res, err := p.checkAndUpdateRecord(context.Background(), "u1", "longest_run", 5000, "meters", activity, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res == nil || res.PreviousValue == nil || res.Improvement == nil {
		t.Fatalf("expected PR with previous value + improvement, got %+v", res)
	}
	if db.setCalls != 1 {
		t.Errorf("expected exactly one SetPersonalRecord call, got %d", db.setCalls)
	}
}

// TestEnrich_HybridRaceHyrox drives the hybrid race detection + station PR path
// through Enrich, exercising checkHybridRaceRecords end to end.
func TestEnrich_HybridRaceHyrox(t *testing.T) {
	p := NewPersonalRecordsProvider()
	p.SetService(&bootstrap.Service{DB: &mocks.MockDatabase{}}) // all records new

	activity := &pbactivity.StandardizedActivity{
		Name: "Hyrox London",
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_WORKOUT,
		Tags: []string{"HYROX"},
		Sessions: []*pbactivity.Session{
			{
				TotalElapsedTime: 4500, // total race time
				StrengthSets: []*pbactivity.StrengthSet{
					{ExerciseName: "SkiErg", DurationSeconds: 240},
					{ExerciseName: "Sled Push", DurationSeconds: 180},
					{ExerciseName: "Wall Ball", DurationSeconds: 300},
					{ExerciseName: "Unknown Station", DurationSeconds: 60}, // normalizes to "" → skipped
					{ExerciseName: "Rest", DurationSeconds: 0},             // zero duration → skipped
				},
			},
		},
	}
	u := prUser()
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["pr_status"] != "pr_detected" {
		t.Fatalf("expected pr_detected for hyrox, got %v", res.Metadata)
	}
	// total_time + skierg + sled_push + wall_balls = 4 PRs.
	if res.Metadata["pr_count"] != "4" {
		t.Errorf("expected 4 hybrid PRs, got %q", res.Metadata["pr_count"])
	}
}

// TestEnrich_CyclingLongestRide drives the cycling branch of checkCardioRecords
// (longest ride + cycling distance thresholds).
func TestEnrich_CyclingLongestRide(t *testing.T) {
	p := NewPersonalRecordsProvider()
	p.SetService(&bootstrap.Service{DB: &mocks.MockDatabase{}})

	activity := &pbactivity.StandardizedActivity{
		Name: "Long Ride",
		Type: pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		Sessions: []*pbactivity.Session{
			{TotalDistance: 60000, TotalElapsedTime: 7200},
		},
	}
	u := prUser()
	res, err := p.Enrich(context.Background(), discardLogger(), activity, u, nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["pr_status"] != "pr_detected" {
		t.Errorf("expected pr_detected for cycling, got %v", res.Metadata)
	}
	if !strings.Contains(res.Description, "Personal Records") {
		t.Errorf("expected PR description, got %q", res.Description)
	}
}

// TestEnrich_ZeroDistanceCardioNoPRs covers checkCardioRecords early return when
// total distance is zero.
func TestEnrich_ZeroDistanceCardioNoPRs(t *testing.T) {
	p := NewPersonalRecordsProvider()
	p.SetService(&bootstrap.Service{DB: &mocks.MockDatabase{}})

	activity := &pbactivity.StandardizedActivity{
		Type:     pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Sessions: []*pbactivity.Session{{TotalDistance: 0, TotalElapsedTime: 600}},
	}
	res, err := p.Enrich(context.Background(), discardLogger(), activity, prUser(), nil, false)
	if err != nil {
		t.Fatalf("Enrich error: %v", err)
	}
	if res.Metadata["pr_status"] != "no_new_prs" {
		t.Errorf("expected no_new_prs for zero-distance run, got %v", res.Metadata)
	}
}
