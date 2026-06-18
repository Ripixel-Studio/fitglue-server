package billing

import "testing"

func TestNewLiveStripeClient(t *testing.T) {
	if NewLiveStripeClient("sk_test_123") == nil {
		t.Fatal("expected non-nil stripe client")
	}
}
