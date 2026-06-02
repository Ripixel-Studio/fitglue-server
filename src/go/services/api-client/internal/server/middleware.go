package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"firebase.google.com/go/v4/auth"
	"github.com/fitglue/server/src/go/internal/infra"
	"github.com/go-chi/chi/v5/middleware"
)

type contextKey string

const userContextKey = contextKey("userToken")

// AuthMiddleware verifies the Firebase ID token
func AuthMiddleware(authClient *auth.Client) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" || !strings.HasPrefix(authHeader, "Bearer ") {
				http.Error(w, "missing or malformed Authorization header", http.StatusUnauthorized)
				return
			}

			tokenStr := strings.TrimPrefix(authHeader, "Bearer ")
			token, err := authClient.VerifyIDToken(r.Context(), tokenStr)
			if err != nil {
				http.Error(w, "invalid token", http.StatusUnauthorized)
				return
			}

			// Add token details to context
			ctx := context.WithValue(r.Context(), userContextKey, token)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// LoggerMiddleware sets up structured logging on endpoints
func LoggerMiddleware(logger infra.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			reqID := middleware.GetReqID(r.Context())

			defer func() {
				logger.Info(r.Context(), "API Request",
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
