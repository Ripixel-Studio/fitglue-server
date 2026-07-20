package firestore

import (
	"testing"
	"time"

	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// --- Helper function tests ---

func TestGetString(t *testing.T) {
	m := map[string]interface{}{
		"present": "hello",
		"wrong":   123,
	}
	if got := getString(m, "present"); got != "hello" {
		t.Errorf("getString present: want hello, got %q", got)
	}
	if got := getString(m, "missing"); got != "" {
		t.Errorf("getString missing: want empty, got %q", got)
	}
	if got := getString(m, "wrong"); got != "" {
		t.Errorf("getString wrong-type: want empty, got %q", got)
	}
}

func TestStringPtrOrNil(t *testing.T) {
	if stringPtrOrNil("") != nil {
		t.Error("stringPtrOrNil empty: want nil")
	}
	p := stringPtrOrNil("x")
	if p == nil || *p != "x" {
		t.Errorf("stringPtrOrNil non-empty: want pointer to x")
	}
}

func TestGetBool(t *testing.T) {
	m := map[string]interface{}{
		"yes":   true,
		"wrong": "nope",
	}
	if !getBool(m, "yes") {
		t.Error("getBool yes: want true")
	}
	if getBool(m, "missing") {
		t.Error("getBool missing: want false")
	}
	if getBool(m, "wrong") {
		t.Error("getBool wrong-type: want false")
	}
}

func TestGetStringSlice(t *testing.T) {
	// []interface{} path (Firestore native)
	m := map[string]interface{}{
		"ifaces": []interface{}{"a", "b", 99, "c"}, // non-string skipped
		"strs":   []string{"x", "y"},
		"wrong":  "notaslice",
	}
	got := getStringSlice(m, "ifaces")
	if len(got) != 3 || got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Errorf("getStringSlice ifaces: got %v", got)
	}
	got = getStringSlice(m, "strs")
	if len(got) != 2 || got[0] != "x" || got[1] != "y" {
		t.Errorf("getStringSlice strs: got %v", got)
	}
	if getStringSlice(m, "missing") != nil {
		t.Error("getStringSlice missing: want nil")
	}
	if getStringSlice(m, "wrong") != nil {
		t.Error("getStringSlice wrong-type: want nil")
	}
}

func TestGetTime(t *testing.T) {
	now := time.Date(2026, 6, 18, 12, 0, 0, 0, time.UTC)
	m := map[string]interface{}{
		"native":    now,
		"rfc3339":   now.Format(time.RFC3339),
		"badstring": "not-a-time",
		"wrong":     12345,
	}
	if ts := getTime(m, "native"); ts == nil || !ts.AsTime().Equal(now) {
		t.Errorf("getTime native: want %v, got %v", now, ts)
	}
	if ts := getTime(m, "rfc3339"); ts == nil || !ts.AsTime().Equal(now) {
		t.Errorf("getTime rfc3339: want %v, got %v", now, ts)
	}
	if getTime(m, "missing") != nil {
		t.Error("getTime missing: want nil")
	}
	if getTime(m, "badstring") != nil {
		t.Error("getTime badstring: want nil")
	}
	if getTime(m, "wrong") != nil {
		t.Error("getTime wrong-type: want nil")
	}
}

// fsRoundTrip simulates Firestore's serialization quirks that matter for the
// converters: it leaves time.Time and most scalars as-is (the converters'
// Go-native ToFirestore output already uses time.Time), but the integer-typed
// enum/number fields written as int32 come back as int64. The maps written as
// []map[string]interface{} come back as []interface{} containing
// map[string]interface{}. We normalize those here so FirestoreToX sees what it
// would see in production.

// --- User round-trip ---

func TestUserRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC))
	orig := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId:             "u-123",
			CreatedAt:          now,
			Tier:               pbuser.UserTier_USER_TIER_ATHLETE,
			TrialEndsAt:        now,
			IsAdmin:            true,
			SyncCountThisMonth: 7,
			AccessEnabled:      true,
			PreventedSyncCount: 3,
			FcmTokens:          []string{"tok1", "tok2"},
		},
		Billing: &pbuser.SubscriptionState{StripeCustomerId: "cus_abc"},
		Integrations: &pbuser.UserIntegrations{
			Hevy:          &pbuser.HevyIntegration{Enabled: true, ApiKey: "k", UserId: "h-uid"},
			Fitbit:        &pbuser.FitbitIntegration{Enabled: true, AccessToken: "fa", RefreshToken: "fr", ExpiresAt: now, FitbitUserId: "fid"},
			Strava:        &pbuser.StravaIntegration{Enabled: true, AccessToken: "sa", RefreshToken: "sr", ExpiresAt: now, AthleteId: 42},
			Parkrun:       &pbuser.ParkrunIntegration{Enabled: true, AthleteId: "A1", CountryUrl: "uk", ConsentGiven: true, CreatedAt: now, LastUsedAt: now},
			Spotify:       &pbuser.SpotifyIntegration{Enabled: true, AccessToken: "spa", RefreshToken: "spr", ExpiresAt: now, SpotifyUserId: "sp", CreatedAt: now, LastUsedAt: now},
			Trainingpeaks: &pbuser.TrainingPeaksIntegration{Enabled: true, AccessToken: "ta", RefreshToken: "tr", ExpiresAt: now, AthleteId: "tp", CreatedAt: now, LastUsedAt: now},
			Intervals:     &pbuser.IntervalsIntegration{Enabled: true, ApiKey: "ik", AthleteId: "ia", CreatedAt: now, LastUsedAt: now},
			Oura:          &pbuser.OuraIntegration{Enabled: true, AccessToken: "oa", RefreshToken: "or", ExpiresAt: now, OuraUserId: "ou", CreatedAt: now, LastUsedAt: now},
			Google:        &pbuser.GoogleIntegration{Enabled: true, AccessToken: "ga", RefreshToken: "gr", ExpiresAt: now, GoogleUserId: "gu", CreatedAt: now, LastUsedAt: now},
			Polar:         &pbuser.PolarIntegration{Enabled: true, AccessToken: "pa", RefreshToken: "pr", ExpiresAt: now, PolarUserId: "pu", CreatedAt: now, LastUsedAt: now},
			Wahoo:         &pbuser.WahooIntegration{Enabled: true, AccessToken: "wa", RefreshToken: "wr", ExpiresAt: now, WahooUserId: "wu", CreatedAt: now, LastUsedAt: now},
			Github:        &pbuser.GitHubIntegration{Enabled: true, AccessToken: "gha", RefreshToken: "ghr", ExpiresAt: now, GithubUserId: "ghu", GithubUsername: "ghn", Scope: "repo", CreatedAt: now, LastUsedAt: now},
			AppleHealth:   &pbuser.AppleHealthIntegration{Enabled: true, CreatedAt: now, LastUsedAt: now},
			HealthConnect: &pbuser.HealthConnectIntegration{Enabled: true, CreatedAt: now, LastUsedAt: now},
		},
	}

	m := UserToFirestore(orig)
	// Firestore serializes proto int32 fields as int64.
	for _, k := range []string{"sync_count_this_month", "prevented_sync_count"} {
		if v, ok := m[k].(int32); ok {
			m[k] = int64(v)
		}
	}
	got := FirestoreToUser(m)

	if got.UserId != "u-123" {
		t.Errorf("UserId: got %q", got.UserId)
	}
	if got.CreatedAt.AsTime() != now.AsTime() {
		t.Errorf("CreatedAt mismatch")
	}
	if got.Tier != pbuser.UserTier_USER_TIER_ATHLETE {
		t.Errorf("Tier: got %v", got.Tier)
	}
	if !got.IsAdmin || !got.AccessEnabled {
		t.Errorf("admin/access flags lost")
	}
	if got.SyncCountThisMonth != 7 || got.PreventedSyncCount != 3 {
		t.Errorf("counts mismatch: %d %d", got.SyncCountThisMonth, got.PreventedSyncCount)
	}
	if got.Billing == nil || got.Billing.StripeCustomerId != "cus_abc" {
		t.Errorf("billing lost")
	}
	if len(got.FcmTokens) != 2 || got.FcmTokens[0] != "tok1" {
		t.Errorf("fcm tokens mismatch: %v", got.FcmTokens)
	}
	if got.Integrations == nil {
		t.Fatal("integrations lost")
	}
	in := got.Integrations
	if in.Hevy == nil || in.Hevy.ApiKey != "k" {
		t.Error("hevy lost")
	}
	if in.Fitbit == nil || in.Fitbit.FitbitUserId != "fid" {
		t.Error("fitbit lost")
	}
	if in.Strava == nil || in.Strava.AthleteId != 42 {
		t.Errorf("strava athlete id: %v", in.Strava)
	}
	if in.Parkrun == nil || in.Parkrun.AthleteId != "A1" || !in.Parkrun.ConsentGiven {
		t.Error("parkrun lost")
	}
	if in.Spotify == nil || in.Spotify.SpotifyUserId != "sp" {
		t.Error("spotify lost")
	}
	if in.Trainingpeaks == nil || in.Trainingpeaks.AthleteId != "tp" {
		t.Error("trainingpeaks lost")
	}
	if in.Intervals == nil || in.Intervals.ApiKey != "ik" {
		t.Error("intervals lost")
	}
	if in.Oura == nil || in.Oura.OuraUserId != "ou" {
		t.Error("oura lost")
	}
	if in.Google == nil || in.Google.GoogleUserId != "gu" {
		t.Error("google lost")
	}
	if in.Polar == nil || in.Polar.PolarUserId != "pu" {
		t.Error("polar lost")
	}
	if in.Wahoo == nil || in.Wahoo.WahooUserId != "wu" {
		t.Error("wahoo lost")
	}
	if in.Github == nil || in.Github.GithubUsername != "ghn" || in.Github.Scope != "repo" {
		t.Error("github lost")
	}
	if in.AppleHealth == nil || !in.AppleHealth.Enabled {
		t.Error("apple_health lost")
	}
	if in.HealthConnect == nil || !in.HealthConnect.Enabled {
		t.Error("health_connect lost")
	}
}

func TestUserToFirestore_HobbyistDefault(t *testing.T) {
	orig := &user.Record{
		UserProfile: &pbuser.UserProfile{
			UserId:    "u",
			CreatedAt: timestamppb.Now(),
			Tier:      pbuser.UserTier_USER_TIER_HOBBYIST,
		},
	}
	m := UserToFirestore(orig)
	if m["tier"] != "hobbyist" {
		t.Errorf("tier: got %v", m["tier"])
	}
	// No integrations -> map should not include integrations key.
	if _, ok := m["integrations"]; ok {
		t.Error("integrations should be absent when nil")
	}
	// No billing -> no stripe key.
	if _, ok := m["stripe_customer_id"]; ok {
		t.Error("stripe_customer_id should be absent when billing nil")
	}
	// No trial_ends_at.
	if _, ok := m["trial_ends_at"]; ok {
		t.Error("trial_ends_at should be absent when nil")
	}
}

func TestFirestoreToUser_TierVariants(t *testing.T) {
	cases := []struct {
		name string
		val  interface{}
		want pbuser.UserTier
	}{
		{"string-athlete", "athlete", pbuser.UserTier_USER_TIER_ATHLETE},
		{"string-pro", "pro", pbuser.UserTier_USER_TIER_ATHLETE},
		{"string-other", "hobbyist", pbuser.UserTier_USER_TIER_HOBBYIST},
		{"int64-2", int64(2), pbuser.UserTier_USER_TIER_ATHLETE},
		{"int64-1", int64(1), pbuser.UserTier_USER_TIER_HOBBYIST},
		{"int-2", int(2), pbuser.UserTier_USER_TIER_ATHLETE},
		{"int-1", int(1), pbuser.UserTier_USER_TIER_HOBBYIST},
		{"float-2", float64(2), pbuser.UserTier_USER_TIER_ATHLETE},
		{"float-1", float64(1), pbuser.UserTier_USER_TIER_HOBBYIST},
		{"unknown-type", true, pbuser.UserTier_USER_TIER_HOBBYIST},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FirestoreToUser(map[string]interface{}{"tier": c.val})
			if got.Tier != c.want {
				t.Errorf("got %v want %v", got.Tier, c.want)
			}
		})
	}
	// Missing tier defaults to hobbyist.
	if FirestoreToUser(map[string]interface{}{}).Tier != pbuser.UserTier_USER_TIER_HOBBYIST {
		t.Error("missing tier should default to hobbyist")
	}
}

func TestFirestoreToUser_NumericCountVariants(t *testing.T) {
	m := map[string]interface{}{
		"sync_count_this_month": int64(5),
		"prevented_sync_count":  float64(9),
	}
	got := FirestoreToUser(m)
	if got.SyncCountThisMonth != 5 {
		t.Errorf("sync count: %d", got.SyncCountThisMonth)
	}
	if got.PreventedSyncCount != 9 {
		t.Errorf("prevented count: %d", got.PreventedSyncCount)
	}

	m = map[string]interface{}{
		"sync_count_this_month": int(8),
		"prevented_sync_count":  int(4),
		"fcm_tokens":            []string{"a"},
	}
	got = FirestoreToUser(m)
	if got.SyncCountThisMonth != 8 || got.PreventedSyncCount != 4 {
		t.Errorf("int counts mismatch")
	}
	if len(got.FcmTokens) != 1 || got.FcmTokens[0] != "a" {
		t.Errorf("fcm []string path: %v", got.FcmTokens)
	}
}

// --- Pipeline round-trip (additional to existing pipeline tests) ---

