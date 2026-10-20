package layout_test

import (
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/layout"
)

// grid builds a Layout of weight-1 rows and Widgets from ids per row.
func grid(rows ...[]string) layout.Layout {
	var l layout.Layout
	for _, ids := range rows {
		r := layout.Row{Weight: 1}
		for _, id := range ids {
			r.Cells = append(r.Cells, layout.Cell{ID: id, Weight: 1})
		}
		l.Rows = append(l.Rows, r)
	}
	return l
}

// shape writes l as rows split by " / ", cells by spaces and a stack's rows
// in brackets: "p [s / a]" is p beside a stack of s over a.
func shape(l layout.Layout) string { return rowsShape(l.Rows) }

func rowsShape(rows []layout.Row) string {
	var out []string
	for _, r := range rows {
		var cells []string
		for _, c := range r.Cells {
			if c.IsStack() {
				cells = append(cells, "["+rowsShape(c.Rows)+"]")
			} else {
				cells = append(cells, c.ID)
			}
		}
		out = append(out, strings.Join(cells, " "))
	}
	return strings.Join(out, " / ")
}

// parse is shape's inverse, every weight 1.
func parse(s string) layout.Layout {
	rows, _ := parseRows(strings.Fields(strings.NewReplacer("[", " [ ", "]", " ] ").Replace(s)), 0)
	return layout.Layout{Rows: rows}
}

func parseRows(tok []string, i int) ([]layout.Row, int) {
	rows := []layout.Row{{Weight: 1}}
	for i < len(tok) {
		last := &rows[len(rows)-1]
		switch t := tok[i]; t {
		case "]":
			return rows, i + 1
		case "/":
			rows = append(rows, layout.Row{Weight: 1})
			i++
		case "[":
			var inner []layout.Row
			inner, i = parseRows(tok, i+1)
			last.Cells = append(last.Cells, layout.Cell{Weight: 1, Rows: inner})
		default:
			last.Cells = append(last.Cells, layout.Cell{ID: t, Weight: 1})
			i++
		}
	}
	return rows, i
}

var area = layout.Size{W: 120, H: 40}

var left, right, up, down = layout.Left, layout.Right, layout.Up, layout.Down

// presses moves id by each of dirs as arrange mode does, remembering what
// each press went into, and returns the shape after every press.
func presses(t *testing.T, l layout.Layout, id string, dirs ...layout.Dir) []string {
	t.Helper()
	var via layout.Via
	var out []string
	for _, d := range dirs {
		l, via = l.Move(id, d, area, via)
		if err := normal(l); err != "" {
			t.Errorf("%s %v leaves %q with %s", id, d, shape(l), err)
		}
		out = append(out, shape(l))
	}
	return out
}

func TestEachArrowPressMovesOnePositionIntoTheNextWidgetThenPastIt(t *testing.T) {
	for _, c := range []struct {
		name  string
		start string
		id    string
		dirs  []layout.Dir
		want  []string
	}{
		{"→ goes into the row neighbour's cell above it, → again past it", "a b c / d e", "a",
			[]layout.Dir{right, right}, []string{"[a / b] c / d e", "b a c / d e"}},
		{"← undoes each →", "a b c / d e", "a",
			[]layout.Dir{right, left, right, right, left, left},
			[]string{"[a / b] c / d e", "a b c / d e", "[a / b] c / d e", "b a c / d e", "[a / b] c / d e", "a b c / d e"}},
		{"↓ joins the row of a Widget alone below, beside it, and ↓ again goes below it", "a / b", "a",
			[]layout.Dir{down, down}, []string{"a b", "b / a"}},
		{"↑ undoes each ↓", "a / b", "a",
			[]layout.Dir{down, up, down, down, up, up}, []string{"a b", "a / b", "a b", "b / a", "a b", "a / b"}},
		{"↓↓ swaps Widgets stacked in a cell, ↑↑ back", "p [s / a]", "s",
			[]layout.Dir{down, down, up, up}, []string{"p s a", "p [a / s]", "p s a", "p [s / a]"}},
		{"←← takes a Widget from beside a full-height column into it, →→ back", "p [m / s]", "m",
			[]layout.Dir{left, left, right, right}, []string{"p m s", "[m / p] s", "p m s", "p [m / s]"}},
		{"the first ↑ from a shared row is a row of its own, then into the next row, then past", "a b c / d e", "d",
			[]layout.Dir{up, up, up}, []string{"a b c / d / e", "a d b c / e", "a [d / b] c / e"}},
		{"→ into a stack is a row of its own at the Widget's height", "m [s / a]", "m",
			[]layout.Dir{right, right}, []string{"s / m / a", "[s / a] m"}},
		{"↑ at a stack's top is a row of its own above the stack's row", "p [s / a] / z", "s",
			[]layout.Dir{up}, []string{"s / p a / z"}},
		{"↓ alone in the last row stays", "a b / c", "c",
			[]layout.Dir{down}, []string{"a b / c"}},
		{"↓ sharing the last row opens a full-width row below", "a b / c", "b",
			[]layout.Dir{down}, []string{"a / b / c"}},
		{"↑ sharing the first row opens a full-width row above", "a b / c", "b",
			[]layout.Dir{up}, []string{"b / a / c"}},
		{"↓ into a stack below is its first row", "c / p [s / a]", "c",
			[]layout.Dir{down}, []string{"p [c / s / a]"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := presses(t, parse(c.start), c.id, c.dirs...); !slices.Equal(got, c.want) {
				t.Errorf("from %q moving %s %v:\ngot  %q\nwant %q", c.start, c.id, c.dirs, got, c.want)
			}
		})
	}
}

