// Package host is the ghud Bubble Tea program: it owns the Widgets, input
// modes and focus, and draws the dashboard. Everything outside the process
// goes through Options.Ports.
package host

import (
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/clock"
)

type Options struct {
	Paths paths.Paths
	Ports ports.Ports
	Theme theme.Theme
}

type Model struct {
	opts    Options
	w, h    int
	now     time.Time
	widgets []placed
}

type placed struct {
	id     string
	typ    widget.Type
	widget widget.Widget
}

type tickMsg struct{ at time.Time }

var quitKey = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))

func New(o Options) Model {
	return Model{
		opts:    o,
		now:     o.Ports.Clock.Now(),
		widgets: []placed{{id: clock.Type.Name, typ: clock.Type, widget: clock.Type.New()}},
	}
}

func (m Model) Init() tea.Cmd { return m.nextTick() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
	case tickMsg:
		m.now = msg.at
		return m, m.nextTick()
	case tea.KeyPressMsg:
		if key.Matches(msg, quitKey) {
			return m, tea.Quit
		}
	}
	return m, nil
}

// nextTick schedules one redraw at the next boundary of the fastest live view,
// so a 1s clock flips on the wall-clock second rather than drifting.
func (m Model) nextTick() tea.Cmd {
	var d time.Duration
	for _, w := range m.widgets {
		if w.typ.Tick > 0 && (d == 0 || w.typ.Tick < d) {
			d = w.typ.Tick
		}
	}
	if d == 0 {
		return nil
	}
	next := m.now.Truncate(d).Add(d)
	return m.opts.Ports.Clock.After(next.Sub(m.now), func(t time.Time) tea.Msg { return tickMsg{at: t} })
}

func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	v.BackgroundColor = m.opts.Theme.Color(theme.Bg)
	if m.w == 0 || m.h == 0 || len(m.widgets) == 0 {
		return v
	}
	w := m.widgets[0]
	v.SetContent(render.Draw(m.opts.Theme, render.Tile{
		Title: w.typ.Title,
		W:     m.w,
		H:     m.h,
		Payload: w.widget.Payload(widget.Frame{
			Now:  m.now,
			Size: widget.Size{W: m.w - 2, H: m.h - 2},
		}),
	}))
	return v
}
