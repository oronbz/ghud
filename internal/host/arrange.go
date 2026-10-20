package host

import (
	"reflect"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/visor/internal/layout"
	"github.com/oronbz/visor/internal/theme"
)

// arrangeKey handles a key in arrange mode, which works on the focused Widget.
// Only ⏎ writes layout.json, and only when the Layout changed since m.
func (m Model) arrangeKey(msg tea.KeyPressMsg) Model {
	switch {
	case key.Matches(msg, keys.Keep):
		m.arranging, m.via = false, layout.Via{}
		if !reflect.DeepEqual(m.layout, m.before) {
			m = m.saveLayout()
		}
	case key.Matches(msg, keys.Revert):
		m.arranging, m.layout, m.via = false, m.before, layout.Via{}
	case key.Matches(msg, keys.Shift):
		m.layout, m.via = m.layout.Move(m.focus, dirOf(msg), layout.Size{W: m.w, H: m.h - footerHeight}, m.via)
	case key.Matches(msg, keys.Grow):
		m.layout, m.via = m.layout.Resize(m.focus, dirOf(msg)), layout.Via{}
	}
	return m
}

func dirOf(msg tea.KeyPressMsg) layout.Dir {
	switch unicode.ToLower(msg.Code) {
	case 'h', tea.KeyLeft:
		return layout.Left
	case 'j', tea.KeyDown:
		return layout.Down
	case 'k', tea.KeyUp:
		return layout.Up
	}
	return layout.Right
}

// saveLayout writes the Layout, leaving out the on-screen-only ids of
// duplicate Widgets, and reports a failure as a toast.
func (m Model) saveLayout() Model {
	var ids []string
	for id := range m.widgets {
		if !strings.Contains(id, "~") {
			ids = append(ids, id)
		}
	}
	if err := layout.Save(m.opts.Paths.Config, m.layout.Join(ids)); err != nil {
		m.toast = toast{text: "layout.json: " + err.Error(), tok: theme.Bad}
	}
	return m
}
