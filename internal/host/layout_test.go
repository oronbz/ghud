package host_test

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/theme"
)

func TestDefaultLayoutPlacesClockThenCalendarInOneRow(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21})

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

func TestWidgetWeightsHoldTheirProportionsWithinARow(t *testing.T) {
	root := savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "clock", "weight": 1}, {"id": "calendar", "weight": 2}]}
	]}}`)
	h := hosttest.New(t, hosttest.Options{Root: root})
	for _, c := range []struct{ w, clock, calendar int }{{60, 20, 40}, {99, 33, 66}, {150, 50, 100}} {
		h.Resize(c.w, 12)
		if got := cornersOn(h.Line(0)); !slices.Equal(got, []int{0, c.clock}) {
			t.Errorf("at width %d tiles start at %v, want [0 %d]", c.w, got, c.clock)
		}
		if x, _, _ := h.Find("Calendar"); x != c.clock+3 {
			t.Errorf("at width %d Calendar title at x=%d, want %d", c.w, x, c.clock+3)
		}
		if got, want := len([]rune(h.Line(0))), c.clock+c.calendar; got != want {
			t.Errorf("at width %d the row is %d cells, want %d", c.w, got, want)
		}
	}
}

func TestRowWeightsHoldTheirProportionsAsTheTerminalGrows(t *testing.T) {
	root := savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "calendar", "weight": 1}]},
		{"weight": 2, "widgets": [{"id": "clock", "weight": 1}]}
	]}}`)
	h := hosttest.New(t, hosttest.Options{Root: root})
	for _, c := range []struct{ h, calendar int }{{42, 14}, {18, 6}, {27, 9}} {
		h.Resize(60, c.h+1)
		if _, y, _ := h.Find("╭─ Calendar"); y != 0 {
			t.Errorf("at height %d Calendar starts on row %d, want 0", c.h, y)
		}
		if _, y, _ := h.Find("╭─ Clock"); y != c.calendar {
			t.Errorf("at height %d Clock starts on row %d, want %d", c.h, y, c.calendar)
		}
		if got := h.Line(c.h - 1); !strings.HasPrefix(got, "╰") {
			t.Errorf("at height %d the last row is %q, want a bottom border", c.h, got)
		}
	}
}

func TestSavedLayoutJoinsTheWidgetsThatExist(t *testing.T) {
	root := savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "gone", "weight": 5}, {"id": "clock", "weight": 1}]}
	]}}`)
	h := hosttest.New(t, hosttest.Options{Width: 60, Height: 20, Root: root})

	if _, y, _ := h.Find("╭─ Clock"); y != 0 {
		t.Errorf("Clock starts on row %d, want 0 as the only saved Widget left", y)
	}
	if got := cornersOn(h.Line(0)); !slices.Equal(got, []int{0}) {
		t.Errorf("first row has tiles at %v, want Clock alone at [0]", got)
	}
	if _, y, ok := h.Find("╭─ Calendar"); !ok || y != 10 {
		t.Errorf("Calendar, missing from the Layout, starts on row %d (shown %v), want a new last row at 10", y, ok)
	}
}

// savedLayout writes layout.json into a fresh root and returns the root.
func savedLayout(t *testing.T, json string) string {
	t.Helper()
	root := t.TempDir()
	dir := paths.Under(root).Config
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "layout.json"), []byte(json), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// cornersOn lists the x of every top-left tile corner on a line.
func cornersOn(line string) []int {
	var xs []int
	for x, r := range []rune(line) {
		if r == '╭' {
			xs = append(xs, x)
		}
	}
	return xs
}

// columns is the text of line between cells from (inclusive) and to (exclusive).
func columns(line string, from, to int) string {
	r := []rune(line)
	for len(r) < to {
		r = append(r, ' ')
	}
	return string(r[from:to])
}
