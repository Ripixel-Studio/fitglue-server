package infra

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/grpc"
)

func TestNewLoggerAndMethods(t *testing.T) {
	l := NewLogger()
	ctx := context.Background()
	// These should not panic.
	l.Debug(ctx, "debug")
	l.Info(ctx, "info", "k", "v")
	l.Warn(ctx, "warn")
	l.Error(ctx, "error", "err", "boom")
	if l.With("component", "x") == nil {
		t.Error("With should return a logger")
	}
	if NewLoggerWithComponent("svc") == nil {
		t.Error("NewLoggerWithComponent should return a logger")
	}
}

func TestWrapSlogLogger(t *testing.T) {
	base := slog.New(slog.NewTextHandler(&bytes.Buffer{}, nil))
	l := WrapSlogLogger(base)
	if l == nil {
		t.Fatal("expected wrapped logger")
	}
	l.Info(context.Background(), "hi")
}

func TestGCPHandlerOptions_RemapsKeys(t *testing.T) {
	var buf bytes.Buffer
	h := slog.NewJSONHandler(&buf, GCPHandlerOptions(slog.LevelInfo))
	logger := slog.New(h)
	logger.Info("hello")
	out := buf.String()
	if !strings.Contains(out, `"severity"`) {
		t.Errorf("expected level remapped to severity, got %s", out)
	}
	if !strings.Contains(out, `"message"`) {
		t.Errorf("expected msg remapped to message, got %s", out)
	}
}

func TestComponentHandler(t *testing.T) {
	var buf bytes.Buffer
	json := slog.NewJSONHandler(&buf, nil)
	ch := &ComponentHandler{Handler: json}

	// Component set via WithAttrs is prepended to the message.
	logger := slog.New(ch).With("component", "pipeline")
	logger.Info("started")
	out := buf.String()
	if !strings.Contains(out, "[pipeline] started") {
		t.Errorf("expected [pipeline] prefix, got %s", out)
	}

	// WithGroup should not panic and returns a handler.
	if ch.WithGroup("g") == nil {
		t.Error("WithGroup returned nil")
	}
	if !ch.Enabled(context.Background(), slog.LevelInfo) {
		t.Error("Enabled should be true for info")
	}
}

func TestComponentHandler_NoComponent(t *testing.T) {
	var buf bytes.Buffer
	ch := &ComponentHandler{Handler: slog.NewJSONHandler(&buf, nil)}
	slog.New(ch).Info("plain")
	if !strings.Contains(buf.String(), "plain") {
		t.Errorf("expected plain message, got %s", buf.String())
	}
	if strings.Contains(buf.String(), "[]") {
		t.Error("should not prepend empty component brackets")
	}
}

func TestLoggingUnaryInterceptor(t *testing.T) {
	interceptor := LoggingUnaryInterceptor(NewLogger())
	info := &grpc.UnaryServerInfo{FullMethod: "/svc/Method"}

	t.Run("success", func(t *testing.T) {
		resp, err := interceptor(context.Background(), "req", info,
			func(ctx context.Context, req interface{}) (interface{}, error) { return "ok", nil })
		if err != nil || resp != "ok" {
			t.Errorf("unexpected: resp=%v err=%v", resp, err)
		}
	})

	t.Run("error", func(t *testing.T) {
		_, err := interceptor(context.Background(), "req", info,
			func(ctx context.Context, req interface{}) (interface{}, error) { return nil, errors.New("fail") })
		if err == nil {
			t.Error("expected error to propagate")
		}
	})
}

func TestLoggingMiddleware(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("hi"))
	})
	h := LoggingMiddleware(NewLogger(), next)

	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/x", nil))
	if w.Code != http.StatusTeapot {
		t.Errorf("expected status passthrough 418, got %d", w.Code)
	}
}

func TestGRPCDial_Local(t *testing.T) {
	// Non-https target uses insecure credentials and creates a lazy client without
	// connecting, so this returns immediately.
	conn, err := GRPCDial("localhost:50051")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if conn == nil {
		t.Fatal("expected non-nil conn")
	}
	_ = conn.Close()
}

func TestInitSentry_NoDSN(t *testing.T) {
	t.Setenv("SENTRY_DSN", "")
	t.Setenv("GOOGLE_CLOUD_PROJECT", "fitglue-server-dev")
	// Should not panic with an empty DSN (Sentry disabled).
	InitSentry()
}
