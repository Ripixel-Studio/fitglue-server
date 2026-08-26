// Command showcase-reboost re-runs a user's showcase history through a
// showcase-only pipeline, in chronological order, so that every showcase carries
// typed booster enrichments and (for strength sessions) Hevy sets — and so that
// order-dependent boosters (personal records, streaks, effort history, training
// load) are rebuilt from the beginning.
//
// For each session between -from and now it builds a StandardizedActivity from
// the richest data available:
//
//   - an existing showcase's durable blob (records, laps, sets, markers) when the
//     showcase already has one — this keeps FIT-derived detail intact;
//   - otherwise Strava detail + streams (via the shared sourcemap) for cardio, and
//     the Hevy workout for strength;
//   - a Hevy workout matched to a Strava activity within ±10 min contributes its
//     sets to that activity (Hevy is authoritative for strength; the Strava copy is
//     Hevy's/FitGlue's own sync of the same session and is not ingested twice).
//
// It then publishes one ActivityPayload per session to topic-raw-activity with
// PipelineId pre-set (the splitter passes it straight through), tagged with
// backfill_* metadata: the showcase uploader overwrites the matched showcase in
// place (URL, created_at, title preserved) or creates a new one, and the
// orchestrator keeps the source description verbatim. It waits for each pipeline
// run to reach a terminal state before publishing the next, which is what makes
// the state-mutating boosters see history in order.
//
// Dry-run by default:
//
//	go run ./cmd/showcase-reboost -user <uid> -pipeline <showcase-only pipeline id>            # plan only
//	go run ./cmd/showcase-reboost -user <uid> -pipeline <id> -apply [-limit N] [-only <key,...>]
//
// Progress is journaled to -state (JSON) so an interrupted run resumes where it
// stopped. Requires ADC with Firestore/GCS/Pub/Sub/Secret Manager access.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	firestore "cloud.google.com/go/firestore"
	"cloud.google.com/go/pubsub" //nolint:staticcheck // v1 client, same as services/pipeline
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	"github.com/google/uuid"
	"google.golang.org/api/iterator"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/activity"
	infra "github.com/fitglue/server/src/go/internal/infra"
	shared "github.com/fitglue/server/src/go/pkg"
	stravaapi "github.com/fitglue/server/src/go/pkg/api/strava"
	stravamap "github.com/fitglue/server/src/go/pkg/domain/sourcemap/strava"
	infrapubsub "github.com/fitglue/server/src/go/pkg/infrastructure/pubsub"
	"github.com/fitglue/server/src/go/pkg/sourceplugins"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
	pbpipeline "github.com/fitglue/server/src/go/pkg/types/pb/models/pipeline"
	pbplugin "github.com/fitglue/server/src/go/pkg/types/pb/models/plugin"
)

const (
	stravaAPI        = "https://www.strava.com/api/v3"
	hevyAPI          = "https://api.hevyapp.com/v1"
	hevyStravaWindow = 10 * time.Minute // Hevy workout ↔ its own Strava sync
	showcaseWindow   = 5 * time.Minute  // session ↔ existing showcase
)

type stravaSummary struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Type      string    `json:"type"`
	SportType string    `json:"sport_type"`
	StartDate string    `json:"start_date"`
	Manual    bool      `json:"manual"`
	start     time.Time // parsed
}

type hevyWorkout struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	StartTime string `json:"start_time"`
	start     time.Time
}

// session is one thing the user did, from whichever sources describe it.
type session struct {
	key      string // stable id for the state journal
	start    time.Time
	strava   *stravaSummary
	hevy     *hevyWorkout
	existing *pbactivity.ShowcaseProfileEntry
}

type journalEntry struct {
	ExecutionID string    `json:"execution_id"`
	ShowcaseID  string    `json:"showcase_id,omitempty"`
	Status      string    `json:"status"`
	At          time.Time `json:"at"`
}

