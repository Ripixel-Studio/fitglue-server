// SERVER-8 / SERVER-9: logger.Error calls without an "error" attribute reached
// Sentry as bare CaptureMessage events with none of their slog attrs attached, so
// "failed to send verification email" arrived with no "err", and the parkrun
// checker's diagnostics ("reason", "input_id", ...) were dropped on the floor.
// The SentryHandler must forward the attrs as per-event context for every capture
// path, and the context must NOT leak onto later events.
package sentry_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/fitglue/server/src/go/internal/infra"
	fgsentry "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
)

func TestSentryHandler_MessageCapturesCarryAttrs(t *testing.T) {
	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatalf("init: %v", err)
	}
	defer func() {
		_ = fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: &captureTransport{}}, nil)
	}()

	logger := infra.NewLogger()
	ctx := context.Background()

	// 1. No "error" attr at all (the SERVER-9 shape).
	logger.Error(ctx, "failed to send verification email", "err", errors.New("failed to send email: EOF"), "email", "x@y.z")
	// 2. A string "error" attr.
	logger.Error(ctx, "parkrun checker: pending input still unresolved this run", "error", "stringy", "reason", "results_not_published")
	// 3. A real error attr (CaptureException path).
	logger.Error(ctx, "sync failed", "error", errors.New("boom"), "user_id", "u1")
	// 4. A later message with no attrs must not inherit any of the above.
	logger.Error(ctx, "bare")

	fgsentry.Flush(time.Second)
	if len(tr.events) != 4 {
		t.Fatalf("expected 4 events, got %d", len(tr.events))
	}

	get := func(i int, key string) interface{} {
		c, ok := tr.events[i].Contexts[key]
		if !ok {
			t.Fatalf("event %d: missing context %q (have %v)", i, key, keys(tr.events[i].Contexts))
		}
		return c["value"]
	}

	if tr.events[0].Message != "failed to send verification email" {
		t.Errorf("event 0 message = %q", tr.events[0].Message)
	}
	if got := get(0, "email"); got != "x@y.z" {
		t.Errorf("event 0 email = %v", got)
	}
	if got, ok := get(0, "err").(error); !ok || got.Error() != "failed to send email: EOF" {
		t.Errorf("event 0 err = %v", get(0, "err"))
	}
	if got := get(1, "reason"); got != "results_not_published" {
		t.Errorf("event 1 reason = %v", got)
	}
	if got := get(2, "user_id"); got != "u1" {
		t.Errorf("event 2 user_id = %v", got)
	}
	for _, k := range []string{"email", "err", "reason", "user_id", "error"} {
		if _, leaked := tr.events[3].Contexts[k]; leaked {
			t.Errorf("context %q leaked onto an unrelated later event", k)
		}
	}
}

func keys(m map[string]map[string]interface{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
