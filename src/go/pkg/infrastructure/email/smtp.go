package email

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/smtp"
	"time"
)

type SMTPSender struct {
	host     string
	port     int
	from     string
	password string
}

func NewSMTPSender(host string, port int, from string, password string) *SMTPSender {
	return &SMTPSender{
		host:     host,
		port:     port,
		from:     from,
		password: password,
	}
}

func (s *SMTPSender) SendEmail(ctx context.Context, to string, subject string, htmlContent string) error {
	addr := fmt.Sprintf("%s:%d", s.host, s.port)
	auth := smtp.PlainAuth("", s.from, s.password, s.host)

	// Build the email headers and body
	var body bytes.Buffer
	body.WriteString(fmt.Sprintf("From: \"FitGlue\" <%s>\r\n", s.from))
	body.WriteString(fmt.Sprintf("To: %s\r\n", to))
	body.WriteString(fmt.Sprintf("Subject: %s\r\n", subject))
	body.WriteString("MIME-Version: 1.0\r\n")
	body.WriteString("Content-Type: text/html; charset=UTF-8\r\n")
	body.WriteString("\r\n")
	body.WriteString(htmlContent)

	err := smtp.SendMail(addr, auth, s.from, []string{to}, body.Bytes())
	if err != nil && isTransientSMTPError(err) && ctx.Err() == nil {
		// The relay occasionally drops the connection before the handshake
		// completes ("failed to send email: EOF", SERVER-9). One retry after a
		// short pause covers that without risking a duplicate send: the error
		// classes below all mean the message was never accepted for delivery.
		select {
		case <-ctx.Done():
			return fmt.Errorf("failed to send email: %w", err)
		case <-time.After(sendRetryDelay):
		}
		err = smtp.SendMail(addr, auth, s.from, []string{to}, body.Bytes())
	}
	if err != nil {
		return fmt.Errorf("failed to send email: %w", err)
	}

	return nil
}

// sendRetryDelay is the pause before the single SMTP retry.
var sendRetryDelay = 500 * time.Millisecond

// isTransientSMTPError reports whether err is a connection-level failure that
// happened before the server accepted the message, so a retry cannot double-send.
// SMTP protocol rejections (a *textproto.Error, e.g. 550 mailbox unavailable) are
// deliberately not retried.
func isTransientSMTPError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return true
	}
	return false
}
