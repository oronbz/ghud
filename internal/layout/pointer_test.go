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
	}{{21, 5, layout.Left}, {37, 5, layout.Right}, {30, 1, layout.Up}, {30, 8, layout.Down}} {
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
	seam := layout.Zone{Kind: layout.WidgetSeam, Path: layout.Path{0}, Index: 0}

	got := l.Split(f, seam, 39)
	moved := layout.Compute(got, nil, 90, 10, layout.Pointer{})
	if b, _ := moved.Tile("b"); b.X != 40 {
		t.Errorf("b starts at %d after moving the a|b seam to column 39, want 40", b.X)
	}
	if c, _ := moved.Tile("c"); c.X != 60 || c.W != 30 {
		t.Errorf("c moved to %+v, want it untouched at 60, 30 wide", c.Rect)
	}
	if w := got.Rows[0].Cells[1].Weight; w != 1 {
		t.Errorf("hidden h's weight changed to %v", w)
	}

	squeezed := layout.Compute(l.Split(f, seam, 2), nil, 90, 10, layout.Pointer{})
	if a, _ := squeezed.Tile("a"); a.W != layout.MinTile.W {
		t.Errorf("a is %d wide after a seam drag past its minimum, want %d", a.W, layout.MinTile.W)
	}
}

func TestASeamDragLeavesNoWeightUnder5PercentOfItsSiblings(t *testing.T) {
	for _, c := range []struct {
		name string
		l    layout.Layout
		seam layout.Zone
		w, h int
	}{
		{"row seam", parse("a / b"), layout.Zone{Kind: layout.RowSeam, Path: layout.Path{}, Index: 0}, 40, 400},
		{"widget seam", parse("a b"), layout.Zone{Kind: layout.WidgetSeam, Path: layout.Path{0}, Index: 0}, 800, 10},
	} {
		f := layout.Compute(c.l, nil, c.w, c.h, layout.Pointer{})
		for _, at := range []int{0, c.w + c.h} {
			got := c.l.Split(f, c.seam, at)
			var ws []float64
			if c.seam.Kind == layout.RowSeam {
				ws = []float64{got.Rows[0].Weight, got.Rows[1].Weight}
			} else {
				ws = []float64{got.Rows[0].Cells[0].Weight, got.Rows[0].Cells[1].Weight}
			}
			if sum := ws[0] + ws[1]; sum != 2 || min(ws[0], ws[1]) < 0.1 {
				t.Errorf("%s dragged to %d gives weights %v, want each at least 5%% of their 2", c.name, at, ws)
			}
		}
	}
}

func TestDropsOnATilesSidesJoinItsRowOrStackWithIt(t *testing.T) {
	for _, c := range []struct {
		start, id, target string
		side              layout.Dir
		want              string
	}{
		{"a b c / d", "d", "b", layout.Left, "a d b c"},
		{"a b c / d", "d", "b", layout.Right, "a b d c"},
		{"a b c / d", "d", "b", layout.Up, "a [d / b] c"},
		{"a b c / d", "d", "b", layout.Down, "a [b / d] c"},
		{"a / b c", "c", "a", layout.Down, "a / c / b"},
		{"p [s / a] / d", "d", "s", layout.Down, "p [s / d / a]"},
	} {
		got := parse(c.start).Drop(c.id, layout.Drop{Kind: layout.OnTile, Target: c.target, Side: c.side})
		if shape(got) != c.want {
			t.Errorf("from %q dropping %s on the %v of %s gives %q, want %q", c.start, c.id, c.side, c.target, shape(got), c.want)
		}
	}
	halved := parse("a b / d").Drop("d", layout.Drop{Kind: layout.OnTile, Target: "b", Side: layout.Left})
	if ws := halved.Rows[0].Cells; ws[1].Weight != 0.5 || ws[2].Weight != 0.5 {
		t.Errorf("a side drop gives weights %v %v, want b's 1 halved between them", ws[1].Weight, ws[2].Weight)
	}
}

