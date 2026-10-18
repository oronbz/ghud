// Package host is the ghud Bubble Tea program: it owns the Widgets, input
// modes and focus, and draws the dashboard. Everything outside the process
// goes through Options.Ports.
package host

import (
	"fmt"
	"image"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/layout"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/calendar"
	"github.com/oronbz/ghud/internal/widgets/clock"
)

type Options struct {
	Paths paths.Paths
	Ports ports.Ports
	Theme theme.Theme
	// Types replaces the built-in Widget types, one Widget each; tests use it
	// to stage Layouts the built-ins can't fill.
	Types []widget.Type
}

type Model struct {
	opts    Options
	w, h    int
	now     time.Time
	widgets map[string]placed
	layout  layout.Layout
	// focus is the focused Widget's id, "" when ghud is unfocused; last
	// survives unfocusing so focus can return to it.
	focus, last string
	keyMap      bool
}

type placed struct {
	typ    widget.Type
	widget widget.Widget
}

type tickMsg struct{ at time.Time }

// builtins are the Widget types compiled into ghud, in config order.
var builtins = []widget.Type{clock.Type, calendar.Type}

func New(o Options) Model {
	m := Model{opts: o, now: o.Ports.Clock.Now(), widgets: map[string]placed{}}
	types := o.Types
	if types == nil {
		types = builtins
	}
	var ids []string
	for _, t := range types {
		m.widgets[t.Name] = placed{typ: t, widget: t.New()}
		ids = append(ids, t.Name)
	}
	// An unreadable layout.json falls back to the default rather than blocking launch.
	saved, err := layout.Load(o.Paths.Config)
	if err != nil {
		saved = layout.Default()
	}
	m.layout = saved.Join(ids)
	return m
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
		return m.key(msg)
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
	if m.w == 0 || m.h == 0 {
		return v
	}
	var layers []*lipgloss.Layer
	if f := m.frame(); f.TooSmall {
		layers = append(layers, lipgloss.NewLayer(m.tooSmall(f.Need)))
	} else {
		for _, t := range f.Tiles {
			w := m.widgets[t.ID]
			layers = append(layers, lipgloss.NewLayer(render.Draw(m.opts.Theme, render.Tile{
				Title:   t.Title,
				W:       t.W,
				H:       t.H,
				Focused: t.ID == m.focus,
				Payload: w.widget.Payload(widget.Frame{
					Now:     m.now,
					Size:    widget.Size{W: t.W - 2, H: t.H - 2},
					Focused: t.ID == m.focus,
				}),
			})).X(t.X).Y(t.Y))
		}
		layers = append(layers, lipgloss.NewLayer(m.footer()).Y(m.h-footerHeight))
	}
	if m.keyMap {
		layers = append(layers, m.keyMapLayer())
	}
	canvas := lipgloss.NewCanvas(m.w, m.h)
	lipgloss.NewCompositor(layers...).Draw(canvas, image.Rect(0, 0, m.w, m.h))
	v.SetContent(canvas.Render())
	return v
}

func (m Model) tooSmall(need layout.Size) string {
	text := ansi.Truncate(fmt.Sprintf("too small (needs %d×%d)", need.W, need.H), m.w, "…")
	pad := (m.w - ansi.StringWidth(text)) / 2
	lines := make([]string, m.h)
	lines[(m.h-1)/2] = strings.Repeat(" ", pad) + m.opts.Theme.Fg(theme.Warn).Render(text)
	return strings.Join(lines, "\n")
}

// Zones are the hit zones of the frame View draws.
func (m Model) Zones() []layout.Zone { return m.frame().Zones }

func (m Model) frame() layout.Frame {
	f := layout.Compute(m.layout, m.titles(), m.w, m.h-footerHeight)
	f.Need.H += footerHeight
	return f
}

func (m Model) titles() map[string]string {
	titles := make(map[string]string, len(m.widgets))
	for id, w := range m.widgets {
		titles[id] = w.typ.Title
	}
	return titles
}
