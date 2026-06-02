package server

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/go-chi/chi/v5/middleware"
)

// RawBodyContextKey is the context key for the raw request body payload
type RawBodyContextKey struct{}

// RawBodyMiddleware reads the request body completely and stores it in context
// This is necessary because webhooks require cryptographical verification of the raw payload
func RawBodyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "failed to read body", http.StatusBadRequest)
			return
		}

		// Restore the body so other handlers/middlewares can still read it
		r.Body = io.NopCloser(bytes.NewBuffer(body))

		next.ServeHTTP(w, r)
	})
}

// LoggerMiddleware sets up structured logging on endpoints
func LoggerMiddleware(logger infra.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			reqID := middleware.GetReqID(r.Context())

			defer func() {
				logger.Info(r.Context(), "Webhook Request",
					"method", r.Method,
					"path", r.URL.Path,
					"status", ww.Status(),
					"duration_ms", time.Since(start).Milliseconds(),
					"req_id", reqID,
					"bytes_written", ww.BytesWritten(),
				)
			}()

			next.ServeHTTP(ww, r)
		})
	}
}

// JSONResponseMiddleware adds content-type application/json to responses
func JSONResponseMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		next.ServeHTTP(w, r)
	})
}

// SentryRecoveryMiddleware catches panics, logs them (which routes through SentryHandler
// to capture the exception in Sentry), and returns HTTP 500.
// Use in place of chi's middleware.Recoverer.
func SentryRecoveryMiddleware(logger infra.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					err, ok := rec.(error)
					if !ok {
						err = fmt.Errorf("panic: %v", rec)
					}
					logger.Error(r.Context(), "Panic recovered in HTTP handler", "error", err, "path", r.URL.Path)
					http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
				}
			}()
			next.ServeHTTP(w, r)
		})
	}
}
