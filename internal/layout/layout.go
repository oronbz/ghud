// Package layout is the Layout engine: a pure pass from a Layout and a
// terminal size to the tiles to draw and the hit zones to test against, so
// drawing and mouse handling can never disagree about geometry.
package layout

// Layout is the saved arrangement: weighted rows of weighted Widgets.
type Layout struct {
	Rows   []Row    `json:"rows"`
	Hidden []string `json:"hidden,omitempty"`
}

type Row struct {
	Weight  float64 `json:"weight"`
	Widgets []Cell  `json:"widgets"`
}

type Cell struct {
	ID     string  `json:"id"`
	Weight float64 `json:"weight"`
}

// Default is the built-in Layout used until layout.json exists. Ids are the
// Widget type names; Join drops the ones whose type isn't available.
func Default() Layout {
	return Layout{Rows: []Row{
		{Weight: 10, Widgets: []Cell{{"clock", 3}, {"calendar", 3}, {"meetings", 5}}},
		{Weight: 12, Widgets: []Cell{{"tasks", 4}, {"notes", 4}, {"cpu", 4}}},
		{Weight: 16, Widgets: []Cell{{"prs", 6}, {"jira", 4}, {"weather", 3}}},
	}}
}

// Join fits l to the Widgets that exist, in their configured order: entries
// with no Widget are dropped, empty rows go, and Widgets the Layout doesn't
// mention are appended in a new last row.
func (l Layout) Join(ids []string) Layout {
	exists := make(map[string]bool, len(ids))
	for _, id := range ids {
		exists[id] = true
	}
	placed := map[string]bool{}
	var out Layout
	for _, r := range l.Rows {
		nr := Row{Weight: r.Weight}
		for _, c := range r.Widgets {
			if exists[c.ID] && !placed[c.ID] {
				placed[c.ID] = true
				nr.Widgets = append(nr.Widgets, c)
			}
		}
		if len(nr.Widgets) > 0 {
			out.Rows = append(out.Rows, nr)
		}
	}
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
