package framework

import (
	"errors"
	"testing"
)

func TestTerminalError(t *testing.T) {
	err := NewTerminalError("bad data")
	if err.Error() != "bad data" {
		t.Errorf("Error() = %q, want %q", err.Error(), "bad data")
	}

	// It should be detectable via errors.As.
	wrapped := error(err)
	var te *TerminalError
	if !errors.As(wrapped, &te) {
		t.Error("expected errors.As to match *TerminalError")
	}
	if te.Message != "bad data" {
		t.Errorf("Message = %q", te.Message)
	}
}

func TestRetryableError(t *testing.T) {
	cause := errors.New("source data not ready")
	err := NewRetryableError("lagged retry failed", cause)

	if got, want := err.Error(), "lagged retry failed: source data not ready"; got != want {
		t.Errorf("Error() = %q, want %q", got, want)
	}

	// Detectable via errors.As even when wrapped further up the stack.
	wrapped := error(err)
	var re *RetryableError
	if !errors.As(wrapped, &re) {
		t.Error("expected errors.As to match *RetryableError")
	}

	// Unwrap exposes the cause so errors.Is keeps working.
	if !errors.Is(err, cause) {
		t.Error("expected errors.Is to match the wrapped cause")
	}

	// A RetryableError with no cause reports just its message.
	if got := NewRetryableError("just a message", nil).Error(); got != "just a message" {
		t.Errorf("Error() with nil cause = %q, want %q", got, "just a message")
	}
}
