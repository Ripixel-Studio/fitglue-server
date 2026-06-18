package parkrun

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchDirectHTTP_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("expected User-Agent header to be set")
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("<html>ok</html>"))
	}))
	defer srv.Close()

	html, err := fetchDirectHTTP(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if html != "<html>ok</html>" {
		t.Errorf("unexpected body: %q", html)
	}
}

func TestFetchDirectHTTP_AcceptedStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte("accepted-body"))
	}))
	defer srv.Close()

	html, err := fetchDirectHTTP(context.Background(), srv.Client(), srv.URL)
	if err != nil {
		t.Fatalf("202 should be accepted, got error: %v", err)
	}
	if html != "accepted-body" {
		t.Errorf("unexpected body: %q", html)
	}
}

func TestFetchDirectHTTP_BadStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte("blocked"))
	}))
	defer srv.Close()

	_, err := fetchDirectHTTP(context.Background(), srv.Client(), srv.URL)
	if err == nil {
		t.Fatal("expected error for 403 status")
	}
	if !strings.Contains(err.Error(), "http_status_403") {
		t.Errorf("expected http_status_403 in error, got %v", err)
	}
}

func TestFetchDirectHTTP_BadURL(t *testing.T) {
	_, err := fetchDirectHTTP(context.Background(), &http.Client{}, "http://[::1]:namedport")
	if err == nil {
		t.Fatal("expected error for malformed URL")
	}
}

func TestFetchDirectHTTP_ConnRefused(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Port 1 is not listening.
	_, err := fetchDirectHTTP(ctx, &http.Client{Timeout: time.Second}, "http://127.0.0.1:1/")
	if err == nil {
		t.Fatal("expected connection error")
	}
	if !strings.Contains(err.Error(), "fetch results") {
		t.Errorf("expected fetch results error wrap, got %v", err)
	}
}

func TestFetchViaPlaywright_FallbackToDirect(t *testing.T) {
	// With PARKRUN_FETCHER_URL unset, FetchViaPlaywright falls back to direct HTTP.
	t.Setenv("PARKRUN_FETCHER_URL", "")

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("<html>fallback</html>"))
	}))
	defer srv.Close()

	html, err := FetchViaPlaywright(context.Background(), slog.Default(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if html != "<html>fallback</html>" {
		t.Errorf("unexpected html: %q", html)
	}
}
