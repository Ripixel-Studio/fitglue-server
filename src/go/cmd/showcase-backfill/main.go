// Command showcase-backfill restores lost activity data for showcases whose
// GCS payloads were lifecycle-deleted before the durable showcase-assets
// bucket existed (pre-2026-07-17; see #34, #35).
//
// Strava holds the enriched activities FitGlue uploaded — including the
// generated descriptions and full time series (heart rate, GPS, altitude,
// speed, cadence, power, temperature). For each showcase whose payload is
// missing, the tool matches the Strava activity by start time, reconstructs a
// StandardizedActivity from streams + summary, writes JSON and a regenerated
// FIT file to the durable bucket, and repoints the showcase document.
//
// Typed enrichments (weather, effort score, zone tables) are NOT restored —
// they survive only as rendered text inside the description. Hevy strength
// sets are likewise out of scope (a future Hevy-sourced backfill could add
// them).
//
// Dry-run by default:
//
//	go run ./cmd/showcase-backfill -user <uid>            # plan only
//	go run ./cmd/showcase-backfill -user <uid> -apply     # write
//
// Requires ADC with Firestore/GCS/Secret Manager access to the target project.
// Idempotent: showcases whose payload object exists are skipped, so an
// interrupted run (e.g. Strava rate limit) can simply be re-run.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"math"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	firestore "cloud.google.com/go/firestore"
	secretmanager "cloud.google.com/go/secretmanager/apiv1"
	"cloud.google.com/go/secretmanager/apiv1/secretmanagerpb"
	gcs "cloud.google.com/go/storage"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/fitglue/server/src/go/internal/activity"
	"github.com/fitglue/server/src/go/pkg/domain/file_generators"
	"github.com/fitglue/server/src/go/pkg/infrastructure/storage"
	pbactivity "github.com/fitglue/server/src/go/pkg/types/pb/models/activity"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
)

const (
	stravaAPI   = "https://www.strava.com/api/v3"
	matchWindow = 5 * time.Minute
	callPause   = 9 * time.Second // 2 calls per restore: stays inside Strava's 200 req/15min
)

func main() {
	var (
		userID  = flag.String("user", "", "FitGlue user ID (required)")
		project = flag.String("project", "fitglue-server-prod", "GCP project")
		apply   = flag.Bool("apply", false, "write changes (default is dry-run)")
		limit   = flag.Int("limit", 0, "max showcases to restore this run (0 = all)")
	)
	flag.Parse()
	if *userID == "" {
		flag.Usage()
		os.Exit(2)
	}

	ctx := context.Background()
	fs, err := firestore.NewClient(ctx, *project)
	must(err, "firestore client")
	defer fs.Close()
	gcsClient, err := gcs.NewClient(ctx)
	must(err, "gcs client")
	adapter := &storage.StorageAdapter{Client: gcsClient}
	store := activity.NewFirestoreStore(fs)

	artifactsBucket := *project + "-artifacts"
	durableBucket := *project + "-showcase-assets"

	// 1. Every showcase for the user, oldest first.
	entries, err := store.ListShowcases(ctx, *userID)
	must(err, "list showcases")
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].StartTime.AsTime().Before(entries[j].StartTime.AsTime())
	})
	log.Printf("user %s: %d showcases", *userID, len(entries))

	// 2. Candidates: payload object missing or never written.
	type candidate struct {
		sc *pbactivity.ShowcasedActivity
	}
	var candidates []candidate
	for _, e := range entries {
		sc, err := store.GetShowcase(ctx, *userID, e.ShowcaseId)
		if err != nil || sc == nil {
			log.Printf("  skip %s: fetch failed: %v", e.ShowcaseId, err)
			continue
		}
		if payloadExists(ctx, gcsClient, sc.ActivityDataUri) {
			continue
		}
		candidates = append(candidates, candidate{sc: sc})
	}
	log.Printf("candidates with missing payloads: %d", len(candidates))
	if len(candidates) == 0 {
		return
	}

	// 3. Strava token + full activity list for local matching.
	strava, err := newStravaClient(ctx, fs, *project, *userID)
	must(err, "strava auth")
	oldest := candidates[0].sc.StartTime.AsTime()
	acts, err := strava.listActivities(ctx, oldest.Add(-24*time.Hour))
	must(err, "list strava activities")
	log.Printf("strava activities fetched: %d (since %s)", len(acts), oldest.Format("2006-01-02"))

	restored, failed := 0, 0
	for _, c := range candidates {
		if *limit > 0 && restored >= *limit {
			log.Printf("limit %d reached", *limit)
			break
		}
		sc := c.sc
		match := closestActivity(acts, sc.StartTime.AsTime())
		if match == nil {
			log.Printf("  MISS  %-46s %s — no Strava activity within %s", sc.ShowcaseId, sc.StartTime.AsTime().Format("2006-01-02"), matchWindow)
			failed++
			continue
		}
		if !*apply {
			log.Printf("  PLAN  %-46s ← strava %d %q", sc.ShowcaseId, match.ID, match.Name)
			restored++
			continue
		}

		if err := restore(ctx, strava, adapter, store, durableBucket, *userID, sc, match); err != nil {
			log.Printf("  FAIL  %s: %v", sc.ShowcaseId, err)
			failed++
			continue
		}
		log.Printf("  DONE  %-46s ← strava %d", sc.ShowcaseId, match.ID)
		restored++
		time.Sleep(callPause)
	}
	log.Printf("summary: restored=%d failed=%d dryRun=%v (artifacts bucket %s untouched)", restored, failed, !*apply, artifactsBucket)
}

