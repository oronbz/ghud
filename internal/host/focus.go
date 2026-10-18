package host

import (
	"math"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/layout"
)

type keyMap struct {
	Quit, Unfocus, NextRow, PrevRow, Left, Right key.Binding
	MoveLeft, MoveDown, MoveUp, MoveRight        key.Binding
	Refocus, Jump, Help                          key.Binding
}

var keys = keyMap{
	Quit:      key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit")),
	Unfocus:   key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "unfocus")),
	NextRow:   key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next row")),
	PrevRow:   key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("S-tab", "previous row")),
	Left:      key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h ←", "previous Widget in the row")),
	Right:     key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l →", "next Widget in the row")),
	MoveLeft:  key.NewBinding(key.WithKeys("H", "shift+left"), key.WithHelp("H S-←", "focus left")),
	MoveDown:  key.NewBinding(key.WithKeys("J", "shift+down"), key.WithHelp("J S-↓", "focus down")),
	MoveUp:    key.NewBinding(key.WithKeys("K", "shift+up"), key.WithHelp("K S-↑", "focus up")),
	MoveRight: key.NewBinding(key.WithKeys("L", "shift+right"), key.WithHelp("L S-→", "focus right")),
	Refocus:   key.NewBinding(key.WithKeys("m"), key.WithHelp("m", "focus the last-focused Widget")),
	Jump:      key.NewBinding(key.WithKeys("1", "2", "3", "4", "5", "6", "7", "8", "9"), key.WithHelp("1–9", "jump to the Nth Widget")),
	Help:      key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "key map")),
}

// Bindings lists every host key binding, in key map order.
func Bindings() []key.Binding {
	k := keys
	return []key.Binding{
		k.NextRow, k.PrevRow, k.Left, k.Right, k.MoveLeft, k.MoveDown, k.MoveUp, k.MoveRight,
		k.Jump, k.Refocus, k.Unfocus, k.Help, k.Quit,
	}
}

func (m Model) key(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	if mod, ok := loneModifier(msg.Code); ok {
		if !msg.IsRepeat {
			m.held = mod
		}
		return m, nil
	}
	// Any other key means the modifier is no longer held alone, and a lost
	// release can't leave the footer stuck.
	m.held = 0
	if m.keyMap {
		if key.Matches(msg, keys.Help, keys.Unfocus, keys.Quit) {
			m.keyMap = false
		}
		return m, nil
	}
	switch {
	case key.Matches(msg, keys.Help):
		m.keyMap = true
	case key.Matches(msg, keys.Quit):
		return m, tea.Quit
	case key.Matches(msg, keys.Unfocus):
		m.focus = ""
	case key.Matches(msg, keys.Refocus):
		if m.focus == "" {
			m.focus = m.lastFocus()
		}
	case key.Matches(msg, keys.Jump):
		if id := m.nth(int(msg.Code - '1')); id != "" {
			m.focus = id
		}
	case key.Matches(msg, keys.NextRow, keys.PrevRow, keys.Left, keys.Right, keys.MoveLeft, keys.MoveDown, keys.MoveUp, keys.MoveRight):
		if m.focus == "" {
			m.focus = m.lastFocus()
			break
		}
		ri, ci := m.find(m.focus)
		rows := m.layout.Rows
		switch {
		case key.Matches(msg, keys.Left, keys.MoveLeft):
			m.focus = rows[ri].Widgets[max(ci-1, 0)].ID
		case key.Matches(msg, keys.Right, keys.MoveRight):
			m.focus = rows[ri].Widgets[min(ci+1, len(rows[ri].Widgets)-1)].ID
		case key.Matches(msg, keys.MoveDown):
			m.focus = m.nearest(min(ri+1, len(rows)-1))
		case key.Matches(msg, keys.MoveUp):
			m.focus = m.nearest(max(ri-1, 0))
		case key.Matches(msg, keys.NextRow):
			m.focus = m.nearest((ri + 1) % len(rows))
		case key.Matches(msg, keys.PrevRow):
			m.focus = m.nearest((ri - 1 + len(rows)) % len(rows))
		}
	}
	if m.focus != "" {
		m.last = m.focus
	}
	return m, nil
}

// nearest is the Widget in row ri horizontally closest to the focused
// Widget's centre.
func (m Model) nearest(ri int) string {
	rects := m.rects()
	cur := rects[m.focus]
	cx := cur.X + cur.W/2
	best, bestD := "", math.MaxInt
	for _, c := range m.layout.Rows[ri].Widgets {
		r := rects[c.ID]
		d := 0
		if cx < r.X {
			d = r.X - cx
		} else if cx >= r.X+r.W {
			d = cx - (r.X + r.W - 1)
		}
		if d < bestD {
			best, bestD = c.ID, d
		}
	}
	return best
}

// rects places every Widget. While the terminal is too small to draw the
// grid, it lays out at the minimum size so focus still moves sensibly.
func (m Model) rects() map[string]layout.Rect {
	f := m.frame()
	if f.TooSmall {
		f = layout.Compute(m.layout, m.titles(), f.Need.W, f.Need.H)
	}
	out := make(map[string]layout.Rect, len(f.Tiles))
	for _, t := range f.Tiles {
		out[t.ID] = t.Rect
	}
	return out
}

// nth is the id of the Nth Widget counting row by row from 0, or "".
func (m Model) nth(n int) string {
	for _, r := range m.layout.Rows {
		if n < len(r.Widgets) {
			return r.Widgets[n].ID
		}
		n -= len(r.Widgets)
	}
	return ""
}

// lastFocus is the Widget focus returns to: the last one focused, or the
// first on a fresh start or once that one has left the Layout.
func (m Model) lastFocus() string {
	if ri, _ := m.find(m.last); ri >= 0 {
		return m.last
	}
	return m.nth(0)
}

func (m Model) find(id string) (row, index int) {
	for ri, r := range m.layout.Rows {
		for ci, c := range r.Widgets {
			if c.ID == id {
				return ri, ci
			}
		}
	}
	return -1, -1
}
