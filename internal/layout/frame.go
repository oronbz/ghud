package layout

import "github.com/charmbracelet/x/ansi"

type Rect struct{ X, Y, W, H int }

// Tile is one Widget's outer rectangle, border included.
type Tile struct {
	ID    string
	Title string
	Rect
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
	}
	return "unknown"
}

// Zone is a hit-test rectangle. ID names the Widget for Body and Title. For
// seams, Row is the row above or containing the seam and Index the Widget to
// its left.
type Zone struct {
	Kind ZoneKind
	Rect
	ID         string
	Row, Index int
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
}

// Compute lays l out over a w×h area. titles gives each Widget's border title.
func Compute(l Layout, titles map[string]string, w, h int) Frame {
	var f Frame
	for _, r := range l.Rows {
		f.Need.W = max(f.Need.W, len(r.Widgets)*MinTile.W)
	}
	f.Need.H = len(l.Rows) * MinTile.H
	if w < f.Need.W || h < f.Need.H {
		f.TooSmall = true
		return f
	}

	rowWeights := make([]float64, len(l.Rows))
	for i, r := range l.Rows {
		rowWeights[i] = r.Weight
	}
	y := 0
	heights := split(h, rowWeights, MinTile.H)
	for ri, rh := range heights {
		r := l.Rows[ri]
		weights := make([]float64, len(r.Widgets))
		for j, c := range r.Widgets {
			weights[j] = c.Weight
		}
		x := 0
		widths := split(w, weights, MinTile.W)
		for j, cw := range widths {
			id := r.Widgets[j].ID
			t := Tile{ID: id, Title: titles[id], Rect: Rect{x, y, cw, rh}}
			f.Tiles = append(f.Tiles, t)
			f.Zones = append(f.Zones,
				Zone{Kind: Body, ID: id, Row: ri, Index: j, Rect: Rect{x + 1, y + 1, cw - 2, rh - 2}},
				Zone{Kind: Title, ID: id, Row: ri, Index: j, Rect: Rect{x + 1, y, min(ansi.StringWidth(t.Title)+3, cw-2), 1}},
			)
			if j < len(widths)-1 {
				f.Zones = append(f.Zones, Zone{Kind: WidgetSeam, Row: ri, Index: j, Rect: Rect{x + cw - 1, y + 1, 2, rh - 2}})
			}
			x += cw
		}
		y += rh
		if ri < len(heights)-1 {
			f.Zones = append(f.Zones, Zone{Kind: RowSeam, Row: ri, Rect: Rect{0, y - 1, w, 2}})
		}
	}
	return f
}

// split shares total cells across weights with largest-remainder rounding.
// Parts whose share falls below least are pinned at least and the rest is
// re-shared, so proportions hold wherever there's room. The caller ensures
// total >= least*len(weights).
func split(total int, weights []float64, least int) []int {
	out := make([]int, len(weights))
	pinned := make([]bool, len(weights))
	for {
		left, sum := total, 0.0
		for i, w := range weights {
			if pinned[i] {
				left -= least
			} else {
				sum += max(w, 0)
			}
		}
		changed := false
		for i, w := range weights {
			if !pinned[i] && (sum <= 0 || float64(left)*max(w, 0)/sum < float64(least)) {
				pinned[i], out[i], changed = true, least, true
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
