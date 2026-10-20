package layout

import (
	"math"
	"slices"
)

// Toward is the tile focus moves to from id going d: the nearest tile that
// way, preferring tiles that overlap id's across the direction of travel.
// It is id itself when there's none.
func (f Frame) Toward(id string, d Dir) string {
	t, ok := f.Tile(id)
	if !ok {
		return id
	}
	best, bestScore := id, [3]float64{math.Inf(1)}
	for _, o := range f.Tiles {
		if o.ID == id {
			continue
		}
		var gap, overlap int
		switch d {
		case Right:
			gap = o.X - (t.X + t.W)
		case Left:
			gap = t.X - (o.X + o.W)
		case Down:
			gap = o.Y - (t.Y + t.H)
		default:
			gap = t.Y - (o.Y + o.H)
		}
		if gap < 0 {
			continue
		}
		tc, oc := centre(t.Rect), centre(o.Rect)
		off := math.Abs(oc.x - tc.x)
		if d.across() {
			overlap = min(t.Y+t.H, o.Y+o.H) - max(t.Y, o.Y)
			off = math.Abs(oc.y - tc.y)
		} else {
			overlap = min(t.X+t.W, o.X+o.W) - max(t.X, o.X)
		}
		apart := 1.0
		if overlap > 0 {
			apart = 0
		}
		if score := [3]float64{apart, float64(gap), off}; slices.Compare(score[:], bestScore[:]) < 0 {
			best, bestScore = o.ID, score
		}
	}
	return best
}

// NextRow is Tab: the tile in the top-level row step rows on from id's,
// wrapping, nearest id's centre horizontally (the higher one on a tie).
func (f Frame) NextRow(id string, step int) string {
	t, ok := f.Tile(id)
	if !ok {
		return id
	}
	var rows []int
	for _, o := range f.Tiles {
		if n := len(rows); n == 0 || rows[n-1] != o.Path[0] {
			rows = append(rows, o.Path[0])
		}
	}
	k := 0
	for i, r := range rows {
		if r == t.Path[0] {
			k = i
		}
	}
	next := rows[((k+step)%len(rows)+len(rows))%len(rows)]
	cx := t.X + t.W/2
	best, bestScore := id, [2]float64{math.Inf(1)}
	for _, o := range f.Tiles {
		if o.Path[0] != next {
			continue
		}
		d := 0
		if cx < o.X {
			d = o.X - cx
		} else if cx >= o.X+o.W {
			d = cx - (o.X + o.W - 1)
		}
		if score := [2]float64{float64(d), float64(o.Y)}; slices.Compare(score[:], bestScore[:]) < 0 {
			best, bestScore = o.ID, score
		}
	}
	return best
}
