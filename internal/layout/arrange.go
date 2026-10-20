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

func (d Dir) step() int {
	if d == Left || d == Up {
		return -1
	}
	return 1
}

func (d Dir) across() bool { return d == Left || d == Right }

func (d Dir) opposite() Dir { return [...]Dir{Right, Left, Down, Up}[d] }

// Via is what arrange mode remembers between presses: the Widget the last
// press went into and which way, so the next press that way goes past it,
// and the press the opposite arrow undoes. The zero Via is none.
type Via struct {
	ID   string
	Dir  Dir
	back *press
}

// press is one keyboard move, kept so the opposite arrow can put back the
// Layout and the Via from before it.
type press struct {
	id            string
	d             Dir
	before, after Layout
	via           Via
}

// Move is the keyboard move: one position per press, into the next Widget
// that way and then past it, the opposite arrow undoing each press. area is
// the dashboard's size, and via what the last press left; the Via returned
// is for the next press.
func (l Layout) Move(id string, d Dir, area Size, via Via) (Layout, Via) {
	b := via.back
	if b == nil || b.id != id || !l.same(b.after) {
		via, b = Via{}, nil
	}
	if b != nil && b.d == d.opposite() {
		return b.before.clone(), b.via
	}
	out, next := l.move(id, d, area, via)
	next.back = via.back
	if !out.same(l) {
		next.back = &press{id: id, d: d, before: l.clone(), after: out.clone(), via: via}
	}
	return out, next
}

func (l Layout) move(id string, d Dir, area Size, via Via) (Layout, Via) {
	p := l.Find(id)
	if p == nil || l.IsHidden(id) {
		return l.clone(), Via{}
	}
	need := l.needRows(l.Rows)
	f := Compute(l, nil, max(area.W, need.W), max(area.H, need.H), Pointer{})
	me := centre(f.rects[p.key()])
	step := d.step()
	rp, ci := p[:len(p)-1], p[len(p)-1]
	cp, ri := rp[:len(rp)-1], rp[len(rp)-1]
	cells := l.rowAt(rp).Widgets
	out := l.clone()

	if d.across() {
		j := l.nextShown(cells, ci, step)
		switch {
		case j >= 0 && cells[j].IsStack():
			sp := rp.with(j)
			at := 0
			for i, r := range cells[j].Rows {
				if l.rowShown(r) && centre(f.rects[sp.with(i).key()]).y < me.y {
					at = i + 1
				}
			}
			out.insertRow(sp, at, out.take(p))
			return out.normalise(), Via{}
		case j >= 0:
			nb := rp.with(j)
			out.stackWith(nb, out.take(p), me.y <= centre(f.rects[nb.key()]).y)
			return out.normalise(), Via{ID: cells[j].ID, Dir: d}
		case len(cp) == 0:
			return l.column(id, d, f.share(id)), Via{}
		}
		cell := out.take(p)
		pr, pi := cp[:len(cp)-1], cp[len(cp)-1]
		cell.Weight = out.meanCell(pr)
		out.insertCell(pr, pi+max(step, 0), cell)
		return out.normalise(), Via{}
	}

	if slices.ContainsFunc(cells, func(c Cell) bool { return c.ID != id && l.shown(c) }) {
		cell := out.take(p)
		if via.ID != "" && via.Dir == d {
			if j := slices.IndexFunc(cells, func(c Cell) bool { return c.ID == via.ID && c.ID != id && l.shown(c) }); j >= 0 {
				out.stackWith(rp.with(j), cell, step < 0)
				return out.normalise(), Via{}
			}
		}
		out.insertRow(cp, ri+max(step, 0), cell)
		return out.normalise(), Via{}
	}

	rows := *l.rowsAt(cp)
	j := l.nextRowShown(rows, ri, step)
	if j < 0 {
		if len(cp) == 0 {
			return out, Via{}
		}
		cell := out.take(p)
		pr := cp[:len(cp)-1]
		out.insertRow(pr[:len(pr)-1], pr[len(pr)-1]+max(step, 0), cell)
		return out.normalise(), Via{}
	}
	tp := cp.with(j)
	near, best := -1, math.Inf(1)
	for k, c := range rows[j].Widgets {
		if !l.shown(c) {
			continue
		}
		r := f.rects[tp.with(k).key()]
		if g := gap(me.x, float64(r.X), float64(r.X+r.W)); g < best {
			near, best = k, g
		}
	}
	cell := out.take(p)
	if rows[j].Widgets[near].IsStack() {
		sp := tp.with(near)
		at := 0
		if step < 0 {
			at = len(rows[j].Widgets[near].Rows)
		}
		out.insertRow(sp, at, cell)
		return out.normalise(), Via{}
	}
	cell.Weight = out.meanCell(tp)
	at := near
	if me.x > centre(f.rects[tp.with(near).key()]).x {
		at++
	}
	out.insertCell(tp, at, cell)
	return out.normalise(), Via{ID: rows[j].Widgets[near].ID, Dir: d}
}

type point struct{ x, y float64 }

