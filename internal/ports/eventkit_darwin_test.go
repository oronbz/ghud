//go:build darwin && cgo

package ports

import (
	"testing"
	"time"

	"github.com/BRO3886/go-eventkit/reminders"
)

func TestEKAuthorizationStatusMapsToAccess(t *testing.T) {
	for status, want := range map[int]Access{
		0: AccessUndetermined, // notDetermined
		1: AccessDenied,       // restricted
		2: AccessDenied,       // denied
		3: AccessGranted,      // fullAccess
		4: AccessDenied,       // writeOnly can't read events
	} {
		if got := accessOf(status); got != want {
			t.Errorf("EKAuthorizationStatus %d = %v, want %v", status, got, want)
		}
	}
}

func TestADueDayWithoutATimeIsDateOnly(t *testing.T) {
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.Local)
	timed := time.Date(2026, 10, 20, 16, 30, 0, 0, time.Local)
	for _, c := range []struct {
		due      *time.Time
		dateOnly bool
	}{
		{nil, false},
		{ptr(day.UTC()), true},
		{ptr(timed.UTC()), false},
	} {
		got := reminderOf(reminders.Reminder{ID: "r", Title: "Pay rent", DueDate: c.due})
		if got.DateOnly != c.dateOnly {
			t.Errorf("due %v: DateOnly = %v, want %v", c.due, got.DateOnly, c.dateOnly)
		}
		if c.due != nil && (!got.Due.Equal(*c.due) || got.Due.Location() != time.Local) {
			t.Errorf("due %v came back as %v, want the same instant in local time", c.due, got.Due)
		}
	}
}

func ptr(t time.Time) *time.Time { return &t }
