package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

// TokenSource yields a Firebase ID token to authenticate api-client requests.
type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

// StaticTokenSource returns a fixed ID token supplied by the operator.
type StaticTokenSource string

func (s StaticTokenSource) Token(context.Context) (string, error) {
	return string(s), nil
}

// RefreshTokenSource exchanges a Firebase refresh token for ID tokens via the
// Secure Token API, caching each ID token until shortly before expiry.
// Firebase may rotate the refresh token on exchange; the rotated value is kept.
type RefreshTokenSource struct {
	apiKey   string
	endpoint string
	client   *http.Client
	now      func() time.Time

	mu           sync.Mutex
	refreshToken string
	idToken      string
	expiry       time.Time
}

const secureTokenEndpoint = "https://securetoken.googleapis.com/v1/token"

// refreshSkew is how long before actual expiry a cached token is discarded.
const refreshSkew = time.Minute

func NewRefreshTokenSource(apiKey, refreshToken string) *RefreshTokenSource {
	return &RefreshTokenSource{
		apiKey:       apiKey,
		endpoint:     secureTokenEndpoint,
		client:       &http.Client{Timeout: 15 * time.Second},
		now:          time.Now,
		refreshToken: refreshToken,
	}
}

func (r *RefreshTokenSource) Token(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	if r.idToken != "" && r.now().Add(refreshSkew).Before(r.expiry) {
		return r.idToken, nil
	}

	form := url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {r.refreshToken},
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.endpoint+"?key="+url.QueryEscape(r.apiKey),
		strings.NewReader(form.Encode()))
	if err != nil {
		return "", fmt.Errorf("building token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := r.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("exchanging refresh token: %w", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", fmt.Errorf("reading token response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("token exchange failed (HTTP %d): %s", resp.StatusCode, truncate(string(body), 300))
	}

	var parsed struct {
		IDToken      string `json:"id_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    string `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return "", fmt.Errorf("decoding token response: %w", err)
	}
	if parsed.IDToken == "" {
		return "", fmt.Errorf("token exchange returned no id_token")
	}

	ttl := time.Hour
	if secs, err := strconv.Atoi(parsed.ExpiresIn); err == nil && secs > 0 {
		ttl = time.Duration(secs) * time.Second
	}
	r.idToken = parsed.IDToken
	r.expiry = r.now().Add(ttl)
	if parsed.RefreshToken != "" {
		r.refreshToken = parsed.RefreshToken
	}
	return r.idToken, nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
