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
	return palette([tokenCount]string{
		Bg: "#13171d", Sel: "#222a36", Line: "#3d4757", Faint: "#4e5766", Dim: "#7e8797",
		Fg: "#d3d9e2", Bright: "#f5f7fa", Accent: "#8aa9ff", Warn: "#e4ae5c", Ok: "#86c497",
		Bad: "#ef7d7d", Info: "#b59dff", Alt: "#6cc3d8",
	})
}

// Light is the prototype palette's hues for a light background.
func Light() Theme {
	return palette([tokenCount]string{
		Bg: "#f7f8fa", Sel: "#e4e8ef", Line: "#c5ccd8", Faint: "#a9b1bf", Dim: "#6b7383",
		Fg: "#2b313b", Bright: "#0e1116", Accent: "#3f63d8", Warn: "#a86a12", Ok: "#2f8a4a",
		Bad: "#c9433f", Info: "#7a55d6", Alt: "#1f8aa3",
	})
}

func palette(hex [tokenCount]string) Theme {
	var t Theme
	for i, h := range hex {
		t.colors[i] = lipgloss.Color(h)
	}
	return t
}

// Terminal holds the terminal's answers to the colour queries; a nil colour went unanswered.
type Terminal struct {
	Bg, Fg color.Color
}

// Resolve builds the theme named in config.toml for what the terminal answered.
func Resolve(name string, term Terminal) Theme {
	if term.Bg != nil && !IsDark(term.Bg) {
		return Light()
	}
	return Dark()
}

// IsDark reports whether c has an HSL lightness under one half.
func IsDark(c color.Color) bool {
	r, g, b, _ := c.RGBA()
	return max(r, g, b)+min(r, g, b) < 0xffff
}

// IsZero reports whether t was never built, as in an unset option.
func (t Theme) IsZero() bool { return t.colors[Bg] == nil }

func (t Theme) Color(tok Token) color.Color { return t.colors[tok] }

func (t Theme) Fg(tok Token) lipgloss.Style { return lipgloss.NewStyle().Foreground(t.colors[tok]) }