func TestARowSeamDropOpensARowAcrossItsContainer(t *testing.T) {
	l := layout.Layout{Rows: []layout.Row{
		row(1, w("p", 3), stack(7, row(1, w("s", 1)), row(1, w("a", 1)))),
		row(1, w("d", 1)),
	}}
	f := layout.Compute(l, nil, 100, 40, layout.Pointer{})
	inStack, ok := f.DropAt("d", 50, 10)
	if !ok || inStack.Kind != layout.OnSeam || inStack.Bar != (layout.Rect{X: 30, Y: 10, W: 70, H: 1}) {
		t.Fatalf("DropAt the stack's seam = %v %+v, want a seam drop with a bar across the stack", ok, inStack)
	}
	if got, want := shape(l.Drop("d", inStack)), "p [s / d / a]"; got != want {
		t.Errorf("dropping on the stack's seam gives %q, want %q", got, want)
	}
	across, ok := f.DropAt("s", 50, 20)
	if !ok || across.Kind != layout.OnSeam || across.Bar.W != 100 {
		t.Fatalf("DropAt the dashboard's row seam = %v %+v, want a seam drop across the dashboard", ok, across)
	}
	if got, want := shape(l.Drop("s", across)), "p a / s / d"; got != want {
		t.Errorf("dropping on the dashboard's seam gives %q, want %q", got, want)
	}
	for _, c := range []struct {
		y    int
		want string
	}{{0, "s / p a / d"}, {39, "p a / d / s"}} {
		edge, ok := f.DropAt("s", 50, c.y)
		if !ok || edge.Kind != layout.OnEdge || edge.Bar.W != 100 {
			t.Fatalf("DropAt row %d = %v %+v, want the dashboard's edge", c.y, ok, edge)
		}
		if got := shape(l.Drop("s", edge)); got != c.want {
			t.Errorf("dropping on row %d gives %q, want a full-width row: %q", c.y, got, c.want)
		}
	}
}

func TestEverySeamDragsOnlyItsTwoSiblings(t *testing.T) {
	l := layout.Layout{Rows: []layout.Row{
		row(1, w("p", 3), stack(7, row(1, w("s", 1)), row(1, w("a", 1), w("b", 1)), row(1, w("c", 1)))),
	}}
	f := layout.Compute(l, nil, 100, 30, layout.Pointer{})
	var inner, rows layout.Zone
	for _, z := range f.Zones {
		if z.Kind == layout.WidgetSeam && len(z.Path) == 3 {
			inner = z
		}
		if z.Kind == layout.RowSeam && z.Index == 0 {
			rows = z
		}
	}
	got := rects(layout.Compute(l.Split(f, inner, 50), nil, 100, 30, layout.Pointer{}))
	if got["a"].W != 21 || got["b"].X != 51 || got["s"] != rects(f)["s"] || got["p"] != rects(f)["p"] {
		t.Errorf("dragging a|b to 50 gives %+v, want a to end at 50 and nothing else moved", got)
	}
	got = rects(layout.Compute(l.Split(f, rows, 13), nil, 100, 30, layout.Pointer{}))
	if got["s"].H != 14 || got["a"].Y != 14 || got["c"] != rects(f)["c"] || got["p"] != rects(f)["p"] {
		t.Errorf("dragging s's seam to 13 gives %+v, want s 14 tall and c, p untouched", got)
	}
}

func TestAHoveredSeamInAStackLightsOnlyItsOwnBorders(t *testing.T) {
	f := layout.Compute(column(), nil, 100, 20, layout.Pointer{})
	var seam layout.Zone
	for _, z := range f.Zones {
		if z.Kind == layout.RowSeam {
			seam = z
		}
	}
	lit := layout.Compute(column(), nil, 100, 20, layout.Pointer{Seam: &seam}).Tint
	want := []layout.Rect{{X: 31, Y: 9, W: 68, H: 1}, {X: 31, Y: 10, W: 68, H: 1}}
	if len(lit) != 2 || lit[0] != want[0] || lit[1] != want[1] {
		t.Errorf("tint %+v, want spotify's bottom and agents' top borders %+v", lit, want)
	}
}
