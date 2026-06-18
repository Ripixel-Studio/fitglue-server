package notifications

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/internal/infra"
)

func TestSendPushNotification_NoTokens(t *testing.T) {
	// With no client but empty tokens, the adapter returns early without sending.
	a := &FCMAdapter{logger: infra.NewLogger()}
	if err := a.SendPushNotification(context.Background(), "u1", "title", "body", nil, nil); err != nil {
		t.Errorf("expected nil error for empty tokens, got %v", err)
	}
}
