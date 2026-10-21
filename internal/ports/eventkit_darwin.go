//go:build darwin && cgo

package ports

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework EventKit -framework Foundation
#import <EventKit/EventKit.h>
static int visor_ek_status(int entity) {
	return (int)[EKEventStore authorizationStatusForEntityType:(EKEntityType)entity];
}
*/
import "C"

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/BRO3886/go-eventkit/calendar"
	"github.com/BRO3886/go-eventkit/reminders"
)

// macEventKit reads EventKit through go-eventkit. Its New functions ask the
// user for access when they haven't answered yet, so they only ever run from
// Request or once Status says access is granted.
type macEventKit struct {
	mu        sync.Mutex
	cal       *calendar.Client
	rem       *reminders.Client
	unaskable [2]bool
}

func newEventKit() EventKit { return &macEventKit{} }

func (k *macEventKit) Status(e Entity) Access {
	k.mu.Lock()
	defer k.mu.Unlock()
	return k.status(e)
}

// status is Status with k.mu held.
func (k *macEventKit) status(e Entity) Access {
	client := e == Events && k.cal != nil || e == Reminders && k.rem != nil
	// EKEntityTypeEvent is 0 and EKEntityTypeReminder 1, like Entity.
	return accessWith(accessOf(int(C.visor_ek_status(C.int(e)))), client, k.unaskable[e])
}

// accessWith refines EventKit's status a: in a terminal it can stay
// undetermined after a grant made moments ago in this process, which a
// client go-eventkit made shows, or after a Request no prompt answered.
// Any other status is EventKit's own, so access revoked meanwhile shows.
func accessWith(a Access, client, unaskable bool) Access {
	switch {
	case a != AccessUndetermined:
		return a
	case client:
		return AccessGranted
	case unaskable:
		return AccessUnaskable
	}
	return a
}

// accessOf maps an EKAuthorizationStatus; write-only access can't read, so
// it counts as denied.
func accessOf(status int) Access {
	switch status {
	case 0:
		return AccessUndetermined
	case 3:
		return AccessGranted
	}
	return AccessDenied
}

func (k *macEventKit) Request(_ context.Context, e Entity) (Access, error) {
	var err error
	switch e {
	case Events:
		_, err = k.calendar()
	case Reminders:
		_, err = k.reminders()
	}
	k.mu.Lock()
	defer k.mu.Unlock()
	k.unaskable[e] = false
	a := k.status(e)
	if err != nil && !errors.Is(err, calendar.ErrAccessDenied) && !errors.Is(err, reminders.ErrAccessDenied) {
		return a, err
	}
	if a == AccessUndetermined {
		k.unaskable[e] = true
		a = AccessUnaskable
	}
	return a, nil
}

func (k *macEventKit) Events(_ context.Context, from, to time.Time) ([]Event, error) {
	if k.Status(Events) != AccessGranted {
		return nil, ErrNoAccess
	}
	c, err := k.calendar()
	if err != nil {
		return nil, err
	}
	evs, err := c.Events(from, to)
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, len(evs))
	for _, e := range evs {
		out = append(out, Event{
			ID: e.ID, Title: e.Title, Start: e.StartDate, End: e.EndDate, AllDay: e.AllDay,
			Location: e.Location, Calendar: e.Calendar,
			Declined: e.SelfStatus == calendar.ParticipantStatusDeclined,
			Canceled: e.Status == calendar.StatusCanceled,
		})
	}
	return out, nil
}

func (k *macEventKit) Reminders(_ context.Context) ([]Reminder, error) {
	if k.Status(Reminders) != AccessGranted {
		return nil, ErrNoAccess
	}
	r, err := k.reminders()
	if err != nil {
		return nil, err
	}
	rs, err := r.Reminders(reminders.WithCompleted(false))
	if err != nil {
		return nil, err
	}
	out := make([]Reminder, 0, len(rs))
	for _, r := range rs {
		out = append(out, reminderOf(r))
	}
	return out, nil
}

// EventKit keeps a due day without a time as date
// components, which go-eventkit returns as local midnight.
func reminderOf(r reminders.Reminder) Reminder {
	out := Reminder{ID: r.ID, Title: r.Title, List: r.List}
	if r.DueDate != nil {
		out.Due = r.DueDate.In(time.Local)
		h, m, s := out.Due.Clock()
		out.DateOnly = h == 0 && m == 0 && s == 0
	}
	return out
}

func (k *macEventKit) Complete(_ context.Context, id string) error {
	if k.Status(Reminders) != AccessGranted {
		return ErrNoAccess
	}
	r, err := k.reminders()
	if err != nil {
		return err
	}
	_, err = r.CompleteReminder(id)
	return err
}

func (k *macEventKit) calendar() (*calendar.Client, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.cal == nil {
		c, err := calendar.New()
		if err != nil {
			return nil, err
		}
		k.cal = c
	}
	return k.cal, nil
}

func (k *macEventKit) reminders() (*reminders.Client, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.rem == nil {
		r, err := reminders.New()
		if err != nil {
			return nil, err
		}
		k.rem = r
	}
	return k.rem, nil
}