func TestPipelineRoundTrip_ConfigsAndDisabled(t *testing.T) {
	orig := &pbpipeline.PipelineConfig{
		Id:           "p1",
		Name:         "My Pipe",
		Source:       "SOURCE_HEVY",
		Disabled:     true,
		Destinations: []pbplugin.DestinationType{pbplugin.DestinationType_DESTINATION_STRAVA},
		Enrichers: []*pbpipeline.EnricherConfig{
			{ProviderType: pbplugin.EnricherProviderType_ENRICHER_PROVIDER_WORKOUT_SUMMARY, TypedConfig: map[string]string{"a": "b"}, NonBlocking: true},
		},
		SourceConfig: map[string]string{"sk": "sv"},
		DestinationConfigs: map[string]*pbpipeline.DestinationConfig{
			"strava": {Config: map[string]string{"ck": "cv"}, ExcludedEnrichers: []string{"e1", "e2"}},
		},
	}

	m := PipelineToFirestore(orig)

	// Simulate Firestore normalization.
	if eList, ok := m["enrichers"].([]map[string]interface{}); ok {
		es := make([]interface{}, len(eList))
		for i, e := range eList {
			if pt, ok := e["provider_type"].(int32); ok {
				e["provider_type"] = int64(pt)
			}
			if tc, ok := e["typed_config"].(map[string]string); ok {
				mi := map[string]interface{}{}
				for k, v := range tc {
					mi[k] = v
				}
				e["typed_config"] = mi
			}
			es[i] = e
		}
		m["enrichers"] = es
	}
	if dList, ok := m["destinations"].([]pbplugin.DestinationType); ok {
		ds := make([]interface{}, len(dList))
		for i, d := range dList {
			ds[i] = int64(d)
		}
		m["destinations"] = ds
	}
	// Firestore serializes map[string]string as map[string]interface{}.
	if sc, ok := m["source_config"].(map[string]string); ok {
		mi := map[string]interface{}{}
		for k, v := range sc {
			mi[k] = v
		}
		m["source_config"] = mi
	}
	if dc, ok := m["destination_configs"].(map[string]interface{}); ok {
		for _, raw := range dc {
			if obj, ok := raw.(map[string]interface{}); ok {
				if cfg, ok := obj["config"].(map[string]string); ok {
					mi := map[string]interface{}{}
					for k, v := range cfg {
						mi[k] = v
					}
					obj["config"] = mi
				}
				if ex, ok := obj["excluded_enrichers"].([]string); ok {
					ei := make([]interface{}, len(ex))
					for i, e := range ex {
						ei[i] = e
					}
					obj["excluded_enrichers"] = ei
				}
			}
		}
	}

	got := FirestoreToPipeline(m)
	if got.Name != "My Pipe" || !got.Disabled {
		t.Errorf("name/disabled mismatch: %q %v", got.Name, got.Disabled)
	}
	if got.SourceConfig["sk"] != "sv" {
		t.Errorf("source config lost: %v", got.SourceConfig)
	}
	dc := got.DestinationConfigs["strava"]
	if dc == nil || dc.Config["ck"] != "cv" {
		t.Fatalf("destination config lost: %v", got.DestinationConfigs)
	}
	if len(dc.ExcludedEnrichers) != 2 || dc.ExcludedEnrichers[0] != "e1" {
		t.Errorf("excluded enrichers lost: %v", dc.ExcludedEnrichers)
	}
	if !got.Enrichers[0].NonBlocking || got.Enrichers[0].TypedConfig["a"] != "b" {
		t.Errorf("enricher config lost: %v", got.Enrichers[0])
	}
}

// --- Execution round-trip ---

func TestExecutionRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC))
	uid := "u1"
	orig := &pbpipeline.ExecutionRecord{
		ExecutionId:         "exec-1",
		Service:             "pipeline",
		Status:              pbpipeline.ExecutionStatus_STATUS_SUCCESS,
		Timestamp:           now,
		UserId:              &uid,
		TriggerType:         "webhook",
		StartTime:           now,
		EndTime:             now,
		PipelineExecutionId: nil,
	}
	m := ExecutionToFirestore(orig)
	// int32 status -> int64 from Firestore
	if s, ok := m["status"].(int32); ok {
		m["status"] = int64(s)
	}
	// ExecutionToFirestore stores the *string optional fields directly; Firestore
	// dereferences them to plain strings (or omits nil). Normalize for round-trip.
	for _, k := range []string{"user_id", "test_run_id", "error_message", "inputs_json", "outputs_json", "pipeline_execution_id"} {
		if sp, ok := m[k].(*string); ok {
			if sp == nil {
				delete(m, k)
			} else {
				m[k] = *sp
			}
		}
	}
	got := FirestoreToExecution(m)
	if got.ExecutionId != "exec-1" || got.Service != "pipeline" {
		t.Errorf("ids mismatch")
	}
	if got.Status != pbpipeline.ExecutionStatus_STATUS_SUCCESS {
		t.Errorf("status: %v", got.Status)
	}
	if got.UserId == nil || *got.UserId != "u1" {
		t.Errorf("user id ptr lost")
	}
	if got.TriggerType != "webhook" {
		t.Errorf("trigger type: %q", got.TriggerType)
	}
	// Empty optional strings -> nil pointers.
	if got.ErrorMessage != nil || got.OutputsJson != nil || got.InputsJson != nil {
		t.Errorf("empty optionals should be nil")
	}
}

func TestFirestoreToExecution_StatusVariants(t *testing.T) {
	// string status
	got := FirestoreToExecution(map[string]interface{}{"status": "STATUS_SUCCESS"})
	if got.Status != pbpipeline.ExecutionStatus_STATUS_SUCCESS {
		t.Errorf("string status: %v", got.Status)
	}
	// unknown string -> unspecified
	got = FirestoreToExecution(map[string]interface{}{"status": "NOPE"})
	if got.Status != pbpipeline.ExecutionStatus_STATUS_UNSPECIFIED {
		t.Errorf("unknown string status: %v", got.Status)
	}
	// int status
	got = FirestoreToExecution(map[string]interface{}{"status": int(1)})
	if got.Status != pbpipeline.ExecutionStatus(1) {
		t.Errorf("int status: %v", got.Status)
	}
}

// --- Counter round-trip ---

func TestCounterRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC))
	orig := &pbuser.Counter{Id: "c1", Count: 99, LastUpdated: now}
	m := CounterToFirestore(orig)
	// int64 stays int64 in Firestore.
	got := FirestoreToCounter(m)
	if got.Id != "c1" || got.Count != 99 {
		t.Errorf("counter mismatch: %+v", got)
	}
	// float64 and int count variants.
	if FirestoreToCounter(map[string]interface{}{"count": float64(7)}).Count != 7 {
		t.Error("float count")
	}
	if FirestoreToCounter(map[string]interface{}{"count": int(3)}).Count != 3 {
		t.Error("int count")
	}
}

