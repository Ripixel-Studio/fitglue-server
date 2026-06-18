package mock

import (
	"context"
	"strings"
	"testing"

	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
)

func TestMockUploader(t *testing.T) {
	u := New()
	if u.Name() != "mock" {
		t.Errorf("name = %q", u.Name())
	}

	id := "act-123"
	payload := &pbevents.ActivityPayload{ActivityId: &id}
	externalID, err := u.Create(context.Background(), payload, nil)
	if err != nil {
		t.Fatalf("Create err: %v", err)
	}
	if !strings.HasPrefix(externalID, "mock-act-123-") {
		t.Errorf("unexpected mock external id: %q", externalID)
	}

	if err := u.Update(context.Background(), payload, nil, nil); err != nil {
		t.Errorf("Update err: %v", err)
	}
}
