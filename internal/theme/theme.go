// Package theme maps ghud's 13 colour tokens to colours. Widgets and payloads
// only ever name tokens, so a theme recolours everything, plugins included.
package theme

import (
	"image/color"

	"charm.land/lipgloss/v2"
)

type Token int

const (
	Bg Token = iota
	Sel
	Line
	Faint
	Dim
	Fg
	Bright
	Accent
	Warn
	Ok
	Bad
	Info
	Alt
	tokenCount
)

var names = [tokenCount]string{"bg", "sel", "line", "faint", "dim", "fg", "bright", "accent", "warn", "ok", "bad", "info", "alt"}

func (t Token) String() string {
	if t < 0 || t >= tokenCount {
		return "unknown"
	}
	return names[t]
}

// Tokens lists every token in canonical order.
func Tokens() []Token {
	ts := make([]Token, tokenCount)
	for i := range ts {
		ts[i] = Token(i)
	}
	return ts
}

type Theme struct {
	colors [tokenCount]color.Color
}

// New builds a theme from a colour per token; tokens left out fall back to the ghud dark palette.
func New(colors map[Token]color.Color) Theme {
	t := Dark()
	for tok, c := range colors {
		t.colors[tok] = c
	}
	return t
}

// Dark is the prototype's truecolor palette.
func Dark() Theme {
	hex := [tokenCount]string{
		Bg: "#13171d", Sel: "#222a36", Line: "#3d4757", Faint: "#4e5766", Dim: "#7e8797",
		Fg: "#d3d9e2", Bright: "#f5f7fa", Accent: "#8aa9ff", Warn: "#e4ae5c", Ok: "#86c497",
		Bad: "#ef7d7d", Info: "#b59dff", Alt: "#6cc3d8",
	}
	var t Theme
	for i, h := range hex {
		t.colors[i] = lipgloss.Color(h)
	}
	return t
}

func (t Theme) Color(tok Token) color.Color { return t.colors[tok] }

func (t Theme) Fg(tok Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(t.colors[tok]) }
