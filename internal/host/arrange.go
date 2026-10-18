package host

import (
	"reflect"
	"strings"
	"unicode"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/layout"
)

// arrangeKey handles a key in arrange mode, which works on the focused Widget.
// Only ⏎ writes layout.json, and only when the Layout changed since m.
func (m Model) arrangeKey(msg tea.KeyPressMsg) Model {
	switch {
	case key.Matches(msg, keys.Keep):
		m.arranging = false
		if !reflect.DeepEqual(m.layout, m.before) {
			m = m.saveLayout()
		}
	case key.Matches(msg, keys.Revert):
		m.arranging, m.layout = false, m.before
	case key.Matches(msg, keys.Shift):
		m.layout = m.layout.Move(m.focus, dirOf(msg))
	case key.Matches(msg, keys.Grow):
		m.layout = m.layout.Resize(m.focus, dirOf(msg))
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
		m.toast = "layout.json: " + err.Error()
	}
	return m
}