func main() {
	var (
		userID   = flag.String("user", "", "FitGlue user ID (required)")
		pipeline = flag.String("pipeline", "", "showcase-only pipeline ID to run (required)")
		project  = flag.String("project", "fitglue-server-prod", "GCP project")
		from     = flag.String("from", "2026-01-01", "earliest session date (YYYY-MM-DD, UTC)")
		apply    = flag.Bool("apply", false, "publish (default is dry-run)")
		limit    = flag.Int("limit", 0, "max sessions to publish this run (0 = all)")
		only     = flag.String("only", "", "comma-separated session keys to process (see dry-run output)")
		state    = flag.String("state", "showcase-reboost-state.json", "journal of completed sessions (resume support)")
		timeout  = flag.Duration("timeout", 6*time.Minute, "max wait per pipeline run before moving on")
		newOnly  = flag.Bool("new-only", false, "skip sessions that already have a showcase")
	)
	flag.Parse()
	if *userID == "" || *pipeline == "" {
		flag.Usage()
		os.Exit(2)
	}
	fromT, err := time.Parse("2006-01-02", *from)
	must(err, "parse -from")

	ctx := context.Background()
	fs, err := firestore.NewClient(ctx, *project)
	must(err, "firestore")
	defer func() { _ = fs.Close() }()
	store := activity.NewFirestoreStore(fs)

	// Pipeline must exist, be enabled and be showcase-only — this is the guard
	// against accidentally re-uploading a year of history to Strava/Hevy.
	pdoc, err := fs.Collection("users").Doc(*userID).Collection("pipelines").Doc(*pipeline).Get(ctx)
	must(err, "pipeline doc")
	dests, _ := pdoc.Data()["destinations"].([]interface{})
	if len(dests) != 1 || fmt.Sprint(dests[0]) != "DESTINATION_SHOWCASE" {
		log.Fatalf("pipeline %s destinations = %v; refusing to run anything but a showcase-only pipeline", *pipeline, dests)
	}

	journal := loadJournal(*state)
	onlySet := map[string]bool{}
	for _, k := range strings.Split(*only, ",") {
		if k = strings.TrimSpace(k); k != "" {
			onlySet[k] = true
		}
	}

	// ---- sources ----
	integ := loadIntegrations(ctx, fs, *userID)
	strava, err := newStravaClient(ctx, *project, integ)
	must(err, "strava auth")
	stravaActs, err := strava.list(ctx, fromT.Add(-24*time.Hour))
	must(err, "list strava")
	hevy := &hevyClient{key: str(integ["hevy"], "api_key", "apiKey"), http: &http.Client{Timeout: 60 * time.Second}}
	var hevyWorkouts []*hevyWorkout
	if hevy.key != "" {
		hevyWorkouts, err = hevy.list(ctx, fromT.Add(-24*time.Hour))
		must(err, "list hevy")
	}
	entries, err := listShowcaseEntries(ctx, fs, *userID)
	must(err, "list showcases")
	dated := 0
	for _, e := range entries {
		if e.StartTime != nil {
			dated++
		}
	}
	log.Printf("user %s: strava=%d hevy=%d showcases=%d (%d with start time) since %s", *userID, len(stravaActs), len(hevyWorkouts), len(entries), dated, fromT.Format("2006-01-02"))

	// ---- build sessions ----
	sessions := buildSessions(stravaActs, hevyWorkouts, entries, fromT)
	log.Printf("sessions: %d", len(sessions))

	pub := &infrapubsub.PubSubAdapter{Logger: infra.NewLoggerWithComponent("showcase-reboost")}
	if *apply {
		pc, err := pubsub.NewClient(ctx, *project)
		must(err, "pubsub")
		defer func() { _ = pc.Close() }()
		pub.Client = pc
	}

	published, skipped, failed := 0, 0, 0
	for _, s := range sessions {
		if len(onlySet) > 0 && !onlySet[s.key] {
			continue
		}
		if *newOnly && s.existing != nil {
			continue
		}
		if j, ok := journal[s.key]; ok && j.Status == "SUCCESS" {
			skipped++
			continue
		}
		if *limit > 0 && published+failed >= *limit {
			log.Printf("limit %d reached", *limit)
			break
		}
		if !*apply {
			log.Printf("  PLAN  %-34s %s %s", s.key, s.start.Format("2006-01-02 15:04"), describe(s))
			published++
			continue
		}

		act, raw, err := buildActivity(ctx, store, strava, hevy, *userID, s)
		if err != nil {
			log.Printf("  FAIL  %s: build: %v", s.key, err)
			journal[s.key] = journalEntry{Status: "BUILD_FAILED", At: time.Now()}
			saveJournal(*state, journal)
			failed++
			continue
		}

		execID := fmt.Sprintf("%s-%s", uuid.NewString(), *pipeline)
		payload := &pbevents.ActivityPayload{
			Source:               act.Source,
			UserId:               *userID,
			Timestamp:            act.StartTime,
			OriginalPayloadJson:  raw,
			StandardizedActivity: act,
			PipelineId:           pipeline,
			PipelineExecutionId:  &execID,
			Metadata: map[string]string{
				"backfill":                      "true",
				"backfill_verbatim_description": "true",
				"backfill_session_key":          s.key,
			},
		}
		if s.existing != nil {
			payload.Metadata["backfill_target_showcase_id"] = s.existing.ShowcaseId
		}
		ce, err := infrapubsub.NewCloudEvent("com.fitglue.cloud_event.showcase_reboost", "com.fitglue.backfill", payload)
		must(err, "cloud event")
		ce.SetID(execID)
		ce.SetTime(time.Now())
		ce.SetExtension("pipeline_execution_id", execID)
		if _, err := pub.PublishCloudEvent(ctx, shared.TopicRawActivity, ce); err != nil {
			log.Printf("  FAIL  %s: publish: %v", s.key, err)
			failed++
			continue
		}
		status, showcaseID := waitForRun(ctx, fs, *userID, execID, *timeout)
		journal[s.key] = journalEntry{ExecutionID: execID, ShowcaseID: showcaseID, Status: status, At: time.Now()}
		saveJournal(*state, journal)
		if status == "SUCCESS" {
			log.Printf("  DONE  %-34s %s → %s", s.key, s.start.Format("2006-01-02"), showcaseID)
			published++
		} else {
			log.Printf("  %-5s %-34s %s exec=%s", status, s.key, s.start.Format("2006-01-02"), execID)
			failed++
		}
	}
	log.Printf("summary: processed=%d skipped(done)=%d failed=%d dryRun=%v", published, skipped, failed, !*apply)
}

