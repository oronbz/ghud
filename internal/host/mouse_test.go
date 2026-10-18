package host_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

func TestGhudReportsMouseMotionUnlessMouseIsFalse(t *testing.T) {
	on := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 12})
	if got := on.View().MouseMode; got != tea.MouseModeAllMotion {
		t.Errorf("mouse mode %v by default, want all motion (1003)", got)
	}

	off := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 12, Config: "mouse = false\n"})
	if got := off.View().MouseMode; got != tea.MouseModeNone {
		t.Errorf("mouse mode %v with mouse = false, want none", got)
	}
}

const twoOverOne = `{"default": {"rows": [
	{"weight": 1, "widgets": [{"id": "a", "weight": 1}, {"id": "b", "weight": 1}]},
	{"weight": 1, "widgets": [{"id": "c", "weight": 1}]}
]}}`

func TestADraggedTitleShowsASmallGhostWithTheTitleAndABarOnTheDropSide(t *testing.T) {
	h := abc(t, twoOverOne)
	x, y, _ := h.Find("─ Charlie ")
	h.Grab(x+3, y)
	h.DragTo(2, 7)

	left, top, right, bottom := 1<<30, 1<<30, -1, -1
	for gy, line := range strings.Split(h.Screen(), "\n") {
		for gx, r := range []rune(line) {
			if r == '┄' || r == '┆' {
				left, top, right, bottom = min(left, gx), min(top, gy), max(right, gx), max(bottom, gy)
			}
		}
	}
	if right < 0 {
		t.Fatalf("no dashed ghost while dragging Charlie:\n%s", h.Screen())
	}
	if w, gh := right-left+1, bottom-top+1; w > 34 || gh > 6 {
		t.Errorf("ghost is %d×%d, want at most 34×6:\n%s", w, gh, h.Screen())
	}
	if !strings.Contains(h.Line(top), "Charlie") {
		t.Errorf("ghost's top edge %q doesn't hold the title", h.Line(top))
	}
	for by := 0; by < 15; by++ {
		if c := h.Cell(0, by); c == nil || c.Content != "┃" {
			t.Fatalf("no heavy bar down Alpha's left side at row %d:\n%s", by, h.Screen())
		}
	}
	if tok, _ := h.Fg(0, 7); tok != theme.Accent {
		t.Errorf("drop bar drawn in %v, want accent", tok)
	}
	if saved, _ := layout.Load(h.Paths.Config); shapeOf(saved) != "a b / c" {
		t.Errorf("layout.json reads %q mid-drag, want it untouched until the drop", shapeOf(saved))
	}

	h.Drop(2, 7)
	if strings.ContainsAny(h.Screen(), "┄┆┃") {
		t.Errorf("ghost or bar left on screen after the drop:\n%s", h.Screen())
	}
}

func TestHoveringATitleShowsTheGrabHandle(t *testing.T) {
	h := abc(t, twoOverOne)
	x, y, _ := h.Find("─ Bravo ")
	h.Hover(x+3, y)
	if _, _, ok := h.Find("─ ⠿ Bravo "); !ok {
		t.Fatalf("no grab handle on the hovered title:\n%s", h.Line(y))
	}
	if strings.Count(h.Screen(), "⠿") != 1 {
		t.Errorf("grab handle on more than the hovered title:\n%s", h.Screen())
	}
	h.Hover(x+3, y+5)
	if strings.Contains(h.Screen(), "⠿") {
		t.Errorf("grab handle stays after the pointer leaves the title:\n%s", h.Line(y))
	}
}

// abc is a 96×31 host of the blank Widgets Alpha, Bravo and Charlie, arranged by layoutJSON.
func abc(t *testing.T, layoutJSON string) *hosttest.Host {
	t.Helper()
	var types []widget.Type
	for _, title := range phoneticTitles[:3] {
		types = append(types, widget.Type{
			Name:  strings.ToLower(title[:1]),
			Title: title,
			New:   func(widget.Config) widget.Widget { return blank{} },
		})
	}
	return hosttest.New(t, hosttest.Options{Width: 96, Height: 31, Root: savedLayout(t, layoutJSON), Types: types})
}

func TestDraggingATitleOntoEachSideOfAWidgetRearrangesAndWritesTheLayout(t *testing.T) {
	// 96×31: Alpha and Bravo share the top row (y 0–14), Charlie fills the
	// bottom one (y 15–29).
	for _, c := range []struct {
		name, drag string
		x, y       int
		want       string
	}{
		{"left of Alpha joins its row before it", "Charlie", 2, 7, "c a b"},
		{"right of Bravo joins its row after it", "Charlie", 93, 7, "a b c"},
		{"top of Charlie opens a row above it", "Bravo", 48, 16, "a / b / c"},
		{"bottom of Charlie opens a row below it", "Alpha", 48, 28, "b / c / a"},
	} {
		t.Run(c.name, func(t *testing.T) {
			h := abc(t, twoOverOne)
			x, y, ok := h.Find("─ " + c.drag + " ")
			if !ok {
				t.Fatalf("%s's title not on screen:\n%s", c.drag, h.Screen())
			}
			h.Drag(x+2, y, c.x, c.y)

			saved, err := layout.Load(h.Paths.Config)
			if err != nil {
				t.Fatalf("layout.json not written after the drop: %v", err)
			}
			if got := shapeOf(saved); got != c.want {
				t.Errorf("dropping %s at %d,%d saved %q, want %q:\n%s", c.drag, c.x, c.y, got, c.want, h.Screen())
			}
		})
	}
}
