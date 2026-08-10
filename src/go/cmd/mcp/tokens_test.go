package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStaticTokenSource(t *testing.T) {
	tok, err := StaticTokenSource("abc").Token(context.Background())
	if err != nil || tok != "abc" {
		t.Fatalf("Token() = %q, %v; want abc, nil", tok, err)
	}
}

func newTestRefreshSource(t *testing.T, handler http.HandlerFunc) *RefreshTokenSource {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)
	src := NewRefreshTokenSource("test-key", "initial-rt")
	src.endpoint = srv.URL
	return src
}

func TestRefreshTokenSourceExchangesAndCaches(t *testing.T) {
	calls := 0
	src := newTestRefreshSource(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if got := r.Form.Get("grant_type"); got != "refresh_token" {
			t.Errorf("grant_type = %q", got)
		}
		if got := r.Form.Get("refresh_token"); got != "initial-rt" {
			t.Errorf("refresh_token = %q", got)
		}
		if got := r.URL.Query().Get("key"); got != "test-key" {
			t.Errorf("key = %q", got)
		}
		fmt.Fprint(w, `{"id_token":"id-1","refresh_token":"rotated-rt","expires_in":"3600"}`)
	})

	for i := 0; i < 3; i++ {
		tok, err := src.Token(context.Background())
		if err != nil {
			t.Fatalf("Token() error = %v", err)
		}
		if tok != "id-1" {
			t.Fatalf("Token() = %q, want id-1", tok)
		}
	}
	if calls != 1 {
		t.Errorf("exchange calls = %d, want 1 (cached)", calls)
	}
	if src.refreshToken != "rotated-rt" {
		t.Errorf("refreshToken = %q, want rotated value kept", src.refreshToken)
	}
}

func TestRefreshTokenSourceRefreshesNearExpiry(t *testing.T) {
	calls := 0
	src := newTestRefreshSource(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		fmt.Fprintf(w, `{"id_token":"id-%d","expires_in":"3600"}`, calls)
	})

	now := time.Now()
	src.now = func() time.Time { return now }

	if tok, _ := src.Token(context.Background()); tok != "id-1" {
		t.Fatalf("first Token() = %q", tok)
	}

	// Just inside the skew window: must re-exchange.
	now = now.Add(time.Hour - 30*time.Second)
	tok, err := src.Token(context.Background())
	if err != nil {
		t.Fatalf("Token() error = %v", err)
	}
	if tok != "id-2" || calls != 2 {
		t.Errorf("Token() = %q after %d calls; want id-2 after 2", tok, calls)
	}
}

func TestRefreshTokenSourceErrors(t *testing.T) {
	t.Run("http error", func(t *testing.T) {
		src := newTestRefreshSource(t, func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, `{"error":{"message":"TOKEN_EXPIRED"}}`, http.StatusBadRequest)
		})
		if _, err := src.Token(context.Background()); err == nil {
			t.Fatal("Token() error = nil, want exchange failure")
		}
	})
	t.Run("missing id_token", func(t *testing.T) {
		src := newTestRefreshSource(t, func(w http.ResponseWriter, r *http.Request) {
			fmt.Fprint(w, `{}`)
		})
		if _, err := src.Token(context.Background()); err == nil {
			t.Fatal("Token() error = nil, want missing id_token failure")
		}
	})
}