func centre(r Rect) point { return point{float64(r.X) + float64(r.W)/2, float64(r.Y) + float64(r.H)/2} }

// gap is how far v is outside [lo, hi), 0 inside.
func gap(v, lo, hi float64) float64 {
	switch {
	case v < lo:
		return lo - v
	case v >= hi:
		return v - hi + 1
	}
	return 0
}

// column makes id a full-height column on side of the dashboard, share of
// its width; everything else keeps its rows in a Stack beside it.
func (l Layout) column(id string, side Dir, share float64) Layout {
	out := l.clone()
	p := out.Find(id)
	if p == nil {
		return out
	}
	cell := out.take(p)
	cell.Weight = round3(share * 10)
	rest := Cell{Weight: round3((1 - share) * 10), Rows: tidy(out.Rows)}
	cells := []Cell{cell, rest}
	if side == Right {
		cells = []Cell{rest, cell}
	}
	out.Rows = []Row{{Weight: 1, Widgets: cells}}
	return out.normalise()
}

// openRow moves id into a new row of its own in the container at cp,
// before its row at (len = last).
func (l Layout) openRow(id string, cp Path, at int) Layout {
	out := l.clone()
	p := out.Find(id)
	if p == nil || len(cp)%2 != 0 || !l.has(cp) || len(cp) > 0 && !l.cellAt(cp).IsStack() {
		return out
	}
	out.insertRow(cp, at, out.take(p))
	return out.normalise()
}

// take leaves an empty cell where p was, for normalise to remove, so every
// other Path stays valid until then, and returns what was there.
func (l *Layout) take(p Path) Cell {
	c := l.cellAt(p)
	out := *c
	*c = Cell{}
	return out
}

func (l *Layout) insertCell(rp Path, at int, c Cell) {
	r := l.rowAt(rp)
	r.Widgets = slices.Insert(r.Widgets, min(max(at, 0), len(r.Widgets)), c)
}

// insertRow puts c in a row of its own in the container at cp, before its
// row at, as tall as its rows are on average.
func (l *Layout) insertRow(cp Path, at int, c Cell) {
	rows := l.rowsAt(cp)
	var ws []float64
	for _, r := range *rows {
		if l.rowShown(r) {
			ws = append(ws, r.Weight)
		}
	}
	c.Weight = 1
	*rows = slices.Insert(*rows, min(max(at, 0), len(*rows)), Row{Weight: mean(ws), Widgets: []Cell{c}})
}

// stackWith turns the Widget at p into a Stack of it and c, c on top when
// first.
func (l *Layout) stackWith(p Path, c Cell, first bool) {
	t := l.cellAt(p)
	pair := []Cell{{ID: c.ID, Weight: 1}, {ID: t.ID, Weight: 1}}
	if !first {
		pair[0], pair[1] = pair[1], pair[0]
	}
	*t = Cell{Weight: t.Weight, Rows: []Row{{Weight: 1, Widgets: pair[:1]}, {Weight: 1, Widgets: pair[1:]}}}
}

func (l Layout) meanCell(rp Path) float64 {
	var ws []float64
	for _, c := range l.rowAt(rp).Widgets {
		if l.shown(c) {
			ws = append(ws, c.Weight)
		}
	}
	return mean(ws)
}

func mean(ws []float64) float64 {
	if len(ws) == 0 {
		return 1
	}
	sum := 0.0
	for _, w := range ws {
		sum += w
	}
	return round3(sum / float64(len(ws)))
}

// ResizeStep is how much of its row (or of the height) one keyboard resize
// gives or takes.
const ResizeStep = 0.1

// minShare is the smallest share of its siblings' total a weight is given.
const minShare = 0.05

// Resize is the keyboard resize: Right and Left widen and narrow id, Down
// and Up make it taller and shorter, in the nearest container (going out
// from id) where it, or the Stack it's in, has a sibling that way. Only
// visible cells and rows share the space; the others keep their proportions.
func (l Layout) Resize(id string, d Dir) Layout {
	out := l.clone()
	p := out.Find(id)
	if p == nil || out.IsHidden(id) {
		return out
	}
	delta := ResizeStep * float64(d.step())
	for k := len(p); k >= 2; k -= 2 {
		rp := p[:k-1]
		var ws []*float64
		i := -1
		if d.across() {
			cells := out.rowAt(rp).Widgets
			for j := range cells {
				if out.shown(cells[j]) {
					if j == p[k-1] {
						i = len(ws)
					}
					ws = append(ws, &cells[j].Weight)
				}
			}
		} else {
			rows := *out.rowsAt(rp[:k-2])
			for j := range rows {
				if out.rowShown(rows[j]) {
					if j == rp[k-2] {
						i = len(ws)
					}
					ws = append(ws, &rows[j].Weight)
				}
			}
		}
		if len(ws) >= 2 {
			reshare(ws, i, delta)
			return out
		}
	}
	return out
}

// reshare moves *ws[i]'s share of the total by delta, keeping it between
// minShare and 1-minShare, and leaves the others' weights (and so their
// proportions to each other) alone.
func reshare(ws []*float64, i int, delta float64) {
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