func restore(ctx context.Context, strava *stravaClient, adapter *storage.StorageAdapter,
	store *activity.FirestoreStore, durableBucket, userID string,
	sc *pbactivity.ShowcasedActivity, match *stravaActivity) error {

	detail, err := strava.getActivity(ctx, match.ID)
	if err != nil {
		return fmt.Errorf("detail: %w", err)
	}
	streams, err := strava.getStreams(ctx, match.ID)
	if err != nil {
		return fmt.Errorf("streams: %w", err)
	}

	act := reconstruct(userID, sc, detail, streams)

	// GetShowcase hydrates from an EnrichedActivityEvent envelope — a bare
	// StandardizedActivity unmarshals to an empty event under DiscardUnknown.
	event := &pbevents.EnrichedActivityEvent{
		ActivityId:   sc.ActivityId,
		UserId:       userID,
		Name:         sc.Title,
		Description:  act.Description,
		ActivityData: act,
	}
	jsonBytes, err := protojson.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	dataObj := fmt.Sprintf("showcase_data/%s/%s_data.json", userID, sc.ShowcaseId)
	if err := adapter.Write(ctx, durableBucket, dataObj, jsonBytes); err != nil {
		return fmt.Errorf("write json: %w", err)
	}
	sc.ActivityDataUri = fmt.Sprintf("gs://%s/%s", durableBucket, dataObj)

	if fitBytes, err := file_generators.GenerateFitFile(act); err == nil {
		fitObj := fmt.Sprintf("showcase_data/%s/%s.fit", userID, sc.ShowcaseId)
		if err := adapter.Write(ctx, durableBucket, fitObj, fitBytes); err != nil {
			return fmt.Errorf("write fit: %w", err)
		}
		sc.FitFileUri = fmt.Sprintf("gs://%s/%s", durableBucket, fitObj)
	} else {
		// A FIT that can't be generated (e.g. no records at all) shouldn't
		// block restoring the JSON payload.
		log.Printf("        fit generation skipped for %s: %v", sc.ShowcaseId, err)
	}

	if _, err := store.UpdateShowcase(ctx, userID, sc); err != nil {
		return fmt.Errorf("update doc: %w", err)
	}
	return nil
}

