package hevy

import (
	"context"
	"testing"

	"github.com/fitglue/server/src/go/pkg/domain/user"
	pbevents "github.com/fitglue/server/src/go/pkg/types/pb/models/events"
)

func TestUploader_NewAndName(t *testing.T) {
	u := New(nil)
	if u == nil || u.Name() != "hevy" {
		t.Fatalf("unexpected uploader: %+v", u)
	}
}

func TestUploader_Create_NoApiKey(t *testing.T) {
	u := New(nil)
	// No PipelineExecutionId -> skips the DB idempotency guard and reaches the
	// API-key validation, which fails because the user has no Hevy integration.
	payload := &pbevents.ActivityPayload{}
	userRec := &user.Record{}
	if _, err := u.Create(context.Background(), payload, userRec); err == nil {
		t.Error("expected error when user has no Hevy API key")
	}
}

func TestUploader_Update_NoApiKey(t *testing.T) {
	u := New(nil)
	if err := u.Update(context.Background(), &pbevents.ActivityPayload{}, &user.Record{}, nil); err == nil {
		t.Error("expected error when user has no Hevy API key")
	}
}
