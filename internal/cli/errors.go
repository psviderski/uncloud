package cli

// CancelledError signals an interactive command was aborted by the user at a confirmation prompt.
type CancelledError struct {
	Message string
}

func (e *CancelledError) Error() string {
	return e.Message
}

// Cancelled returns a CancelledError carrying the given user-facing message.
func Cancelled(msg string) error {
	return &CancelledError{Message: msg}
}