// reconstruct builds a StandardizedActivity from the Strava detail + streams.
func reconstruct(userID string, sc *pbactivity.ShowcasedActivity, d *stravaDetail, s *stravaStreams) *pbactivity.StandardizedActivity {
	start := sc.StartTime.AsTime()

	var records []*pbactivity.Record
	n := len(s.Time)
	minHR, maxHR, sumHR, hrCount := math.MaxInt32, 0, 0, 0
	for i := 0; i < n; i++ {
		r := &pbactivity.Record{Timestamp: timestamppb.New(start.Add(time.Duration(s.Time[i]) * time.Second))}
		if i < len(s.HeartRate) && s.HeartRate[i] > 0 {
			r.HeartRate = int32(s.HeartRate[i])
			sumHR += s.HeartRate[i]
			hrCount++
			if s.HeartRate[i] > maxHR {
				maxHR = s.HeartRate[i]
			}
			if s.HeartRate[i] < minHR {
				minHR = s.HeartRate[i]
			}
		}
		if i < len(s.LatLng) && len(s.LatLng[i]) == 2 {
			r.PositionLat = s.LatLng[i][0]
			r.PositionLong = s.LatLng[i][1]
		}
		if i < len(s.Altitude) {
			r.Altitude = s.Altitude[i]
		}
		if i < len(s.Velocity) {
			r.Speed = s.Velocity[i]
		}
		if i < len(s.Cadence) {
			r.Cadence = int32(s.Cadence[i])
		}
		if i < len(s.Watts) {
			r.Power = int32(s.Watts[i])
		}
		if i < len(s.Distance) {
			r.Distance = s.Distance[i]
		}
		records = append(records, r)
	}

	sess := &pbactivity.Session{
		StartTime:        timestamppb.New(start),
		TotalElapsedTime: float64(d.ElapsedTime),
		TotalDistance:    d.Distance,
		Laps: []*pbactivity.Lap{{
			StartTime:        timestamppb.New(start),
			TotalElapsedTime: float64(d.ElapsedTime),
			TotalDistance:    d.Distance,
			Records:          records,
		}},
	}
	if d.Calories > 0 {
		v := d.Calories
		sess.TotalCalories = &v
	}
	if hrCount > 0 {
		avg := int32(sumHR / hrCount)
		mx, mn := int32(maxHR), int32(minHR)
		sess.AvgHeartRate = &avg
		sess.MaxHeartRate = &mx
		sess.MinHeartRate = &mn
	}

	return &pbactivity.StandardizedActivity{
		Id:          sc.ActivityId,
		ExternalId:  fmt.Sprintf("%d", d.ID),
		UserId:      userID,
		Name:        sc.Title,
		Type:        sc.ActivityType,
		StartTime:   timestamppb.New(start),
		Description: d.Description, // as uploaded by FitGlue, enrichment text included
		Sessions:    []*pbactivity.Session{sess},
	}
}

func payloadExists(ctx context.Context, client *gcs.Client, uri string) bool {
	if uri == "" || !strings.HasPrefix(uri, "gs://") {
		return false
	}
	parts := strings.SplitN(strings.TrimPrefix(uri, "gs://"), "/", 2)
	if len(parts) != 2 {
		return false
	}
	_, err := client.Bucket(parts[0]).Object(parts[1]).Attrs(ctx)
	return err == nil
}

// ---- Strava client ----

type stravaClient struct {
	http  *http.Client
	token string
}

type stravaActivity struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	StartDate string `json:"start_date"`
	startTime time.Time
}

type stravaDetail struct {
	ID          int64   `json:"id"`
	Description string  `json:"description"`
	Distance    float64 `json:"distance"`
	ElapsedTime int     `json:"elapsed_time"`
	Calories    float64 `json:"calories"`
}

type stravaStreams struct {
	Time      []int       `json:"-"`
	HeartRate []int       `json:"-"`
	LatLng    [][]float64 `json:"-"`
	Altitude  []float64   `json:"-"`
	Velocity  []float64   `json:"-"`
	Cadence   []int       `json:"-"`
	Watts     []int       `json:"-"`
	Distance  []float64   `json:"-"`
}