func TestTheOppositeArrowUndoesAPressExactly(t *testing.T) {
	for _, c := range []struct {
		start, id string
		dirs      []layout.Dir
	}{
		{"a b c", "a", []layout.Dir{down, up}},
		{"a b c", "b", []layout.Dir{up, down}},
		{"a b c / d e", "a", []layout.Dir{right, down, up, left}},
		{"a b c / d e", "d", []layout.Dir{up, up, up, down, down, down}},
		{"p [s / a]", "s", []layout.Dir{left, down, up, right}},
	} {
		l := parse(c.start)
		l.Rows[0].Cells[0].Weight = 3
		got := l
		var via layout.Via
		for _, d := range c.dirs {
			got, via = got.Move(c.id, d, area, via)
		}
		if !reflect.DeepEqual(got, l) {
			t.Errorf("%s %v from %q gives %q %+v, want it back as it was %+v", c.id, c.dirs, c.start, shape(got), got, l)
		}
	}
}

func TestUndoingAPressStillLetsTheNextOneGoPast(t *testing.T) {
	if got := presses(t, parse("a / b c"), "a", down, down, up, down); got[3] != "b [c / a]" {
		t.Errorf("↓ ↓ ↑ ↓ gives %q, want ↓ again past c after undoing the step past it", got[3])
	}
}

func TestADropAResizeOrASeamDragForgetsWhatTheLastPressWentInto(t *testing.T) {
	start := parse("a / b c / d")
	moved, via := start.Move("a", down, area, layout.Via{})
	if shape(moved) != "b a c / d" || via.ID != "c" {
		t.Fatalf("↓ gives %q into %q, want a into c's row", shape(moved), via.ID)
	}
	f := layout.Compute(moved, nil, area.W, area.H, layout.Pointer{})
	for _, c := range []struct {
		name string
		l    layout.Layout
		want string
	}{
		{"a drop", moved.Drop("d", layout.Drop{Kind: layout.OnEdge, Side: up}), "d / b c / a"},
		{"a resize", moved.Resize("d", down), "b c / a / d"},
		{"a seam drag", moved.Split(f, layout.Zone{Kind: layout.RowSeam, Path: layout.Path{}, Index: 0}, 25), "b c / a / d"},
	} {
		if got, _ := c.l.Move("a", down, area, via); shape(got) != c.want {
			t.Errorf("after %s ↓ gives %q, want %q as if nothing was remembered", c.name, shape(got), c.want)
		}
		if got, _ := c.l.Move("a", up, area, via); reflect.DeepEqual(got, start) {
			t.Errorf("after %s ↑ undid the ↓ from before it", c.name)
		}
	}
}

func TestASwapWithAnyNeighbourTakesTwoPresses(t *testing.T) {
	for _, c := range []struct {
		start, id string
		dir       layout.Dir
		want      string
	}{
		{"a b", "a", right, "b a"},
		{"a b", "b", left, "b a"},
		{"a / b", "a", down, "b / a"},
		{"a / b", "b", up, "b / a"},
		{"p [s / a]", "s", down, "p [a / s]"},
		{"p [s / a]", "a", up, "p [a / s]"},
		{"p [s / a]", "s", left, "[s / p] a"},
	} {
		if got := presses(t, parse(c.start), c.id, c.dir, c.dir); got[1] != c.want {
			t.Errorf("%s %v twice from %q gives %q, want %q", c.id, c.dir, c.start, got[1], c.want)
		}
	}
}

