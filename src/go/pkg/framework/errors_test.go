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
