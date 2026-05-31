package oauth

import (
	"bytes"
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	fitglueerrors "github.com/fitglue/server/src/go/pkg/errors"
)

// Transport is an http.RoundTripper that authenticates all requests
// using the provided TokenSource.
type Transport struct {
	// Source supplies the token to be used.
	Source TokenSource

	// Base is the base RoundTripper used to make the actual HTTP requests.
	// If nil, http.DefaultTransport is used.
	Base http.RoundTripper

	// Logger for diagnostic output.
	Logger infra.Logger
}

func (t *Transport) RoundTrip(req *http.Request) (*http.Response, error) {
	// 0. Get Base Transport
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	// 1. Get Token (Proactive check happens here)
	ctx := req.Context()
	token, err := t.Source.Token(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth: cannot get token: %w", err)
	}

	// 2. Clone Request and Set Header
	req2 := cloneRequest(req)
	req2.Header.Set("Authorization", "Bearer "+token.AccessToken)

	// 2a. Buffer the request body so it can be replayed on a 401 retry.
	// Go's HTTP transport reads and exhausts the body on the first RoundTrip;
	// without buffering, the retry attempt sends ContentLength>0 with an empty
	// body, causing "ContentLength=N with Body length 0".
	var bodySnapshot []byte
	if req2.Body != nil && req2.Body != http.NoBody {
		bodySnapshot, err = io.ReadAll(req2.Body)
		req2.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("oauth: failed to buffer request body: %w", err)
		}
		req2.Body = io.NopCloser(bytes.NewReader(bodySnapshot))
	}

	// 3. Execute Request
	resp, err := base.RoundTrip(req2)
	if err != nil {
		return nil, err
	}

	// 4. Reactive Retry (401)
	if resp.StatusCode == http.StatusUnauthorized {
		// Drain body to allow connection reuse
		resp.Body.Close()

		if t.Logger != nil {
			t.Logger.Warn(ctx, "Got 401 Unauthorized, attempting force refresh", "url", req.URL.String())
		}

		// Force Refresh
		token, err = t.Source.ForceRefresh(ctx)
		if err != nil {
			return nil, fitglueerrors.ErrIntegrationAuthFailed.WithCause(fmt.Errorf("oauth force refresh failed: %w", err))
		}

		// Update Header and reset body for replay
		req2.Header.Set("Authorization", "Bearer "+token.AccessToken)
		if bodySnapshot != nil {
			req2.Body = io.NopCloser(bytes.NewReader(bodySnapshot))
		}

		// Retry Request
		return base.RoundTrip(req2)
	}

	return resp, nil
}

// cloneRequest returns a clone of the provided *http.Request.
// The clone is a shallow copy of the struct and its Header map.
func cloneRequest(r *http.Request) *http.Request {
	// shallow copy of the struct
	r2 := new(http.Request)
	*r2 = *r
	// deep copy of the Header
	r2.Header = make(http.Header, len(r.Header))
	for k, s := range r.Header {
		r2.Header[k] = append([]string(nil), s...)
	}
	return r2
}

// UsageTrackingTransport wraps a RoundTripper and updates the user's last_used_at
// timestamp on successful requests.
type UsageTrackingTransport struct {
	Base     http.RoundTripper
	Service  *bootstrap.Service
	UserID   string
	Provider string
	Logger   infra.Logger
}

func (t *UsageTrackingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)

	// If request was successful (at transport level), update usage stats asynchronously
	if err == nil {
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			updateErr := t.Service.DB.UpdateUser(ctx, t.UserID, map[string]interface{}{
				"integrations": map[string]interface{}{
					t.Provider: map[string]interface{}{
						"last_used_at": time.Now(),
					},
				},
			})
			if updateErr != nil {
				if t.Logger != nil {
					t.Logger.Warn(ctx, "Failed to track usage", "provider", t.Provider, "user_id", t.UserID, "error", updateErr)
				}
			}
		}()
	}

	return resp, err
}