func TestPastTheDashboardsSideEdgeIsAFullHeightColumnLikeAnEdgeDrop(t *testing.T) {
	l := parse("a b / c d / e")
	f := layout.Compute(l, nil, area.W, area.H, layout.Pointer{})
	drop, ok := f.DropAt("a", 0, 20)
	if !ok || drop.Kind != layout.OnEdge || drop.Side != layout.Left {
		t.Fatalf("DropAt the left edge = %v %+v, want the dashboard's left edge", ok, drop)
	}
	if drop.Bar != (layout.Rect{X: 0, Y: 0, W: 1, H: area.H}) {
		t.Errorf("edge drop bar %+v, want the dashboard's whole left edge", drop.Bar)
	}
	dropped := l.Drop("a", drop)
	if got, want := shape(dropped), "a [b / c d / e]"; got != want {
		t.Errorf("left edge drop gives %q, want %q", got, want)
	}
	cells := dropped.Rows[0].Cells
	if share := cells[0].Weight / (cells[0].Weight + cells[1].Weight); share < 0.49 || share > 0.51 {
		t.Errorf("the column has %.3f of the width, want the half a had", share)
	}
	keyed, _ := l.Move("a", left, area, layout.Via{})
	if !reflect.DeepEqual(keyed, dropped) {
		t.Errorf("← past the left edge gives %q %+v, want the same as the edge drop", shape(keyed), keyed)
	}
	if got := presses(t, l, "d", right); got[0] != "[a b / c / e] d" {
		t.Errorf("→ past the right edge gives %q, want d a full-height column on the right", got[0])
	}
}

func TestAColumnKeepsItsShareBetween15And60Percent(t *testing.T) {
	for _, c := range []struct {
		start string
		want  float64
	}{{"a b c d e f g h", 0.15}, {"a / b", 0.6}, {"a b c", 1.0 / 3}} {
		f := layout.Compute(parse(c.start), nil, 160, 20, layout.Pointer{})
		drop, _ := f.DropAt("a", 0, 10)
		if drop.Share < c.want-0.01 || drop.Share > c.want+0.01 {
			t.Errorf("from %q a column gets share %.3f, want %.3f", c.start, drop.Share, c.want)
		}
	}
}

func TestEveryOperationLeavesTheLayoutNormalised(t *testing.T) {
	starts := []string{"a b c / d e", "p [s / a b / m]", "[a / b] c / d [e / f]", "x / p [s / q [r / t] / a]"}
	for _, start := range starts {
		l := parse(start)
		if err := normal(l); err != "" {
			t.Fatalf("start %q isn't normal: %s", start, err)
		}
		f := layout.Compute(l, nil, area.W, area.H, layout.Pointer{})
		for _, id := range l.IDs() {
			for _, d := range []layout.Dir{left, right, up, down} {
				once, via := l.Move(id, d, area, layout.Via{})
				twice, _ := once.Move(id, d, area, via)
				for _, got := range []layout.Layout{once, twice} {
					if err := normal(got); err != "" {
						t.Errorf("%q: %s %v gives %q with %s", start, id, d, shape(got), err)
					}
					if n := len(got.IDs()); n != len(l.IDs()) {
						t.Errorf("%q: %s %v gives %q, %d Widgets, want %d", start, id, d, shape(got), n, len(l.IDs()))
					}
				}
				for _, target := range l.IDs() {
					got := l.Drop(id, layout.Drop{Target: target, Side: d})
					if err := normal(got); err != "" {
						t.Errorf("%q: dropping %s %v of %s gives %q with %s", start, id, d, target, shape(got), err)
					}
				}
			}
			for x := 0; x < area.W; x += 3 {
				for y := 0; y < area.H; y += 2 {
					if drop, ok := f.DropAt(id, x, y); ok {
						if got := l.Drop(id, drop); normal(got) != "" {
							t.Errorf("%q: dropping %s at %d,%d gives %q with %s", start, id, x, y, shape(got), normal(got))
						}
					}
				}
			}
		}
	}
}

// normal describes how l breaks the normal form, or is "".
func normal(l layout.Layout) string {
	var check func(rows []layout.Row, top bool) string
	check = func(rows []layout.Row, top bool) string {
		if !top && len(rows) < 2 {
			return "a stack of fewer than two rows"
		}
		for _, r := range rows {
			if len(r.Cells) == 0 {
				return "an empty row"
			}
			if len(r.Cells) == 1 && r.Cells[0].IsStack() {
				return "a row holding only a stack"
			}
			sum := 0.0
			for _, c := range r.Cells {
				sum += c.Weight
			}
			for _, c := range r.Cells {
				if !c.IsStack() && c.ID == "" {
					return "an empty cell"
				}
				if c.Weight < sum*0.05-0.001 {
					return "a weight under 5% of its row"
				}
				if c.IsStack() {
					if err := check(c.Rows, false); err != "" {
						return err
					}
				}
			}
		}
		return ""
	}
	return check(l.Rows, true)
}

