// Package layout is the Layout engine: a pure pass from a Layout and a
// terminal size to the tiles to draw and the hit zones to test against, so
// drawing and mouse handling can never disagree about geometry.
package layout

import (
	"math"
	"slices"
	"strconv"
	"strings"
)

// Layout is the saved arrangement: weighted rows of weighted cells, where a
// cell is a Widget or a stack holding rows of its own.
type Layout struct {
	Rows   []Row    `json:"rows"`
	Hidden []string `json:"hidden,omitempty"`
}

type Row struct {
	Weight  float64 `json:"weight"`
	Widgets []Cell  `json:"widgets"`
}

// Cell is a Widget (ID set) or a stack (Rows set), weighted within its row.
type Cell struct {
	ID     string  `json:"id,omitempty"`
	Weight float64 `json:"weight"`
	Rows   []Row   `json:"rows,omitempty"`
}

// IsStack reports whether c holds rows rather than a Widget.
func (c Cell) IsStack() bool { return len(c.Rows) > 0 }

// Path addresses a place in a Layout: row and cell indexes alternate from
// the dashboard down. An odd-length Path is a row; an even-length one is a
// cell, or for a stack also the container of its rows. The empty Path is the
// dashboard itself.
type Path []int

func (p Path) key() string {
	var b strings.Builder
	for _, i := range p {
		b.WriteString(strconv.Itoa(i))
		b.WriteByte('.')
	}
	return b.String()
}

func (p Path) with(i ...int) Path { return append(slices.Clone(p), i...) }

// Default is the built-in Layout used until layout.json exists. Ids are the
// Widget type names; Join drops the ones whose type isn't available.
func Default() Layout {
	w := func(id string, weight float64) Cell { return Cell{ID: id, Weight: weight} }
	return Layout{Rows: []Row{
		{Weight: 12, Widgets: []Cell{w("clock", 3), w("calendar", 3), w("meetings", 4)}},
		{Weight: 12, Widgets: []Cell{w("tasks", 3), w("reminders", 4), w("notes", 3), w("cpu", 3)}},
		{Weight: 12, Widgets: []Cell{w("prs", 6), w("jira", 4), w("weather", 3)}},
	}}
}

// Join fits l to the Widgets that exist, in their configured order: entries
// with no Widget are dropped, what that leaves empty goes, and Widgets the
// Layout doesn't mention are appended in a new last row.
func (l Layout) Join(ids []string) Layout {
	exists := make(map[string]bool, len(ids))
	for _, id := range ids {
		exists[id] = true
	}
	placed := map[string]bool{}
	var keep func(rows []Row) []Row
	keep = func(rows []Row) []Row {
		out := make([]Row, 0, len(rows))
		for _, r := range rows {
			nr := Row{Weight: r.Weight}
			for _, c := range r.Widgets {
				switch {
				case c.IsStack():
					nr.Widgets = append(nr.Widgets, Cell{Weight: c.Weight, Rows: keep(c.Rows)})
				case exists[c.ID] && !placed[c.ID]:
					placed[c.ID] = true
					nr.Widgets = append(nr.Widgets, c)
				}
			}
			out = append(out, nr)
		}
		return out
	}
	out := Layout{Rows: keep(l.Rows)}
	out.Rows = tidy(out.Rows)
	var extra Row
	for _, id := range ids {
		if !placed[id] {
			placed[id] = true
			extra.Widgets = append(extra.Widgets, Cell{ID: id, Weight: 1})
		}
	}
	if len(extra.Widgets) > 0 {
		extra.Weight = 1
		if n := len(out.Rows); n > 0 {
			extra.Weight = out.Rows[n-1].Weight
		}
		out.Rows = append(out.Rows, extra)
	}
	for _, id := range l.Hidden {
		if exists[id] {
			out.Hidden = append(out.Hidden, id)
		}
	}
	return out
}

// IDs lists every Widget in l, hidden or not, in reading order: row by row,
// and through a stack's rows where it stands.
func (l Layout) IDs() []string {
	var out []string
	var walk func(rows []Row)
	walk = func(rows []Row) {
		for _, r := range rows {
			for _, c := range r.Widgets {
				if c.IsStack() {
					walk(c.Rows)
				} else {
					out = append(out, c.ID)
				}
			}
		}
	}
	walk(l.Rows)
	return out
}

// normalise is what every change ends with: tidy, then no weight under
// minShare of its siblings' total, so nothing gets squeezed to nothing.
func (l Layout) normalise() Layout {
	l.Rows = tidy(l.Rows)
	var floor func(rows []Row)
	floor = func(rows []Row) {
		ws := make([]*float64, len(rows))
		for i := range rows {
			ws[i] = &rows[i].Weight
			cs := make([]*float64, len(rows[i].Widgets))
			for j := range rows[i].Widgets {
				c := &rows[i].Widgets[j]
				cs[j] = &c.Weight
				floor(c.Rows)
			}
			atLeast(cs)
		}
		atLeast(ws)
	}
	floor(l.Rows)
	return l
}

func atLeast(ws []*float64) {
	sum := 0.0
	for _, w := range ws {
		sum += *w
	}
	for _, w := range ws {
		*w = max(*w, round3(sum*minShare))
	}
}

// tidy removes empty cells, rows and stacks, turns a one-row stack into
// cells of its parent row and a row holding only a stack into rows of its
// container, rescaling weights so nothing changes size on screen.
func tidy(rows []Row) []Row {
	for {
		next := tidyOnce(rows)
		if slices.EqualFunc(next, rows, sameRow) {
			return next
		}
		rows = next
	}
}

func tidyOnce(rows []Row) []Row {
	var out []Row
	for _, r := range rows {
		var cells []Cell
		for _, c := range r.Widgets {
			if !c.IsStack() {
				if c.ID != "" {
					cells = append(cells, c)
				}
				continue
			}
			c.Rows = tidyOnce(c.Rows)
			switch len(c.Rows) {
			case 0:
			case 1:
				cells = append(cells, rescale(c.Rows[0].Widgets, cellWeights, c.Weight, len(r.Widgets) == 1)...)
			default:
				cells = append(cells, c)
			}
		}
		switch {
		case len(cells) == 0:
		case len(cells) == 1 && cells[0].IsStack():
			inner := make([]Row, len(cells[0].Rows))
			copy(inner, cells[0].Rows)
			out = append(out, rescale(inner, rowWeights, r.Weight, len(rows) == 1)...)
		default:
			out = append(out, Row{Weight: r.Weight, Widgets: cells})
		}
	}
	return out
}

// rescale gives xs the total weight of the one they replace. When they
// replace the only one in their place, proportions are all that matter, so
// the weights are kept as they are.
func rescale[T any](xs []T, weight func(*T) *float64, total float64, alone bool) []T {
	out := slices.Clone(xs)
	if alone {
		return out
	}
	sum := 0.0
	for i := range out {
		sum += *weight(&out[i])
	}
	for i := range out {
		if sum > 0 {
			*weight(&out[i]) = round3(*weight(&out[i]) / sum * total)
		}
	}
	return out
}

func cellWeights(c *Cell) *float64 { return &c.Weight }

func rowWeights(r *Row) *float64 { return &r.Weight }

func sameRow(a, b Row) bool {
	return a.Weight == b.Weight && slices.EqualFunc(a.Widgets, b.Widgets, func(x, y Cell) bool {
		return x.ID == y.ID && x.Weight == y.Weight && slices.EqualFunc(x.Rows, y.Rows, sameRow)
	})
}

func round3(x float64) float64 { return max(math.Round(x*1000)/1000, 0.001) }
