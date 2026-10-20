package layout

import (
	"math"
	"slices"
)

// HandleGlyph is the grab handle drawn before a hovered or dragged title.
const HandleGlyph = "⠿ "

// GhostMax is the largest the dashed ghost of a dragged Widget gets.
var GhostMax = Size{W: 34, H: 6}

// Pointer is what the mouse is doing, as far as drawing goes.
type Pointer struct {
	// Handle is the Widget whose title shows the grab handle.
	Handle string
	// Seam is the hovered or dragged seam, matched to a seam Zone by Kind,
	// Row and Index.
	Seam *Zone
	Drag *Drag
}

// Drag is a Widget being moved by its title.
type Drag struct {
	ID string
	// X and Y are the pointer; DX and DY where it holds the tile, from its corner.
	X, Y, DX, DY int
	Drop         *Drop
}

type DropKind int

const (
	// OnTile is a side of the Target tile: Left and Right join its row,
	// Up and Down stack with it in its cell.
	OnTile DropKind = iota
	// OnEdge is a side of the dashboard: Left and Right make a full-height
	// column Share of the width, Up and Down a full-width row.
	OnEdge
	// OnSeam is a row seam: a new row in the container at Path, before row At.
	OnSeam
)

// Drop is where a dragged Widget lands, and Bar the line that marks it.
type Drop struct {
	Kind   DropKind
	Target string
	Side   Dir
	Path   Path
	At     int
	Share  float64
	Bar    Rect
}

// hitOrder ranks overlapping zones: a title sits in a row seam's cells, and
// Items and hints sit inside bodies and seams.
var hitOrder = []ZoneKind{Hint, Item, Title, WidgetSeam, RowSeam, Body}

// Hit is the zone under a cell, the highest ranked when zones overlap.
func (f Frame) Hit(x, y int) (Zone, bool) {
	for _, k := range hitOrder {
		for _, z := range f.Zones {
			if z.Kind == k && z.Contains(x, y) {
				return z, true
			}
		}
	}
	return Zone{}, false
}

func (f Frame) Tile(id string) (Tile, bool) {
	for _, t := range f.Tiles {
		if t.ID == id {
			return t, true
		}
	}
	return Tile{}, false
}

// DropAt is where Widget id lands when released over x, y: the dashboard's
// edge (its outermost two columns, or its top or bottom line), a row seam,
// or else the side of the tile under the pointer that it's nearest,
// relative to the tile's size.
func (f Frame) DropAt(id string, x, y int) (Drop, bool) {
	if f.rects == nil {
		return Drop{}, false
	}
	switch {
	case x <= 1:
		return Drop{Kind: OnEdge, Side: Left, Share: f.share(id), Bar: Rect{0, 0, 1, f.h}}, true
	case x >= f.w-2:
		return Drop{Kind: OnEdge, Side: Right, Share: f.share(id), Bar: Rect{f.w - 1, 0, 1, f.h}}, true
	case y <= 0:
		return Drop{Kind: OnEdge, Side: Up, Bar: Rect{0, 0, f.w, 1}}, true
	case y >= f.h-1:
		return Drop{Kind: OnEdge, Side: Down, Bar: Rect{0, f.h - 1, f.w, 1}}, true
	}
	for _, z := range f.Zones {
		if z.Kind == RowSeam && z.Contains(x, y) {
			return Drop{Kind: OnSeam, Path: z.Path, At: z.Index + 1, Bar: Rect{z.X, y, z.W, 1}}, true
		}
	}
	for _, t := range f.Tiles {
		if t.ID == id || !t.Contains(x, y) {
			continue
		}
		dx := (float64(x-t.X)+0.5)/float64(t.W) - 0.5
		dy := (float64(y-t.Y)+0.5)/float64(t.H) - 0.5
		d := Drop{Kind: OnTile, Target: t.ID, Side: Down, Bar: Rect{t.X, t.Y + t.H - 1, t.W, 1}}
		switch {
		case math.Abs(dx) > math.Abs(dy) && dx < 0:
			d.Side, d.Bar = Left, Rect{t.X, t.Y, 1, t.H}
		case math.Abs(dx) > math.Abs(dy):
			d.Side, d.Bar = Right, Rect{t.X + t.W - 1, t.Y, 1, t.H}
		case dy < 0:
			d.Side, d.Bar = Up, Rect{t.X, t.Y, t.W, 1}
		}
		return d, true
	}
	return Drop{}, false
}

// share is the part of the width id's tile has, as a full-height column
// keeps it: between 15% and 60%.
func (f Frame) share(id string) float64 {
	share := 0.3
	if t, ok := f.Tile(id); ok && f.w > 0 {
		share = float64(t.W) / float64(f.w)
	}
	return min(max(share, 0.15), 0.6)
}

