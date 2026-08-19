package framework

// TerminalError represents an error that won't succeed on retry (e.g. malformed data).
// By returning this from a Cloud Function handler, the framework will still log
// the failure and report it to Sentry, but it will ACK the message to Pub/Sub
// (by returning a nil error to the CloudEvents SDK) to prevent infinite retries.
type TerminalError struct {
	Message string
}

func (e *TerminalError) Error() string {
	return e.Message
}

func NewTerminalError(msg string) *TerminalError {
	return &TerminalError{Message: msg}
}

// RetryableError represents a transient failure that is expected to succeed on a
// later delivery (e.g. upstream activity data that has not yet propagated).
// Returning this from a Cloud Function handler tells the framework to NACK the
// message so Pub/Sub redelivers it with backoff — exactly like any other returned
// error — but NOT to report it to Sentry or log it at Error level. A retry that
// will self-heal is normal operation, not an incident; reporting it turns routine
// backoff traffic into a recurring Sentry issue (this was the reported SERVER-7).
// Contrast with TerminalError, which ACKs (no retry) but is still reported.
type RetryableError struct {
	Message string
	Err     error
}

func (e *RetryableError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap exposes the underlying cause so errors.As / errors.Is continue to match
// through the wrapper.
func (e *RetryableError) Unwrap() error {
	return e.Err
}

// NewRetryableError wraps cause as a transient, retry-without-alerting failure.
func NewRetryableError(msg string, cause error) *RetryableError {
	return &RetryableError{Message: msg, Err: cause}
}