// --- PersonalRecord round-trip ---

func TestPersonalRecordRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC))
	prev := 1300.0
	imp := 100.0
	orig := &pbuser.PersonalRecord{
		RecordType:    "fastest_5k",
		Value:         1200,
		Unit:          "seconds",
		ActivityId:    "act-1",
		AchievedAt:    now,
		ActivityType:  pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		PreviousValue: &prev,
		Improvement:   &imp,
	}
	m := PersonalRecordToFirestore(orig)
	if at, ok := m["activity_type"].(int32); ok {
		m["activity_type"] = int64(at)
	}
	got := FirestoreToPersonalRecord(m)
	if got.RecordType != "fastest_5k" || got.Value != 1200 || got.Unit != "seconds" {
		t.Errorf("scalar fields mismatch: %+v", got)
	}
	if got.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("activity type: %v", got.ActivityType)
	}
	if got.PreviousValue == nil || *got.PreviousValue != 1300 {
		t.Errorf("previous value lost")
	}
	if got.Improvement == nil || *got.Improvement != 100 {
		t.Errorf("improvement lost")
	}
}

func TestFirestoreToPersonalRecord_NumberVariants(t *testing.T) {
	m := map[string]interface{}{
		"value":          int64(10),
		"previous_value": int64(5),
		"improvement":    int(2),
	}
	got := FirestoreToPersonalRecord(m)
	if got.Value != 10 {
		t.Errorf("int64 value: %v", got.Value)
	}
	if got.PreviousValue == nil || *got.PreviousValue != 5 {
		t.Errorf("int64 prev")
	}
	if got.Improvement == nil || *got.Improvement != 2 {
		t.Errorf("int improvement")
	}
	// int value path
	if FirestoreToPersonalRecord(map[string]interface{}{"value": int(4)}).Value != 4 {
		t.Error("int value path")
	}
}

// --- PendingInput round-trip ---

func TestPendingInputRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC))
	orig := &pbpipeline.PendingInput{
		ActivityId:                 "act-1",
		UserId:                     "u1",
		Status:                     pbpipeline.PendingInput_STATUS_WAITING,
		RequiredFields:             []string{"f1", "f2"},
		InputData:                  map[string]string{"k": "v"},
		CreatedAt:                  now,
		UpdatedAt:                  now,
		CompletedAt:                now,
		AutoPopulated:              true,
		ContinuedWithoutResolution: true,
		EnricherProviderId:         "ep",
		AutoDeadline:               now,
		LinkedActivityId:           "linked",
		PipelineId:                 "pipe",
		OriginalPayloadUri:         "gs://x",
		ProviderMetadata:           map[string]string{"pk": "pv"},
		SourceDisplayName:          "Morning Run",
		SourceActivityType:         "ACTIVITY_TYPE_RUN",
		SourceStartTime:            now,
		SourceActivitySource:       "SOURCE_HEVY",
	}
	m := PendingInputToFirestore(orig)
	if s, ok := m["status"].(int32); ok {
		m["status"] = int64(s)
	}
	// Firestore returns slices/maps as []interface{} / map[string]interface{}.
	m["required_fields"] = []interface{}{"f1", "f2"}
	m["input_data"] = map[string]interface{}{"k": "v"}
	m["provider_metadata"] = map[string]interface{}{"pk": "pv"}

	got := FirestoreToPendingInput(m)
	if got.ActivityId != "act-1" || got.UserId != "u1" {
		t.Errorf("ids mismatch")
	}
	if got.Status != pbpipeline.PendingInput_STATUS_WAITING {
		t.Errorf("status: %v", got.Status)
	}
	if len(got.RequiredFields) != 2 || got.RequiredFields[1] != "f2" {
		t.Errorf("required fields: %v", got.RequiredFields)
	}
	if got.InputData["k"] != "v" {
		t.Errorf("input data: %v", got.InputData)
	}
	if !got.AutoPopulated || !got.ContinuedWithoutResolution {
		t.Errorf("flags lost")
	}
	if got.EnricherProviderId != "ep" || got.LinkedActivityId != "linked" || got.PipelineId != "pipe" {
		t.Errorf("string fields lost")
	}
	if got.OriginalPayloadUri != "gs://x" {
		t.Errorf("payload uri lost")
	}
	if got.ProviderMetadata["pk"] != "pv" {
		t.Errorf("provider metadata: %v", got.ProviderMetadata)
	}
	// Source display metadata must survive the round-trip so the web can show which
	// activity a pending input relates to (title + when it took place), not just the source.
	if got.SourceDisplayName != "Morning Run" || got.SourceActivityType != "ACTIVITY_TYPE_RUN" {
		t.Errorf("source display fields lost: name=%q type=%q", got.SourceDisplayName, got.SourceActivityType)
	}
	if got.SourceActivitySource != "SOURCE_HEVY" {
		t.Errorf("source activity source lost: %q", got.SourceActivitySource)
	}
	if got.SourceStartTime == nil || !got.SourceStartTime.AsTime().Equal(now.AsTime()) {
		t.Errorf("source start time lost: %v", got.SourceStartTime)
	}
}

// TestPendingInputToFirestore_NonBlockingRoundTrips guards the skip-on-non-blocking
// behaviour: if the non_blocking flag is dropped on persist, SubmitInput reads it back
// as false and re-runs the pipeline when the user skips a non-blocking pending input.
func TestPendingInputToFirestore_NonBlockingRoundTrips(t *testing.T) {
	now := timestamppb.Now()
	orig := &pbpipeline.PendingInput{
		ActivityId:  "act-nb",
		CreatedAt:   now,
		UpdatedAt:   now,
		CompletedAt: now,
		NonBlocking: true,
	}
	m := PendingInputToFirestore(orig)
	if m["non_blocking"] != true {
		t.Fatalf("non_blocking not written to Firestore map: %v", m["non_blocking"])
	}
	if got := FirestoreToPendingInput(m); !got.NonBlocking {
		t.Error("expected NonBlocking to survive Firestore round-trip")
	}
}

func TestPendingInputToFirestore_OmitsEmptyOptional(t *testing.T) {
	orig := &pbpipeline.PendingInput{
		ActivityId:  "a",
		CreatedAt:   timestamppb.Now(),
		UpdatedAt:   timestamppb.Now(),
		CompletedAt: timestamppb.Now(),
	}
	m := PendingInputToFirestore(orig)
	if _, ok := m["auto_deadline"]; ok {
		t.Error("auto_deadline should be omitted when nil")
	}
	if _, ok := m["original_payload_uri"]; ok {
		t.Error("original_payload_uri should be omitted when empty")
	}
	if _, ok := m["provider_metadata"]; ok {
		t.Error("provider_metadata should be omitted when empty")
	}
	if _, ok := m["source_display_name"]; ok {
		t.Error("source_display_name should be omitted when empty")
	}
	if _, ok := m["source_start_time"]; ok {
		t.Error("source_start_time should be omitted when nil")
	}
}

