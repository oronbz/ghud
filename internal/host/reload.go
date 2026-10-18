package host

import (
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/theme"
)

// filesChangedMsg says files ghud watches changed on disk.
type filesChangedMsg struct{ files []string }

// watch points the file watcher at every file ghud reads: config.toml,
// layout.json and each Watcher Widget's files.
func (m Model) watch() Model {
	var files []string
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
	for _, id := range m.ids() {
		if slices.ContainsFunc(watched(m.widgets[id].widget), func(f string) bool { return slices.Contains(msg.files, f) }) {
			cmds = append(cmds, m.refresh(id))
		}
	}
	return m, tea.Batch(cmds...)
}
