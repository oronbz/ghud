// Package meetings is the Meetings Widget type: events from every calendar
// on the Mac, grouped by day, with a now-line, live events and countdowns.
// It reads EventKit through ports.EventKit, which only darwin builds back
// with the real store, so the host registers the type on darwin only.
package meetings

import (
	"context"
	"fmt"
	"math"
	"net/url"
	"slices"
	"strconv"
	"time"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/ekaccess"
)

var Type = widget.Type{
	Name:        "meetings",
	Title:       "Meetings",
	Description: "Today's meetings from every calendar on this Mac, then the following days (macOS).",
	Settings: []widget.Setting{{
		Name: "days", Kind: widget.Int, Default: int64(3),
		Description: "How many days to show, today included.",
		Why:         "A taller Widget fills its spare rows with the following days, up to this many.",
	}},
	Refresh: time.Minute,
	New: func(c widget.Config) widget.Widget {
		return &meetings{ek: c.Ports.EventKit, days: max(1, int(c.Settings.Int("days"))), twelve: c.TimeFormat == "12h"}
	},
}

type meetings struct {
	ek     ports.EventKit
	days   int
	twelve bool
}

func (m *meetings) Refresh(ctx context.Context, now time.Time) (widget.Payload, error) {
	if err := ekaccess.Check(m.ek, ports.Events); err != nil {
		return widget.Payload{}, err
	}
	loc := now.Location()
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	end := today.AddDate(0, 0, m.days)
	evs, err := m.ek.Events(ctx, today, end)
	if err != nil {
		return widget.Payload{}, err
	}
	evs = slices.DeleteFunc(evs, func(e ports.Event) bool { return e.Declined || e.Canceled || !e.End.After(now) })
	for i := range evs {
		evs[i].Start, evs[i].End = evs[i].Start.In(loc), evs[i].End.In(loc)
	}
	// All-day events start at local midnight, so they lead their day.
	slices.SortStableFunc(evs, func(a, b ports.Event) int { return a.Start.Compare(b.Start) })

	places := slices.ContainsFunc(evs, func(e ports.Event) bool { return e.Location != "" })
	p := widget.Payload{
		Columns: []widget.Column{{}, {Flex: true}, {Priority: 1, Right: true}},
		Empty:   "No more meetings today",
		Rule:    &widget.Rule{Before: len(evs), Text: m.clock(now), Token: theme.Bad},
	}
	if m.days > 1 {
		p.Empty = fmt.Sprintf("No more meetings in the next %d days", m.days)
	}
	ruled := false
	for i, e := range evs {
		d := dayOf(e.Start, today)
		if !ruled && (d > 0 || !e.AllDay && e.Start.After(now)) {
			p.Rule.Before, ruled = i, true
		}
		if d == 0 && !e.AllDay {
			p.Count++
		}
		it := m.item(e, d, now)
		if places {
			it.Fields = slices.Insert(it.Fields, 2, widget.F(theme.Faint, e.Location))
		}
		p.Items = append(p.Items, it)
	}
	if places {
		p.Columns = slices.Insert(p.Columns, 2, widget.Column{Priority: 2})
	}
	return p, nil
}

func (m *meetings) item(e ports.Event, d int, now time.Time) widget.Item {
	live := !e.AllDay && !e.Start.After(now)
	when := widget.Field{{Text: m.clock(e.Start), Token: theme.Dim}}
	switch {
	case e.AllDay:
		when = widget.F(theme.Dim, "all-day")
	case live:
		when = append(when, widget.Span{Text: " ●", Token: theme.Ok})
	}
	var due widget.Field
	switch {
	case e.AllDay:
	case live:
		due = widget.Field{{Text: "now", Token: theme.Ok, Bold: true}}
	case d == 0:
		mins := max(1, int(math.Round(e.Start.Sub(now).Minutes())))
		tok := theme.Dim
		if mins <= 15 {
			tok = theme.Warn
		}
		due = widget.F(tok, "in "+span(mins))
	default:
		due = widget.F(theme.Faint, span(int(math.Round(e.End.Sub(e.Start).Minutes()))))
	}
	return widget.Item{
		ID:     e.ID + "@" + strconv.FormatInt(e.Start.Unix(), 10),
		Group:  group(e.Start, d),
		Fields: []widget.Field{when, widget.F(theme.Fg, e.Title), due},
		Actions: []widget.Action{{
			ID: "open", Label: "open in Calendar", Primary: true, Verb: widget.Open,
			Target: "ical://ekevent/" + url.PathEscape(e.ID) + "?method=show&options=more",
		}},
	}
}

func (m *meetings) Invoke(ctx context.Context, inv widget.Invocation) (widget.Result, error) {
	if inv.Action == ekaccess.Grant {
		return ekaccess.Request(ctx, m.ek, ports.Events)
	}
	return widget.Result{}, nil
}

func (m *meetings) clock(t time.Time) string {
	if m.twelve {
		return t.Format("3:04 PM")
	}
	return t.Format("15:04")
}

// dayOf counts days from today to t; events that began before today count as today's.
func dayOf(t, today time.Time) int {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, today.Location())
	return max(0, int(math.Round(day.Sub(today).Hours()/24)))
}

func group(t time.Time, d int) string {
	switch d {
	case 0:
		return "Today"
	case 1:
		return "Tomorrow"
	}
	return t.Format("Mon 2 Jan")
}

// span is a length of time the way the prototype writes it: 40m, 2h, 1h 30m.
func span(mins int) string {
	switch {
	case mins < 60:
		return fmt.Sprintf("%dm", mins)
	case mins%60 == 0:
		return fmt.Sprintf("%dh", mins/60)
	}
	return fmt.Sprintf("%dh %dm", mins/60, mins%60)
}
