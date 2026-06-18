package intervals

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	c := NewClient("api-key", "athlete-1")
	if c == nil {
		t.Fatal("expected non-nil client")
	}
	if c.apiKey != "api-key" || c.athleteID != "athlete-1" {
		t.Errorf("unexpected client fields: %+v", c)
	}
	if c.client == nil {
		t.Error("expected http client to be initialised")
	}
}

// cancelledCtx returns a context that is already cancelled so client methods
// fail fast in doRequest without making a real network call to intervals.icu.
func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

func TestClientMethods_CancelledContext(t *testing.T) {
	c := NewClient("api-key", "athlete-1")
	ctx := cancelledCtx()

	if _, err := c.ListActivities(ctx, ListActivitiesParams{Oldest: "2024-01-01", Newest: "2024-02-01"}); err == nil {
		t.Error("ListActivities: expected error")
	}
	if _, err := c.GetActivity(ctx, 1); err == nil {
		t.Error("GetActivity: expected error")
	}
	if _, err := c.CreateActivity(ctx, &Activity{Name: "x"}); err == nil {
		t.Error("CreateActivity: expected error")
	}
	if _, err := c.UpdateActivity(ctx, 1, &Activity{Name: "x"}); err == nil {
		t.Error("UpdateActivity: expected error")
	}
	if _, err := c.DownloadFITFile(ctx, 1); err == nil {
		t.Error("DownloadFITFile: expected error")
	}
	if _, err := c.UploadFITFile(ctx, []byte("data")); err == nil {
		t.Error("UploadFITFile: expected error")
	}
}

func TestVerifyCredentials_CancelledContext(t *testing.T) {
	if err := VerifyCredentials(cancelledCtx(), "api-key", "athlete-1"); err == nil {
		t.Error("expected error with cancelled context")
	}
}
