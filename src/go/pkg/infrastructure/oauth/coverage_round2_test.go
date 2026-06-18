package oauth

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	domainuser "github.com/fitglue/server/src/go/pkg/domain/user"
	"github.com/fitglue/server/src/go/pkg/testing/mocks"
	pbuser "github.com/fitglue/server/src/go/pkg/types/pb/models/user"

	"github.com/stretchr/testify/assert"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// canceledCtx returns an already-canceled context so that http.DefaultClient.Do
// fails fast without reaching the network. This lets refreshToken cover its
// secret reads, provider→tokenURL switch, request construction and body building
// up to (and including) the Do-error branch for every provider.
func canceledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// TestRefreshToken_PerProviderRequestBuilding drives refreshToken directly for
// each supported provider. Both secrets are present, so execution reaches the
// HTTP exchange, which fails fast on the canceled context.
func TestRefreshToken_PerProviderRequestBuilding(t *testing.T) {
	providers := []string{"strava", "fitbit", "trainingpeaks", "polar", "google", "github", "spotify"}
	for _, p := range providers {
		t.Run(p, func(t *testing.T) {
			up := upper(p)
			t.Setenv(up+"_CLIENT_ID", "id")
			t.Setenv(up+"_CLIENT_SECRET", "secret")

			src := NewFirestoreTokenSourceFromDB(&mocks.MockDatabase{}, "user-123", p)
			_, err := src.refreshToken(canceledCtx(), "refresh-token")
			// Network is never reached (context canceled), so the Do call errors.
			assert.Error(t, err)
			assert.Contains(t, err.Error(), "refresh request failed")
		})
	}
}

func TestRefreshToken_MissingClientSecret(t *testing.T) {
	// client-id present, client-secret missing -> error on the second getSecret.
	t.Setenv("STRAVA_CLIENT_ID", "id")
	t.Setenv("STRAVA_CLIENT_SECRET", "")
	src := NewFirestoreTokenSourceFromDB(&mocks.MockDatabase{}, "u", "strava")
	_, err := src.refreshToken(context.Background(), "r")
	assert.ErrorContains(t, err, "STRAVA_CLIENT_SECRET")
}

func TestRefreshToken_UnsupportedProvider(t *testing.T) {
	// Secrets exist for the env-var name derived from the provider, so getSecret
	// passes and the provider→tokenURL switch hits its default branch.
	t.Setenv("BOGUS_CLIENT_ID", "id")
	t.Setenv("BOGUS_CLIENT_SECRET", "secret")
	src := NewFirestoreTokenSourceFromDB(&mocks.MockDatabase{}, "u", "bogus")
	_, err := src.refreshToken(context.Background(), "r")
	assert.ErrorContains(t, err, "unsupported provider for refresh: bogus")
}

// upper mirrors the env-var name transformation getSecret performs on the
// provider string (uppercase, hyphens→underscores).
func upper(s string) string {
	out := make([]byte, len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' {
			c -= 'a' - 'A'
		}
		if c == '-' {
			c = '_'
		}
		out[i] = c
	}
	return string(out)
}

// TestUsageTrackingTransport_RoundTrip covers both the success branch (which
// spawns the async usage-update goroutine) and the error branch.
func TestUsageTrackingTransport_RoundTrip(t *testing.T) {
	t.Run("SuccessTriggersUsageUpdate", func(t *testing.T) {
		updated := make(chan struct{}, 1)
		db := &mocks.MockDatabase{
			UpdateUserFunc: func(_ context.Context, _ string, _ map[string]interface{}) error {
				select {
				case updated <- struct{}{}:
				default:
				}
				return nil
			},
		}
		svc := &bootstrap.Service{DB: db}
		base := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return makeResp(http.StatusOK, "ok"), nil
		})
		tr := &UsageTrackingTransport{Base: base, Service: svc, UserID: "u", Provider: "strava"}
		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		resp, err := tr.RoundTrip(req)
		assert.NoError(t, err)
		assert.Equal(t, http.StatusOK, resp.StatusCode)
		// Wait for the async goroutine to invoke UpdateUser.
		select {
		case <-updated:
		case <-time.After(2 * time.Second):
			t.Error("expected async usage update to call UpdateUser")
		}
	})

	t.Run("ErrorSkipsUsageUpdate", func(t *testing.T) {
		db := &mocks.MockDatabase{
			UpdateUserFunc: func(_ context.Context, _ string, _ map[string]interface{}) error {
				t.Error("UpdateUser must not be called on transport error")
				return nil
			},
		}
		svc := &bootstrap.Service{DB: db}
		base := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return nil, assertErr{}
		})
		tr := &UsageTrackingTransport{Base: base, Service: svc, UserID: "u", Provider: "strava"}
		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		_, err := tr.RoundTrip(req)
		assert.Error(t, err)
		// Give any (erroneous) goroutine a chance to run before the test ends.
		time.Sleep(50 * time.Millisecond)
	})
}

