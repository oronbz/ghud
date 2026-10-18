// Package calendar is the Calendar Widget type: a month grid with today highlighted.
package calendar

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "calendar",
	Title:       "Calendar",
	Description: "A month grid with today highlighted; n and p change the month.",
	New: func(c widget.Config) widget.Widget {
		return &calendar{first: weekday(c.WeekStart)}
	},
}

// gridWidth is seven 2-cell days with a space between each.
const gridWidth = 7*3 - 1

type calendar struct {
	first time.Weekday
	// offset is the shown month relative to today's; Actions change it off the UI thread.
	offset atomic.Int32
}

func (c *calendar) Next(now time.Time) time.Time {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 0, 0, 0, 0, now.Location())
}

func (c *calendar) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	switch inv.Action {
	case "next":
		c.offset.Add(1)
	case "previous":
		c.offset.Add(-1)
	}
	return widget.Result{}, nil
}

func (c *calendar) Payload(f widget.Frame) widget.Payload {
	today := f.Now
	first := time.Date(today.Year(), today.Month()+time.Month(c.offset.Load()), 1, 0, 0, 0, 0, today.Location())
	days := first.AddDate(0, 1, -1).Day()
	showsToday := first.Year() == today.Year() && first.Month() == today.Month()

	lines := []widget.Line{
		{Spans: []widget.Span{{Text: centre(first.Format("January 2006")), Token: theme.Fg, Bold: true}}},
		c.header(),
	}
	var week []widget.Span
	col := c.column(first.Weekday())
	if col > 0 {
		week = append(week, widget.Span{Text: strings.Repeat(" ", col*3)})
	}
	for d := 1; d <= days; d++ {
		if col > 0 {
			week = append(week, widget.Span{Text: " "})
		}
		day := first.AddDate(0, 0, d-1).Weekday()
		s := widget.Span{Text: fmt.Sprintf("%2d", d), Token: dayToken(day)}
		if showsToday && d == today.Day() {
			s = widget.Span{Text: s.Text, Token: theme.Bg, Bg: theme.Accent, Bold: true}
		}
		week = append(week, s)
		col++
		if col == 7 || d == days {
			lines = append(lines, padded(week))
			week, col = nil, 0
		}
	}
	return widget.Payload{Center: true, Lines: lines, Actions: []widget.Action{
		{ID: "previous", Key: "p", Label: "previous"},
		{ID: "next", Key: "n", Label: "next"},
	}}
}

// column is where day d sits in a week starting on c.first.
func (c *calendar) column(d time.Weekday) int { return (int(d) - int(c.first) + 7) % 7 }

func (c *calendar) header() widget.Line {
	var spans []widget.Span
	for col := range 7 {
		day := time.Weekday((int(c.first) + col) % 7)
		tok := theme.Dim
		if weekend(day) {
			tok = theme.Faint
		}
		text := day.String()[:2]
		if col > 0 {
			text = " " + text
		}
		spans = append(spans, widget.Span{Text: text, Token: tok})
	}
	return widget.Line{Spans: spans}
}

func dayToken(d time.Weekday) theme.Token {
	if weekend(d) {
		return theme.Faint
	}
	return theme.Fg
}

// weekend is Friday and Saturday, as prototyped.
func weekend(d time.Weekday) bool { return d == time.Friday || d == time.Saturday }

func weekday(name string) time.Weekday {
	for d := time.Sunday; d <= time.Saturday; d++ {
		if strings.EqualFold(d.String(), name) {
			return d
		}
	}
	return time.Monday
}

func centre(s string) string {
	gap := gridWidth - len(s)
	return strings.Repeat(" ", gap/2) + s + strings.Repeat(" ", gap-gap/2)
}

// padded fills a short last week to the grid width, so the host's per-line
// centring keeps every column aligned.
func padded(spans []widget.Span) widget.Line {
	n := 0
	for _, s := range spans {
		n += len(s.Text)
	}
	if n < gridWidth {
		spans = append(spans, widget.Span{Text: strings.Repeat(" ", gridWidth-n)})
	}
	return widget.Line{Spans: spans}
}
