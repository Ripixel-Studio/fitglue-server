package oauth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"

	shared "github.com/fitglue/server/src/go/pkg"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
)

// nonRefreshableProviders are providers whose OAuth tokens don't expire
// and don't use refresh tokens (e.g. GitHub). For these providers we skip
// the refresh-token requirement and never attempt token refresh.
var nonRefreshableProviders = map[string]bool{
	"github": true,
}

// Token represents the OAuth token structure we care about
type Token struct {
	AccessToken  string
	RefreshToken string
	Expiry       time.Time
}

// TokenSource returns a valid token.
// It is safe for concurrent use by multiple goroutines.
type TokenSource interface {
	Token(context.Context) (*Token, error)
	ForceRefresh(context.Context) (*Token, error)
}

// FirestoreTokenSource reads from Firestore and refreshes if necessary.
type FirestoreTokenSource struct {
	db       shared.Database
	userID   string
	provider string
	mu       sync.Mutex
}

func NewFirestoreTokenSource(svc *bootstrap.Service, userID, provider string) *FirestoreTokenSource {
	return &FirestoreTokenSource{
		db:       svc.DB,
		userID:   userID,
		provider: provider,
	}
}

// NewFirestoreTokenSourceFromDB creates a token source from a shared.Database directly,
// for use in contexts that don't have a full bootstrap.Service (e.g. webhook providers).
func NewFirestoreTokenSourceFromDB(db shared.Database, userID, provider string) *FirestoreTokenSource {
	return &FirestoreTokenSource{
		db:       db,
		userID:   userID,
		provider: provider,
	}
}

// ForceRefresh forcibly refreshes the token regardless of expiry.
func (s *FirestoreTokenSource) ForceRefresh(ctx context.Context) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Non-refreshable providers cannot be refreshed; the user must re-connect.
	if nonRefreshableProviders[s.provider] {
		return nil, fmt.Errorf("%s tokens cannot be refreshed; user must re-connect", s.provider)
	}

	// 1. Fetch refresh token explicitly from DB again to be safe
	userData, err := s.db.GetUser(ctx, s.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if userData.Integrations == nil {
		return nil, fmt.Errorf("user has no integrations linked")
	}

	var refreshToken string
	switch s.provider {
	case "strava":
		if userData.Integrations.Strava == nil || !userData.Integrations.Strava.Enabled {
			return nil, fmt.Errorf("strava not linked/enabled")
		}
		refreshToken = userData.Integrations.Strava.RefreshToken
	case "fitbit":
		if userData.Integrations.Fitbit == nil || !userData.Integrations.Fitbit.Enabled {
			return nil, fmt.Errorf("fitbit not linked/enabled")
		}
		refreshToken = userData.Integrations.Fitbit.RefreshToken
	case "trainingpeaks":
		if userData.Integrations.Trainingpeaks == nil || !userData.Integrations.Trainingpeaks.Enabled {
			return nil, fmt.Errorf("trainingpeaks not linked/enabled")
		}
		refreshToken = userData.Integrations.Trainingpeaks.RefreshToken
	case "polar":
		if userData.Integrations.Polar == nil || !userData.Integrations.Polar.Enabled {
			return nil, fmt.Errorf("polar not linked/enabled")
		}
		refreshToken = userData.Integrations.Polar.RefreshToken
	case "google":
		if userData.Integrations.Google == nil || !userData.Integrations.Google.Enabled {
			return nil, fmt.Errorf("google not linked/enabled")
		}
		refreshToken = userData.Integrations.Google.RefreshToken
	case "spotify":
		if userData.Integrations.Spotify == nil || !userData.Integrations.Spotify.Enabled {
			return nil, fmt.Errorf("spotify not linked/enabled")
		}
		refreshToken = userData.Integrations.Spotify.RefreshToken
	default:
		return nil, fmt.Errorf("unknown provider %s", s.provider)
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("missing refresh token for %s", s.provider)
	}

	return s.refreshToken(ctx, refreshToken)
}

