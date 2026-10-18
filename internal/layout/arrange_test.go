package layout_test

import (
	"slices"
	"testing"

	"github.com/oronbz/ghud/internal/layout"
)

// grid builds a Layout of weight-1 rows and Widgets from ids per row.
func grid(rows ...[]string) layout.Layout {
	var l layout.Layout
	for _, ids := range rows {
		r := layout.Row{Weight: 1}
		for _, id := range ids {
			r.Widgets = append(r.Widgets, layout.Cell{ID: id, Weight: 1})
		}
		l.Rows = append(l.Rows, r)
	}
	return l
}

func shape(l layout.Layout) [][]string {
	var out [][]string
	for _, r := range l.Rows {
		var ids []string
		for _, c := range r.Widgets {
			ids = append(ids, c.ID)
		}
		out = append(out, ids)
	}
	return out
}

func sameShape(a, b [][]string) bool {
	return slices.EqualFunc(a, b, slices.Equal)
}

func TestMoveWithinARowAndAcrossRows(t *testing.T) {
	l := grid([]string{"a", "b", "c"}, []string{"d", "e"})
	for _, c := range []struct {
		name string
		id   string
		dir  layout.Dir
		want [][]string
	}{
		{"right swaps with the next Widget", "a", layout.Right, [][]string{{"b", "a", "c"}, {"d", "e"}}},
		{"left swaps with the previous Widget", "c", layout.Left, [][]string{{"a", "c", "b"}, {"d", "e"}}},
		{"left at the row start stays", "a", layout.Left, [][]string{{"a", "b", "c"}, {"d", "e"}}},
		{"right at the row end stays", "e", layout.Right, [][]string{{"a", "b", "c"}, {"d", "e"}}},
		{"down joins the next row at the same position", "b", layout.Down, [][]string{{"a", "c"}, {"d", "b", "e"}}},
		{"down past the row's length joins at its end", "c", layout.Down, [][]string{{"a", "b"}, {"d", "e", "c"}}},
		{"up joins the previous row", "e", layout.Up, [][]string{{"a", "e", "b", "c"}, {"d"}}},
		{"down past the last row opens a new row", "e", layout.Down, [][]string{{"a", "b", "c"}, {"d"}, {"e"}}},
		{"up past the first row opens a new row", "b", layout.Up, [][]string{{"b"}, {"a", "c"}, {"d", "e"}}},
		{"unknown id changes nothing", "z", layout.Down, [][]string{{"a", "b", "c"}, {"d", "e"}}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := shape(l.Move(c.id, c.dir)); !sameShape(got, c.want) {
				t.Errorf("Move(%s, %v) = %v, want %v", c.id, c.dir, got, c.want)
			}
		})
	}
	if got := shape(l); !sameShape(got, [][]string{{"a", "b", "c"}, {"d", "e"}}) {
		t.Errorf("Move changed the Layout it was called on: %v", got)
	}
}

func TestMovingTheLastWidgetOutOfARowRemovesTheRow(t *testing.T) {
	l := grid([]string{"a"}, []string{"b"}, []string{"c"})
	if got, want := shape(l.Move("b", layout.Up)), [][]string{{"b", "a"}, {"c"}}; !sameShape(got, want) {
		t.Errorf("got %v, want %v", got, want)
	}
	if got, want := shape(l.Move("c", layout.Down)), [][]string{{"a"}, {"b"}, {"c"}}; !sameShape(got, want) {
		t.Errorf("pushing a lone Widget past the edge gave %v, want no change %v", got, want)
	}
}

