// Package widget is the contract between the host and Widget types. Built-in
// Widget types implement it directly; the external plugin host adapts
// JSON-RPC to it. Widgets describe what to show; the host draws it (ADR 0001).
package widget

import (
	"time"

	"github.com/oronbz/ghud/internal/theme"
)

// Type is a kind of Widget, placed on the dashboard as one or more Widgets.
type Type struct {
	// Name is the `type` key in config.toml.
	Name string
	// Title is the default title drawn in the Widget's top border.
	Title string
	// Tick is how often a live view redraws; zero means it never ticks.
	Tick time.Duration
	New  func() Widget
}

type Widget interface {
	Payload(Frame) Payload
}

type Size struct{ W, H int }

// Frame is what the host knows when it asks a Widget for its payload.
type Frame struct {
	Now time.Time
	// Size is the tile's inner size, border excluded.
	Size    Size
	Focused bool
}

// Payload is a live view's styled lines.
type Payload struct {
	Lines []Line
	// Center centres the block vertically and each line horizontally.
	Center bool
}

type Line struct {
	Spans []Span
	// Big asks the host to draw the line's text in 3-row half-block digits.
	Big bool
}

type Span struct {
	Text  string
	Token theme.Token
	// Bg fills the span's cells; the zero value, theme.Bg, leaves the background alone.
	Bg   theme.Token
	Bold bool
}

func Text(tok theme.Token, s string) Line { return Line{Spans: []Span{{Text: s, Token: tok}}} }
