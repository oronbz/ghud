package layout

type Rect struct{ X, Y, W, H int }

type Size struct{ W, H int }

// Tile is one Widget's outer rectangle, border included.
type Tile struct {
	ID string
	Rect
}

// Frame is the result of one layout pass.
type Frame struct {
	Tiles []Tile
}

// Compute lays l out over a w×h area.
func Compute(l Layout, w, h int) Frame {
	var f Frame
	rowWeights := make([]float64, len(l.Rows))
	for i, r := range l.Rows {
		rowWeights[i] = r.Weight
	}
	y := 0
	for i, rh := range split(h, rowWeights) {
		r := l.Rows[i]
		weights := make([]float64, len(r.Widgets))
		for j, c := range r.Widgets {
			weights[j] = c.Weight
		}
		x := 0
		for j, cw := range split(w, weights) {
			f.Tiles = append(f.Tiles, Tile{ID: r.Widgets[j].ID, Rect: Rect{x, y, cw, rh}})
			x += cw
		}
		y += rh
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