// listShowcaseEntries reads the user's showcases straight from Firestore. Some older
// docs (rewritten via protojson by cmd/showcase-backfill) hold start_time as an
// RFC 3339 string rather than a Timestamp, which the typed store surfaces as a nil
// start — and a nil start can never match a session. Both forms are handled here.
func listShowcaseEntries(ctx context.Context, fs *firestore.Client, userID string) ([]*pbactivity.ShowcaseProfileEntry, error) {
	iter := fs.Collection("showcased_activities").Where("user_id", "==", userID).Documents(ctx)
	defer iter.Stop()
	var out []*pbactivity.ShowcaseProfileEntry
	for {
		doc, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, err
		}
		data := doc.Data()
		e := &pbactivity.ShowcaseProfileEntry{ShowcaseId: doc.Ref.ID}
		e.Title, _ = data["title"].(string)
		switch v := data["start_time"].(type) {
		case time.Time:
			e.StartTime = timestamppb.New(v)
		case string:
			if t, err := time.Parse(time.RFC3339Nano, v); err == nil {
				e.StartTime = timestamppb.New(t)
			}
		}
		out = append(out, e)
	}
	return out, nil
}

// ---- session assembly ----

func buildSessions(stravaActs []*stravaSummary, hevyWorkouts []*hevyWorkout, entries []*pbactivity.ShowcaseProfileEntry, from time.Time) []*session {
	usedStrava := map[int64]bool{}
	var out []*session
	for _, h := range hevyWorkouts {
		if h.start.Before(from) {
			continue
		}
		s := &session{key: "hevy:" + h.ID, start: h.start, hevy: h}
		if m := nearestStrava(stravaActs, h.start, hevyStravaWindow, usedStrava); m != nil {
			usedStrava[m.ID] = true
			s.strava = m
		}
		out = append(out, s)
	}
	for _, a := range stravaActs {
		if usedStrava[a.ID] || a.start.Before(from) {
			continue
		}
		out = append(out, &session{key: fmt.Sprintf("strava:%d", a.ID), start: a.start, strava: a})
	}
	usedEntry := map[string]bool{}
	sort.Slice(out, func(i, j int) bool { return out[i].start.Before(out[j].start) })
	for _, s := range out {
		var best *pbactivity.ShowcaseProfileEntry
		var bestD time.Duration
		for _, e := range entries {
			if e.StartTime == nil || usedEntry[e.ShowcaseId] {
				continue
			}
			d := absDur(e.StartTime.AsTime().Sub(s.start))
			if d <= showcaseWindow && (best == nil || d < bestD) {
				best, bestD = e, d
			}
		}
		if best != nil {
			usedEntry[best.ShowcaseId] = true
			s.existing = best
		}
	}
	// Hevy holds both the user's own log and FitGlue's Hevy-destination copy of the
	// same session, at the same start time. A Hevy session that has no showcase of
	// its own and sits within the showcase window of another Hevy session is that
	// copy: skip it rather than mint a duplicate showcase. Sessions that do map to
	// an existing showcase are always kept, so existing duplicates stay consistent.
	// The same applies to Strava-only sessions (Strava also ends up with two copies
	// of a session when both the user's device and FitGlue uploaded it).
	var deduped []*session
	for i, s := range out {
		if s.existing == nil {
			dup := false
			for j, o := range out {
				if i != j && absDur(o.start.Sub(s.start)) <= showcaseWindow && (o.existing != nil || j < i) {
					dup = true
					break
				}
			}
			if dup {
				log.Printf("  DUP   %-34s %s %s — same time as another session, skipped", s.key, s.start.Format("2006-01-02 15:04"), describe(s))
				continue
			}
		}
		deduped = append(deduped, s)
	}
	return deduped
}