func newStravaClient(ctx context.Context, fs *firestore.Client, project, userID string) (*stravaClient, error) {
	// Integrations live as a map field on the user document.
	doc, err := fs.Collection("users").Doc(userID).Get(ctx)
	if err != nil {
		return nil, fmt.Errorf("user doc: %w", err)
	}
	integrations, err := doc.DataAt("integrations")
	if err != nil {
		return nil, fmt.Errorf("user has no integrations field: %w", err)
	}
	stravaAny, ok := integrations.(map[string]interface{})["strava"]
	if !ok {
		return nil, fmt.Errorf("no strava integration on user")
	}
	data, _ := stravaAny.(map[string]interface{})
	str := func(keys ...string) string {
		for _, k := range keys {
			if v, ok := data[k].(string); ok && v != "" {
				return v
			}
		}
		return ""
	}
	refreshToken := str("refresh_token", "refreshToken")
	if refreshToken == "" {
		return nil, fmt.Errorf("no strava refresh token on integration")
	}

	sm, err := secretmanager.NewClient(ctx)
	if err != nil {
		return nil, err
	}
	defer sm.Close()
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
		return nil, fmt.Errorf("strava-client-id: %w", err)
	}
	clientSecret, err := secret("strava-client-secret")
	if err != nil {
		return nil, fmt.Errorf("strava-client-secret: %w", err)
	}

	// Always exchange the refresh token — simpler than trusting a stored expiry,
	// and Strava returns the same refresh token so nothing needs writing back.
	form := url.Values{
		"client_id": {clientID}, "client_secret": {clientSecret},
		"grant_type": {"refresh_token"}, "refresh_token": {refreshToken},
	}
	resp, err := http.PostForm(stravaAPI[:len(stravaAPI)-len("/api/v3")]+"/oauth/token", form)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("strava token refresh HTTP %d: %.200s", resp.StatusCode, body)
	}
	var tok struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(body, &tok); err != nil || tok.AccessToken == "" {
		return nil, fmt.Errorf("strava token refresh: bad response")
	}
	return &stravaClient{http: &http.Client{Timeout: 60 * time.Second}, token: tok.AccessToken}, nil
}

func (c *stravaClient) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, stravaAPI+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 32<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		return fmt.Errorf("strava rate limited (429) — re-run later; completed restores are skipped automatically")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("strava %s HTTP %d: %.200s", path, resp.StatusCode, body)
	}
	return json.Unmarshal(body, out)
}

func (c *stravaClient) listActivities(ctx context.Context, after time.Time) ([]stravaActivity, error) {
	var all []stravaActivity
	for page := 1; ; page++ {
		var batch []stravaActivity
		path := fmt.Sprintf("/athlete/activities?per_page=200&page=%d&after=%d", page, after.Unix())
		if err := c.get(ctx, path, &batch); err != nil {
			return nil, err
		}
		if len(batch) == 0 {
			break
		}
		all = append(all, batch...)
		time.Sleep(callPause)
	}
	for i := range all {
		t, err := time.Parse(time.RFC3339, all[i].StartDate)
		if err == nil {
			all[i].startTime = t
		}
	}
	return all, nil
}

func (c *stravaClient) getActivity(ctx context.Context, id int64) (*stravaDetail, error) {
	var d stravaDetail
	if err := c.get(ctx, fmt.Sprintf("/activities/%d", id), &d); err != nil {
		return nil, err
	}
	return &d, nil
}

func (c *stravaClient) getStreams(ctx context.Context, id int64) (*stravaStreams, error) {
	raw := map[string]struct {
		Data json.RawMessage `json:"data"`
	}{}
	path := fmt.Sprintf("/activities/%d/streams?key_by_type=true&keys=time,heartrate,latlng,altitude,velocity_smooth,cadence,watts,distance", id)
	if err := c.get(ctx, path, &raw); err != nil {
		return nil, err
	}
	s := &stravaStreams{}
	dec := func(key string, out any) {
		if v, ok := raw[key]; ok {
			_ = json.Unmarshal(v.Data, out)
		}
	}
	dec("time", &s.Time)
	dec("heartrate", &s.HeartRate)
	dec("latlng", &s.LatLng)
	dec("altitude", &s.Altitude)
	dec("velocity_smooth", &s.Velocity)
	dec("cadence", &s.Cadence)
	dec("watts", &s.Watts)
	dec("distance", &s.Distance)
	return s, nil
}

func closestActivity(acts []stravaActivity, target time.Time) *stravaActivity {
	var best *stravaActivity
	bestDiff := matchWindow + time.Second
	for i := range acts {
		diff := acts[i].startTime.Sub(target)
		if diff < 0 {
			diff = -diff
		}
		if diff < bestDiff {
			bestDiff = diff
			best = &acts[i]
		}
	}
	return best
}

func must(err error, what string) {
	if err != nil {
		log.Fatalf("%s: %v", what, err)
	}
}