type assertErr struct{}

func (assertErr) Error() string { return "dial fail" }

func TestNewFirestoreTokenSource_FromBootstrap(t *testing.T) {
	svc := &bootstrap.Service{DB: &mocks.MockDatabase{}}
	src := NewFirestoreTokenSource(svc, "user-123", "strava")
	assert.NotNil(t, src)
	assert.Equal(t, "user-123", src.userID)
	assert.Equal(t, "strava", src.provider)
	assert.NotNil(t, src.db)
}

// TestToken_PerProviderSwitchCases exercises the provider switch in Token() for
// each refreshable provider, returning a valid (not-yet-expired) token so the
// happy-path return is reached for every case.
func TestToken_PerProviderSwitchCases(t *testing.T) {
	future := timestamppb.New(time.Now().Add(2 * time.Hour))
	cases := []struct {
		provider string
		integ    *pbuser.UserIntegrations
	}{
		{"fitbit", &pbuser.UserIntegrations{Fitbit: &pbuser.FitbitIntegration{Enabled: true, AccessToken: "a", RefreshToken: "r", ExpiresAt: future}}},
		{"trainingpeaks", &pbuser.UserIntegrations{Trainingpeaks: &pbuser.TrainingPeaksIntegration{Enabled: true, AccessToken: "a", RefreshToken: "r", ExpiresAt: future}}},
		{"polar", &pbuser.UserIntegrations{Polar: &pbuser.PolarIntegration{Enabled: true, AccessToken: "a", RefreshToken: "r", ExpiresAt: future}}},
		{"google", &pbuser.UserIntegrations{Google: &pbuser.GoogleIntegration{Enabled: true, AccessToken: "a", RefreshToken: "r", ExpiresAt: future}}},
		{"spotify", &pbuser.UserIntegrations{Spotify: &pbuser.SpotifyIntegration{Enabled: true, AccessToken: "a", RefreshToken: "r", ExpiresAt: future}}},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			integ := tc.integ
			db := &mocks.MockDatabase{
				GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
					return &domainuser.Record{Integrations: integ}, nil
				},
			}
			tok, err := NewFirestoreTokenSourceFromDB(db, "u", tc.provider).Token(context.Background())
			assert.NoError(t, err)
			assert.Equal(t, "a", tok.AccessToken)
		})
	}
}

// TestForceRefresh_PerProviderSwitchCases exercises the provider switch in
// ForceRefresh() for the remaining providers. Each has a refresh token, so
// execution proceeds to refreshToken which fails fast on a missing secret.
func TestForceRefresh_PerProviderSwitchCases(t *testing.T) {
	cases := []struct {
		provider string
		integ    *pbuser.UserIntegrations
		envVar   string
	}{
		{"fitbit", &pbuser.UserIntegrations{Fitbit: &pbuser.FitbitIntegration{Enabled: true, RefreshToken: "r"}}, "FITBIT_CLIENT_ID"},
		{"trainingpeaks", &pbuser.UserIntegrations{Trainingpeaks: &pbuser.TrainingPeaksIntegration{Enabled: true, RefreshToken: "r"}}, "TRAININGPEAKS_CLIENT_ID"},
		{"polar", &pbuser.UserIntegrations{Polar: &pbuser.PolarIntegration{Enabled: true, RefreshToken: "r"}}, "POLAR_CLIENT_ID"},
		{"google", &pbuser.UserIntegrations{Google: &pbuser.GoogleIntegration{Enabled: true, RefreshToken: "r"}}, "GOOGLE_CLIENT_ID"},
		{"spotify", &pbuser.UserIntegrations{Spotify: &pbuser.SpotifyIntegration{Enabled: true, RefreshToken: "r"}}, "SPOTIFY_CLIENT_ID"},
	}
	for _, tc := range cases {
		t.Run(tc.provider, func(t *testing.T) {
			t.Setenv(tc.envVar, "") // force getSecret failure inside refreshToken
			integ := tc.integ
			db := &mocks.MockDatabase{
				GetUserFunc: func(_ context.Context, _ string) (*domainuser.Record, error) {
					return &domainuser.Record{Integrations: integ}, nil
				},
			}
			_, err := NewFirestoreTokenSourceFromDB(db, "u", tc.provider).ForceRefresh(context.Background())
			assert.ErrorContains(t, err, tc.envVar)
		})
	}
}

func TestNewClientWithUsageTracking_Constructors(t *testing.T) {
	logger := infra.NewLoggerWithComponent("test")
	src := &fakeTokenSource{token: "t"}
	svc := &bootstrap.Service{DB: &mocks.MockDatabase{}}

	c := NewClientWithUsageTracking(src, svc, "u", "strava", logger)
	assert.NotNil(t, c)
	assert.NotNil(t, c.Transport)

	c2 := NewClientWithUsageTrackingHTTP1(src, svc, "u", "strava", logger)
	assert.NotNil(t, c2)
	assert.NotNil(t, c2.Transport)
}
