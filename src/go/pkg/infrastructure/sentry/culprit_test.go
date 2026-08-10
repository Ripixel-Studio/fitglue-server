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

func TestStripInstrumentationFrames_StripsRegisteredLoggerWrapper(t *testing.T) {
	// Reproduces SERVER-3 (same class as SERVER-2, one layer up): an
	// errors.errorString logged via infra's (*slogger).Error. The shim frame is
	// in-app and sits between the real caller and the not-in-app slog frames, so
	// stripping only this package's frames leaves (*slogger).Error as the culprit.
	// Registering the wrapper must strip it too. The infra package cannot be
	// imported here (it depends on this one), so the wrapper frame is constructed
	// the way sentry-go would emit it and registered by hand.
	const loggerPkg = "github.com/fitglue/server/src/go/internal/infra"
	RegisterLoggerWrapper(loggerPkg, "(*slogger).")

	event := &sentry.Event{
		Exception: []sentry.Exception{{
			Type: "*errors.errorString",
			Stacktrace: &sentry.Stacktrace{Frames: []sentry.Frame{
				// Ordered oldest→youngest, matching sentry-go.
				{Module: "github.com/fitglue/server/src/go/internal/pipeline", Function: "run", InApp: true},
				{Module: loggerPkg, Function: "(*slogger).Error", InApp: true},
				{Module: "log/slog", Function: "(*Logger).log"},
				{Module: pkgPath, Function: "(*SentryHandler).Handle", InApp: true},
				{Module: pkgPath, Function: "CaptureException", InApp: true},
			}},
		}},
	}
	stripInstrumentationFrames(event)

	frames := event.Exception[0].Stacktrace.Frames
	for _, f := range frames {
		if f.Module == pkgPath || f.Function == "(*slogger).Error" {
			t.Errorf("instrumentation frame not stripped: %s.%s", f.Module, f.Function)
		}
	}
	// Youngest in-app frame must now be the real application call site.
	var youngestInApp sentry.Frame
	for _, f := range frames {
		if f.InApp {
			youngestInApp = f
		}
	}
	if youngestInApp.Function != "run" {
		t.Errorf("culprit = %s.%s, want the real caller pipeline.run", youngestInApp.Module, youngestInApp.Function)
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