func TestResizeChangesTheWidgetsShareOfItsRowOrItsRowsShareOfTheHeight(t *testing.T) {
	l := grid([]string{"a", "b"}, []string{"c"})
	wide := func(l layout.Layout) float64 {
		return l.Rows[0].Cells[0].Weight / (l.Rows[0].Cells[0].Weight + l.Rows[0].Cells[1].Weight)
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
	if got := l.Resize("c", layout.Right); got.Rows[1].Cells[0].Weight != 1 {
		t.Errorf("widening a Widget alone in its row changed its weight to %v", got.Rows[1].Cells[0].Weight)
	}
	if l.Rows[0].Cells[0].Weight != 1 || l.Rows[0].Weight != 1 {
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

func TestResizeUsesTheNearestContainerWithASiblingThatWay(t *testing.T) {
	l := column()
	share := func(a, b float64) float64 { return a / (a + b) }
	if s := l.Resize("agents", layout.Right).Rows[0].Cells; share(s[1].Weight, s[0].Weight) < 0.79 {
		t.Errorf("→ on agents, alone in its row, gives its stack %.3f of the width, want 0.8", share(s[1].Weight, s[0].Weight))
	}
	if rs := l.Resize("agents", layout.Down).Rows[0].Cells[1].Rows; share(rs[1].Weight, rs[0].Weight) < 0.59 {
		t.Errorf("↓ on agents gives it %.3f of the stack's height, want 0.6", share(rs[1].Weight, rs[0].Weight))
	}
	if same := l.Resize("priorities", layout.Down); !reflect.DeepEqual(same, l) {
		t.Errorf("↓ on a full-height column changed the Layout to %+v", same)
	}
}

func TestResizeSharesOnlyAmongVisibleWidgets(t *testing.T) {
	l := grid([]string{"a", "h", "b"})
	l.Hidden = []string{"h"}
	got := l.Resize("a", layout.Right)
	ws := got.Rows[0].Cells
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
	f := layout.Compute(l, nil, 40, 10, layout.Pointer{})
	if len(f.Tiles) != 1 || f.Tiles[0].ID != "a" || f.Tiles[0].Rect != (layout.Rect{X: 0, Y: 0, W: 40, H: 10}) {
		t.Errorf("tiles %+v, want a alone filling the screen", f.Tiles)
	}
	shown := l.SetHidden("c", false)
	if !slices.Equal(shown.Hidden, []string{"b"}) {
		t.Errorf("showing c leaves Hidden = %v, want [b]", shown.Hidden)
	}
	if got := layout.Compute(shown, nil, 40, 10, layout.Pointer{}).Tiles; len(got) != 2 || got[1].ID != "c" || got[1].Y != 5 {
		t.Errorf("tiles %+v, want c back in its own row below a", got)
	}
}

func TestMovesStepOverHiddenWidgetsAndRows(t *testing.T) {
	l := parse("a h b / g / c")
	l.Hidden = []string{"h", "g"}
	if got := presses(t, l, "a", right); got[0] != "h [a / b] / g / c" {
		t.Errorf("→ over a hidden Widget gives %q, want into b's cell", got[0])
	}
	if got := presses(t, l, "c", up); got[0] != "a h c b / g" {
		t.Errorf("↑ over a hidden row gives %q, want into the row above it", got[0])
	}
}

func TestJoinDropsWidgetsThatAreGoneFromStacksAndUnwindsWhatThatLeaves(t *testing.T) {
	l := layout.Layout{Rows: []layout.Row{
		row(1, w("p", 3), stack(7, row(1, w("s", 1)), row(1, w("gone", 1)))),
		row(1, w("x", 1)),
	}, Hidden: []string{"gone", "s"}}
	got := l.Join([]string{"p", "s", "x", "new"})
	if want := "p s / x / new"; shape(got) != want {
		t.Errorf("Join gives %q, want %q", shape(got), want)
	}
	if ws := got.Rows[0].Cells; ws[0].Weight != 3 || ws[1].Weight != 7 {
		t.Errorf("weights %v %v, want s to take its stack's 7 beside p's 3", ws[0].Weight, ws[1].Weight)
	}
	if !slices.Equal(got.Hidden, []string{"s"}) {
		t.Errorf("Hidden %v, want [s]", got.Hidden)
	}
	if none := l.Join(nil); none.Rows != nil {
		t.Errorf("Join with no Widgets gives rows %+v, want none", none.Rows)
	}
}

func TestNewRowsAreAsTallAsTheirContainersRowsOnAverage(t *testing.T) {
	l := parse("a b / c")
	l.Rows[0].Weight, l.Rows[1].Weight = 7, 3
	got, _ := l.Move("b", layout.Up, area, layout.Via{})
	if got.Rows[0].Weight != 5 || got.Rows[1].Weight != 7 {
		t.Errorf("row weights %v, %v, want the new row at the mean 5 above a's 7", got.Rows[0].Weight, got.Rows[1].Weight)
	}
}