// MaxErrorBodySize is the maximum size of error body to capture for logging
const MaxErrorBodySize = 500

// ErrorLoggingTransport wraps a RoundTripper and logs HTTP error responses
// with their response bodies for better debugging and error messages.
type ErrorLoggingTransport struct {
	Base   http.RoundTripper
	Logger *slog.Logger
}

func (t *ErrorLoggingTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.Base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil {
		return resp, err
	}

	// Only capture body for error responses
	if resp.StatusCode >= 400 {
		bodyBytes, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()

		if readErr == nil {
			bodyStr := string(bodyBytes)
			if len(bodyStr) > MaxErrorBodySize {
				bodyStr = bodyStr[:MaxErrorBodySize] + "..."
			}

			logger := t.Logger
			if logger == nil {
				logger = slog.Default()
			}

			logger.Error("HTTP error response",
				"url", req.URL.String(),
				"method", req.Method,
				"status", resp.StatusCode,
				"body", bodyStr)
		}

		// Re-wrap body so the caller can still read it
		resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))
	}

	return resp, nil
}

// NewClientWithErrorLogging creates an HTTP client with automatic error response logging.
// Use this for non-OAuth clients (like Hevy API key auth) that still need error body capture.
func NewClientWithErrorLogging(logger *slog.Logger, provider string, timeout time.Duration) *http.Client {
	if timeout == 0 {
		timeout = 30 * time.Second
	}

	return &http.Client{
		Timeout: timeout,
		Transport: &ErrorLoggingTransport{
			Logger: logger.With("component", "http-client", "provider", provider),
		},
	}
}

// NewClientWithUsageTracking creates an HTTP client that automatically handles OAuth,
// tracks usage stats in Firestore, and logs HTTP error responses with their bodies.
func NewClientWithUsageTracking(source TokenSource, service *bootstrap.Service, userID, provider string, logger infra.Logger) *http.Client {
	oauthLogger := logger.With("component", "oauth")
	// Stack: Client → ErrorLogging → UsageTracking → OAuth → Network
	oauthTransport := &Transport{Source: source, Logger: oauthLogger}

	usageTransport := &UsageTrackingTransport{
		Base:     oauthTransport,
		Service:  service,
		UserID:   userID,
		Provider: provider,
		Logger:   oauthLogger,
	}

	errorLoggingTransport := &ErrorLoggingTransport{
		Base:   usageTransport,
		Logger: slog.Default().With("component", "http-client", "provider", provider, "user_id", userID),
	}

	return &http.Client{
		Transport: errorLoggingTransport,
	}
}

// NewClientWithUsageTrackingHTTP1 is identical to NewClientWithUsageTracking but forces
// HTTP/1.1 by disabling TLS ALPN h2 negotiation. Use this for multipart file upload
// endpoints (e.g. Strava /uploads) where HTTP/2 stream errors are observed.
func NewClientWithUsageTrackingHTTP1(source TokenSource, service *bootstrap.Service, userID, provider string, logger infra.Logger) *http.Client {
	oauthLogger := logger.With("component", "oauth")

	// Force HTTP/1.1: empty TLSNextProto disables Go's automatic h2 upgrade.
	http1Transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			MinVersion: tls.VersionTLS12,
		},
		TLSNextProto: make(map[string]func(authority string, c *tls.Conn) http.RoundTripper),
	}

	oauthTransport := &Transport{Source: source, Base: http1Transport, Logger: oauthLogger}

	usageTransport := &UsageTrackingTransport{
		Base:     oauthTransport,
		Service:  service,
		UserID:   userID,
		Provider: provider,
		Logger:   oauthLogger,
	}

	errorLoggingTransport := &ErrorLoggingTransport{
		Base:   usageTransport,
		Logger: slog.Default().With("component", "http-client", "provider", provider, "user_id", userID),
	}

	return &http.Client{
		Transport: errorLoggingTransport,
	}
}
