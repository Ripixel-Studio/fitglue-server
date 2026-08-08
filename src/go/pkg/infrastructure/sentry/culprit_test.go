package sentry

import (
	"testing"

	"github.com/getsentry/sentry-go"
)

func TestStripInstrumentationFrames_RemovesOwnFrames(t *testing.T) {
	event := &sentry.Event{
		Exception: []sentry.Exception{{
			Type: "*fmt.wrapError",
			Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{
				{Module: "github.com/fitglue/server/src/go/internal/pipeline", Function: "run", InApp: true},
				{Module: "log/slog", Function: "(*Logger).Error"},
				{Module: pkgPath, Function: "(*SentryHandler).Handle", InApp: true},
				{Module: pkgPath, Function: "CaptureException", InApp: true},
			}},
		}},
	}
	stripInstrumentationFrames(event)

	frames := event.Exception[0].Stacktrace.Frames
	for _, f := range frames {
		if f.Module == pkgPath {
			t.Errorf("instrumentation frame not stripped: %s.%s", f.Module, f.Function)
		}
	}
	if len(frames) != 2 {
		t.Fatalf("expected 2 frames after stripping, got %d", len(frames))
	}
	// Youngest frame is now real application code, which becomes the culprit.
	if frames[len(frames)-1].Module == pkgPath {
		t.Error("youngest frame still points at instrumentation package")
	}
}

func TestStripInstrumentationFrames_KeepsFramesWhenAllStripped(t *testing.T) {
	// An event whose stack is entirely instrumentation frames must keep them
	// rather than emit an empty (invalid) stack trace.
	event := &sentry.Event{
		Exception: []sentry.Exception{{
			Type: "*fmt.wrapError",
			Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{
				{Module: pkgPath, Function: "(*SentryHandler).Handle"},
				{Module: pkgPath, Function: "CaptureException"},
			}},
		}},
	}
	stripInstrumentationFrames(event)
	if got := len(event.Exception[0].Stacktrace.Frames); got != 2 {
		t.Errorf("expected frames preserved when all are instrumentation, got %d", got)
	}
}

func TestBeforeSend_ScrubsSensitiveHeaders(t *testing.T) {
	event := &sentry.Event{Request: &sentry.Request{Headers: map[string]string{
		"Authorization": "Bearer secret",
		"Cookie":        "session=abc",
		"X-Trace":       "keep",
	}}}
	beforeSend(event, nil)
	h := event.Request.Headers
	if _, ok := h["Authorization"]; ok {
		t.Error("Authorization header should be scrubbed")
	}
	if _, ok := h["Cookie"]; ok {
		t.Error("Cookie header should be scrubbed")
	}
	if h["X-Trace"] != "keep" {
		t.Error("non-sensitive header should be retained")
	}
}
