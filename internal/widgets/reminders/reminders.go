// Package reminders is the Reminders Widget type: the open reminders from
// every list in Reminders.app. Like Meetings it reads EventKit through
// ports.EventKit, so the host registers it on darwin only.
package reminders

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/ekaccess"
)

var Type = widget.Type{
	Name:        "reminders",
	Title:       "Reminders",
	Description: "Open reminders from every list in Reminders.app (macOS).",
	Refresh:     time.Minute,
	Fetches:     true,
	OptIn:       true,
	New: func(c widget.Config) widget.Widget {
		return &reminders{ek: c.Ports.EventKit, twelve: c.TimeFormat == "12h"}
	},
}

type reminders struct {
	ek     ports.EventKit
	twelve bool
}

func (r *reminders) Refresh(ctx context.Context, now time.Time) (widget.Payload, error) {
	if err := ekaccess.Check(r.ek, ports.Reminders); err != nil {
		return widget.Payload{}, err
	}
	rs, err := r.ek.Reminders(ctx)
	if err != nil {
		return widget.Payload{}, err
	}
	slices.SortStableFunc(rs, func(a, b ports.Reminder) int {
		if a.Due.IsZero() || b.Due.IsZero() {
			return cmp.Compare(btoi(a.Due.IsZero()), btoi(b.Due.IsZero()))
		}
		return a.Due.Compare(b.Due)
	})

	lists := map[string]bool{}
	for _, rm := range rs {
		lists[rm.List] = true
	}
	p := widget.Payload{
		Columns: []widget.Column{{}, {Flex: true}, {Priority: 1, Right: true}},
		Empty:   "No open reminders",
		Count:   len(rs),
	}
	if len(lists) > 1 {
		p.Columns = slices.Insert(p.Columns, 2, widget.Column{Priority: 2})
	}
	for _, rm := range rs {
		due, tok := r.dueLabel(rm, now)
		it := widget.Item{
			ID:     rm.ID,
			Fields: []widget.Field{widget.F(theme.Dim, "○"), widget.F(theme.Fg, rm.Title), widget.F(tok, due)},
			Actions: []widget.Action{
				{ID: "complete", Key: "space", Label: "complete", Verb: widget.Complete, Value: rm.Title},
				{ID: "open", Label: "open in Reminders", Primary: true, Verb: widget.Open,
					Target: "x-apple-reminderkit://REMCDReminder/" + rm.ID},
			},
		}
		if len(lists) > 1 {
			it.Fields = slices.Insert(it.Fields, 2, widget.F(theme.Faint, rm.List))
		}
		p.Items = append(p.Items, it)
	}
	return p, nil
}

func (r *reminders) Invoke(ctx context.Context, inv widget.Invocation) (widget.Result, error) {
	switch inv.Action {
	case ekaccess.Grant:
		return ekaccess.Request(ctx, r.ek, ports.Reminders)
	case "complete":
		if err := r.ek.Complete(ctx, inv.Item); err != nil {
			return widget.Result{}, err
		}
		return widget.Result{Toast: fmt.Sprintf("Completed “%s”", inv.Base)}, nil
	}
	return widget.Result{}, fmt.Errorf("reminders has no Action %q", inv.Action)
}

// dueLabel reads like Tasks' due labels; a time of day shows as the time
// while it's still today.
func (r *reminders) dueLabel(rm ports.Reminder, now time.Time) (string, theme.Token) {
	if rm.Due.IsZero() {
		return "", theme.Dim
	}
	loc := now.Location()
	due := rm.Due.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	day := time.Date(due.Year(), due.Month(), due.Day(), 0, 0, 0, 0, loc)
	days := int(day.Sub(today).Hours() / 24)
	switch {
	case days < 0, days == 0 && !rm.DateOnly && due.Before(now):
		return "overdue", theme.Bad
	case days == 0 && rm.DateOnly:
		return "today", theme.Warn
	case days == 0:
		return r.clock(due), theme.Warn
	case days == 1:
		return "tomorrow", theme.Dim
	case days < 7:
		return due.Format("Mon"), theme.Dim
	case due.Year() == now.Year():
		return due.Format("2 Jan"), theme.Dim
	}
	return due.Format("2 Jan 2006"), theme.Dim
}

func (r *reminders) clock(t time.Time) string {
	if r.twelve {
		return t.Format("3:04 PM")
	}
	return t.Format("15:04")
}

func btoi(b bool) int {
	if b {
		return 1
	}
	return 0
}
