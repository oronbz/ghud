package ports

import (
	"context"
	"errors"
	"time"
)

// ErrNoAccess is what EventKit reads return without granted access.
var ErrNoAccess = errors.New("no access to EventKit")

type noEventKit struct{}

func (noEventKit) Status(Entity) Access { return AccessUnsupported }

func (noEventKit) Request(context.Context, Entity) (Access, error) { return AccessUnsupported, nil }

func (noEventKit) Events(context.Context, time.Time, time.Time) ([]Event, error) {
	return nil, ErrNoAccess
}

func (noEventKit) Reminders(context.Context) ([]Reminder, error) { return nil, ErrNoAccess }

func (noEventKit) Complete(context.Context, string) error { return ErrNoAccess }