func TestFirestoreToPendingInput_StringSliceAndIntStatus(t *testing.T) {
	// []string path for required_fields and int status path.
	m := map[string]interface{}{
		"required_fields": []string{"x"},
		"status":          int(1),
	}
	got := FirestoreToPendingInput(m)
	if len(got.RequiredFields) != 1 || got.RequiredFields[0] != "x" {
		t.Errorf("required_fields []string path: %v", got.RequiredFields)
	}
	if got.Status != pbpipeline.PendingInput_Status(1) {
		t.Errorf("int status: %v", got.Status)
	}
}

// --- ShowcasedActivity round-trip ---

func TestShowcasedActivityRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 1, 10, 0, 0, 0, 0, time.UTC))
	execID := "exec-9"
	orig := &pbactivity.ShowcasedActivity{
		ShowcaseId:          "sc-1",
		ActivityId:          "act-1",
		Title:               "Title",
		Description:         "Desc",
		ActivityType:        pbactivity.ActivityType_ACTIVITY_TYPE_RIDE,
		Source:              pbactivity.ActivitySource_SOURCE_STRAVA,
		AppliedEnrichments:  []string{"e1"},
		Tags:                []string{"t1", "t2"},
		FitFileUri:          "gs://fit",
		OwnerDisplayName:    "Owner",
		ActivityDataUri:     "gs://data",
		PhotoUrls:           []string{"http://p1"},
		StartTime:           now,
		CreatedAt:           now,
		ExpiresAt:           now,
		PipelineExecutionId: &execID,
		Enrichments:         &pbactivity.ActivityEnrichments{},
	}
	m := ShowcasedActivityToFirestore(orig)
	for _, k := range []string{"activity_type", "source"} {
		if v, ok := m[k].(int32); ok {
			m[k] = int64(v)
		}
	}
	m["applied_enrichments"] = []interface{}{"e1"}
	m["tags"] = []interface{}{"t1", "t2"}
	m["photo_urls"] = []interface{}{"http://p1"}

	got := FirestoreToShowcasedActivity(m)
	if got.ShowcaseId != "sc-1" || got.Title != "Title" || got.Description != "Desc" {
		t.Errorf("strings mismatch")
	}
	if got.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_RIDE {
		t.Errorf("activity type: %v", got.ActivityType)
	}
	if got.Source != pbactivity.ActivitySource_SOURCE_STRAVA {
		t.Errorf("source: %v", got.Source)
	}
	if len(got.AppliedEnrichments) != 1 || len(got.Tags) != 2 || len(got.PhotoUrls) != 1 {
		t.Errorf("slices mismatch")
	}
	if got.FitFileUri != "gs://fit" || got.ActivityDataUri != "gs://data" || got.OwnerDisplayName != "Owner" {
		t.Errorf("uri/owner mismatch")
	}
	if got.PipelineExecutionId == nil || *got.PipelineExecutionId != "exec-9" {
		t.Errorf("exec id lost")
	}
	if got.Enrichments == nil {
		t.Errorf("enrichments lost (protojson round-trip)")
	}
}

func TestShowcasedActivityToFirestore_NilOptionals(t *testing.T) {
	orig := &pbactivity.ShowcasedActivity{ShowcaseId: "sc"}
	m := ShowcasedActivityToFirestore(orig)
	if m["start_time"] != nil {
		t.Error("start_time should be nil")
	}
	if m["expires_at"] != nil {
		t.Error("expires_at should be nil")
	}
	if m["pipeline_execution_id"] != "" {
		t.Errorf("pipeline_execution_id should be empty string, got %v", m["pipeline_execution_id"])
	}
	if _, ok := m["created_at"]; ok {
		t.Error("created_at should be omitted when nil")
	}
}

// --- ShowcaseProfileEntry round-trip ---

func TestShowcaseProfileEntryRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 1, 11, 0, 0, 0, 0, time.UTC))
	hr := int32(150)
	kcal := int32(500)
	orig := &pbactivity.ShowcaseProfileEntry{
		ShowcaseId:        "sc-1",
		Title:             "Entry",
		ActivityType:      pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING,
		Source:            pbactivity.ActivitySource_SOURCE_HEVY,
		RouteThumbnailUrl: "http://thumb",
		DistanceMeters:    1000.5,
		DurationSeconds:   600.5,
		TotalSets:         10,
		TotalReps:         100,
		TotalWeightKg:     250.5,
		BoosterCount:      2,
		DestinationCount:  3,
		StartTime:         now,
		AvgHeartRate:      &hr,
		CaloriesKcal:      &kcal,
		Sparkline:         &pbactivity.EntrySparkline{Metric: "hr", Values: []float64{1, 2, 3}},
	}
	m := ShowcaseProfileEntryToFirestore(orig)
	for _, k := range []string{"activity_type", "source", "total_sets", "total_reps", "booster_count", "destination_count"} {
		if v, ok := m[k].(int32); ok {
			m[k] = int64(v)
		}
	}
	if v, ok := m["avg_heart_rate"].(int32); ok {
		m["avg_heart_rate"] = int64(v)
	}
	if v, ok := m["calories_kcal"].(int32); ok {
		m["calories_kcal"] = int64(v)
	}
	// Sparkline values come back as []interface{} from Firestore.
	if sm, ok := m["sparkline"].(map[string]interface{}); ok {
		sm["values"] = []interface{}{float64(1), float64(2), float64(3)}
	}

	got := FirestoreToShowcaseProfileEntry(m)
	if got.Title != "Entry" || got.RouteThumbnailUrl != "http://thumb" {
		t.Errorf("strings mismatch")
	}
	if got.ActivityType != pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING {
		t.Errorf("activity type: %v", got.ActivityType)
	}
	if got.Source != pbactivity.ActivitySource_SOURCE_HEVY {
		t.Errorf("source: %v", got.Source)
	}
	if got.DistanceMeters != 1000.5 || got.DurationSeconds != 600.5 || got.TotalWeightKg != 250.5 {
		t.Errorf("floats mismatch")
	}
	if got.TotalSets != 10 || got.TotalReps != 100 || got.BoosterCount != 2 || got.DestinationCount != 3 {
		t.Errorf("int32 fields mismatch")
	}
	if got.AvgHeartRate == nil || *got.AvgHeartRate != 150 {
		t.Errorf("avg hr lost")
	}
	if got.CaloriesKcal == nil || *got.CaloriesKcal != 500 {
		t.Errorf("calories lost")
	}
	if got.Sparkline == nil || got.Sparkline.Metric != "hr" || len(got.Sparkline.Values) != 3 {
		t.Errorf("sparkline lost: %+v", got.Sparkline)
	}
}

