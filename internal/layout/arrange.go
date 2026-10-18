package layout

import (
	"math"
	"slices"
)

// Dir is a direction for keyboard moves and resizes.
type Dir int

const (
	Left Dir = iota
	Right
	Up
	Down
)

func (d Dir) String() string {
	return [...]string{"left", "right", "up", "down"}[d]
}

// Find is the row and index of id in l.Rows, or -1, -1.
func (l Layout) Find(id string) (row, index int) {
	for ri, r := range l.Rows {
		for ci, c := range r.Widgets {
			if c.ID == id {
				return ri, ci
			}
		}
	}
	return -1, -1
}

// Move is the keyboard move: left and right swap id with its visible
// neighbour, up and down join the adjacent visible row at the same visible
// position, and pushing past the top or bottom edge opens a new row.
func (l Layout) Move(id string, d Dir) Layout {
	ri, ci := l.Find(id)
	if ri < 0 || l.IsHidden(id) {
		return l.clone()
	}
	step := 1
	if d == Left || d == Up {
		step = -1
	}
	vis := l.visibleIn(ri)
	k := slices.Index(vis, ci)
	if d == Left || d == Right {
		out := l.clone()
		if j := k + step; j >= 0 && j < len(vis) {
			ws := out.Rows[ri].Widgets
			ws[ci], ws[vis[j]] = ws[vis[j]], ws[ci]
		}
		return out
	}
	t := ri + step
	for t >= 0 && t < len(l.Rows) && len(l.visibleIn(t)) == 0 {
		t += step
	}
	if t < 0 || t >= len(l.Rows) {
		if d == Up {
			return l.OpenRow(id, ri)
		}
		return l.OpenRow(id, ri+1)
	}
	tv := l.visibleIn(t)
	if k < len(tv) {
		return l.JoinRow(id, t, tv[k])
	}
	return l.JoinRow(id, t, tv[len(tv)-1]+1)
}

// ResizeStep is how much of its row (or of the height) one keyboard resize
// gives or takes.
const ResizeStep = 0.1

// Resize is the keyboard resize: Right and Left widen and narrow id within
// its row, Down and Up make its row taller and shorter. Only visible Widgets
// and rows share the space; the others keep their proportions.
func (l Layout) Resize(id string, d Dir) Layout {
	out := l.clone()
	ri, ci := out.Find(id)
	if ri < 0 || out.IsHidden(id) {
		return out
	}
	delta := ResizeStep
	if d == Left || d == Up {
		delta = -delta
	}
	if d == Left || d == Right {
		vis := out.visibleIn(ri)
		ws := make([]*float64, len(vis))
		for i, j := range vis {
			ws[i] = &out.Rows[ri].Widgets[j].Weight
		}
		reshare(ws, slices.Index(vis, ci), delta)
		return out
	}
	var ws []*float64
	k := 0
	for r := range out.Rows {
		if len(out.visibleIn(r)) > 0 {
			if r == ri {
				k = len(ws)
			}
			ws = append(ws, &out.Rows[r].Weight)
		}
	}
	reshare(ws, k, delta)
	return out
}

// reshare moves *ws[i]'s share of the total by delta, keeping it between
// minShare and 1-minShare, and leaves the others' weights (and so their
// proportions to each other) alone.
func reshare(ws []*float64, i int, delta float64) {
	const minShare = 0.05
	if len(ws) < 2 {
		return
	}
	sum := 0.0
	for _, w := range ws {
		sum += max(*w, 0)
	}
	rest := sum - max(*ws[i], 0)
	if sum <= 0 || rest <= 0 {
		return
	}
	share := min(max(*ws[i]/sum+delta, minShare), 1-minShare)
	// Solve w/(w+rest) = share for w, rounded so layout.json stays readable.
	*ws[i] = max(math.Round(share*rest/(1-share)*1000)/1000, 0.001)
}

// JoinRow moves id into row, before the Widget at index (len = at the end).
// row and index refer to l before the move; a row left empty is removed.
func (l Layout) JoinRow(id string, row, index int) Layout {
	out := l.clone()
	ri, ci := out.Find(id)
	if ri < 0 || row < 0 || row >= len(out.Rows) {
		return out
	}
	cell := out.Rows[ri].Widgets[ci]
	out.Rows[ri].Widgets = slices.Delete(out.Rows[ri].Widgets, ci, ci+1)
	if ri == row && ci < index {
		index--
	}
	ws := out.Rows[row].Widgets
	out.Rows[row].Widgets = slices.Insert(ws, min(max(index, 0), len(ws)), cell)
	return out.compact()
}

// OpenRow moves id into a new row of its own, before row at (len = last).
// The new row takes the weight of the row id left; a row left empty is removed.
func (l Layout) OpenRow(id string, at int) Layout {
	out := l.clone()
	ri, ci := out.Find(id)
	if ri < 0 {
		return out
	}
	cell := out.Rows[ri].Widgets[ci]
	weight := out.Rows[ri].Weight
	out.Rows[ri].Widgets = slices.Delete(out.Rows[ri].Widgets, ci, ci+1)
	out.Rows = slices.Insert(out.Rows, min(max(at, 0), len(out.Rows)), Row{Weight: weight, Widgets: []Cell{cell}})
	return out.compact()
}

// SetHidden hides or shows id. A hidden Widget keeps its place, so showing it
// puts it back where it was.
func (l Layout) SetHidden(id string, hidden bool) Layout {
	out := l.clone()
	out.Hidden = slices.DeleteFunc(out.Hidden, func(h string) bool { return h == id })
	if hidden {
		out.Hidden = append(out.Hidden, id)
	}
	return out
}

// Visible is l without its hidden Widgets and the rows they leave empty:
// what Compute lays out.
func (l Layout) Visible() Layout {
	var out Layout
	for ri, r := range l.Rows {
		nr := Row{Weight: r.Weight}
		for _, ci := range l.visibleIn(ri) {
			nr.Widgets = append(nr.Widgets, r.Widgets[ci])
		}
		if len(nr.Widgets) > 0 {
			out.Rows = append(out.Rows, nr)
		}
	}
	return out
}

func (l Layout) IsHidden(id string) bool { return slices.Contains(l.Hidden, id) }

func (l Layout) visibleIn(ri int) []int {
	var out []int
	for ci, c := range l.Rows[ri].Widgets {
		if !l.IsHidden(c.ID) {
			out = append(out, ci)
		}
	}
	return out
}

func (l Layout) compact() Layout {
	l.Rows = slices.DeleteFunc(l.Rows, func(r Row) bool { return len(r.Widgets) == 0 })
	return l
}

// clone deep-copies l so operations never alias the Layout they started from.
func (l Layout) clone() Layout {
	out := Layout{Hidden: slices.Clone(l.Hidden)}
	for _, r := range l.Rows {
		out.Rows = append(out.Rows, Row{Weight: r.Weight, Widgets: slices.Clone(r.Widgets)})
	}
	return out
}
