package host

import (
	"strings"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var pickerKeys = struct{ Down, Up, Toggle, Close key.Binding }{
	Down:   key.NewBinding(key.WithKeys("j", "down")),
	Up:     key.NewBinding(key.WithKeys("k", "up")),
	Toggle: key.NewBinding(key.WithKeys("space", "enter")),
	Close:  key.NewBinding(key.WithKeys("w", "esc", "q")),
}

// pickerKey handles a key in the w list. Every toggle is written at once.
func (m Model) pickerKey(msg tea.KeyPressMsg) (Model, tea.Cmd) {
	ids := m.pickerIDs()
	switch {
	case key.Matches(msg, pickerKeys.Close):
		m.picking = false
	case key.Matches(msg, pickerKeys.Down):
		m.cursor = min(m.cursor+1, len(ids)-1)
	case key.Matches(msg, pickerKeys.Up):
		m.cursor = max(m.cursor-1, 0)
	case key.Matches(msg, pickerKeys.Toggle) && len(ids) > 0:
		id := ids[m.cursor]
		m = m.setHidden(id, !m.layout.IsHidden(id))
		if !m.layout.IsHidden(id) {
			return m.shown(id)
		}
	}
	return m, nil
}

// setHidden shows or hides Widget id and writes the Layout at once. Hiding
// the focused Widget unfocuses ghud.
func (m Model) setHidden(id string, hidden bool) Model {
	m.layout = m.layout.SetHidden(id, hidden)
	if m.layout.IsHidden(m.focus) {
		m.focus = ""
	}
	return m.saveLayout()
}

// pickerIDs lists every Widget, hidden or not, in Layout order.
func (m Model) pickerIDs() []string {
	var ids []string
	for _, r := range m.layout.Rows {
		for _, c := range r.Widgets {
			ids = append(ids, c.ID)
		}
	}
	return ids
}

func (m Model) pickerLayer() *lipgloss.Layer {
	ids := m.pickerIDs()
	textW := 0
	for _, id := range ids {
		textW = max(textW, ansi.StringWidth(m.widgets[id].title))
	}
	lines := make([]widget.Line, 0, len(ids))
	for i, id := range ids {
		mark, tok := "[x] ", theme.Fg
		if m.layout.IsHidden(id) {
			mark, tok = "[ ] ", theme.Dim
		}
		bg := theme.Bg
		if i == m.cursor {
			bg = theme.Sel
		}
		title := m.widgets[id].title
		text := " " + mark + title + strings.Repeat(" ", textW-ansi.StringWidth(title)+1)
		lines = append(lines, widget.Line{Spans: []widget.Span{{Text: text, Token: tok, Bg: bg}}})
	}
	w := min(m.w, textW+8)
	h := min(m.h, len(lines)+2)
	box := render.Draw(m.opts.Theme, render.Tile{Title: "Widgets", W: w, H: h, Focused: true, Payload: widget.Payload{Lines: lines}})
	return lipgloss.NewLayer(box).X((m.w - w) / 2).Y((m.h - h) / 2).Z(1)
}
