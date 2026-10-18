package host_test

import (
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
)

func TestDefaultLayoutPlacesClockThenCalendarInOneRow(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 20})

	if got, want := h.Line(0), "╭─ Clock ────────────────────────────────────────╮╭─ Calendar ─────────────────────────────────────╮"; got != want {
		t.Errorf("top row:\n%s\nwant:\n%s", got, want)
	}
	if got, want := h.Line(19), "╰────────────────────────────────────────────────╯╰────────────────────────────────────────────────╯"; got != want {
		t.Errorf("bottom row:\n%s\nwant:\n%s", got, want)
	}
}

func TestCalendarShowsTheMonthGridWithTodayHighlighted(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 20, Now: sundayAfternoon})

	want := []string{
		"                  October 2026",
		"              Su Mo Tu We Th Fr Sa",
		"                           1  2  3",
		"               4  5  6  7  8  9 10",
		"              11 12 13 14 15 16 17",
		"              18 19 20 21 22 23 24",
		"              25 26 27 28 29 30 31",
	}
	for i, w := range want {
		if got := strings.TrimRight(columns(h.Line(6+i), 51, 99), " "); got != w {
			t.Errorf("calendar line %d: %q, want %q", i, got, w)
		}
	}

	cell := func(text string) (int, int) {
		x, y, ok := h.Find(text)
		if !ok {
			t.Fatalf("%q not on screen:\n%s", text, h.Screen())
		}
		return x, y
	}
	checks := []struct {
		what string
		text string
		fg   theme.Token
	}{
		{"month", "October 2026", theme.Fg},
		{"weekday header", "Su Mo", theme.Dim},
		{"weekend header", "Fr Sa", theme.Faint},
		{"weekday", "15 16", theme.Fg},
		{"weekend day", "16 17", theme.Faint},
	}
	for _, c := range checks {
		x, y := cell(c.text)
		if got, ok := h.Fg(x, y); !ok || got != c.fg {
			t.Errorf("%s %q drawn in %v (theme colour %v), want %v", c.what, c.text, got, ok, c.fg)
		}
	}
	if x, y := cell("October 2026"); !h.Bold(x, y) {
		t.Error("month title not bold")
	}

	x, y := cell("18 19")
	for dx := range 2 {
		if bg, ok := h.Bg(x+dx, y); !ok || bg != theme.Accent {
			t.Errorf("today cell %d background %v (theme colour %v), want accent", dx, bg, ok)
		}
		if fg, ok := h.Fg(x+dx, y); !ok || fg != theme.Bg {
			t.Errorf("today cell %d foreground %v (theme colour %v), want bg", dx, fg, ok)
		}
	}
	if bg, ok := h.Bg(x+3, y); ok && bg == theme.Accent {
		t.Error("the day after today is highlighted too")
	}
}

// columns is the text of line between cells from (inclusive) and to (exclusive).
func columns(line string, from, to int) string {
	r := []rune(line)
	for len(r) < to {
		r = append(r, ' ')
	}
	return string(r[from:to])
}
