package host

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/config"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// dashboard is the palette title of Actions that belong to no Widget.
const dashboard = "Dashboard"

// configEditedMsg reports a host Action's config.toml edit, made off the UI thread.
type configEditedMsg struct {
	id     string
	action bound
	added  config.Widget
	// line is the block an edit settings Action opens the editor at.
	line int
	err  error
}

// addEntries are the palette's Add Widget Actions, one per Widget type.
func (m Model) addEntries() []entry {
	var es []entry
	for _, t := range m.edits.Types {
		a := bound{Action: widget.Action{ID: "add", Label: "add " + t.Title + " Widget"}, by: addWidget, typ: t.Name}
		es = append(es, entry{title: dashboard, action: a})
	}
	return es
}

// editConfig makes a host Action's edit to config.toml.
func (m Model) editConfig(id string, a bound) tea.Cmd {
	edits, ref := m.edits, m.widgets[id].ref
	return func() tea.Msg {
		msg := configEditedMsg{id: id, action: a}
		switch a.by {
		case addWidget:
			msg.added, msg.err = edits.AddWidget(a.typ)
		case removeWidget:
			msg.err = edits.RemoveWidget(ref)
		case editSettings:
			msg.line, msg.err = edits.Locate(ref)
		}
		return msg
	}
}

// configEdited brings the dashboard in line with an edit to config.toml.
func (m Model) configEdited(msg configEditedMsg) (Model, tea.Cmd) {
	if msg.err != nil {
		return m.say(capitalise(msg.action.Label)+" failed: "+msg.err.Error(), theme.Bad)
	}
	switch msg.action.by {
	case editSettings:
		return m, m.opts.Ports.Editor.Edit(m.edits.Path, msg.line, func(err error) tea.Msg { return editedMsg{id: msg.id, err: err} })
	case removeWidget:
		return m.removed(msg.id)
	}
	return m.added(msg.added)
}

func (m Model) added(wc config.Widget) (Model, tea.Cmd) {
	ticking := slices.ContainsFunc(m.ids(), func(id string) bool {
		_, ok := m.widgets[id].widget.(widget.Ticker)
		return ok
	})
	p := m.place(wc, config.Ref{ID: wc.ID})
	m.widgets[wc.ID] = p
	m.layout = m.layout.Join(m.ids())
	m = m.saveLayout()
	m, toast := m.say("Added "+p.title, theme.Fg)
	cmds := []tea.Cmd{toast, m.refresh(wc.ID)}
	// One tick chain serves every live view; start it only if none is running.
	if !ticking {
		cmds = append(cmds, m.nextTick())
	}
	return m, tea.Batch(cmds...)
}

func (m Model) removed(id string) (Model, tea.Cmd) {
	gone := m.widgets[id]
	delete(m.widgets, id)
	delete(m.fetched, id)
	delete(m.lists, id)
	for other, p := range m.widgets {
		if p.ref.ID == gone.ref.ID && p.ref.N > gone.ref.N {
			p.ref.N--
			m.widgets[other] = p
		}
	}
	if m.focus == id {
		m.focus = ""
	}
	if m.last == id {
		m.last = ""
	}
	m.layout = m.layout.Join(m.ids())
	m = m.saveLayout()
	return m.say("Removed "+gone.title, theme.Fg)
}
