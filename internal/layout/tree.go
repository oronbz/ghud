package layout

import "slices"

// shown reports whether c draws: a Widget that isn't hidden, or a Stack
// with something shown in it.
func (l Layout) shown(c Cell) bool {
	if c.IsStack() {
		return slices.ContainsFunc(c.Rows, l.rowShown)
	}
	return c.ID != "" && !l.IsHidden(c.ID)
}

func (l Layout) rowShown(r Row) bool { return slices.ContainsFunc(r.Cells, l.shown) }

func (l Layout) IsHidden(id string) bool { return slices.Contains(l.Hidden, id) }

// rowsAt is the rows of the container at p: the dashboard's, or a Stack's.
func (l *Layout) rowsAt(p Path) *[]Row {
	if len(p) == 0 {
		return &l.Rows
	}
	return &l.cellAt(p).Rows
}

func (l *Layout) rowAt(p Path) *Row { return &(*l.rowsAt(p.parent()))[p.last()] }

func (l *Layout) cellAt(p Path) *Cell { return &l.rowAt(p.parent()).Cells[p.last()] }

func (l Layout) has(p Path) bool {
	rows := l.Rows
	for k, i := range p {
		if k%2 == 0 {
			if i < 0 || i >= len(rows) {
				return false
			}
			continue
		}
		cells := rows[p[k-1]].Cells
		if i < 0 || i >= len(cells) {
			return false
		}
		rows = cells[i].Rows
	}
	return true
}

// Find is the Path of Widget id's cell, or nil.
func (l Layout) Find(id string) Path {
	var walk func(rows []Row, at Path) Path
	walk = func(rows []Row, at Path) Path {
		for ri, r := range rows {
			for ci, c := range r.Cells {
				switch {
				case c.IsStack():
					if p := walk(c.Rows, at.with(ri, ci)); p != nil {
						return p
					}
				case c.ID == id:
					return at.with(ri, ci)
				}
			}
		}
		return nil
	}
	return walk(l.Rows, Path{})
}

// shownWeights points at the weights of the shown xs, i being where xs[at]
// is among them, or -1.
func shownWeights[T any](xs []T, shown func(T) bool, weight func(*T) *float64, at int) (ws []*float64, i int) {
	i = -1
	for j := range xs {
		if shown(xs[j]) {
			if j == at {
				i = len(ws)
			}
			ws = append(ws, weight(&xs[j]))
		}
	}
	return ws, i
}

// shownParts are the indexes, weights and least sizes of the shown xs.
func shownParts[T any](xs []T, shown func(T) bool, weight func(*T) *float64, need func(T) int) (idx []int, weights []float64, least []int) {
	for j := range xs {
		if shown(xs[j]) {
			idx = append(idx, j)
			weights = append(weights, *weight(&xs[j]))
			least = append(least, need(xs[j]))
		}
	}
	return idx, weights, least
}

// nextShown is the index of the first of xs after i in step's direction
// that shown, or -1.
func nextShown[T any](xs []T, i, step int, shown func(T) bool) int {
	for j := i + step; j >= 0 && j < len(xs); j += step {
		if shown(xs[j]) {
			return j
		}
	}
	return -1
}

// clone deep-copies l so operations never alias the Layout they started from.
func (l Layout) clone() Layout {
	return Layout{Rows: cloneRows(l.Rows), Hidden: slices.Clone(l.Hidden)}
}

func cloneRows(rows []Row) []Row {
	if rows == nil {
		return nil
	}
	out := make([]Row, len(rows))
	for i, r := range rows {
		out[i] = Row{Weight: r.Weight, Cells: make([]Cell, len(r.Cells))}
		for j, c := range r.Cells {
			out[i].Cells[j] = Cell{ID: c.ID, Weight: c.Weight, Rows: cloneRows(c.Rows)}
		}
	}
	return out
}
