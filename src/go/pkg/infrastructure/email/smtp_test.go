package email

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/textproto"
	"sync/atomic"
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

// SERVER-9: the relay dropped the connection mid-handshake ("EOF"); a single
// retry covers that, while protocol rejections must not be retried (double-send risk).
func TestIsTransientSMTPError(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"eof", io.EOF, true},
		{"wrapped eof", fmt.Errorf("x: %w", io.EOF), true},
		{"unexpected eof", io.ErrUnexpectedEOF, true},
		{"net error", &net.OpError{Op: "dial", Err: errors.New("refused")}, true},
		{"smtp rejection", &textproto.Error{Code: 550, Msg: "mailbox unavailable"}, false},
		{"other", errors.New("auth failed"), false},
	}
	for _, c := range cases {
		if got := isTransientSMTPError(c.err); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

// A server that accepts the TCP connection and immediately closes it produces
// EOF on the first attempt; the sender must try again exactly once.
func TestSMTPSender_RetriesOnceOnEOF(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	var accepts atomic.Int32
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			accepts.Add(1)
			c.Close()
		}
	}()
	old := sendRetryDelay
	sendRetryDelay = 10 * time.Millisecond
	defer func() { sendRetryDelay = old }()

	port := ln.Addr().(*net.TCPAddr).Port
	s := NewSMTPSender("127.0.0.1", port, "from@x.com", "pw")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	err = s.SendEmail(ctx, "to@x.com", "subject", "<p>hi</p>")
	if err == nil {
		t.Fatal("expected error from a server that hangs up")
	}
	if !errors.Is(err, io.EOF) {
		t.Errorf("expected EOF-derived error, got %v", err)
	}
	if got := accepts.Load(); got != 2 {
		t.Errorf("expected exactly 2 connection attempts (1 retry), got %d", got)
	}
}
