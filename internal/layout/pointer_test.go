package layout_test

import (
	"testing"

	"github.com/oronbz/visor/internal/layout"
)

func TestHitRanksTheTitleOverTheRowSeamItSitsIn(t *testing.T) {
	f := layout.Compute(grid([]string{"a"}, []string{"b"}), map[string]string{"a": "A", "b": "B"}, 40, 20, layout.Pointer{})
	if z, ok := f.Hit(2, 10); !ok || z.Kind != layout.Title || z.ID != "b" {
		t.Errorf("Hit on b's title = %v %+v, want b's title", ok, z)
	}
	if z, ok := f.Hit(20, 10); !ok || z.Kind != layout.RowSeam {
		t.Errorf("Hit on b's top border past its title = %v %+v, want the row seam", ok, z)
	}
	if z, ok := f.Hit(20, 5); !ok || z.Kind != layout.Body || z.ID != "a" {
		t.Errorf("Hit inside a = %v %+v, want a's body", ok, z)
	}
}

func TestDropAtPicksTheNearestSideOfTheWidgetUnderThePointer(t *testing.T) {
	f := layout.Compute(grid([]string{"a", "b"}), nil, 40, 10, layout.Pointer{})
	for _, c := range []struct {
		x, y int
		want layout.Dir
	}{{21, 5, layout.Left}, {38, 5, layout.Right}, {30, 0, layout.Up}, {30, 9, layout.Down}} {
		if d, ok := f.DropAt("a", c.x, c.y); !ok || d.Target != "b" || d.Side != c.want {
			t.Errorf("DropAt(%d,%d) = %v %+v, want %v of b", c.x, c.y, ok, d, c.want)
		}
	}
	if d, ok := f.DropAt("a", 5, 5); ok {
		t.Errorf("DropAt over the dragged Widget itself = %+v, want no drop", d)
	}
}

func TestSplitMovesASeamBetweenVisibleNeighboursOnly(t *testing.T) {
	l := grid([]string{"a", "h", "b", "c"}).SetHidden("h", true)
	f := layout.Compute(l, nil, 90, 10, layout.Pointer{})
	seam := layout.Zone{Kind: layout.WidgetSeam, Row: 0, Index: 0}

	got := l.Split(f, seam, 39)
	moved := layout.Compute(got, nil, 90, 10, layout.Pointer{})
	if b, _ := moved.Tile("b"); b.X != 40 {
		t.Errorf("b starts at %d after moving the a|b seam to column 39, want 40", b.X)
	}
	if c, _ := moved.Tile("c"); c.X != 60 || c.W != 30 {
		t.Errorf("c moved to %+v, want it untouched at 60, 30 wide", c.Rect)
	}
	if w := got.Rows[0].Widgets[1].Weight; w != 1 {
		t.Errorf("hidden h's weight changed to %v", w)
	}

	squeezed := layout.Compute(l.Split(f, seam, 2), nil, 90, 10, layout.Pointer{})
	if a, _ := squeezed.Tile("a"); a.W != layout.MinTile.W {
		t.Errorf("a is %d wide after a seam drag past its minimum, want %d", a.W, layout.MinTile.W)
	}
}
