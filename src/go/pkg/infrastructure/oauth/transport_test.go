package oauth

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

// fakeTokenSource is a controllable TokenSource for tests.
type fakeTokenSource struct {
	token        string
	refreshToken string
	tokenErr     error
	refreshErr   error
	refreshed    bool
}

func (f *fakeTokenSource) Token(context.Context) (*Token, error) {
	if f.tokenErr != nil {
		return nil, f.tokenErr
	}
	return &Token{AccessToken: f.token}, nil
}

func (f *fakeTokenSource) ForceRefresh(context.Context) (*Token, error) {
	f.refreshed = true
	if f.refreshErr != nil {
		return nil, f.refreshErr
	}
	return &Token{AccessToken: f.refreshToken}, nil
}

// roundTripFunc adapts a function to http.RoundTripper.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func makeResp(code int, body string) *http.Response {
	return &http.Response{
		StatusCode: code,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func TestTransport_RoundTrip_Success(t *testing.T) {
	var seenAuth string
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		seenAuth = r.Header.Get("Authorization")
		return makeResp(http.StatusOK, "ok"), nil
	})
	tr := &Transport{Source: &fakeTokenSource{token: "tok-123"}, Base: base}

	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d", resp.StatusCode)
	}
	if seenAuth != "Bearer tok-123" {
		t.Errorf("auth header = %q", seenAuth)
	}
}

func TestTransport_RoundTrip_TokenError(t *testing.T) {
	tr := &Transport{Source: &fakeTokenSource{tokenErr: errors.New("no token")}}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Error("expected error when token cannot be obtained")
	}
}

func TestTransport_RoundTrip_401Retries(t *testing.T) {
	calls := 0
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return makeResp(http.StatusUnauthorized, "expired"), nil
		}
		return makeResp(http.StatusOK, "ok"), nil
	})
	src := &fakeTokenSource{token: "old", refreshToken: "new"}
	tr := &Transport{Source: src, Base: base}

	req, _ := http.NewRequest(http.MethodPost, "https://example.com", strings.NewReader("payload"))
	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200 after retry, got %d", resp.StatusCode)
	}
	if !src.refreshed {
		t.Error("expected a force refresh on 401")
	}
	if calls != 2 {
		t.Errorf("expected 2 calls (initial + retry), got %d", calls)
	}
}

func TestTransport_RoundTrip_401RefreshFails(t *testing.T) {
	base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return makeResp(http.StatusUnauthorized, "expired"), nil
	})
	src := &fakeTokenSource{token: "old", refreshErr: errors.New("refresh failed")}
	tr := &Transport{Source: src, Base: base}
	req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
	if _, err := tr.RoundTrip(req); err == nil {
		t.Error("expected error when refresh fails")
	}
}

func TestErrorLoggingTransport(t *testing.T) {
	t.Run("error response body re-readable", func(t *testing.T) {
		base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return makeResp(http.StatusInternalServerError, "server boom"), nil
		})
		tr := &ErrorLoggingTransport{Base: base}
		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		resp, err := tr.RoundTrip(req)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "server boom" {
			t.Errorf("body should still be readable, got %q", string(body))
		}
	})

	t.Run("transport error propagates", func(t *testing.T) {
		base := roundTripFunc(func(r *http.Request) (*http.Response, error) {
			return nil, errors.New("dial fail")
		})
		tr := &ErrorLoggingTransport{Base: base}
		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		if _, err := tr.RoundTrip(req); err == nil {
			t.Error("expected transport error to propagate")
		}
	})
}

// levelCaptureHandler records the level of every log record it handles.
type levelCaptureHandler struct {
	levels []slog.Level
}

func (h *levelCaptureHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *levelCaptureHandler) Handle(_ context.Context, r slog.Record) error {
	h.levels = append(h.levels, r.Level)
	return nil
}
func (h *levelCaptureHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *levelCaptureHandler) WithGroup(string) slog.Handler      { return h }

// TestErrorLoggingTransport_LogsBelowErrorLevel is a regression test for the
// reported SERVER-C. Every provider response with status >= 400 was logged at
// Error level; the production slog default is wrapped in a SentryHandler that
// captures Error-level records, so each one was reported to Sentry as the
// constant message "HTTP error response". That collapsed unrelated, mostly
// expected provider failures (401 token expiry, 429 rate limits, 400/409
// validation) from every user into a single noisy issue. The diagnostic log
// must stay below Error level so it is never captured to Sentry.
func TestErrorLoggingTransport_LogsBelowErrorLevel(t *testing.T) {
	for _, status := range []int{
		http.StatusBadRequest,
		http.StatusUnauthorized,
		http.StatusTooManyRequests,
		http.StatusConflict,
		http.StatusInternalServerError,
	} {
		capture := &levelCaptureHandler{}
		base := roundTripFunc(func(*http.Request) (*http.Response, error) {
			return makeResp(status, "boom"), nil
		})
		tr := &ErrorLoggingTransport{Base: base, Logger: slog.New(capture)}

		req, _ := http.NewRequest(http.MethodGet, "https://example.com", nil)
		if _, err := tr.RoundTrip(req); err != nil {
			t.Fatalf("status %d: unexpected err: %v", status, err)
		}
		if len(capture.levels) != 1 {
			t.Fatalf("status %d: expected exactly 1 log record, got %d", status, len(capture.levels))
		}
		if capture.levels[0] >= slog.LevelError {
			t.Errorf("status %d: logged at %v; must be below Error so it is not captured to Sentry", status, capture.levels[0])
		}
	}
}

func TestNewClientWithErrorLogging(t *testing.T) {
	c := NewClientWithErrorLogging(slog.Default(), "hevy", 0)
	if c == nil {
		t.Fatal("expected client")
	}
	if c.Timeout == 0 {
		t.Error("expected default timeout to be applied")
	}
}
