package widget

import "time"

// A Source reports its Status through the error Refresh returns: nil is ok,
// a SetupError is needs setup, a RateLimitError pauses its credential, and
// any other error is stale while older data shows, else error. An error may
// also implement Reason() string, the few words drawn on the stale marker,
// and Hint() string, how to fix it, drawn under an error.

// SetupError puts a Widget in needs setup until a refresh succeeds.
type SetupError struct {
	Hint string
	// Action, when set, fixes it: it becomes the Widget's primary Action,
	// run through the Widget's Invoke like any other.
	Action *Action
}

func (e *SetupError) Error() string { return e.Hint }

// RateLimitError pauses refreshes of every Widget sharing the Source's
// Credential until Until.
type RateLimitError struct {
	Until time.Time
	Err   error
}

func (e *RateLimitError) Error() string { return e.Err.Error() }
func (e *RateLimitError) Unwrap() error { return e.Err }
func (e *RateLimitError) Reason() string {
	return "rate limited"
}
