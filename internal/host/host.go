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
	"github.com/oronbz/ghud/internal/widgets/tasks"
)

type Options struct {
	Paths paths.Paths
	// ConfigFile overrides Paths.Config/config.toml (--config, GHUD_CONFIG).
	ConfigFile string
	// Locale gives time_format and week_start their defaults.
	Locale config.Locale
	Ports  ports.Ports
	Theme  theme.Theme
	// Types replaces the built-in Widget types; with no config.toml there is
	// one Widget of each. Tests use it to stage Layouts the built-ins can't fill.
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
	toast       toast
	// fetched holds each Source's last Payload; lists, each list's selection.
	// Like every map here they're shared by Model copies, and only Update writes them.
	fetched map[string]widget.Payload
	lists   map[string]listState
	// overlay is the open confirm or text prompt, which takes every key.
	overlay *overlay
	// held is the modifier (Shift or Ctrl) held on its own, which swaps the
	// footer to that modifier's keys; only kitty-protocol terminals report it.
	held tea.KeyMod
}

type placed struct {
	typ    widget.Type
	title  string
	widget widget.Widget
	// setup, when set, is the needs-setup hint drawn instead of the Widget.
	setup string
}

type tickMsg struct{ at time.Time }

// Builtins are the Widget types compiled into ghud, in reference order.
var Builtins = []widget.Type{clock.Type, calendar.Type, tasks.Type}

func New(o Options) Model {
	m := Model{opts: o, now: o.Ports.Clock.Now(), widgets: map[string]placed{},
		fetched: map[string]widget.Payload{}, lists: map[string]listState{}}
	types := o.Types
	if types == nil {
		types = Builtins
	}
	file := o.ConfigFile
	if file == "" {
		file = filepath.Join(o.Paths.Config, config.FileName)
	}
	cfg, err := config.Load(file, types, o.Locale)
	var problems []string
	if err != nil {
		problems = append(problems, err.Error())
	}
	m.toast = toast{text: toastOf(append(problems, cfg.Problems...)), tok: theme.Warn}
	var ids []string
	for _, wc := range cfg.Widgets {
		id := wc.ID
		// Widgets sharing an id are all in needs setup; suffixes keep them apart on screen only.
		for n := 2; m.widgets[id].title != ""; n++ {
			id = fmt.Sprintf("%s~%d", wc.ID, n)
		}
		p := placed{title: wc.Title, setup: wc.Setup}
		if i := slices.IndexFunc(types, func(t widget.Type) bool { return t.Name == wc.Type }); i >= 0 {
			p.typ = types[i]
		}
		if p.setup == "" {
			p.widget = p.typ.New(widget.Config{Settings: wc.Settings, TimeFormat: cfg.TimeFormat, WeekStart: cfg.WeekStart, DataDir: o.Paths.Data})
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

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.nextTick()}
	for _, id := range m.ids() {
		cmds = append(cmds, m.refresh(id))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		for _, id := range m.ids() {
			m.clampList(id)
		}
	case refreshedMsg:
		return m.refreshed(msg)
	case actedMsg:
		return m.acted(msg)
	case toastExpiredMsg:
		if m.toast.seq == msg.seq {
			m.toast = toast{}
		}
	case tickMsg:
		m.now = msg.at
		return m, m.nextTick()
	case tea.KeyPressMsg:
		return m.key(msg)
	case tea.KeyReleaseMsg:
		if _, ok := loneModifier(msg.Code); ok {
			m.held = 0
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
	// Report-all-keys and event types make lone modifier presses and releases
	// arrive, for the modifier footer.
	v.KeyboardEnhancements = tea.KeyboardEnhancements{
		ReportEventTypes:           true,
		ReportAlternateKeys:        true,
		ReportAllKeysAsEscapeCodes: true,
		ReportAssociatedText:       true,
	}
	if m.w == 0 || m.h == 0 {
		return v
	}
	var layers []*lipgloss.Layer
	if f := m.frame(); f.TooSmall {
		layers = append(layers, lipgloss.NewLayer(m.tooSmall(f.Need)))
	} else {
		for _, t := range f.Tiles {
			tile := render.Tile{
				Title:   t.Title,
				W:       t.W,
				H:       t.H,
				Focused: t.ID == m.focus,
				Payload: m.payloadIn(t.ID, t.Rect),
				List:    m.lists[t.ID].render(),
			}
			if tile.Focused {
				tile.Hints = m.hints(t.ID)
			}
			layers = append(layers, lipgloss.NewLayer(render.Draw(m.opts.Theme, tile)).X(t.X).Y(t.Y))
		}
		layers = append(layers, lipgloss.NewLayer(m.footer()).Y(m.h-footerHeight))
	}
	if m.keyMap {
		layers = append(layers, m.keyMapLayer())
	}
	if m.overlay != nil {
		layers = append(layers, m.overlayLayer())
	}
	canvas := lipgloss.NewCanvas(m.w, m.h)
	lipgloss.NewCompositor(layers...).Draw(canvas, image.Rect(0, 0, m.w, m.h))
	v.SetContent(canvas.Render())
	return v
}

// payloadIn is what Widget id shows in a tile at r, border included.
func (m Model) payloadIn(id string, r layout.Rect) widget.Payload {
	w := m.widgets[id]
	if w.setup != "" {
		return widget.Payload{Center: true, Lines: []widget.Line{
			widget.Text(theme.Warn, "needs setup"),
			widget.Text(theme.Dim, w.setup),
		}}
	}
	switch v := w.widget.(type) {
	case widget.Source:
		return m.fetched[id]
	case widget.View:
		return v.Payload(widget.Frame{Now: m.now, Size: widget.Size{W: r.W - 2, H: r.H - 2}, Focused: id == m.focus})
	}
	return widget.Payload{}
}

// ids lists the placed Widgets in a stable order.
func (m Model) ids() []string {
	ids := make([]string, 0, len(m.widgets))
	for id := range m.widgets {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	return ids
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
	f := layout.Compute(m.layout, m.titles(), m.w, m.h-footerHeight)
	f.Need.H += footerHeight
	return f
}

func (m Model) titles() map[string]string {
	titles := make(map[string]string, len(m.widgets))
	for id, w := range m.widgets {
		titles[id] = w.title
	}
	return titles
}