func nearestStrava(acts []*stravaSummary, t time.Time, window time.Duration, used map[int64]bool) *stravaSummary {
	var best *stravaSummary
	var bestD time.Duration
	for _, a := range acts {
		if used[a.ID] {
			continue
		}
		d := absDur(a.start.Sub(t))
		if d <= window && (best == nil || d < bestD) {
			best, bestD = a, d
		}
	}
	return best
}

func describe(s *session) string {
	var parts []string
	if s.hevy != nil {
		parts = append(parts, fmt.Sprintf("hevy=%q", s.hevy.Title))
	}
	if s.strava != nil {
		parts = append(parts, fmt.Sprintf("strava=%d(%s)", s.strava.ID, s.strava.SportType))
	}
	if s.existing != nil {
		parts = append(parts, "→ reboost "+s.existing.ShowcaseId)
	} else {
		parts = append(parts, "→ NEW showcase")
	}
	return strings.Join(parts, " ")
}

// buildActivity assembles the richest StandardizedActivity for a session.
func buildActivity(ctx context.Context, store *activity.FirestoreStore, strava *stravaClient, hevy *hevyClient, userID string, s *session) (*pbactivity.StandardizedActivity, string, error) {
	var act *pbactivity.StandardizedActivity
	raw := ""

	// 1. Existing showcase blob — keeps FIT-derived laps/records/sets exactly as they were.
	if s.existing != nil {
		sc, err := store.GetShowcase(ctx, userID, s.existing.ShowcaseId)
		if err != nil {
			return nil, "", fmt.Errorf("get showcase: %w", err)
		}
		if sc != nil && sc.ActivityData != nil && len(sc.ActivityData.Sessions) == 1 && hasContent(sc.ActivityData) {
			act = sc.ActivityData
			act.UserId = userID
			act.Id = ""
			act.PipelineRunStatus = ""
			if act.Name == "" {
				act.Name = sc.Title
			}
			if act.Type == pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
				act.Type = sc.ActivityType
			}
			if act.Source == pbactivity.ActivitySource_SOURCE_UNSPECIFIED {
				act.Source = sc.Source
			}
			if act.Description == "" {
				act.Description = sc.Description
			}
			if act.StartTime == nil {
				act.StartTime = sc.StartTime
			}
		}
	}

	// 2. Strava detail + streams for anything not already captured.
	if act == nil && s.strava != nil {
		body, streams, err := strava.detailAndStreams(ctx, s.strava.ID)
		if err != nil {
			return nil, "", fmt.Errorf("strava: %w", err)
		}
		act, err = stravamap.MapToStandardizedActivity(body, userID, streams)
		if err != nil {
			return nil, "", fmt.Errorf("strava map: %w", err)
		}
		raw = string(body)
	}

	// 3. Hevy sets — the authoritative strength record.
	if s.hevy != nil {
		hbody, err := hevy.get(ctx, s.hevy.ID)
		if err != nil {
			return nil, "", fmt.Errorf("hevy: %w", err)
		}
		hact, err := sourceplugins.HevyMapToStandardizedActivity(hbody, userID)
		if err != nil {
			return nil, "", fmt.Errorf("hevy map: %w", err)
		}
		if act == nil {
			act = hact
			raw = string(hbody)
		} else if len(act.Sessions) == 1 && len(act.Sessions[0].StrengthSets) == 0 && len(hact.Sessions) == 1 {
			act.Sessions[0].StrengthSets = hact.Sessions[0].StrengthSets
			if act.Workout == nil {
				act.Workout = hact.Workout
			}
			if act.Source == pbactivity.ActivitySource_SOURCE_STRAVA || act.Source == pbactivity.ActivitySource_SOURCE_UNSPECIFIED {
				act.Source = pbactivity.ActivitySource_SOURCE_HEVY
				act.ExternalId = hact.ExternalId
			}
			if act.Type == pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED {
				act.Type = pbactivity.ActivityType_ACTIVITY_TYPE_WEIGHT_TRAINING
			}
		}
	}

	if act == nil {
		return nil, "", fmt.Errorf("no source data")
	}
	if len(act.Sessions) != 1 {
		return nil, "", fmt.Errorf("expected 1 session, got %d", len(act.Sessions))
	}
	if act.Sessions[0].TotalElapsedTime <= 0 {
		return nil, "", fmt.Errorf("session has no elapsed time")
	}
	if act.StartTime == nil {
		act.StartTime = timestamppb.New(s.start)
	}
	if act.Sessions[0].StartTime == nil {
		act.Sessions[0].StartTime = act.StartTime
	}
	if act.Type == pbactivity.ActivityType_ACTIVITY_TYPE_UNSPECIFIED && s.strava != nil {
		act.Type = stravamap.MapActivityType(stravaapi.ActivityType(s.strava.SportType))
	}
	return act, raw, nil
}