func TestResizeChangesTheWidgetsShareOfItsRowOrItsRowsShareOfTheHeight(t *testing.T) {
	l := grid([]string{"a", "b"}, []string{"c"})
	wide := func(l layout.Layout) float64 {
		return l.Rows[0].Widgets[0].Weight / (l.Rows[0].Widgets[0].Weight + l.Rows[0].Widgets[1].Weight)
	}
	tall := func(l layout.Layout) float64 { return l.Rows[0].Weight / (l.Rows[0].Weight + l.Rows[1].Weight) }
	near := func(a, b float64) bool { return a-b < 0.01 && b-a < 0.01 }

	if got := wide(l.Resize("a", layout.Right)); !near(got, 0.6) {
		t.Errorf("Right grows a to %.3f of its row, want 0.6", got)
	}
	if got := wide(l.Resize("a", layout.Left)); !near(got, 0.4) {
		t.Errorf("Left shrinks a to %.3f of its row, want 0.4", got)
	}
	if got := tall(l.Resize("c", layout.Down)); !near(got, 0.4) {
		t.Errorf("Down grows c's row to 0.6, leaving the first row %.3f of the height, want 0.4", got)
	}
	if got := tall(l.Resize("c", layout.Up)); !near(got, 0.6) {
		t.Errorf("Up shrinks c's row to 0.4, leaving the first row %.3f of the height, want 0.6", got)
	}
	if got := l.Resize("c", layout.Right); got.Rows[1].Widgets[0].Weight != 1 {
		t.Errorf("widening a Widget alone in its row changed its weight to %v", got.Rows[1].Widgets[0].Weight)
	}
	if l.Rows[0].Widgets[0].Weight != 1 || l.Rows[0].Weight != 1 {
		t.Error("Resize changed the Layout it was called on")
	}

	squeezed := l
	for range 20 {
		squeezed = squeezed.Resize("a", layout.Left)
	}
	if got := wide(squeezed); got <= 0 {
		t.Errorf("shrinking repeatedly left a with share %.3f, want it kept above zero", got)
	}
}

func TestResizeSharesOnlyAmongVisibleWidgets(t *testing.T) {
	l := grid([]string{"a", "h", "b"})
	l.Hidden = []string{"h"}
	got := l.Resize("a", layout.Right)
	ws := got.Rows[0].Widgets
	if share := ws[0].Weight / (ws[0].Weight + ws[2].Weight); share < 0.59 || share > 0.61 {
		t.Errorf("a's share of the visible row %.3f, want 0.6", share)
	}
	if ws[1].Weight != 1 {
		t.Errorf("hidden Widget's weight changed to %v", ws[1].Weight)
	}
}

func TestHiddenWidgetsKeepTheirPlaceButAreNotLaidOut(t *testing.T) {
	l := grid([]string{"a", "b"}, []string{"c"}).SetHidden("b", true).SetHidden("c", true).SetHidden("c", true)
	if !slices.Equal(l.Hidden, []string{"b", "c"}) {
		t.Fatalf("Hidden = %v, want [b c]", l.Hidden)
	}
	f := layout.Compute(l, nil, 40, 10)
	if len(f.Tiles) != 1 || f.Tiles[0].ID != "a" || f.Tiles[0].Rect != (layout.Rect{X: 0, Y: 0, W: 40, H: 10}) {
		t.Errorf("tiles %+v, want a alone filling the screen", f.Tiles)
	}
	shown := l.SetHidden("c", false)
	if !slices.Equal(shown.Hidden, []string{"b"}) {
		t.Errorf("showing c leaves Hidden = %v, want [b]", shown.Hidden)
	}
	if got := layout.Compute(shown, nil, 40, 10).Tiles; len(got) != 2 || got[1].ID != "c" || got[1].Y != 5 {
		t.Errorf("tiles %+v, want c back in its own row below a", got)
	}
}

func TestMovesStepOverHiddenWidgetsAndRows(t *testing.T) {
	l := grid([]string{"a", "h", "b"}, []string{"g"}, []string{"c"})
	l.Hidden = []string{"h", "g"}
	if got, want := shape(l.Move("a", layout.Right)), [][]string{{"b", "h", "a"}, {"g"}, {"c"}}; !sameShape(got, want) {
		t.Errorf("Right over a hidden Widget gave %v, want %v", got, want)
	}
	if got, want := shape(l.Move("c", layout.Up)), [][]string{{"c", "a", "h", "b"}, {"g"}}; !sameShape(got, want) {
		t.Errorf("Up over a hidden row gave %v, want %v", got, want)
	}
}

func TestOpenRowGetsTheWeightOfTheRowItLeft(t *testing.T) {
	l := grid([]string{"a", "b"}, []string{"c"})
	l.Rows[0].Weight = 7
	got := l.Move("b", layout.Up)
	if got.Rows[0].Weight != 7 || got.Rows[1].Weight != 7 {
		t.Errorf("row weights %v, %v, want both 7", got.Rows[0].Weight, got.Rows[1].Weight)
	}
}
