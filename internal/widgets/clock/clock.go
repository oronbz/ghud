// Package clock is the Clock Widget type: the time in big digits plus the date.
package clock

import (
	"time"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "clock",
	Title:       "Clock",
	Description: "The time in big digits, plus the date.",
	Tick:        time.Second,
	Settings: []widget.Setting{{
		Name: "seconds", Kind: widget.Bool, Default: true,
		Description: "Show seconds.",
		Why:         "Turn it off for a calmer clock that changes once a minute.",
	}},
	New: func(c widget.Config) widget.Widget { return clock{seconds: c.Settings.Bool("seconds")} },
}

// bigWidth is the width of HH:MM:SS in half-block digits.
const bigWidth = 27

type clock struct{ seconds bool }

func (c clock) Payload(f widget.Frame) widget.Payload {
	t := f.Now.Format("15:04")
	if c.seconds {
		t = f.Now.Format("15:04:05")
	}
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
