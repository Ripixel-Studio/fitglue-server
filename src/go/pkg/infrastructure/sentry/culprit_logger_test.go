// Reproduction for SERVER-2: errors reported through the real infra logger
// (infra.NewLogger → infra.(*slogger).Error) had their culprit attributed to
// (*slogger).Error instead of the application call site. Because every logged
// error shares that frame — and a bare errors.New shares the generic
// *errors.errorString type — Sentry collapsed unrelated failures into one issue.
//
// This is the same class of bug as SERVER-1 (culprit was CaptureException), one
// layer up: SERVER-1's fix stripped this package's frames but not the infra
// logger wrapper that production actually uses. The prior black-box test drove a
// bare slog.Logger and so never exercised the wrapper.
//
// Lives in sentry_test (external) so caller frames survive stripping, exactly as
// production callers do; importing internal/infra here is legal because infra
// depends on this package, not the reverse.
package sentry_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	fgsentry "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
)

// originViaInfraLogger stands in for real application code reporting an error the
// house way: infra.Logger.Error with an "error" attribute.
func originViaInfraLogger(logger infra.Logger) {
	logger.Error(context.Background(), "sync failed", "error", errors.New("connection refused"))
}

func TestInfraLogger_CulpritIsRealCaller(t *testing.T) {
	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatal(err)
	}
	// Reset the global hub after the test so other tests aren't affected.
	defer func() {
		_ = fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: &captureTransport{}}, nil)
	}()

	logger := infra.NewLoggerWithComponent("pipeline")
	originViaInfraLogger(logger)
	fgsentry.Flush(time.Second)

	if len(tr.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(tr.events))
	}
	e := tr.events[0]
	assertNoInstrumentationFrames(t, e)

	frame := youngestInAppFrame(t, e)
	if strings.HasPrefix(frame.Function, "(*slogger).") {
		t.Fatalf("culprit frame = %s.%s, still the logger wrapper — Sentry groups every logged error together (SERVER-2)", frame.Module, frame.Function)
	}
	if !strings.HasSuffix(frame.Function, "originViaInfraLogger") {
		t.Errorf("culprit frame = %s.%s, want the real caller originViaInfraLogger", frame.Module, frame.Function)
	}
}