func TestFirestoreToShowcaseProfileEntry_Float64Values(t *testing.T) {
	// Exercise float64 number branches for the numeric fields.
	m := map[string]interface{}{
		"distance_meters":   float64(5),
		"duration_seconds":  float64(6),
		"total_sets":        float64(1),
		"total_reps":        float64(2),
		"total_weight_kg":   float64(7),
		"booster_count":     float64(3),
		"destination_count": float64(4),
		"avg_heart_rate":    float64(120),
		"calories_kcal":     float64(300),
		"sparkline":         map[string]interface{}{"metric": "pace", "values": []float64{9, 8}},
	}
	got := FirestoreToShowcaseProfileEntry(m)
	if got.DistanceMeters != 5 || got.DurationSeconds != 6 || got.TotalWeightKg != 7 {
		t.Errorf("float fields: %+v", got)
	}
	if got.TotalSets != 1 || got.TotalReps != 2 || got.BoosterCount != 3 || got.DestinationCount != 4 {
		t.Errorf("int from float: %+v", got)
	}
	if got.AvgHeartRate == nil || *got.AvgHeartRate != 120 {
		t.Error("avg hr from float")
	}
	if got.CaloriesKcal == nil || *got.CaloriesKcal != 300 {
		t.Error("calories from float")
	}
	if got.Sparkline == nil || len(got.Sparkline.Values) != 2 {
		t.Errorf("sparkline []float64 path: %+v", got.Sparkline)
	}
}

// --- ShowcaseProfile round-trip ---

func TestShowcaseProfileRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 1, 12, 0, 0, 0, 0, time.UTC))
	orig := &pbactivity.ShowcaseProfile{
		Slug:                 "slug",
		UserId:               "u1",
		DisplayName:          "Name",
		TotalActivities:      5,
		TotalDistanceMeters:  1234.5,
		TotalDurationSeconds: 6789.0,
		TotalSets:            10,
		TotalReps:            20,
		TotalWeightKg:        30.5,
		Subtitle:             "sub",
		Bio:                  "bio",
		ProfilePictureUrl:    "http://pic",
		Visible:              true,
		LatestActivityAt:     now,
		CreatedAt:            now,
		UpdatedAt:            now,
		DefaultDestination:   true,
		Theme: &pbactivity.ShowcaseTheme{
			ThemeId:           "th",
			CustomAccentColor: "#fff",
			AnimationId:       "anim",
			CardStyle:         "card",
		},
		Entries: []*pbactivity.ShowcaseProfileEntry{
			{ShowcaseId: "e1", Title: "E1"},
		},
	}
	m := ShowcaseProfileToFirestore(orig)
	for _, k := range []string{"total_activities"} {
		if v, ok := m[k].(int32); ok {
			m[k] = int64(v)
		}
	}
	// Entries come back as []interface{} of map[string]interface{}.
	if eList, ok := m["entries"].([]map[string]interface{}); ok {
		es := make([]interface{}, len(eList))
		for i, e := range eList {
			es[i] = e
		}
		m["entries"] = es
	}

	got := FirestoreToShowcaseProfile(m)
	if got.Slug != "slug" || got.UserId != "u1" || got.DisplayName != "Name" {
		t.Errorf("ids mismatch")
	}
	if got.TotalActivities != 5 || got.TotalDistanceMeters != 1234.5 || got.TotalDurationSeconds != 6789.0 {
		t.Errorf("totals mismatch: %+v", got)
	}
	if got.Subtitle != "sub" || got.Bio != "bio" || got.ProfilePictureUrl != "http://pic" {
		t.Errorf("profile strings mismatch")
	}
	if !got.Visible || !got.DefaultDestination {
		t.Errorf("bool flags lost")
	}
	if got.Theme == nil || got.Theme.ThemeId != "th" || got.Theme.CardStyle != "card" {
		t.Errorf("theme lost: %+v", got.Theme)
	}
	if len(got.Entries) != 1 || got.Entries[0].ShowcaseId != "e1" {
		t.Errorf("entries lost: %v", got.Entries)
	}
}

func TestFirestoreToShowcaseProfile_VisibleDefaultTrue(t *testing.T) {
	// Missing "visible" key defaults to true (backward compat).
	got := FirestoreToShowcaseProfile(map[string]interface{}{"slug": "s"})
	if !got.Visible {
		t.Error("visible should default to true when absent")
	}
	// Explicit false honored.
	got = FirestoreToShowcaseProfile(map[string]interface{}{"slug": "s", "visible": false})
	if got.Visible {
		t.Error("visible false should be honored")
	}
}

func TestFirestoreToShowcaseProfile_FloatTotals(t *testing.T) {
	m := map[string]interface{}{
		"total_activities":       float64(3),
		"total_distance_meters":  int64(100),
		"total_duration_seconds": int64(200),
	}
	got := FirestoreToShowcaseProfile(m)
	if got.TotalActivities != 3 {
		t.Errorf("total_activities float: %d", got.TotalActivities)
	}
	if got.TotalDistanceMeters != 100 || got.TotalDurationSeconds != 200 {
		t.Errorf("int64 totals: %+v", got)
	}
}

// --- PluginDefault round-trip ---

func TestPluginDefaultRoundTrip(t *testing.T) {
	orig := &pbpipeline.PluginDefault{
		PluginId:  "plugin-1",
		Config:    map[string]string{"k": "v"},
		CreatedAt: 1000,
		UpdatedAt: 2000,
	}
	m := PluginDefaultToFirestore(orig)
	m["config"] = map[string]interface{}{"k": "v"}
	got := FirestoreToPluginDefault(m)
	if got.PluginId != "plugin-1" {
		t.Errorf("plugin id: %q", got.PluginId)
	}
	if got.Config["k"] != "v" {
		t.Errorf("config lost: %v", got.Config)
	}
	if got.CreatedAt != 1000 || got.UpdatedAt != 2000 {
		t.Errorf("timestamps: %d %d", got.CreatedAt, got.UpdatedAt)
	}
}

func TestPluginDefaultToFirestore_OmitsZeroTimestamps(t *testing.T) {
	orig := &pbpipeline.PluginDefault{PluginId: "p"}
	m := PluginDefaultToFirestore(orig)
	if _, ok := m["created_at"]; ok {
		t.Error("created_at should be omitted when zero")
	}
	if _, ok := m["updated_at"]; ok {
		t.Error("updated_at should be omitted when zero")
	}
}

func TestFirestoreToPluginDefault_NumberVariants(t *testing.T) {
	got := FirestoreToPluginDefault(map[string]interface{}{
		"created_at": float64(5),
		"updated_at": int(6),
	})
	if got.CreatedAt != 5 || got.UpdatedAt != 6 {
		t.Errorf("number variants: %d %d", got.CreatedAt, got.UpdatedAt)
	}
}

// --- UploadedActivity round-trip ---

func TestUploadedActivityRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 1, 13, 0, 0, 0, 0, time.UTC))
	orig := &pbactivity.UploadedActivityRecord{
		Id:            "up-1",
		UserId:        "u1",
		Source:        pbactivity.ActivitySource_SOURCE_HEVY,
		ExternalId:    "ext",
		Destination:   pbplugin.DestinationType_DESTINATION_STRAVA,
		DestinationId: "dest",
		StartTime:     now,
		UploadedAt:    now,
	}
	m := UploadedActivityToFirestore(orig)
	for _, k := range []string{"source", "destination"} {
		if v, ok := m[k].(int32); ok {
			m[k] = int64(v)
		}
	}
	got := FirestoreToUploadedActivity(m)
	if got.Id != "up-1" || got.UserId != "u1" || got.ExternalId != "ext" || got.DestinationId != "dest" {
		t.Errorf("strings mismatch: %+v", got)
	}
	if got.Source != pbactivity.ActivitySource_SOURCE_HEVY {
		t.Errorf("source: %v", got.Source)
	}
	if got.Destination != pbplugin.DestinationType_DESTINATION_STRAVA {
		t.Errorf("destination: %v", got.Destination)
	}
	if got.StartTime.AsTime() != now.AsTime() || got.UploadedAt.AsTime() != now.AsTime() {
		t.Errorf("times mismatch")
	}
}

func TestUploadedActivityToFirestore_OmitsNilTimes(t *testing.T) {
	orig := &pbactivity.UploadedActivityRecord{Id: "x"}
	m := UploadedActivityToFirestore(orig)
	if _, ok := m["start_time"]; ok {
		t.Error("start_time should be omitted when nil")
	}
	if _, ok := m["uploaded_at"]; ok {
		t.Error("uploaded_at should be omitted when nil")
	}
}

func TestFirestoreToUploadedActivity_DestinationShortForm(t *testing.T) {
	got := FirestoreToUploadedActivity(map[string]interface{}{"destination": "strava"})
	if got.Destination != pbplugin.DestinationType_DESTINATION_STRAVA {
		t.Errorf("short-form destination: %v", got.Destination)
	}
}

// --- PipelineRun round-trip ---

func TestPipelineRunRoundTrip(t *testing.T) {
	now := timestamppb.New(time.Date(2026, 1, 14, 0, 0, 0, 0, time.UTC))
	statusMsg := "ok"
	pendingID := "pi-1"
	extID := "ext-1"
	errMsg := "boom"
	orig := &pbpipeline.PipelineRun{
		Id:                 "run-1",
		PipelineId:         "pipe-1",
		ActivityId:         "act-1",
		Source:             "SOURCE_HEVY",
		SourceActivityId:   "src-1",
		Title:              "Run",
		Description:        "desc",
		Type:               pbactivity.ActivityType_ACTIVITY_TYPE_RUN,
		Status:             pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED,
		StartTime:          now,
		CreatedAt:          now,
		UpdatedAt:          now,
		StatusMessage:      &statusMsg,
		PendingInputId:     &pendingID,
		OriginalPayloadUri: "gs://payload",
		EnrichedEventUri:   "gs://enriched",
		Boosters: []*pbpipeline.BoosterExecution{
			{ProviderName: "b1", Status: pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK, DurationMs: 123, Metadata: map[string]string{"mk": "mv"}, Error: &errMsg},
		},
		Destinations: []*pbpipeline.DestinationOutcome{
			{Destination: pbplugin.DestinationType_DESTINATION_STRAVA, Status: pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS, ExternalId: &extID, CompletedAt: now},
		},
	}
	m := PipelineRunToFirestore(orig)
	for _, k := range []string{"type", "status"} {
		if v, ok := m[k].(int32); ok {
			m[k] = int64(v)
		}
	}
	// Boosters: []map -> []interface{}, normalize int values + nested metadata.
	if bList, ok := m["boosters"].([]map[string]interface{}); ok {
		bs := make([]interface{}, len(bList))
		for i, b := range bList {
			if s, ok := b["status"].(pbpipeline.ExecutionStepStatus); ok {
				b["status"] = int64(s)
			}
			if d, ok := b["duration_ms"].(int64); ok {
				b["duration_ms"] = d
			}
			if md, ok := b["metadata"].(map[string]string); ok {
				mi := map[string]interface{}{}
				for k, v := range md {
					mi[k] = v
				}
				b["metadata"] = mi
			}
			bs[i] = b
		}
		m["boosters"] = bs
	}
	// Destinations: []map -> []interface{}, normalize enum ints.
	if dList, ok := m["destinations"].([]map[string]interface{}); ok {
		ds := make([]interface{}, len(dList))
		for i, d := range dList {
			if v, ok := d["destination"].(int32); ok {
				d["destination"] = int64(v)
			}
			if v, ok := d["status"].(int32); ok {
				d["status"] = int64(v)
			}
			ds[i] = d
		}
		m["destinations"] = ds
	}

	got := FirestoreToPipelineRun(m)
	if got.Id != "run-1" || got.PipelineId != "pipe-1" || got.Source != "SOURCE_HEVY" {
		t.Errorf("ids mismatch: %+v", got)
	}
	if got.Type != pbactivity.ActivityType_ACTIVITY_TYPE_RUN {
		t.Errorf("type: %v", got.Type)
	}
	if got.Status != pbpipeline.PipelineRunStatus_PIPELINE_RUN_STATUS_SYNCED {
		t.Errorf("status: %v", got.Status)
	}
	if got.StatusMessage == nil || *got.StatusMessage != "ok" {
		t.Errorf("status message lost")
	}
	if got.PendingInputId == nil || *got.PendingInputId != "pi-1" {
		t.Errorf("pending input id lost")
	}
	if got.OriginalPayloadUri != "gs://payload" || got.EnrichedEventUri != "gs://enriched" {
		t.Errorf("uris lost")
	}
	if len(got.Boosters) != 1 {
		t.Fatalf("boosters: %v", got.Boosters)
	}
	b := got.Boosters[0]
	if b.ProviderName != "b1" || b.Status != pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK || b.DurationMs != 123 {
		t.Errorf("booster fields: %+v", b)
	}
	if b.Metadata["mk"] != "mv" {
		t.Errorf("booster metadata: %v", b.Metadata)
	}
	if b.Error == nil || *b.Error != "boom" {
		t.Errorf("booster error lost")
	}
	if len(got.Destinations) != 1 {
		t.Fatalf("destinations: %v", got.Destinations)
	}
	d := got.Destinations[0]
	if d.Destination != pbplugin.DestinationType_DESTINATION_STRAVA || d.Status != pbpipeline.DestinationStatus_DESTINATION_STATUS_SUCCESS {
		t.Errorf("destination outcome enums: %+v", d)
	}
	if d.ExternalId == nil || *d.ExternalId != "ext-1" {
		t.Errorf("external id lost")
	}
	if d.CompletedAt == nil {
		t.Errorf("completed_at lost")
	}
}

