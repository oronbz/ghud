package host

import tea "charm.land/bubbletea/v2"

// picker is the w overlay that shows and hides Widgets.
type picker struct{}

func (m Model) pickerKey(msg tea.KeyPressMsg) Model {
	m.picker = nil
	return m
}
