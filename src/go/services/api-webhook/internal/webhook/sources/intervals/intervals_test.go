package intervals

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProvider_Basics(t *testing.T) {
	p := NewProvider()
	if p.ID() != "intervals" {
		t.Errorf("ID = %q", p.ID())
	}

	w := httptest.NewRecorder()
	p.VerifySubscription(w, httptest.NewRequest(http.MethodGet, "/webhook/intervals", nil))
	if w.Code != http.StatusOK {
		t.Errorf("VerifySubscription expected 200, got %d", w.Code)
	}
}

func TestParseEvent(t *testing.T) {
	p := NewProvider()

	t.Run("valid", func(t *testing.T) {
		body := `{"id": 555, "uid": "i12345", "type": "Ride", "name": "Evening Ride"}`
		evts, err := p.ParseEvent(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(body)))
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(evts) != 1 {
			t.Fatalf("expected 1 event, got %d", len(evts))
		}
		e := evts[0]
		if e.Provider != "intervals" || e.ProviderUID != "i12345" || e.ActivityID != "555" || e.Event != "create" {
			t.Errorf("unexpected event: %+v", e)
		}
	})

	t.Run("invalid json", func(t *testing.T) {
		if _, err := p.ParseEvent(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("{bad"))); err == nil {
			t.Error("expected json error")
		}
	})

	t.Run("missing fields returns no events", func(t *testing.T) {
		evts, err := p.ParseEvent(httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(`{"type":"Ride"}`)))
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if evts != nil {
			t.Errorf("expected nil events for unusable payload, got %v", evts)
		}
	})
}
