package layout

import "math"

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
	// Drag is a title being dragged.
	Drag *Drag
}

// Drag is a Widget being moved by its title.
type Drag struct {
	ID string
	// X and Y are the pointer; DX and DY where it holds the tile, from its corner.
	X, Y, DX, DY int
	// Drop is where the Widget would land if released now.
	Drop *Drop
}

// Drop is where a dragged Widget lands: on Side of Target. Left and Right
// join Target's row; Up and Down open a new row above or below it.
type Drop struct {
	Target string
	Side   Dir
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

// Tile is the tile drawn for id.
func (f Frame) Tile(id string) (Tile, bool) {
	for _, t := range f.Tiles {
		if t.ID == id {
			return t, true
		}
	}
	return Tile{}, false
}

// DropAt is where Widget id lands when released over x, y: the side of the
// Widget under the pointer that the pointer is nearest, relative to its size.
func (f Frame) DropAt(id string, x, y int) (Drop, bool) {
	for _, t := range f.Tiles {
		if t.ID == id || !t.Contains(x, y) {
			continue
		}
		dx := (float64(x-t.X)+0.5)/float64(t.W) - 0.5
		dy := (float64(y-t.Y)+0.5)/float64(t.H) - 0.5
		side := Down
		switch {
		case math.Abs(dx) > math.Abs(dy) && dx < 0:
			side = Left
		case math.Abs(dx) > math.Abs(dy):
			side = Right
		case dy < 0:
			side = Up
		}
		return Drop{Target: t.ID, Side: side}, true
	}
	return Drop{}, false
}

// Drop moves id to where d says.
func (l Layout) Drop(id string, d Drop) Layout {
	tr, tc := l.Find(d.Target)
	if tr < 0 || id == d.Target {
		return l.clone()
	}
	switch d.Side {
	case Left:
		return l.JoinRow(id, tr, tc)
	case Right:
		return l.JoinRow(id, tr, tc+1)
	case Up:
		return l.OpenRow(id, tr)
	}
	return l.OpenRow(id, tr+1)
}

// tint is the border cells of seam s: a widget seam's two facing side
// borders, or a row seam's bottom and top borders between their corners.
func (f Frame) tint(s *Zone) []Rect {
	if s == nil {
		return nil
	}
	for _, z := range f.Zones {
		if z.Kind != s.Kind || z.Row != s.Row || z.Index != s.Index {
			continue
		}
		if z.Kind == WidgetSeam {
			return []Rect{z.Rect}
		}
		if z.Kind != RowSeam {
			return nil
		}
		var out []Rect
		for _, b := range f.Zones {
			switch {
			case b.Kind != Body:
			case b.Row == z.Row:
				out = append(out, Rect{b.X, b.Y + b.H, b.W, 1})
			case b.Row == z.Row+1:
				out = append(out, Rect{b.X, b.Y - 1, b.W, 1})
			}
		}
		return out
	}
	return nil
}

// ghost places the dragged Widget's outline under the pointer, held where it
// was grabbed and kept on screen, and the bar on the side it would drop.
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
	r, ok := f.Tile(d.Drop.Target)
	if !ok {
		return ghost, Rect{}
	}
	switch d.Drop.Side {
	case Left:
		bar = Rect{r.X, r.Y, 1, r.H}
	case Right:
		bar = Rect{r.X + r.W - 1, r.Y, 1, r.H}
	case Up:
		bar = Rect{r.X, r.Y, r.W, 1}
	default:
		bar = Rect{r.X, r.Y + r.H - 1, r.W, 1}
	}
	return ghost, bar
}
