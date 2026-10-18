// Package clock is the Clock Widget type: the time in big digits plus the date.
package clock

import (
	"time"

	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "clock",
	Title:       "Clock",
	Description: "The time in big digits, plus the date.",
	Settings: []widget.Setting{{
		Name: "seconds", Kind: widget.Bool, Default: true,
		Description: "Show seconds.",
		Why:         "Turn it off for a calmer clock that changes once a minute.",
	}},
	New: func(c widget.Config) widget.Widget {
		return clock{seconds: c.Settings.Bool("seconds"), twelve: c.TimeFormat == "12h"}
	},
}

type clock struct{ seconds, twelve bool }

func (c clock) Next(now time.Time) time.Time {
	d := time.Minute
	if c.seconds {
		d = time.Second
	}
	return now.Truncate(d).Add(d)
}

func (c clock) Payload(f widget.Frame) widget.Payload {
	layout := "15:04"
	if c.twelve {
		layout = "3:04"
	}
	if c.seconds {
		layout += ":05"
	}
	if c.twelve {
		layout += " PM"
	}
	t := f.Now.Format(layout)
	for _, d := range []widget.Digits{widget.Block, widget.HalfBlock} {
		// The digits, a blank line and the date.
		if s := render.DigitsSize(t, d); s.W <= f.Size.W && s.H+2 <= f.Size.H {
			return widget.Payload{Center: true, Lines: []widget.Line{
				{Digits: d, Spans: []widget.Span{{Text: t, Token: theme.Bright}}},
				{},
				widget.Text(theme.Dim, f.Now.Format("Monday 2 January")),
			}}
		}
	}
	return widget.Payload{Center: true, Lines: []widget.Line{
		{Spans: []widget.Span{{Text: t, Token: theme.Bright, Bold: true}}},
		widget.Text(theme.Dim, f.Now.Format("Mon 2 Jan")),
	}}
}