func (l Layout) Drop(id string, d Drop) Layout {
	p := l.Find(id)
	if p == nil || l.IsHidden(id) {
		return l.clone()
	}
	switch d.Kind {
	case OnEdge:
		switch d.Side {
		case Left, Right:
			return l.column(id, d.Side, d.Share)
		case Up:
			return l.openRow(id, Path{}, 0)
		}
		return l.openRow(id, Path{}, len(l.Rows))
	case OnSeam:
		return l.openRow(id, d.Path, d.At)
	}
	tp := l.Find(d.Target)
	if tp == nil || id == d.Target {
		return l.clone()
	}
	out := l.clone()
	cell := out.take(p)
	switch d.Side {
	case Left, Right:
		t := out.cellAt(tp)
		t.Weight = round3(t.Weight / 2)
		cell.Weight = t.Weight
		at := tp.last()
		if d.Side == Right {
			at++
		}
		out.insertCell(tp.parent(), at, cell)
	default:
		out.stackWith(tp, cell, d.Side == Up)
	}
	return out.normalise()
}

// Split moves seam s so its first cell (the left cell's right border, or
// the upper row's bottom border) lands on at, a column or a row. Only the
// two cells or rows either side change, keeping their combined weight, and
// neither gets smaller than it needs. f is l's frame.
func (l Layout) Split(f Frame, s Zone, at int) Layout {
	out := l.clone()
	if f.rects == nil || !l.has(s.Path) {
		return out
	}
	var a, b *float64
	var first, span, la, lb int
	siblings := 0.0
	switch {
	case s.Kind == WidgetSeam && s.Path.isRow():
		cells := out.rowAt(s.Path).Cells
		if s.Index < 0 || s.Index >= len(cells) {
			return out
		}
		j := nextShown(cells, s.Index, 1, l.shown)
		if j < 0 {
			return out
		}
		ra, rb := f.rects[s.Path.with(s.Index).key()], f.rects[s.Path.with(j).key()]
		first, span = ra.X, ra.W+rb.W
		la, lb = l.needCell(cells[s.Index]).W, l.needCell(cells[j]).W
		a, b = &cells[s.Index].Weight, &cells[j].Weight
		for _, c := range cells {
			siblings += c.Weight
		}
	case s.Kind == RowSeam && !s.Path.isRow():
		rows := *out.rowsAt(s.Path)
		if s.Index < 0 || s.Index >= len(rows) {
			return out
		}
		j := nextShown(rows, s.Index, 1, l.rowShown)
		if j < 0 {
			return out
		}
		ra, rb := f.rects[s.Path.with(s.Index).key()], f.rects[s.Path.with(j).key()]
		first, span = ra.Y, ra.H+rb.H
		la, lb = l.needRow(rows[s.Index]).H, l.needRow(rows[j]).H
		a, b = &rows[s.Index].Weight, &rows[j].Weight
		for _, r := range rows {
			siblings += r.Weight
		}
	default:
		return out
	}
	if span < la+lb {
		return out
	}
	size := min(max(at-first+1, la), span-lb)
	total, floor := *a+*b, round3(siblings*minShare)
	*a = min(max(round3(total*float64(size)/float64(span)), floor), total-floor)
	*b = round3(total - *a)
	return out.normalise()
}

// tint is the border cells of seam s: a widget seam's two facing side
// borders, or a row seam's bottom and top borders between their corners.
func (f Frame) tint(s *Zone) []Rect {
	if s == nil {
		return nil
	}
	for _, z := range f.Zones {
		if z.Kind != s.Kind || z.Index != s.Index || !slices.Equal(z.Path, s.Path) {
			continue
		}
		if z.Kind == WidgetSeam {
			return []Rect{z.Rect}
		}
		if z.Kind != RowSeam {
			return nil
		}
		var out []Rect
		for _, t := range f.Tiles {
			if t.X < z.X || t.X+t.W > z.X+z.W {
				continue
			}
			switch {
			case t.Y+t.H-1 == z.Y:
				out = append(out, Rect{t.X + 1, z.Y, t.W - 2, 1})
			case t.Y == z.Y+1:
				out = append(out, Rect{t.X + 1, z.Y + 1, t.W - 2, 1})
			}
		}
		return out
	}
	return nil
}

// ghost places the dragged Widget's outline under the pointer, held where it
// was grabbed and kept on screen, and the bar where it would drop.
func (f Frame) ghost(d Drag, w, h int) (ghost, bar Rect) {
	t, ok := f.Tile(d.ID)
	if !ok {
		return Rect{}, Rect{}
	}
	gw, gh := min(t.W, GhostMax.W), min(t.H, GhostMax.H)
	ghost = Rect{d.X - min(d.DX, gw-2), d.Y - min(d.DY, gh-1), gw, gh}
	ghost.X = min(max(ghost.X, 0), w-gw)
	ghost.Y = min(max(ghost.Y, 0), h-gh)
	if d.Drop == nil {
		return ghost, Rect{}
	}
	return ghost, d.Drop.Bar
}
