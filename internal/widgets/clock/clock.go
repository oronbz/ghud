// Package clock is the Clock Widget type: the time in big digits plus the date.
package clock

import (
	"time"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:  "clock",
	Title: "Clock",
	Tick:  time.Second,
	New:   func() widget.Widget { return clock{} },
}

// bigWidth is the width of HH:MM:SS in half-block digits.
const bigWidth = 27

type clock struct{}

func (clock) Payload(f widget.Frame) widget.Payload {
	t := f.Now.Format("15:04:05")
	if f.Size.W < bigWidth || f.Size.H < 5 {
		return widget.Payload{Center: true, Lines: []widget.Line{
			{Spans: []widget.Span{{Text: t, Token: theme.Bright, Bold: true}}},
			widget.Text(theme.Dim, f.Now.Format("Mon 2 Jan")),
		}}
	}
	return widget.Payload{Center: true, Lines: []widget.Line{
		{Big: true, Spans: []widget.Span{{Text: t, Token: theme.Bright}}},
		{},
		widget.Text(theme.Dim, f.Now.Format("Monday 2 January")),
	}}
}
