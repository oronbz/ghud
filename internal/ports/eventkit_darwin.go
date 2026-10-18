//go:build darwin && cgo

package ports

/*
#cgo CFLAGS: -x objective-c -fobjc-arc
#cgo LDFLAGS: -framework EventKit -framework Foundation
#import <EventKit/EventKit.h>
static int ghud_ek_status(int entity) {
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
	mu  sync.Mutex
	cal *calendar.Client
	rem *reminders.Client
}

func newEventKit() EventKit { return &macEventKit{} }

func (k *macEventKit) Status(e Entity) Access {
	// EKEntityTypeEvent is 0 and EKEntityTypeReminder 1, like Entity.
	return accessOf(int(C.ghud_ek_status(C.int(e))))
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
	if err != nil && !errors.Is(err, calendar.ErrAccessDenied) && !errors.Is(err, reminders.ErrAccessDenied) {
		return k.Status(e), err
	}
	return k.Status(e), nil
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
