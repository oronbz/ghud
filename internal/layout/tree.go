package layout

import "slices"

// shown reports whether c draws: a Widget that isn't hidden, or a stack
// with something shown in it.
func (l Layout) shown(c Cell) bool {
	if c.IsStack() {
		return slices.ContainsFunc(c.Rows, l.rowShown)
	}
	return c.ID != "" && !l.IsHidden(c.ID)
}

func (l Layout) rowShown(r Row) bool { return slices.ContainsFunc(r.Widgets, l.shown) }

func (l Layout) IsHidden(id string) bool { return slices.Contains(l.Hidden, id) }

// rowsAt is the rows of the container at p: the dashboard's, or a stack's.
func (l *Layout) rowsAt(p Path) *[]Row {
	if len(p) == 0 {
		return &l.Rows
	}
	return &l.cellAt(p).Rows
}

func (l *Layout) rowAt(p Path) *Row { return &(*l.rowsAt(p[:len(p)-1]))[p[len(p)-1]] }

func (l *Layout) cellAt(p Path) *Cell { return &l.rowAt(p[:len(p)-1]).Widgets[p[len(p)-1]] }

// has reports whether p addresses a place in l.
func (l Layout) has(p Path) bool {
	rows := l.Rows
	for k, i := range p {
		if k%2 == 0 {
			if i < 0 || i >= len(rows) {
				return false
			}
			continue
		}
		cells := rows[p[k-1]].Widgets
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
			for ci, c := range r.Widgets {
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

// nextShown is the index of the first shown cell after i in step's
// direction, or -1.
func (l Layout) nextShown(cells []Cell, i, step int) int {
	for j := i + step; j >= 0 && j < len(cells); j += step {
		if l.shown(cells[j]) {
			return j
		}
	}
	return -1
}

func (l Layout) nextRowShown(rows []Row, i, step int) int {
	for j := i + step; j >= 0 && j < len(rows); j += step {
		if l.rowShown(rows[j]) {
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
		out[i] = Row{Weight: r.Weight, Widgets: make([]Cell, len(r.Widgets))}
		for j, c := range r.Widgets {
			out[i].Widgets[j] = Cell{ID: c.ID, Weight: c.Weight, Rows: cloneRows(c.Rows)}
		}
	}
	return out
}