// Token returns a token, refreshing it if necessary.
func (s *FirestoreTokenSource) Token(ctx context.Context) (*Token, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	// 1. Fetch current token from Firestore
	userData, err := s.db.GetUser(ctx, s.userID)
	if err != nil {
		return nil, fmt.Errorf("failed to get user: %w", err)
	}

	if userData.Integrations == nil {
		return nil, fmt.Errorf("user has no integrations linked")
	}

	var accessToken, refreshToken string
	var expiry time.Time

	switch s.provider {
	case "strava":
		if userData.Integrations.Strava == nil || !userData.Integrations.Strava.Enabled {
			return nil, fmt.Errorf("strava not linked/enabled")
		}
		accessToken = userData.Integrations.Strava.AccessToken
		refreshToken = userData.Integrations.Strava.RefreshToken
		if userData.Integrations.Strava.ExpiresAt != nil {
			expiry = userData.Integrations.Strava.ExpiresAt.AsTime()
		}
	case "fitbit":
		if userData.Integrations.Fitbit == nil || !userData.Integrations.Fitbit.Enabled {
			return nil, fmt.Errorf("fitbit not linked/enabled")
		}
		accessToken = userData.Integrations.Fitbit.AccessToken
		refreshToken = userData.Integrations.Fitbit.RefreshToken
		if userData.Integrations.Fitbit.ExpiresAt != nil {
			expiry = userData.Integrations.Fitbit.ExpiresAt.AsTime()
		}
	case "trainingpeaks":
		if userData.Integrations.Trainingpeaks == nil || !userData.Integrations.Trainingpeaks.Enabled {
			return nil, fmt.Errorf("trainingpeaks not linked/enabled")
		}
		accessToken = userData.Integrations.Trainingpeaks.AccessToken
		refreshToken = userData.Integrations.Trainingpeaks.RefreshToken
		if userData.Integrations.Trainingpeaks.ExpiresAt != nil {
			expiry = userData.Integrations.Trainingpeaks.ExpiresAt.AsTime()
		}
	case "polar":
		if userData.Integrations.Polar == nil || !userData.Integrations.Polar.Enabled {
			return nil, fmt.Errorf("polar not linked/enabled")
		}
		accessToken = userData.Integrations.Polar.AccessToken
		refreshToken = userData.Integrations.Polar.RefreshToken
		if userData.Integrations.Polar.ExpiresAt != nil {
			expiry = userData.Integrations.Polar.ExpiresAt.AsTime()
		}
	case "google":
		if userData.Integrations.Google == nil || !userData.Integrations.Google.Enabled {
			return nil, fmt.Errorf("google not linked/enabled")
		}
		accessToken = userData.Integrations.Google.AccessToken
		refreshToken = userData.Integrations.Google.RefreshToken
		if userData.Integrations.Google.ExpiresAt != nil {
			expiry = userData.Integrations.Google.ExpiresAt.AsTime()
		}
	case "github":
		if userData.Integrations.Github == nil || !userData.Integrations.Github.Enabled {
			return nil, fmt.Errorf("github not linked/enabled")
		}
		accessToken = userData.Integrations.Github.AccessToken
		refreshToken = userData.Integrations.Github.RefreshToken
		if userData.Integrations.Github.ExpiresAt != nil {
			expiry = userData.Integrations.Github.ExpiresAt.AsTime()
		}
	case "spotify":
		if userData.Integrations.Spotify == nil || !userData.Integrations.Spotify.Enabled {
			return nil, fmt.Errorf("spotify not linked/enabled")
		}
		accessToken = userData.Integrations.Spotify.AccessToken
		refreshToken = userData.Integrations.Spotify.RefreshToken
		if userData.Integrations.Spotify.ExpiresAt != nil {
			expiry = userData.Integrations.Spotify.ExpiresAt.AsTime()
		}
	default:
		return nil, fmt.Errorf("unknown provider %s", s.provider)
	}

	if accessToken == "" {
		return nil, fmt.Errorf("missing access token for %s", s.provider)
	}

	// Non-refreshable providers (e.g. GitHub): return the token directly,
	// no refresh token or expiry check needed.
	if nonRefreshableProviders[s.provider] {
		return &Token{
			AccessToken:  accessToken,
			RefreshToken: "",
			Expiry:       expiry,
		}, nil
	}

	if refreshToken == "" {
		return nil, fmt.Errorf("missing refresh token for %s", s.provider)
	}

	// 2. Check Expiry (Proactive Refresh)
	// Refresh if expired or expiring in the next minute
	if !expiry.IsZero() && time.Now().Add(1*time.Minute).After(expiry) {
		return s.refreshToken(ctx, refreshToken)
	}

	return &Token{
		AccessToken:  accessToken,
		RefreshToken: refreshToken,
		Expiry:       expiry,
	}, nil
}

