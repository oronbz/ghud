package host

import (
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
)

// filesChangedMsg says files ghud watches changed on disk.
type filesChangedMsg struct{ files []string }

// watch points the file watcher at every file ghud reads: config.toml,
// layout.json and each Watcher Widget's files.
func (m Model) watch() Model {
	files := []string{m.layoutFile()}
	for _, id := range m.ids() {
		for _, f := range watched(m.widgets[id].widget) {
			if !slices.Contains(files, f) {
				files = append(files, f)
			}
		}
	}
	if err := m.opts.Ports.Watcher.Watch(files); err != nil {
		m.toast = toast{text: "Watching files: " + err.Error(), tok: theme.Bad}
	}
	return m
}

func (m Model) nextChange() tea.Cmd {
	return m.opts.Ports.Watcher.Next(func(files []string) tea.Msg { return filesChangedMsg{files} })
}

// filesChanged reloads what the changed files hold.
func (m Model) filesChanged(msg filesChangedMsg) (Model, tea.Cmd) {
	cmds := []tea.Cmd{m.nextChange()}
	if slices.Contains(msg.files, m.layoutFile()) {
		m = m.reloadLayout()
	}
	for _, id := range m.ids() {
		if slices.ContainsFunc(watched(m.widgets[id].widget), func(f string) bool { return slices.Contains(msg.files, f) }) {
			cmds = append(cmds, m.refresh(id))
		}
	}
	return m, tea.Batch(cmds...)
}

func (m Model) layoutFile() string { return filepath.Join(m.opts.Paths.Config, layout.FileName) }

// reloadLayout adopts the Layout another instance saved: the last writer
// wins. Mid-arrangement it becomes what esc reverts to; a Layout that
// doesn't load leaves the current one.
func (m Model) reloadLayout() Model {
	saved, err := layout.Load(m.opts.Paths.Config)
	if err != nil {
		return m
	}
	l := saved.Join(m.ids())
	if m.arranging {
		m.before = l
		return m
	}
	m.layout = l
	if _, ok := m.rects()[m.focus]; !ok {
		m.focus = ""
	}
	for _, id := range m.ids() {
		m.clampList(id)
	}
	m.sizeEditor()
	return m
}