func hasContent(a *pbactivity.StandardizedActivity) bool {
	for _, sess := range a.Sessions {
		if len(sess.StrengthSets) > 0 {
			return true
		}
		for _, lap := range sess.Laps {
			if len(lap.Records) > 0 {
				return true
			}
		}
	}
	return false
}

// ---- pipeline run polling ----

// waitForRun blocks until the run's own status and its showcase destination are
// terminal (or timeout). Returns the run status name and the showcase ID.
func waitForRun(ctx context.Context, fs *firestore.Client, userID, execID string, timeout time.Duration) (string, string) {
	deadline := time.Now().Add(timeout)
	ref := fs.Collection("users").Doc(userID).Collection("pipeline_runs").Doc(execID)
	for time.Now().Before(deadline) {
		time.Sleep(5 * time.Second)
		doc, err := ref.Get(ctx)
		if err != nil {
			continue
		}
		data := doc.Data()
		runStatus := fmt.Sprint(data["status"])
		switch runStatus {
		case fmt.Sprint(int(pbpipeline.ExecutionStatus_STATUS_FAILED)), pbpipeline.ExecutionStatus_STATUS_FAILED.String():
			return "FAILED", ""
		case fmt.Sprint(int(pbpipeline.ExecutionStatus_STATUS_SKIPPED)), pbpipeline.ExecutionStatus_STATUS_SKIPPED.String():
			return "SKIPPED", ""
		case fmt.Sprint(int(pbpipeline.ExecutionStatus_STATUS_SUCCESS)), pbpipeline.ExecutionStatus_STATUS_SUCCESS.String():
		default:
			continue
		}
		showcaseEnum := fmt.Sprint(int(pbplugin.DestinationType_DESTINATION_SHOWCASE))
		dests, _ := data["destinations"].([]interface{})
		for _, d := range dests {
			m, _ := d.(map[string]interface{})
			dest := fmt.Sprint(m["destination"])
			if dest != pbplugin.DestinationType_DESTINATION_SHOWCASE.String() && dest != showcaseEnum {
				continue
			}
			switch fmt.Sprint(m["status"]) {
			case fmt.Sprint(int(pbpipeline.ExecutionStatus_STATUS_SUCCESS)), pbpipeline.ExecutionStatus_STATUS_SUCCESS.String():
				return "SUCCESS", fmt.Sprint(m["external_id"])
			case fmt.Sprint(int(pbpipeline.ExecutionStatus_STATUS_FAILED)), pbpipeline.ExecutionStatus_STATUS_FAILED.String():
				return "DEST_FAILED", ""
			}
		}
	}
	return "TIMEOUT", ""
}