// refreshToken performs the HTTP exchange to get a new token & updates Firestore
func (s *FirestoreTokenSource) refreshToken(ctx context.Context, refreshToken string) (*Token, error) {
	clientID, err := s.getSecret("client-id")
	if err != nil {
		return nil, err
	}
	clientSecret, err := s.getSecret("client-secret")
	if err != nil {
		return nil, err
	}

	var tokenURL string
	switch s.provider {
	case "strava":
		tokenURL = "https://www.strava.com/oauth/token"
	case "fitbit":
		tokenURL = "https://api.fitbit.com/oauth2/token"
	case "trainingpeaks":
		tokenURL = "https://oauth.trainingpeaks.com/token"
	case "polar":
		tokenURL = "https://polarremote.com/v2/oauth2/token"
	case "google":
		tokenURL = "https://oauth2.googleapis.com/token"
	case "github":
		tokenURL = "https://github.com/login/oauth/access_token"
	case "spotify":
		tokenURL = "https://accounts.spotify.com/api/token"
	default:
		return nil, fmt.Errorf("unsupported provider for refresh: %s", s.provider)
	}

	data := url.Values{}
	// Strava requires client_id/secret in body. Fitbit and Spotify use Basic Auth header (see below).
	if s.provider != "fitbit" && s.provider != "spotify" {
		data.Set("client_id", clientID)
		data.Set("client_secret", clientSecret)
	}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", refreshToken)

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	if s.provider == "fitbit" || s.provider == "spotify" {
		req.SetBasicAuth(clientID, clientSecret)
	}

	// GitHub returns JSON only if we ask for it
	if s.provider == "github" {
		req.Header.Set("Accept", "application/json")
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("refresh failed with status: %d", resp.StatusCode)
	}

	// Parse Response
	var result struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
		ExpiresAt    int64  `json:"expires_at"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode refresh response: %w", err)
	}

	newExpiry := time.Now().Add(time.Duration(result.ExpiresIn) * time.Second)
	if result.ExpiresAt != 0 {
		newExpiry = time.Unix(result.ExpiresAt, 0)
	}

	// Update Firestore using a nested map so that Set+MergeAll computes leaf-level
	// field paths (e.g. integrations.fitbit.access_token) and does not overwrite
	// the entire integration sub-object (which would wipe enabled, fitbit_user_id, etc.).
	// Dotted-path string keys in a flat map are escaped by the Go Firestore SDK and
	// treated as literal field names, not nested paths — only nested maps produce the
	// correct leaf-path field mask.
	providerData := map[string]interface{}{
		"access_token": result.AccessToken,
		"expires_at":   newExpiry,
		"last_used_at": time.Now(),
	}
	// Only update refresh_token if the provider returned a new one.
	// Google and GitHub don't rotate refresh tokens on refresh, so
	// writing the empty response value would wipe the stored token.
	if result.RefreshToken != "" {
		providerData["refresh_token"] = result.RefreshToken
	}
	updateData := map[string]interface{}{
		"integrations": map[string]interface{}{
			s.provider: providerData,
		},
	}

	if err := s.db.UpdateUser(ctx, s.userID, updateData); err != nil {
		return nil, fmt.Errorf("failed to persist new tokens: %w", err)
	}

	// Preserve the original refresh token if the provider didn't return a new one
	newRefreshToken := result.RefreshToken
	if newRefreshToken == "" {
		newRefreshToken = refreshToken
	}

	return &Token{
		AccessToken:  result.AccessToken,
		RefreshToken: newRefreshToken,
		Expiry:       newExpiry,
	}, nil
}

func (s *FirestoreTokenSource) getSecret(keyType string) (string, error) {
	// Environment variables use uppercase with underscores
	// e.g., "strava-client-id" becomes "STRAVA_CLIENT_ID"
	envVarName := strings.ToUpper(strings.ReplaceAll(s.provider, "-", "_")) + "_" + strings.ToUpper(strings.ReplaceAll(keyType, "-", "_"))

	value := os.Getenv(envVarName)
	if value == "" {
		return "", fmt.Errorf("environment variable %s not found", envVarName)
	}

	return value, nil
}
