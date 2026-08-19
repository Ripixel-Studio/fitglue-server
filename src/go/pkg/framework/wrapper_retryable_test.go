// nolint:proto-json
package framework

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/cloudevents/sdk-go/v2/event"
	"github.com/fitglue/server/src/go/pkg/bootstrap"
	fgsentry "github.com/fitglue/server/src/go/pkg/infrastructure/sentry"
	"github.com/getsentry/sentry-go"
)

// captureTransport records every event the Sentry SDK would send, so a test can
// assert exactly how many (if any) errors were reported.
type captureTransport struct{ events []*sentry.Event }

func (t *captureTransport) Configure(_ sentry.ClientOptions) {}
func (t *captureTransport) SendEvent(e *sentry.Event)        { t.events = append(t.events, e) }
func (t *captureTransport) Flush(_ time.Duration) bool       { return true }

func runWrapped(t *testing.T, handlerErr error) *captureTransport {
	t.Helper()

	tr := &captureTransport{}
	if err := fgsentry.Init(fgsentry.Config{DSN: "https://x@example.com/1", Transport: tr}, nil); err != nil {
		t.Fatalf("sentry init: %v", err)
	}

	svc := &bootstrap.Service{DB: &MockDB{}}
	handler := func(ctx context.Context, e event.Event, fwCtx *FrameworkContext) (interface{}, error) {
		return nil, handlerErr
	}
	wrapped := WrapCloudEvent("test-service", svc, handler)

	e := event.New()
	e.SetType("com.fitglue.activity.created")
	e.SetSource("/test")

	returned := wrapped(context.Background(), e)
	// Both plain and retryable errors must be returned to the CloudEvents SDK so
	// Pub/Sub NACKs and redelivers. (Only TerminalError is swallowed to nil.)
	if returned == nil {
		t.Fatalf("expected wrapped handler to return the error (NACK), got nil")
	}
	fgsentry.Flush(time.Second)
	return tr
}

// TestWrapCloudEvent_RetryableErrorNotReported is the regression guard for
// SERVER-7: an expected transient lag retry must NACK for Pub/Sub backoff without
// being reported to Sentry.
func TestWrapCloudEvent_RetryableErrorNotReported(t *testing.T) {
	tr := runWrapped(t, NewRetryableError("lagged retry failed", errors.New("source data not ready")))
	if len(tr.events) != 0 {
		t.Fatalf("retryable error was reported to Sentry (%d events); expected none", len(tr.events))
	}
}

// TestWrapCloudEvent_PlainErrorReported pins the other side of the branch: a
// genuine (non-retryable, non-terminal) failure is still captured to Sentry.
func TestWrapCloudEvent_PlainErrorReported(t *testing.T) {
	tr := runWrapped(t, errors.New("real failure"))
	if len(tr.events) == 0 {
		t.Fatalf("plain error was not reported to Sentry; expected it to be captured")
	}
}
