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

// Frame is the result of one layout pass: what to draw and where to hit.
type Frame struct {
	Tiles []Tile
	Zones []Zone
}

// Compute lays l out over a w×h area. titles gives each Widget's border title.
func Compute(l Layout, titles map[string]string, w, h int) Frame {
	var f Frame
	rowWeights := make([]float64, len(l.Rows))
	for i, r := range l.Rows {
		rowWeights[i] = r.Weight
	}
	y := 0
	heights := split(h, rowWeights)
	for ri, rh := range heights {
		r := l.Rows[ri]
		weights := make([]float64, len(r.Widgets))
		for j, c := range r.Widgets {
			weights[j] = c.Weight
		}
		x := 0
		widths := split(w, weights)
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
func split(total int, weights []float64) []int {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	out := make([]int, len(weights))
	if sum <= 0 {
		return out
	}
	rem := make([]float64, len(weights))
	used := 0
	for i, w := range weights {
		share := float64(total) * w / sum
		out[i] = int(share)
		rem[i] = share - float64(out[i])
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
	return out
}