// ---- journal ----

func loadJournal(path string) map[string]journalEntry {
	j := map[string]journalEntry{}
	b, err := os.ReadFile(path)
	if err == nil {
		_ = json.Unmarshal(b, &j)
	}
	return j
}

func saveJournal(path string, j map[string]journalEntry) {
	b, _ := json.MarshalIndent(j, "", "  ")
	_ = os.WriteFile(path, b, 0o600)
}

// ---- integrations / Strava / Hevy ----

func loadIntegrations(ctx context.Context, fs *firestore.Client, userID string) map[string]map[string]interface{} {
	doc, err := fs.Collection("users").Doc(userID).Get(ctx)
	must(err, "user doc")
	raw, _ := doc.DataAt("integrations")
	out := map[string]map[string]interface{}{}
	for k, v := range raw.(map[string]interface{}) {
		if m, ok := v.(map[string]interface{}); ok {
			out[k] = m
		}
	}
	return out
}

func str(m map[string]interface{}, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

type stravaClient struct {
	http  *http.Client
	token string
}

func newStravaClient(ctx context.Context, project string, integ map[string]map[string]interface{}) (*stravaClient, error) {
	refreshToken := str(integ["strava"], "refresh_token", "refreshToken")
	if refreshToken == "" {
		return nil, fmt.Errorf("no strava refresh token on user")
	}
	sm, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = sm.Close() }()
	secret := func(name string) (string, error) {
		res, err := sm.AccessSecretVersion(ctx, &secretmanagerpb.AccessSecretVersionRequest{
			Name: fmt.Sprintf("projects/%s/secrets/%s/versions/latest", project, name),
		})
		if err != nil {
			return "", err
		}
		return string(res.Payload.Data), nil
	}
	clientID, err := secret("strava-client-id")
	if err != nil {
		return nil, err
	}
	clientSecret, err := secret("strava-client-secret")
	if err != nil {
		return nil, err
	}
	form := url.Values{
		"client_id": {clientID}, "client_secret": {clientSecret},
		"grant_type": {"refresh_token"}, "refresh_token": {refreshToken},
	}
	resp, err := http.PostForm("https://www.strava.com/oauth/token", form)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("strava token refresh HTTP %d", resp.StatusCode)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return nil, fmt.Errorf("strava token refresh: bad response")
	}
	return &stravaClient{http: &http.Client{Timeout: 60 * time.Second}, token: tok.AccessToken}, nil
}

