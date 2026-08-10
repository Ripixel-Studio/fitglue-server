// Black-box reproduction of SERVER-3: an error reported through the shared
// infra.Logger was grouped in Sentry under (*slogger).Error instead of its real
// call site, collapsing unrelated failures into one issue.
//
// Unlike the sentry package's own black-box test — which wires slog directly to
// the SentryHandler and so never exercises the (*slogger) wrapper — this test
// drives the true production path (infra.Logger.Error → (*slogger).Error → slog
// → SentryHandler), which is where the extra in-app frame comes from. Importing
// this package also runs its init(), registering the wrapper with the sentry
// package so its frames are stripped.
package infra_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	fgsentry "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
	"github.com/getsentry/sentry-go"
)

type captureTransport struct{ events []*sentry.Event }

func (t *captureTransport) Configure(sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(e *sentry.Event)      { t.events = append(t.events, e) }
func (t *captureTransport) Flush(time.Duration) bool       { return true }

// reportViaInfraLogger stands in for real application code reporting an error the
// usual way — through the shared infra.Logger, which SentryHandler auto-captures.
func reportViaInfraLogger(logger infra.Logger) {
	logger.Error(context.Background(), "db write failed", "error", errors.New("connection refused"))
}

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

func TestInfraLogger_CulpritIsRealCaller(t *testing.T) {
	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatal(err)
	}

	// WrapSlogLogger yields the real production *slogger, so logger.Error puts an
	// (*slogger).Error frame on the captured stack — the frame that was becoming
	// the culprit. Output is discarded to keep the test quiet.
	logger := infra.WrapSlogLogger(slog.New(fgsentry.NewSentryHandler(slog.NewJSONHandler(io.Discard, nil))))
	reportViaInfraLogger(logger)
	fgsentry.Flush(time.Second)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	e := tr.events[0]

	for _, ex := range e.Exception {
		if ex.Stacktrace == nil {
			continue
		}
		for _, f := range ex.Stacktrace.Frames {
			if strings.HasPrefix(f.Function, "(*slogger).") {
				t.Errorf("logger-shim frame leaked into Sentry stack trace: %s.%s", f.Module, f.Function)
			}
		}
	}

	frame := youngestInAppFrame(t, e)
	if !strings.HasSuffix(frame.Function, "reportViaInfraLogger") {
		t.Errorf("culprit frame = %s.%s, want the real caller reportViaInfraLogger", frame.Module, frame.Function)
	}
}
