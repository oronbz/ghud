package host_test

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
)

const threeAcross = `{"default": {"rows": [
	{"weight": 1, "widgets": [{"id": "a", "weight": 1}, {"id": "b", "weight": 1}, {"id": "c", "weight": 1}]},
	{"weight": 1, "widgets": [{"id": "d", "weight": 1}]}
]}}`

func TestArrangeMoveRightIsKeptWrittenAndRestoredOnRestart(t *testing.T) {
	root := savedLayout(t, threeAcross)
	h := phoneticAt(t, root)

	h.Press("1", "m", "l", "enter")
	if got := order(t, h, "Alpha", "Bravo", "Charlie"); got != "Bravo Alpha Charlie" {
		t.Fatalf("after m l ⏎ the first row reads %q, want Bravo Alpha Charlie:\n%s", got, h.Screen())
	}
	saved, err := layout.Load(h.Paths.Config)
	if err != nil {
		t.Fatalf("layout.json not written: %v", err)
	}
	if id := saved.Rows[0].Widgets[0].ID; id != "b" {
		t.Errorf("saved first Widget %q, want b", id)
	}

	restarted := phoneticAt(t, root)
	if got := order(t, restarted, "Alpha", "Bravo", "Charlie"); got != "Bravo Alpha Charlie" {
		t.Errorf("after a restart the first row reads %q, want Bravo Alpha Charlie:\n%s", got, restarted.Screen())
	}
}

func TestArrangePushingPastTheLastRowOpensARowAndEscRevertsWithoutWriting(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21})
	h.Press("1", "m", "j")
	if _, y, _ := h.Find("━ Clock "); y == 0 {
		t.Fatalf("m j left Clock in the top row:\n%s", h.Screen())
	}
	if got := cornersOn(h.Line(0)); len(got) != 1 {
		t.Errorf("top row has tiles at %v, want Calendar alone:\n%s", got, h.Screen())
	}

	h.Press("esc")
	if got := order(t, h, "Clock", "Calendar"); got != "Clock Calendar" {
		t.Errorf("esc left the top row reading %q, want the arrangement before m:\n%s", got, h.Screen())
	}
	if got := focused(t, h, []string{"Clock", "Calendar"}); got != "Clock" {
		t.Errorf("esc out of arrange mode focuses %q, want Clock still focused", got)
	}
	if strings.Contains(h.Screen(), "┏") {
		t.Errorf("heavy border still drawn after esc:\n%s", h.Screen())
	}
	noLayoutFile(t, h)
}

func TestArrangeModeShowsAHeavyWarnBorderAndTheArrangeBadge(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21})
	h.Press("2", "m")
	x, y, ok := h.Find("┏━ Calendar ")
	if !ok {
		t.Fatalf("no heavy border on the arranged Widget:\n%s", h.Screen())
	}
	for _, c := range []struct {
		what string
		x, y int
	}{{"corner", x, y}, {"side", x, y + 3}, {"bottom", x + 5, 19}} {
		if got, ok := h.Fg(c.x, c.y); !ok || got != theme.Warn {
			t.Errorf("arranged border %s drawn in %v (theme colour %v), want warn", c.what, got, ok)
		}
	}
	if _, _, ok := h.Find("╭─ Clock "); !ok {
		t.Errorf("the other Widget lost its rounded border:\n%s", h.Screen())
	}
	if footer := h.Line(20); !strings.Contains(footer, "ARRANGE") || strings.Contains(footer, "NORMAL") {
		t.Errorf("footer %q, want the ARRANGE badge", footer)
	}
	h.Press("enter")
	if footer := h.Line(20); !strings.Contains(footer, "NORMAL") {
		t.Errorf("footer after ⏎ %q, want NORMAL", footer)
	}
	noLayoutFile(t, h)
}

func TestArrangeResizeReflowsTheGrid(t *testing.T) {
	h := phonetic(t, threeAcross)
	width := func(title string) int {
		x, y, ok := h.Find(title + " ")
		if !ok {
			t.Fatalf("%q not on screen:\n%s", title, h.Screen())
		}
		line := []rune(h.Line(y))
		end := x
		for end < len(line) && line[end] != '╮' && line[end] != '┓' {
			end++
		}
		return end - x + 4
	}
	height := func(title string) int {
		x, y, ok := h.Find(title + " ")
		if !ok {
			t.Fatalf("%q not on screen:\n%s", title, h.Screen())
		}
		bottom := y + 1
		for r := []rune(h.Line(bottom)); x-3 < len(r) && r[x-3] != '╰' && r[x-3] != '┗'; r = []rune(h.Line(bottom)) {
			bottom++
		}
		return bottom - y + 1
	}

	h.Press("2", "m")
	w0, h0 := width("Bravo"), height("Bravo")
	h.Press("L")
	if w := width("Bravo"); w <= w0 {
		t.Errorf("L left Bravo %d wide, want wider than %d:\n%s", w, w0, h.Screen())
	}
	h.Press("H", "H")
	if w := width("Bravo"); w >= w0 {
		t.Errorf("L H H left Bravo %d wide, want narrower than %d:\n%s", w, w0, h.Screen())
	}
	h.Press("J")
	if got := height("Bravo"); got <= h0 {
		t.Errorf("J left Bravo %d tall, want taller than %d:\n%s", got, h0, h.Screen())
	}
	h.Press("K", "K")
	if got := height("Bravo"); got >= h0 {
		t.Errorf("J K K left Bravo %d tall, want shorter than %d:\n%s", got, h0, h.Screen())
	}
	if w := width("Delta"); w != 96 {
		t.Errorf("resizing Bravo changed Delta's width to %d", w)
	}
}

// order lists titles that share the top row, left to right, failing when one
// isn't on that row.
func order(t *testing.T, h *hosttest.Host, titles ...string) string {
	t.Helper()
	xs := map[int]string{}
	for _, title := range titles {
		x, y, ok := h.Find("─ " + title + " ")
		if !ok || y != 0 {
			return title + " not in the top row"
		}
		xs[x] = title
	}
	out := ""
	for x := range h.Buffer().Width() {
		if title, ok := xs[x]; ok {
			if out != "" {
				out += " "
			}
			out += title
		}
	}
	return out
}

func TestTheSavedLayoutFollowsWidgetsAddedToAndRemovedFromTheConfig(t *testing.T) {
	root := savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "clock", "weight": 1}, {"id": "calendar", "weight": 1}]}
	]}}`)
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 21, Root: root, Config: `
[[widget]]
type = "clock"

[[widget]]
type = "clock"
id = "work"
title = "Work"
`})
	if _, y, ok := h.Find("╭─ Work "); !ok || y == 0 {
		t.Fatalf("Work, new in the config, starts on row %d (shown %v), want a new last row:\n%s", y, ok, h.Screen())
	}

	h.Press("1", "m", "J", "enter")
	saved, err := layout.Load(h.Paths.Config)
	if err != nil {
		t.Fatal(err)
	}
	if got := shapeOf(saved); got != "clock / work" {
		t.Errorf("saved Layout %q, want clock / work without the removed calendar", got)
	}
}

func shapeOf(l layout.Layout) string {
	var rows []string
	for _, r := range l.Rows {
		var ids []string
		for _, c := range r.Widgets {
			ids = append(ids, c.ID)
		}
		rows = append(rows, strings.Join(ids, " "))
	}
	return strings.Join(rows, " / ")
}

func noLayoutFile(t *testing.T, h *hosttest.Host) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(h.Paths.Config, layout.FileName)); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("layout.json exists (stat error %v), want nothing written", err)
	}
}
