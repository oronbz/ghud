// Package ekaccess is the needs-setup flow shared by the EventKit Widget
// types (Meetings, Reminders): check access without prompting, ask for it
// from the setup Action, and send a denial to the Privacy pane.
package ekaccess

import (
	"context"

	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/widget"
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
// Source's Refresh returns. It never asks the user. Access is granted per
// terminal app, so the error is local: an instance in another terminal, or
// the next one in this one, checks for itself rather than read it from the
// cache.
func Check(ek ports.EventKit, e ports.Entity) error {
	w := entities[e]
	var setup *widget.SetupError
	switch ek.Status(e) {
	case ports.AccessGranted:
		return nil
	case ports.AccessUndetermined:
		setup = &widget.SetupError{
			Hint:   "Allow access to your " + w.things,
			Action: &widget.Action{ID: Grant, Label: "allow access"},
		}
	case ports.AccessUnaskable:
		// A lagging status or a passing failure looks the same, so asking
		// again stays on offer.
		setup = &widget.SetupError{
			Hint:   "This terminal can't ask for " + w.store + " access; run visor in another",
			Action: &widget.Action{ID: Grant, Label: "ask again"},
		}
	case ports.AccessDenied:
		setup = &widget.SetupError{
			Hint: w.store + " access is off for this terminal",
			Action: &widget.Action{ID: "privacy", Label: "open Privacy settings",
				Verb: widget.Open, Target: w.privacy},
		}
	default:
		setup = &widget.SetupError{Hint: "Not available on this platform"}
	}
	return &widget.LocalError{Err: setup}
}

// Request runs the Grant Action: it asks the user for access to e. A denial
// still succeeds, so the Widget refreshes and offers the Privacy pane, as
// does a terminal that can't ask, whose refreshed hint says so.
func Request(ctx context.Context, ek ports.EventKit, e ports.Entity) (widget.Result, error) {
	a, err := ek.Request(ctx, e)
	if err != nil || a == ports.AccessGranted || a == ports.AccessUnaskable {
		return widget.Result{}, err
	}
	return widget.Result{Toast: "Access to your " + entities[e].things + " wasn't allowed"}, nil
}
