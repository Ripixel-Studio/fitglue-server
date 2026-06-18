package email

import (
	"context"
	"testing"
	"time"
)

func TestNewSMTPSender(t *testing.T) {
	s := NewSMTPSender("smtp.example.com", 587, "noreply@fitglue.tech", "secret")
	if s == nil {
		t.Fatal("expected sender")
	}
	if s.host != "smtp.example.com" || s.port != 587 || s.from != "noreply@fitglue.tech" {
		t.Errorf("unexpected fields: %+v", s)
	}
}

func TestSMTPSender_SendEmail_Error(t *testing.T) {
	// Point at a closed port so the dial fails fast instead of reaching a real server.
	s := NewSMTPSender("127.0.0.1", 1, "from@x.com", "pw")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.SendEmail(ctx, "to@x.com", "subject", "<p>hi</p>"); err == nil {
		t.Error("expected error dialing closed SMTP port")
	}
}
