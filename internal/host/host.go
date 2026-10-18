// Package host is the ghud Bubble Tea program: it owns the Widgets, input
// modes and focus, and draws the dashboard. Everything outside the process
// goes through Options.Ports.
package host

import (
	"fmt"
	"image"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/config"
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
	// ConfigFile overrides Paths.Config/config.toml (--config, GHUD_CONFIG).
	ConfigFile string
	// Locale gives time_format and week_start their defaults.
	Locale config.Locale
	Ports  ports.Ports
	Theme  theme.Theme
}

type Model struct {
	opts    Options
	w, h    int
	now     time.Time
	widgets map[string]placed
	layout  layout.Layout
	// toast is the message on the bottom line; it stays until replaced.
	toast string
}

type placed struct {
	typ    widget.Type
	title  string
	widget widget.Widget
	// setup, when set, is the needs-setup hint drawn instead of the Widget.
	setup string
}

type tickMsg struct{ at time.Time }

var quitKey = key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "quit"))

// Builtins are the Widget types compiled into ghud, in reference order.
var Builtins = []widget.Type{clock.Type, calendar.Type}

func New(o Options) Model {
	m := Model{opts: o, now: o.Ports.Clock.Now(), widgets: map[string]placed{}}
	file := o.ConfigFile
	if file == "" {
		file = filepath.Join(o.Paths.Config, config.FileName)
	}
	cfg, err := config.Load(file, Builtins, o.Locale)
	var problems []string
	if err != nil {
		problems = append(problems, err.Error())
	}
	m.toast = toastOf(append(problems, cfg.Problems...))
	var ids []string
	for _, wc := range cfg.Widgets {
		id := wc.ID
		// Widgets sharing an id are all in needs setup; suffixes keep them apart on screen only.
		for n := 2; m.widgets[id].title != ""; n++ {
			id = fmt.Sprintf("%s~%d", wc.ID, n)
		}
		p := placed{title: wc.Title, setup: wc.Setup}
		if i := slices.IndexFunc(Builtins, func(t widget.Type) bool { return t.Name == wc.Type }); i >= 0 {
			p.typ = Builtins[i]
		}
		if p.setup == "" {
			p.widget = p.typ.New(widget.Config{Settings: wc.Settings, TimeFormat: cfg.TimeFormat, WeekStart: cfg.WeekStart})
		}
		m.widgets[id] = p
		ids = append(ids, id)
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
		if w.setup == "" && w.typ.Tick > 0 && (d == 0 || w.typ.Tick < d) {
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
	f := m.frame()
	if f.TooSmall {
		v.SetContent(m.tooSmall(f.Need))
		return v
	}
	layers := make([]*lipgloss.Layer, 0, len(f.Tiles))
	for _, t := range f.Tiles {
		layers = append(layers, lipgloss.NewLayer(render.Draw(m.opts.Theme, render.Tile{
			Title:   t.Title,
			W:       t.W,
			H:       t.H,
			Payload: m.payload(t.ID, widget.Size{W: t.W - 2, H: t.H - 2}),
		})).X(t.X).Y(t.Y))
	}
	if m.toast != "" {
		text := m.opts.Theme.Fg(theme.Warn).Render(ansi.Truncate(" "+m.toast+" ", m.w-2, "… "))
		layers = append(layers, lipgloss.NewLayer(text).X(1).Y(m.h-1).Z(1))
	}
	canvas := lipgloss.NewCanvas(m.w, m.h)
	lipgloss.NewCompositor(layers...).Draw(canvas, image.Rect(0, 0, m.w, m.h))
	v.SetContent(canvas.Render())
	return v
}

func (m Model) payload(id string, size widget.Size) widget.Payload {
	w := m.widgets[id]
	if w.setup != "" {
		return widget.Payload{Center: true, Lines: []widget.Line{
			widget.Text(theme.Warn, "needs setup"),
			widget.Text(theme.Dim, w.setup),
		}}
	}
	return w.widget.Payload(widget.Frame{Now: m.now, Size: size})
}

// toastOf shows the first message, counting the rest.
func toastOf(msgs []string) string {
	switch len(msgs) {
	case 0:
		return ""
	case 1:
		return msgs[0]
	}
	return fmt.Sprintf("%s (+%d more)", msgs[0], len(msgs)-1)
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
	titles := make(map[string]string, len(m.widgets))
	for id, w := range m.widgets {
		titles[id] = w.title
	}
	return layout.Compute(m.layout, titles, m.w, m.h)
}
