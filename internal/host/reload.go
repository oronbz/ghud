package host

import (
	"path/filepath"
	"slices"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/config"
	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/theme"
)

// filesChangedMsg says files ghud watches changed on disk.
type filesChangedMsg struct{ files []string }

// watch points the file watcher at every file ghud reads: config.toml,
// layout.json and each Watcher Widget's files.
func (m Model) watch() Model {
	files := []string{m.edits.Path, m.layoutFile()}
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
	var fresh []string
	if slices.Contains(msg.files, m.edits.Path) {
		var cmd tea.Cmd
		m, fresh, cmd = m.reloadConfig()
		cmds = append(cmds, cmd)
	}
	if slices.Contains(msg.files, m.layoutFile()) {
		m = m.reloadLayout()
	}
	for _, id := range m.ids() {
		if slices.Contains(fresh, id) || slices.ContainsFunc(watched(m.widgets[id].widget), func(f string) bool { return slices.Contains(msg.files, f) }) {
			cmds = append(cmds, m.refresh(id))
		}
	}
	return m, tea.Batch(cmds...)
}

// reloadConfig applies config.toml as it now is, returning the Widgets it
// built afresh, which need a refresh. A config that doesn't parse leaves
// everything as it was and says why until it parses again.
func (m Model) reloadConfig() (Model, []string, tea.Cmd) {
	cfg, err := config.Load(m.edits.Path, m.edits.Types, m.opts.Locale)
	if err != nil {
		m.sticky = err.Error()
		return m, nil, nil
	}
	m.sticky = toastOf(cfg.Problems)
	var cmds []tea.Cmd
	if cfg.Theme != m.colors.name && !m.colors.pinned {
		m.colors.asking = true
		cmds = append(cmds, m.queryColors())
	}
	m.colors.name, m.colors.over = cfg.Theme, cfg.Colors
	m = m.resolveTheme()

	m, ids, fresh := m.placeAll(cfg)
	m.layout = m.layout.Join(ids)
	if m.arranging {
		m.before = m.before.Join(ids)
	}
	for id := range m.fetched {
		if _, ok := m.widgets[id]; !ok {
			delete(m.fetched, id)
			delete(m.lists, id)
			delete(m.due, id)
		}
	}
	if _, ok := m.rects()[m.focus]; !ok {
		m.focus, m.arranging = "", false
	}
	if _, ok := m.widgets[m.last]; !ok {
		m.last = ""
	}
	if m.editing != nil && !slices.Contains(ids, m.editing.widget) {
		m.editing = nil
	}
	if m.overlay != nil && !slices.Contains(ids, m.overlay.widget) {
		m.overlay = nil
	}
	if m.menu != nil && !slices.Contains(ids, m.menu.widget) {
		m.menu = nil
	}
	for _, id := range m.ids() {
		m.clampList(id)
	}
	m.sizeEditor()
	m = m.watch()
	m.ticks++
	return m, fresh, tea.Batch(append(cmds, m.nextTick())...)
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
