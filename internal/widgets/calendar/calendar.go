// Package calendar is the Calendar Widget type: a month grid with today highlighted.
package calendar

import (
	"fmt"
	"strings"
	"time"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "calendar",
	Title:       "Calendar",
	Description: "This month's grid with today highlighted.",
	New:         func(widget.Config) widget.Widget { return calendar{} },
}

// gridWidth is seven 2-cell days with a space between each.
const gridWidth = 7*3 - 1

type calendar struct{}

func (calendar) Payload(f widget.Frame) widget.Payload {
	today := f.Now
	first := time.Date(today.Year(), today.Month(), 1, 0, 0, 0, 0, today.Location())
	days := first.AddDate(0, 1, -1).Day()

	lines := []widget.Line{
		{Spans: []widget.Span{{Text: centre(first.Format("January 2006")), Token: theme.Fg, Bold: true}}},
		header(),
	}
	var week []widget.Span
	col := int(first.Weekday())
	if col > 0 {
		week = append(week, widget.Span{Text: strings.Repeat(" ", col*3)})
	}
	for d := 1; d <= days; d++ {
		if col > 0 {
			week = append(week, widget.Span{Text: " "})
		}
		s := widget.Span{Text: fmt.Sprintf("%2d", d), Token: dayToken(col)}
		if d == today.Day() {
			s = widget.Span{Text: s.Text, Token: theme.Bg, Bg: theme.Accent, Bold: true}
		}
		week = append(week, s)
		col++
		if col == 7 || d == days {
			lines = append(lines, padded(week))
			week, col = nil, 0
		}
	}
	return widget.Payload{Center: true, Lines: lines}
}

func header() widget.Line {
	var spans []widget.Span
	for col, d := range []string{"Su", "Mo", "Tu", "We", "Th", "Fr", "Sa"} {
		tok := theme.Dim
		if weekend(col) {
			tok = theme.Faint
		}
		if col > 0 {
			d = " " + d
		}
		spans = append(spans, widget.Span{Text: d, Token: tok})
	}
	return widget.Line{Spans: spans}
}

func dayToken(col int) theme.Token {
	if weekend(col) {
		return theme.Faint
	}
	return theme.Fg
}

// weekend is Friday and Saturday, as prototyped.
func weekend(col int) bool { return col >= 5 }

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
