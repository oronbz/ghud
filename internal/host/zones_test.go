package host_test

import (
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/layout"
)

func TestHitZonesMatchTheDrawnBorders(t *testing.T) {
	sideBySide := hosttest.New(t, hosttest.Options{Width: 70, Height: 15})
	stacked := hosttest.New(t, hosttest.Options{Width: 50, Height: 21, Root: savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "clock", "weight": 1}]},
		{"weight": 2, "widgets": [{"id": "calendar", "weight": 1}]}
	]}}`)})

	for name, h := range map[string]*hosttest.Host{"side by side": sideBySide, "stacked": stacked} {
		t.Run(name, func(t *testing.T) {
			screen := grid(h.Screen())
			kinds := map[layout.ZoneKind]int{}
			for _, z := range h.Zones() {
				kinds[z.Kind]++
				switch z.Kind {
				case layout.Body:
					checkBody(t, screen, z)
				case layout.Title:
					checkTitle(t, screen, z)
				case layout.WidgetSeam:
					checkWidgetSeam(t, screen, z)
				case layout.RowSeam:
					checkRowSeam(t, screen, z)
				}
			}
			seam := layout.WidgetSeam
			if name == "stacked" {
				seam = layout.RowSeam
			}
			if kinds[layout.Body] != 2 || kinds[layout.Title] != 2 || kinds[seam] != 1 || len(kinds) != 3 {
				t.Errorf("zone kinds %v, want 2 bodies, 2 titles and one %v", kinds, seam)
			}
		})
	}
}

// checkBody wants the zone to be exactly the inside of a rounded box.
func checkBody(t *testing.T, s [][]rune, z layout.Zone) {
	t.Helper()
	x0, y0, x1, y1 := z.X-1, z.Y-1, z.X+z.W, z.Y+z.H
	if s[y0][x0] != '╭' || s[y0][x1] != '╮' || s[y1][x0] != '╰' || s[y1][x1] != '╯' {
		t.Errorf("%s body %+v is not framed by the four corners", z.ID, z.Rect)
	}
	for y := z.Y; y < y1; y++ {
		if s[y][x0] != '│' || s[y][x1] != '│' {
			t.Errorf("%s body %+v row %d not between side borders", z.ID, z.Rect, y)
		}
	}
	for x := z.X; x < x1; x++ {
		if s[y1][x] != '─' {
			t.Errorf("%s body %+v column %d not above the bottom border", z.ID, z.Rect, x)
		}
	}
}

// checkTitle wants the zone to cover "─ Title " in the top border, right after the corner.
func checkTitle(t *testing.T, s [][]rune, z layout.Zone) {
	t.Helper()
	got := string(s[z.Y][z.X : z.X+z.W])
	if z.H != 1 || s[z.Y][z.X-1] != '╭' || !strings.HasPrefix(got, "─ ") || !strings.HasSuffix(got, " ") {
		t.Errorf("%s title zone %+v covers %q", z.ID, z.Rect, got)
	}
	if want := "─ " + map[string]string{"clock": "Clock", "calendar": "Calendar"}[z.ID] + " "; got != want {
		t.Errorf("%s title zone covers %q, want %q", z.ID, got, want)
	}
}

// checkWidgetSeam wants two facing side borders along the whole shared edge.
func checkWidgetSeam(t *testing.T, s [][]rune, z layout.Zone) {
	t.Helper()
	if z.W != 2 {
		t.Errorf("widget seam %+v is not two cells wide", z.Rect)
	}
	for y := z.Y; y < z.Y+z.H; y++ {
		if got := string(s[y][z.X : z.X+2]); got != "││" {
			t.Errorf("widget seam %+v row %d is %q, want facing borders", z.Rect, y, got)
		}
	}
	if got := string(s[z.Y-1][z.X : z.X+2]); got != "╮╭" {
		t.Errorf("widget seam %+v does not start under the top corners: %q", z.Rect, got)
	}
	if got := string(s[z.Y+z.H][z.X : z.X+2]); got != "╯╰" {
		t.Errorf("widget seam %+v does not end above the bottom corners: %q", z.Rect, got)
	}
}

// checkRowSeam wants the bottom borders of one row over the top borders of the next.
func checkRowSeam(t *testing.T, s [][]rune, z layout.Zone) {
	t.Helper()
	if z.H != 2 || z.X != 0 || z.W != len(s[0]) {
		t.Errorf("row seam %+v does not span two full rows", z.Rect)
	}
	if s[z.Y][0] != '╰' || s[z.Y][z.W-1] != '╯' || s[z.Y+1][0] != '╭' || s[z.Y+1][z.W-1] != '╮' {
		t.Errorf("row seam %+v is not a bottom border over a top border", z.Rect)
	}
}

// grid pads the screen into a rectangle of cells.
func grid(screen string) [][]rune {
	lines := strings.Split(screen, "\n")
	w := 0
	for _, l := range lines {
		w = max(w, len([]rune(l)))
	}
	out := make([][]rune, len(lines))
	for i, l := range lines {
		r := []rune(l)
		out[i] = append(r, []rune(strings.Repeat(" ", w-len(r)))...)
	}
	return out
}
