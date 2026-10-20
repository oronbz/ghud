package layout

import (
	"slices"

	"github.com/charmbracelet/x/ansi"
)

type Rect struct{ X, Y, W, H int }

func (r Rect) Contains(x, y int) bool { return x >= r.X && x < r.X+r.W && y >= r.Y && y < r.Y+r.H }

// Tile is one Widget's outer rectangle, border included.
type Tile struct {
	ID    string
	Title string
	Rect
	// Handle draws the ⠿ grab handle before the title.
	Handle bool
	Path   Path
}

type ZoneKind int

const (
	// Body is a Widget's inside, border excluded.
	Body ZoneKind = iota
	// Title is "─ Title " in the top border, the drag handle for moving a Widget.
	Title
	// WidgetSeam is the two facing side borders of neighbours in a row.
	WidgetSeam
	// RowSeam is the bottom borders of one row over the top borders of the next.
	RowSeam
	// Item is one line of a list showing an Item; Index is the Item's.
	Item
	// Hint is a key hint in the focused Widget's bottom border; Index counts the hints drawn.
	Hint
)

func (k ZoneKind) String() string {
	switch k {
	case Body:
		return "body"
	case Title:
		return "title"
	case WidgetSeam:
		return "widget seam"
	case RowSeam:
		return "row seam"
	case Item:
		return "item"
	case Hint:
		return "hint"
	}
	return "unknown"
}

// Zone is a hit-test rectangle. ID names the Widget for Body and Title. A
// WidgetSeam's Path is its row and Index the cell to its left; a RowSeam's
// Path is its container (the dashboard or a Stack) and Index the row above
// it. Indexes count every cell or row, hidden ones included.
type Zone struct {
	Kind ZoneKind
	Rect
	ID    string
	Path  Path
	Index int
}

type Size struct{ W, H int }

// MinTile is the smallest tile, border included, that draws legibly.
var MinTile = Size{W: 16, H: 5}

// Frame is the result of one layout pass: what to draw and where to hit.
type Frame struct {
	Tiles []Tile
	Zones []Zone
	// TooSmall replaces the grid with a notice asking for Need.
	TooSmall bool
	Need     Size
	// Tint are the border cells of the hovered or dragged seam, drawn lit.
	Tint []Rect
	// Ghost is the dashed outline following a dragged title, and Bar the
	// heavy line marking where it will drop; zero when there's no drag.
	Ghost, Bar Rect

	w, h int
	// rects places every shown row and cell, by Path key.
	rects map[string]Rect
}

// Compute lays l's visible Widgets out over a w×h area. titles gives each
// Widget's border title, and p the pointer's hover or drag to draw.
func Compute(l Layout, titles map[string]string, w, h int, p Pointer) Frame {
	f := Frame{w: w, h: h}
	if !slices.ContainsFunc(l.Rows, l.rowShown) {
		return f
	}
	f.Need = l.needRows(l.Rows)
	if w < f.Need.W || h < f.Need.H {
		f.TooSmall = true
		return f
	}
	f.rects = map[string]Rect{}
	lay := laying{l: l, f: &f, titles: titles, handle: p.Handle}
	lay.rows(l.Rows, Rect{0, 0, w, h}, Path{})
	f.Tint = f.tint(p.Seam)
	if p.Drag != nil {
		f.Ghost, f.Bar = f.ghost(*p.Drag, w, h)
	}
	return f
}

type laying struct {
	l      Layout
	f      *Frame
	titles map[string]string
	handle string
}

func (y laying) rows(rows []Row, area Rect, at Path) {
	var idx []int
	var weights []float64
	var least []int
	for i, r := range rows {
		if y.l.rowShown(r) {
			idx = append(idx, i)
			weights = append(weights, r.Weight)
			least = append(least, y.l.needRow(r).H)
		}
	}
	top := area.Y
	for k, rh := range split(area.H, weights, least) {
		i := idx[k]
		rect := Rect{area.X, top, area.W, rh}
		y.f.rects[at.with(i).key()] = rect
		y.row(rows[i], rect, at.with(i))
		top += rh
		if k < len(idx)-1 {
			y.f.Zones = append(y.f.Zones, Zone{Kind: RowSeam, Path: at, Index: i, Rect: Rect{area.X, top - 1, area.W, 2}})
		}
	}
}

