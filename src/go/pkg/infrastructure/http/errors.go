// Package httputil provides HTTP error handling utilities.
package httputil

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
)

// MaxErrorBodySize is the maximum size of error body to include in error messages
const MaxErrorBodySize = 500

// HTTPError represents an HTTP error with status code and response body
type HTTPError struct {
	StatusCode int
	Status     string
	Body       string
	URL        string
	// Message is an optional caller-supplied prefix (e.g. "Strava upload failed").
	// When set, Error() uses it in place of the generic status text so the
	// rendered message matches what callers previously produced with fmt.Errorf,
	// while the typed StatusCode remains inspectable via errors.As.
	Message string
}

func (e *HTTPError) Error() string {
	prefix := e.Message
	if prefix == "" {
		prefix = e.Status
	}
	if e.Body != "" {
		return fmt.Sprintf("%s (status %d): %s", prefix, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("%s (status %d)", prefix, e.StatusCode)
}

// IsAuthFailure reports whether the response indicates an expired or revoked
// credential (HTTP 401/403) — a user-actionable condition (reconnect the
// integration) rather than a code fault or transient upstream error.
func (e *HTTPError) IsAuthFailure() bool {
	return e.StatusCode == http.StatusUnauthorized || e.StatusCode == http.StatusForbidden
}

// truncate truncates a string to maxLen, adding "..." if truncated
func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}

// ParseErrorResponse checks if the response is an error (4xx/5xx) and returns
// a rich HTTPError containing the response body. Returns nil for success responses.
// The response body is re-wrapped so the caller can still read it.
func ParseErrorResponse(resp *http.Response) error {
	if resp.StatusCode < 400 {
		return nil
	}

	bodyBytes, err := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Re-wrap body so caller can still read it if needed
	resp.Body = io.NopCloser(bytes.NewReader(bodyBytes))

	bodyStr := ""
	if err == nil && len(bodyBytes) > 0 {
		bodyStr = truncate(string(bodyBytes), MaxErrorBodySize)
	}

	return &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     http.StatusText(resp.StatusCode),
		Body:       bodyStr,
		URL:        resp.Request.URL.String(),
	}
}

// WrapResponseError reads the response body and returns a typed *HTTPError.
// Unlike ParseErrorResponse, this does not re-wrap the body (for simple error cases).
//
// It returns *HTTPError (not a flat fmt.Errorf string) so callers can classify the
// failure by status code via errors.As — notably to distinguish an expired/revoked
// credential (401/403) from a genuine fault. The rendered message is unchanged:
// "<message> (status <code>): <body>".
func WrapResponseError(resp *http.Response, message string) error {
	bodyBytes, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	url := ""
	if resp.Request != nil && resp.Request.URL != nil {
		url = resp.Request.URL.String()
	}

	return &HTTPError{
		StatusCode: resp.StatusCode,
		Status:     http.StatusText(resp.StatusCode),
		Body:       truncate(string(bodyBytes), MaxErrorBodySize),
		URL:        url,
		Message:    message,
	}
}