func TestPipelineRunToFirestore_OmitsEmptyCollections(t *testing.T) {
	orig := &pbpipeline.PipelineRun{Id: "r"}
	m := PipelineRunToFirestore(orig)
	if _, ok := m["boosters"]; ok {
		t.Error("boosters should be omitted when empty")
	}
	if _, ok := m["destinations"]; ok {
		t.Error("destinations should be omitted when empty")
	}
	if _, ok := m["start_time"]; ok {
		t.Error("start_time should be omitted when nil")
	}
	if _, ok := m["status_message"]; ok {
		t.Error("status_message should be omitted when nil")
	}
	if _, ok := m["original_payload_uri"]; ok {
		t.Error("original_payload_uri should be omitted when empty")
	}
}

func TestFirestoreToPipelineRun_StringAndIntVariants(t *testing.T) {
	m := map[string]interface{}{
		"type":   int(0),
		"status": float64(1),
		"boosters": []interface{}{
			map[string]interface{}{
				"provider_name": "b",
				"status":        "EXECUTION_STEP_STATUS_OK",
				"duration_ms":   int(5),
				"metadata":      map[string]interface{}{"k": "v"},
			},
		},
		"destinations": []interface{}{
			map[string]interface{}{
				"destination": "STRAVA", // short form (no DESTINATION_ prefix)
				"status":      float64(1),
			},
		},
	}
	got := FirestoreToPipelineRun(m)
	if len(got.Boosters) != 1 {
		t.Fatalf("boosters: %v", got.Boosters)
	}
	if got.Boosters[0].Status != pbpipeline.ExecutionStepStatus_EXECUTION_STEP_STATUS_OK {
		t.Errorf("booster string status: %v", got.Boosters[0].Status)
	}
	if got.Boosters[0].DurationMs != 5 {
		t.Errorf("booster int duration: %d", got.Boosters[0].DurationMs)
	}
	if got.Boosters[0].Metadata["k"] != "v" {
		t.Errorf("booster metadata: %v", got.Boosters[0].Metadata)
	}
	if len(got.Destinations) != 1 {
		t.Fatalf("destinations: %v", got.Destinations)
	}
	if got.Destinations[0].Destination != pbplugin.DestinationType_DESTINATION_STRAVA {
		t.Errorf("destination short-form string: %v", got.Destinations[0].Destination)
	}
}

func TestFirestoreToShowcasedActivity_ActivityDataJSON(t *testing.T) {
	// Exercise the activity_data protojson deserialization branch.
	orig := &pbactivity.StandardizedActivity{Id: "act-xyz", Source: pbactivity.ActivitySource_SOURCE_HEVY}
	b, err := protojson.Marshal(orig)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	m := map[string]interface{}{
		"showcase_id":   "sc",
		"activity_data": string(b),
		"enrichments":   []byte(`{}`), // []byte branch for enrichments
	}
	got := FirestoreToShowcasedActivity(m)
	if got.ActivityData == nil || got.ActivityData.Id != "act-xyz" {
		t.Errorf("activity_data not deserialized: %+v", got.ActivityData)
	}
	if got.Enrichments == nil {
		t.Error("enrichments []byte branch not handled")
	}
}

func TestShowcaseProfileToFirestore_NilOptionals(t *testing.T) {
	orig := &pbactivity.ShowcaseProfile{Slug: "s"}
	m := ShowcaseProfileToFirestore(orig)
	for _, k := range []string{"latest_activity_at", "created_at", "updated_at", "theme"} {
		if m[k] != nil {
			t.Errorf("%s should be nil, got %v", k, m[k])
		}
	}
}

func TestShowcaseProfileEntryToFirestore_NilStartTime(t *testing.T) {
	orig := &pbactivity.ShowcaseProfileEntry{ShowcaseId: "e"}
	m := ShowcaseProfileEntryToFirestore(orig)
	if m["start_time"] != nil {
		t.Errorf("start_time should be nil, got %v", m["start_time"])
	}
	// Optional pointer fields omitted when nil.
	if _, ok := m["avg_heart_rate"]; ok {
		t.Error("avg_heart_rate should be omitted when nil")
	}
	if _, ok := m["calories_kcal"]; ok {
		t.Error("calories_kcal should be omitted when nil")
	}
	if _, ok := m["sparkline"]; ok {
		t.Error("sparkline should be omitted when nil")
	}
}

func TestFirestoreToShowcasedActivity_NumericEnums(t *testing.T) {
	// int64/int/float64 branches for activity_type and source.
	got := FirestoreToShowcasedActivity(map[string]interface{}{
		"activity_type": int64(1),
		"source":        int64(1),
	})
	if got.ActivityType != pbactivity.ActivityType(1) || got.Source != pbactivity.ActivitySource(1) {
		t.Errorf("int64 enums: %v %v", got.ActivityType, got.Source)
	}
	got = FirestoreToShowcasedActivity(map[string]interface{}{
		"activity_type": int(1),
		"source":        float64(1),
	})
	if got.ActivityType != pbactivity.ActivityType(1) || got.Source != pbactivity.ActivitySource(1) {
		t.Errorf("int/float enums: %v %v", got.ActivityType, got.Source)
	}
}

func TestFirestoreToPersonalRecord_FloatPrevImprovement(t *testing.T) {
	got := FirestoreToPersonalRecord(map[string]interface{}{
		"previous_value": float64(3.5),
		"improvement":    float64(1.5),
		"activity_type":  int(1),
	})
	if got.PreviousValue == nil || *got.PreviousValue != 3.5 {
		t.Error("float64 previous_value")
	}
	if got.Improvement == nil || *got.Improvement != 1.5 {
		t.Error("float64 improvement")
	}
	if got.ActivityType != pbactivity.ActivityType(1) {
		t.Errorf("int activity_type: %v", got.ActivityType)
	}
}

func TestFirestoreToUploadedActivity_NumericEnums(t *testing.T) {
	got := FirestoreToUploadedActivity(map[string]interface{}{
		"source":      int(1),
		"destination": float64(1),
	})
	if got.Source != pbactivity.ActivitySource(1) || got.Destination != pbplugin.DestinationType(1) {
		t.Errorf("numeric enums: %v %v", got.Source, got.Destination)
	}
}

func TestFirestoreToShowcaseProfileEntry_SparklineFloat64Slice(t *testing.T) {
	// sparkline values as []float64 (native) branch.
	m := map[string]interface{}{
		"sparkline": map[string]interface{}{"metric": "hr", "values": []float64{1.1, 2.2}},
	}
	got := FirestoreToShowcaseProfileEntry(m)
	if got.Sparkline == nil || len(got.Sparkline.Values) != 2 || got.Sparkline.Values[0] != 1.1 {
		t.Errorf("sparkline []float64 path: %+v", got.Sparkline)
	}
}
