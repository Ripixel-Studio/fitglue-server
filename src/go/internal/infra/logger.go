package infra

import (
	"context"
	"log/slog"
	"os"
	"reflect"

	sentryPkg "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
)

// The slogger wrapper sits on the stack between application code and slog for
// every captured error. Register it so infrastructure/sentry strips its frames
// from Sentry stack traces; otherwise (*slogger).Error becomes the reported
// culprit for every error and Sentry collapses unrelated failures into a single
// issue (the reported SERVER-3).
func init() {
	sentryPkg.RegisterLoggerWrapper(reflect.TypeOf(slogger{}).PkgPath(), "(*slogger).")
}

// Logger provides a structured logging interface, wrapping slog.
type Logger interface {
	Debug(ctx context.Context, msg string, args ...any)
	Info(ctx context.Context, msg string, args ...any)
	Warn(ctx context.Context, msg string, args ...any)
	Error(ctx context.Context, msg string, args ...any)
	With(args ...any) Logger
}

// GCPHandlerOptions returns slog.HandlerOptions configured for Google Cloud Logging.
// It remaps standard slog keys to GCP-expected keys:
//   - "level" → "severity"
//   - "msg" → "message"
func GCPHandlerOptions(level slog.Level) *slog.HandlerOptions {
	return &slog.HandlerOptions{
		Level: level,
		ReplaceAttr: func(groups []string, a slog.Attr) slog.Attr {
			if a.Key == slog.LevelKey {
				return slog.Attr{Key: "severity", Value: a.Value}
			}
			if a.Key == slog.MessageKey {
				return slog.Attr{Key: "message", Value: a.Value}
			}
			return a
		},
	}
}

// NewLogger creates a new default structured logger.
// The handler chain is: JSONHandler → ComponentHandler → SentryHandler
// Error-level logs are automatically captured by Sentry (if initialized).
func NewLogger() Logger {
	jsonHandler := slog.NewJSONHandler(os.Stdout, GCPHandlerOptions(slog.LevelInfo))
	compHandler := &ComponentHandler{Handler: jsonHandler}
	sentryHandler := sentryPkg.NewSentryHandler(compHandler)
	return &slogger{
		logger: slog.New(sentryHandler),
	}
}

// NewLoggerWithComponent creates a structured logger with a named component.
// The component name is prepended as [component] in log messages via ComponentHandler.
func NewLoggerWithComponent(component string) Logger {
	l := NewLogger()
	return l.With("component", component)
}

type slogger struct {
	logger *slog.Logger
}

func (l *slogger) Debug(ctx context.Context, msg string, args ...any) {
	l.logger.DebugContext(ctx, msg, args...)
}

func (l *slogger) Info(ctx context.Context, msg string, args ...any) {
	l.logger.InfoContext(ctx, msg, args...)
}

func (l *slogger) Warn(ctx context.Context, msg string, args ...any) {
	l.logger.WarnContext(ctx, msg, args...)
}

func (l *slogger) Error(ctx context.Context, msg string, args ...any) {
	l.logger.ErrorContext(ctx, msg, args...)
}

func (l *slogger) With(args ...any) Logger {
	return &slogger{
		logger: l.logger.With(args...),
	}
}

// WrapSlogLogger wraps an existing *slog.Logger as an infra.Logger.
// Use this to bridge code that receives *slog.Logger (e.g., enricher providers)
// into functions that require infra.Logger (e.g., NewClientWithUsageTracking).
func WrapSlogLogger(l *slog.Logger) Logger {
	return &slogger{logger: l}
}
