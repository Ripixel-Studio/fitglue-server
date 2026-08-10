package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestAPI(t *testing.T, handler http.HandlerFunc) *APIClient {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	return NewAPIClient(srv.URL, StaticTokenSource("test-token"))
}

func TestAPIClientGet(t *testing.T) {
	api := newTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v2/users/me/activities" {
			t.Errorf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.URL.Query().Get("limit"); got != "5" {
			t.Errorf("limit = %q", got)
		}
		fmt.Fprint(w, `{"activities":[{"id":"a1"}]}`)
	})

	body, err := api.Get(context.Background(), "/users/me/activities", map[string][]string{"limit": {"5"}})
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if !strings.Contains(body, "\n  \"activities\"") {
		t.Errorf("Get() = %q, want pretty-printed JSON", body)
	}
}

func TestAPIClientErrors(t *testing.T) {
	cases := []struct {
		name   string
		status int
		want   string
	}{
		{"unauthorized", http.StatusUnauthorized, "unauthorized"},
		{"not found", http.StatusNotFound, "not found"},
		{"server error", http.StatusInternalServerError, "HTTP 500"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			api := newTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, "boom", tc.status)
			})
			_, err := api.Get(context.Background(), "/users/me", nil)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("Get() error = %v, want containing %q", err, tc.want)
			}
		})
	}
}

func TestAPIClientNonJSONPassthrough(t *testing.T) {
	api := newTestAPI(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, "plain text")
	})
	body, err := api.Get(context.Background(), "/users/me", nil)
	if err != nil {
		t.Fatalf("Get() error = %v", err)
	}
	if body != "plain text" {
		t.Errorf("Get() = %q, want passthrough", body)
	}
}

func TestPrettyJSON(t *testing.T) {
	if got := prettyJSON([]byte(`{"a":1}`)); got != "{\n  \"a\": 1\n}" {
		t.Errorf("prettyJSON = %q", got)
	}
	if got := prettyJSON([]byte("not json")); got != "not json" {
		t.Errorf("prettyJSON passthrough = %q", got)
	}
}

func TestTruncate(t *testing.T) {
	if got := truncate("abcdef", 3); got != "abc…" {
		t.Errorf("truncate = %q", got)
	}
	if got := truncate("ab", 3); got != "ab" {
		t.Errorf("truncate short = %q", got)
	}
}

func TestScrubSecrets(t *testing.T) {
	in := []byte(`{
		"fitbit": {"enabled": true, "accessToken": "tok-a", "refreshToken": "tok-r", "expiresAt": "2026-01-01"},
		"hevy": {"apiKey": "key-1"},
		"profile": {"fcmTokens": ["f1", "f2"], "tier": "HOBBYIST"},
		"pagination": {"next_page_token": "cursor-ok"},
		"nested": [{"client_secret": "s3cret"}]
	}`)
	out := string(scrubSecrets(in))
	for _, leaked := range []string{"tok-a", "tok-r", "key-1", "f1", "s3cret"} {
		if strings.Contains(out, leaked) {
			t.Errorf("scrubSecrets leaked %q", leaked)
		}
	}
	for _, kept := range []string{"2026-01-01", "HOBBYIST", "cursor-ok", "true"} {
		if !strings.Contains(out, kept) {
			t.Errorf("scrubSecrets dropped non-secret %q", kept)
		}
	}
	if !strings.Contains(out, "[redacted]") {
		t.Error("scrubSecrets produced no redaction markers")
	}
}

func TestScrubSecretsNonJSONPassthrough(t *testing.T) {
	if got := string(scrubSecrets([]byte("plain"))); got != "plain" {
		t.Errorf("scrubSecrets non-JSON = %q", got)
	}
}
