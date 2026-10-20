package layout_test

import (
	"testing"

	"github.com/oronbz/visor/internal/layout"
)

func w(id string, weight float64) layout.Cell { return layout.Cell{ID: id, Weight: weight} }

func row(weight float64, cells ...layout.Cell) layout.Row {
	return layout.Row{Weight: weight, Cells: cells}
}

func stack(weight float64, rows ...layout.Row) layout.Cell {
	return layout.Cell{Weight: weight, Rows: rows}
}

// column is priorities as a full-height column beside a stack of spotify
// over agents.
func column() layout.Layout {
	return layout.Layout{Rows: []layout.Row{
		row(1, w("priorities", 3), stack(7, row(1, w("spotify", 1)), row(1, w("agents", 1)))),
	}}
}

func rects(f layout.Frame) map[string]layout.Rect {
	out := map[string]layout.Rect{}
	for _, t := range f.Tiles {
		out[t.ID] = t.Rect
	}
	return out
}

func TestAStackSharesItsCellsHeightAmongItsRows(t *testing.T) {
	got := rects(layout.Compute(column(), nil, 100, 20, layout.Pointer{}))
	want := map[string]layout.Rect{
		"priorities": {X: 0, Y: 0, W: 30, H: 20},
		"spotify":    {X: 30, Y: 0, W: 70, H: 10},
		"agents":     {X: 30, Y: 10, W: 70, H: 10},
	}
	for id, r := range want {
		if got[id] != r {
			t.Errorf("%s at %+v, want %+v", id, got[id], r)
		}
	}
}

func TestTilesComeInReadingOrderThroughStacks(t *testing.T) {
	l := layout.Layout{Rows: []layout.Row{
		row(1, w("a", 1), stack(1, row(1, w("b", 1)), row(1, w("c", 1))), w("d", 1)),
		row(1, w("e", 1)),
	}}
	var ids []string
	for _, t := range layout.Compute(l, nil, 90, 30, layout.Pointer{}).Tiles {
		ids = append(ids, t.ID)
	}
	if got, want := ids, []string{"a", "b", "c", "d", "e"}; !equal(got, want) {
		t.Errorf("tiles %v, want %v", got, want)
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestTheMinimumSizeCountsEveryRowOfAStack(t *testing.T) {
	l := layout.Layout{Rows: []layout.Row{
		row(1, w("p", 1), stack(1, row(1, w("a", 1)), row(1, w("b", 1), w("c", 1)), row(1, w("d", 1)))),
		row(1, w("e", 1)),
	}}
	f := layout.Compute(l, nil, 47, 20, layout.Pointer{})
	if want := (layout.Size{W: 3 * layout.MinTile.W, H: 4 * layout.MinTile.H}); f.Need != want {
		t.Errorf("Need %+v, want %+v", f.Need, want)
	}
	if !f.TooSmall {
		t.Error("47×20 fits a Layout needing 48×20")
	}
	if layout.Compute(l, nil, 48, 20, layout.Pointer{}).TooSmall {
		t.Error("48×20 is too small for a Layout needing 48×20")
	}
}

func TestEverySeamAtEveryDepthIsAZone(t *testing.T) {
	f := layout.Compute(column(), nil, 100, 20, layout.Pointer{})
	var seams []layout.Zone
	for _, z := range f.Zones {
		if z.Kind == layout.WidgetSeam || z.Kind == layout.RowSeam {
			seams = append(seams, z)
		}
	}
	if len(seams) != 2 {
		t.Fatalf("seams %+v, want the column's and the stack's", seams)
	}
	col, rows := seams[0], seams[1]
	if col.Kind != layout.WidgetSeam || col.Rect != (layout.Rect{X: 29, Y: 1, W: 2, H: 18}) {
		t.Errorf("column seam %+v, want the facing borders at 29–30", col)
	}
	if rows.Kind != layout.RowSeam || rows.Rect != (layout.Rect{X: 30, Y: 9, W: 70, H: 2}) {
		t.Errorf("stack's row seam %+v, want across the stack only at 9–10", rows)
	}
	if z, ok := f.Hit(50, 10); !ok || z.Kind != layout.RowSeam {
		t.Errorf("Hit on the stack's seam = %v %+v, want the row seam", ok, z)
	}
}

func TestHidingEveryWidgetInAStackHidesTheStack(t *testing.T) {
	l := column().SetHidden("agents", true)
	if got := rects(layout.Compute(l, nil, 100, 20, layout.Pointer{}))["spotify"]; got != (layout.Rect{X: 30, Y: 0, W: 70, H: 20}) {
		t.Errorf("spotify alone in its stack at %+v, want the stack's whole cell", got)
	}
	l = l.SetHidden("spotify", true)
	if got := rects(layout.Compute(l, nil, 100, 20, layout.Pointer{})); len(got) != 1 || got["priorities"] != (layout.Rect{X: 0, Y: 0, W: 100, H: 20}) {
		t.Errorf("tiles %+v, want priorities alone filling the screen", got)
	}
	l = l.SetHidden("agents", false).SetHidden("spotify", false)
	if got := rects(layout.Compute(l, nil, 100, 20, layout.Pointer{}))["agents"]; got != (layout.Rect{X: 30, Y: 10, W: 70, H: 10}) {
		t.Errorf("agents shown again at %+v, want back in its place in the stack", got)
	}
}
