// Black-box tests that exercise the full capture path from application code in a
// *different* package, reproducing the production symptom: SERVER-1, a
// *fmt.wrapError whose reported culprit was infrastructure/sentry.CaptureException.
// The callers below stand in for real application code; because they live in
// package sentry_test their frames survive instrumentation stripping, exactly as
// production caller frames (in other packages) do.
package sentry_test

import (
	"fmt"
	"io"
	"log/slog"
	"reflect"
	"strings"
	"testing"
	"time"

	fgsentry "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
	"github.com/getsentry/sentry-go"
)

// pkgPath is the import path of the instrumentation package under test.
var pkgPath = reflect.TypeOf(fgsentry.Config{}).PkgPath()

type captureTransport struct{ events []*sentry.Event }

func (t *captureTransport) Configure(_ sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(e *sentry.Event)        { t.events = append(t.events, e) }
func (t *captureTransport) Flush(_ time.Duration) bool       { return true }

// youngestInAppFrame returns the deepest in-app frame of the outermost
// exception — this is what Sentry uses to derive the issue culprit. Frames are
// ordered oldest→youngest, so we scan from the end.
func youngestInAppFrame(t *testing.T, e *sentry.Event) sentry.Frame {
	t.Helper()
	if len(e.Exception) == 0 {
		t.Fatal("event has no exceptions")
	}
	st := e.Exception[len(e.Exception)-1].Stacktrace
	if st == nil || len(st.Frames) == 0 {
		t.Fatal("outermost exception has no stack trace")
	}
	for i := len(st.Frames) - 1; i >= 0; i-- {
		if st.Frames[i].InApp {
			return st.Frames[i]
		}
	}
	t.Fatal("no in-app frame in stack trace")
	return sentry.Frame{}
}

func assertNoInstrumentationFrames(t *testing.T, e *sentry.Event) {
	t.Helper()
	for _, ex := range e.Exception {
		if ex.Stacktrace == nil {
			continue
		}
		for _, f := range ex.Stacktrace.Frames {
			if f.Module == pkgPath {
				t.Errorf("instrumentation frame leaked into Sentry stack trace: %s.%s", f.Module, f.Function)
			}
		}
	}
}

// originViaCaptureException stands in for application code (e.g. the framework
// wrapper) that reports an error by calling CaptureException directly.
func originViaCaptureException() {
	fgsentry.CaptureException(fmt.Errorf("write failed: %w", fmt.Errorf("connection refused")), nil, nil)
}

// originViaLogger stands in for application code that reports an error the usual
// way — logger.Error, which SentryHandler auto-captures.
func originViaLogger(logger *slog.Logger) {
	logger.Error("write failed", "error", fmt.Errorf("write failed: %w", fmt.Errorf("connection refused")))
}

func TestCaptureException_CulpritIsRealCaller(t *testing.T) {
	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatal(err)
	}

	originViaCaptureException()
	fgsentry.Flush(time.Second)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	e := tr.events[0]
	assertNoInstrumentationFrames(t, e)

	frame := youngestInAppFrame(t, e)
	if !strings.HasSuffix(frame.Function, "originViaCaptureException") {
		t.Errorf("culprit frame = %s.%s, want the real caller originViaCaptureException", frame.Module, frame.Function)
	}
}

func TestSentryHandler_CulpritIsRealCaller(t *testing.T) {
	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(fgsentry.NewSentryHandler(slog.NewTextHandler(io.Discard, nil)))
	originViaLogger(logger)
	fgsentry.Flush(time.Second)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	e := tr.events[0]
	assertNoInstrumentationFrames(t, e)

	frame := youngestInAppFrame(t, e)
	if !strings.HasSuffix(frame.Function, "originViaLogger") {
		t.Errorf("culprit frame = %s.%s, want the real caller originViaLogger", frame.Module, frame.Function)
	}
}

// TestDistinctCallersDistinctCulprit guards the grouping property: two errors
// reported from different call sites must expose different culprit frames.
// Before the fix both were infrastructure/sentry.CaptureException, so Sentry
// collapsed them into a single issue (the reported SERVER-1).
func TestDistinctCallersDistinctCulprit(t *testing.T) {
	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatal(err)
	}

	callerA := func() { fgsentry.CaptureException(fmt.Errorf("boom a"), nil, nil) }
	callerB := func() { fgsentry.CaptureException(fmt.Errorf("boom b"), nil, nil) }
	callerA()
	callerB()
	fgsentry.Flush(time.Second)

	if len(tr.events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(tr.events))
	}
	fa := youngestInAppFrame(t, tr.events[0])
	fb := youngestInAppFrame(t, tr.events[1])
	if fa.Module == pkgPath || fb.Module == pkgPath {
		t.Fatal("culprit still points at instrumentation package")
	}
	if fa.Function == fb.Function && fa.Lineno == fb.Lineno {
		t.Errorf("distinct call sites produced identical culprit frame %s:%d — Sentry would group them together", fa.Function, fa.Lineno)
	}
}