func (c *stravaClient) getRaw(ctx context.Context, path string) ([]byte, error) {
	for attempt := 0; ; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, stravaAPI+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", "Bearer "+c.token)
		resp, err := c.http.Do(req)
		if err != nil {
			return nil, err
		}
		body, err := io.ReadAll(io.LimitReader(resp.Body, 64<<20))
		_ = resp.Body.Close()
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			// 15-minute window; wait it out rather than fail a chronological run.
			log.Printf("        strava 429 — sleeping 15m (attempt %d)", attempt+1)
			time.Sleep(15*time.Minute + 10*time.Second)
			continue
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("strava %s: HTTP %d: %.200s", path, resp.StatusCode, body)
		}
		return body, nil
	}
}

func (c *stravaClient) list(ctx context.Context, after time.Time) ([]*stravaSummary, error) {
	var all []*stravaSummary
	for page := 1; page < 50; page++ {
		body, err := c.getRaw(ctx, fmt.Sprintf("/athlete/activities?per_page=200&page=%d&after=%d", page, after.Unix()))
		if err != nil {
			return nil, err
		}
		var batch []*stravaSummary
		if err := json.Unmarshal(body, &batch); err != nil {
			return nil, err
		}
		for _, a := range batch {
			a.start, _ = time.Parse(time.RFC3339, a.StartDate)
			if a.SportType == "" {
				a.SportType = a.Type
			}
		}
		all = append(all, batch...)
		if len(batch) < 200 {
			break
		}
	}
	return all, nil
}

func (c *stravaClient) detailAndStreams(ctx context.Context, id int64) ([]byte, *stravaapi.StreamSet, error) {
	body, err := c.getRaw(ctx, fmt.Sprintf("/activities/%d", id))
	if err != nil {
		return nil, nil, err
	}
	keys := make([]string, len(stravamap.StreamKeys))
	for i, k := range stravamap.StreamKeys {
		keys[i] = string(k)
	}
	var streams *stravaapi.StreamSet
	sbody, err := c.getRaw(ctx, fmt.Sprintf("/activities/%d/streams?key_by_type=true&keys=%s", id, strings.Join(keys, ",")))
	if err == nil {
		var ss stravaapi.StreamSet
		if json.Unmarshal(sbody, &ss) == nil {
			streams = &ss
		}
	} else if !strings.Contains(err.Error(), "HTTP 404") {
		return nil, nil, err
	}
	return body, streams, nil
}

type hevyClient struct {
	key  string
	http *http.Client
}

func (h *hevyClient) getRaw(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, hevyAPI+path, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("api-key", h.key)
	resp, err := h.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 16<<20))
	if err != nil {
		return nil, err
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("hevy %s: HTTP %d: %.200s", path, resp.StatusCode, body)
	}
	return body, nil
}

func (h *hevyClient) list(ctx context.Context, after time.Time) ([]*hevyWorkout, error) {
	var all []*hevyWorkout
	for page := 1; page < 500; page++ {
		body, err := h.getRaw(ctx, fmt.Sprintf("/workouts?page=%d&pageSize=10", page))
		if err != nil {
			return nil, err
		}
		var res struct {
			PageCount int            `json:"page_count"`
			Workouts  []*hevyWorkout `json:"workouts"`
		}
		if err := json.Unmarshal(body, &res); err != nil {
			return nil, err
		}
		stop := false
		for _, w := range res.Workouts {
			w.start, _ = time.Parse(time.RFC3339, w.StartTime)
			if w.start.Before(after) {
				stop = true // newest-first paging
				continue
			}
			all = append(all, w)
		}
		if stop || page >= res.PageCount {
			break
		}
	}
	return all, nil
}

func (h *hevyClient) get(ctx context.Context, id string) ([]byte, error) {
	return h.getRaw(ctx, "/workouts/"+id)
}

func absDur(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}

func must(err error, what string) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}