func (y laying) row(r Row, area Rect, at Path) {
	var idx []int
	var weights []float64
	var least []int
	for j, c := range r.Widgets {
		if y.l.shown(c) {
			idx = append(idx, j)
			weights = append(weights, c.Weight)
			least = append(least, y.l.needCell(c).W)
		}
	}
	x := area.X
	for k, cw := range split(area.W, weights, least) {
		j := idx[k]
		c, p := r.Widgets[j], at.with(j)
		rect := Rect{x, area.Y, cw, area.H}
		y.f.rects[p.key()] = rect
		if c.IsStack() {
			y.rows(c.Rows, rect, p)
		} else {
			y.tile(c.ID, rect, p)
		}
		if k < len(idx)-1 {
			y.f.Zones = append(y.f.Zones, Zone{Kind: WidgetSeam, Path: at, Index: j, Rect: Rect{x + cw - 1, area.Y + 1, 2, area.H - 2}})
		}
		x += cw
	}
}

func (y laying) tile(id string, r Rect, at Path) {
	t := Tile{ID: id, Title: y.titles[id], Rect: r, Handle: id == y.handle, Path: at}
	tw := ansi.StringWidth(t.Title) + 3
	if t.Handle {
		tw += ansi.StringWidth(HandleGlyph)
	}
	y.f.Tiles = append(y.f.Tiles, t)
	y.f.Zones = append(y.f.Zones,
		Zone{Kind: Body, ID: id, Path: at, Rect: Rect{r.X + 1, r.Y + 1, r.W - 2, r.H - 2}},
		Zone{Kind: Title, ID: id, Path: at, Rect: Rect{r.X + 1, r.Y, min(tw, r.W-2), 1}},
	)
}

// needCell is the smallest c draws in: a tile, or what a Stack's rows need.
func (l Layout) needCell(c Cell) Size {
	if c.IsStack() {
		return l.needRows(c.Rows)
	}
	return MinTile
}

func (l Layout) needRow(r Row) Size {
	var n Size
	for _, c := range r.Widgets {
		if l.shown(c) {
			cn := l.needCell(c)
			n.W += cn.W
			n.H = max(n.H, cn.H)
		}
	}
	return n
}

func (l Layout) needRows(rows []Row) Size {
	var n Size
	for _, r := range rows {
		if l.rowShown(r) {
			rn := l.needRow(r)
			n.W = max(n.W, rn.W)
			n.H += rn.H
		}
	}
	return n
}

// split shares total cells across weights with largest-remainder rounding.
// Parts whose share falls below their least are pinned there and the rest is
// re-shared, so proportions hold wherever there's room. The caller ensures
// total covers every least.
func split(total int, weights []float64, least []int) []int {
	out := make([]int, len(weights))
	pinned := make([]bool, len(weights))
	for {
		left, sum := total, 0.0
		for i, w := range weights {
			if pinned[i] {
				left -= least[i]
			} else {
				sum += max(w, 0)
			}
		}
		changed := false
		for i, w := range weights {
			if !pinned[i] && (sum <= 0 || float64(left)*max(w, 0)/sum < float64(least[i])) {
				pinned[i], out[i], changed = true, least[i], true
			}
		}
		if !changed {
			share(out, pinned, weights, left, sum)
			return out
		}
	}
}

func share(out []int, pinned []bool, weights []float64, total int, sum float64) {
	rem := make([]float64, len(weights))
	used := 0
	for i, w := range weights {
		rem[i] = -1
		if pinned[i] {
			continue
		}
		f := float64(total) * w / sum
		out[i] = int(f)
		rem[i] = f - float64(out[i])
		used += out[i]
	}
	for ; used < total; used++ {
		best := 0
		for i := range rem {
			if rem[i] > rem[best] {
				best = i
			}
		}
		out[best]++
		rem[best] = -1
	}
}
