package host_test

import (
	"strings"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
)

var sundayAfternoon = time.Date(2026, 10, 18, 14, 5, 9, 0, time.UTC)

func TestClockShowsTimeAndDateInABorderedTile(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9, Now: sundayAfternoon})

	want := strings.Join([]string{
		"╭─ Clock ──────────────────────────────╮",
		"│                                      │",
		"│      ▀█ █ █   █▀█ █▀▀   █▀█ █▀█      │",
		"│       █ ▀▀█ ▀ █ █ ▀▀█ ▀ █ █ ▀▀█      │",
		"│       ▀   ▀ ▀ ▀▀▀ ▀▀▀ ▀ ▀▀▀ ▀▀▀      │",
		"│                                      │",
		"│          Sunday 18 October           │",
		"│                                      │",
		"╰──────────────────────────────────────╯",
	}, "\n")
	if got := h.Screen(); got != want {
		t.Errorf("screen:\n%s\nwant:\n%s", got, want)
	}
}

func TestQQuits(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{})
	h.Press("j", "Q", "ctrl+q")
	if h.Quit() {
		t.Fatal("quit on a key other than q")
	}
	h.Press("q")
	if !h.Quit() {
		t.Error("q did not quit")
	}
}

func TestUnfocusedClockDrawsWithThemeTokens(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 40, Height: 9})

	checks := []struct {
		what string
		text string
		tok  theme.Token
	}{
		{"border", "╭", theme.Line},
		{"title", "Clock", theme.Fg},
		{"digits", "█▀█", theme.Bright},
		{"date", "Sunday", theme.Dim},
	}
	for _, c := range checks {
		x, y, ok := h.Find(c.text)
		if !ok {
			t.Fatalf("%s %q not on screen:\n%s", c.what, c.text, h.Screen())
		}
		if got, ok := h.Fg(x, y); !ok || got != c.tok {
			t.Errorf("%s drawn in %v (theme colour %v), want %v", c.what, got, ok, c.tok)
		}
	}
}

func TestClockTicksOnTheWallClockSecond(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 20, Height: 4, Now: sundayAfternoon.Add(400 * time.Millisecond)})
	if got, want := h.Line(1), "│     14:05:09     │"; got != want {
		t.Fatalf("at start: %q, want %q", got, want)
	}

	h.Advance(599 * time.Millisecond)
	if got, want := h.Line(1), "│     14:05:09     │"; got != want {
		t.Errorf("just before the second: %q, want %q", got, want)
	}
	h.Advance(time.Millisecond)
	if got, want := h.Line(1), "│     14:05:10     │"; got != want {
		t.Errorf("on the second: %q, want %q", got, want)
	}
	h.Advance(time.Minute)
	if got, want := h.Line(1), "│     14:06:10     │"; got != want {
		t.Errorf("a minute later: %q, want %q", got, want)
	}
	if got, want := h.Line(2), "│    Sun 18 Oct    │"; got != want {
		t.Errorf("date: %q, want %q", got, want)
	}
}
