// Package ekaccess is the needs-setup flow shared by the EventKit Widget
// types (Meetings, Reminders): check access without prompting, ask for it
// from the setup Action, and send a denial to the Privacy pane.
package ekaccess

import (
	"context"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/widget"
)

// Grant is the ID of the setup Action that asks for access; a Widget's
// Invoke hands it to Request.
const Grant = "grant-access"

type words struct {
	things, store, privacy string
}

var entities = map[ports.Entity]words{
	ports.Events:    {"calendars", "Calendar", "x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars"},
	ports.Reminders: {"reminders", "Reminders", "x-apple.systempreferences:com.apple.preference.security?Privacy_Reminders"},
}

// Check is nil when e's access is granted, else the *widget.SetupError a
// Source's Refresh returns. It never asks the user.
func Check(ek ports.EventKit, e ports.Entity) error {
	w := entities[e]
	switch ek.Status(e) {
	case ports.AccessGranted:
		return nil
	case ports.AccessUndetermined:
		return &widget.SetupError{
			Hint:   "Allow access to your " + w.things,
			Action: &widget.Action{ID: Grant, Label: "allow access"},
		}
	case ports.AccessDenied:
		return &widget.SetupError{
			Hint: w.store + " access is off for this terminal",
			Action: &widget.Action{ID: "privacy", Label: "open Privacy settings",
				Verb: widget.Open, Target: w.privacy},
		}
	}
	return &widget.SetupError{Hint: "Not available on this platform"}
}

// Request runs the Grant Action: it asks the user for access to e. A denial
// still succeeds, so the Widget refreshes and offers the Privacy pane.
func Request(ctx context.Context, ek ports.EventKit, e ports.Entity) (widget.Result, error) {
	a, err := ek.Request(ctx, e)
	if err != nil {
		return widget.Result{}, err
	}
	if a != ports.AccessGranted {
		return widget.Result{Toast: "Access to your " + entities[e].things + " wasn't allowed"}, nil
	}
	return widget.Result{}, nil
}
