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
	"github.com/oronbz/ghud/internal/widgets/cpu"
	"github.com/oronbz/ghud/internal/widgets/notes"
	"github.com/oronbz/ghud/internal/widgets/tasks"
)

type Options struct {
	Paths paths.Paths
	// ConfigFile overrides Paths.Config/config.toml (--config, GHUD_CONFIG).
	ConfigFile string
	// Locale gives time_format and week_start their defaults.
	Locale config.Locale
	Ports  ports.Ports
	// Theme, when set, pins the palette (tests use sentinel colours). Left
	// zero, New fills it from config.toml and the terminal's colours, and it
	// stays the theme in use as answers arrive.
	Theme theme.Theme
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
	// arranging is arrange mode on the focused Widget; before is the Layout
	// to revert to.
	arranging bool
	before    layout.Layout
	// picking is the w list of Widgets to show or hide, cursor its selected row.
	picking bool
	cursor  int
	toast   toast
	// fetched holds each Source's last Payload; lists, each list's selection.
	// Like every map here they're shared by Model copies, and only Update writes them.
	fetched map[string]widget.Payload
	lists   map[string]listState
	// due is each scheduled Source's current refresh timer.
	due map[string]int
	// overlay is the open confirm or text prompt, which takes every key.
	overlay *overlay
	// editing is INSERT mode's editor, which takes every key but insertHostKeys.
	editing *editing
	// palette is the open ^K Action palette.
	palette *palette
	// held is the modifier (Shift or Ctrl) held on its own, which swaps the
	// footer to that modifier's keys; only kitty-protocol terminals report it.
	held   tea.KeyMod
	colors colors
	// types are the Widget types config.toml can place; cfg is the last
	// config.toml that parsed, read from configFile.
	types      []widget.Type
	cfg        config.Config
	configFile string
	// sticky is the config.toml problem shown whenever no toast is, until the file is fixed.
	sticky string
	// ticks tells the current redraw timer from ones a config reload replaced.
	ticks int
}

type placed struct {
	typ    widget.Type
	title  string
	widget widget.Widget
	// setup, when set, is the needs-setup hint drawn instead of the Widget.
	setup string
	// block is the [[widget]] block the Widget was built from.
	block config.Widget
}

type tickMsg struct {
	at  time.Time
	seq int
}

// Builtins are the Widget types compiled into ghud, in reference order.
var Builtins = []widget.Type{clock.Type, calendar.Type, tasks.Type, cpu.Type, notes.Type}

func New(o Options) Model {
	m := Model{opts: o, now: o.Ports.Clock.Now(), widgets: map[string]placed{},
		fetched: map[string]widget.Payload{}, lists: map[string]listState{}, due: map[string]int{}}
	types := o.Types
	if types == nil {
		types = Builtins
	}
	file := o.ConfigFile
	if file == "" {
		file = filepath.Join(o.Paths.Config, config.FileName)
	}
	m.types, m.configFile = types, file
	cfg, err := config.Load(file, types, o.Locale)
	var problems []string
	if err != nil {
		problems = append(problems, err.Error())
	}
	m.sticky = toastOf(append(problems, cfg.Problems...))
	m.colors = colors{pinned: !o.Theme.IsZero(), name: cfg.Theme, over: cfg.Colors, asking: o.Theme.IsZero()}
	m = m.resolveTheme()
	m, ids, _ := m.place(cfg)
	// An unreadable layout.json falls back to the default rather than blocking launch.
	saved, err := layout.Load(o.Paths.Config)
	if err != nil {
		saved = layout.Default()
	}
	m.layout = saved.Join(ids)
	return m.watch()
}

// place builds cfg's Widgets and returns their ids in config order, and
// the ids of those it built afresh. A Widget whose block, and the globals
// Widgets are built from, haven't changed is kept as it was, state and all.
func (m Model) place(cfg config.Config) (Model, []string, []string) {
	old := m.widgets
	sameGlobals := cfg.TimeFormat == m.cfg.TimeFormat && cfg.WeekStart == m.cfg.WeekStart
	m.widgets, m.cfg = make(map[string]placed, len(cfg.Widgets)), cfg
	var ids, fresh []string
	for _, wc := range cfg.Widgets {
		id := wc.ID
		// Widgets sharing an id are all in needs setup; suffixes keep them apart on screen only.
		for n := 2; m.widgets[id].title != ""; n++ {
			id = fmt.Sprintf("%s~%d", wc.ID, n)
		}
		ids = append(ids, id)
		if p, ok := old[id]; ok && sameGlobals && p.block.Equal(wc) {
			m.widgets[id] = p
			continue
		}
		p := placed{title: wc.Title, setup: wc.Setup, block: wc}
		if i := slices.IndexFunc(m.types, func(t widget.Type) bool { return t.Name == wc.Type }); i >= 0 {
			p.typ = m.types[i]
		}
		if p.setup == "" {
			p.widget = p.typ.New(widget.Config{Settings: wc.Settings, TimeFormat: cfg.TimeFormat, WeekStart: cfg.WeekStart, DataDir: m.opts.Paths.Data, Ports: m.opts.Ports})
		}
		m.widgets[id] = p
		fresh = append(fresh, id)
	}
	return m, ids, fresh
}

func (m Model) Init() tea.Cmd {
	cmds := []tea.Cmd{m.nextTick(), m.queryColors(), m.nextChange()}
	for _, id := range m.ids() {
		cmds = append(cmds, m.refresh(id))
	}
	return tea.Batch(cmds...)
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m, ok := m.colorAnswer(msg); ok {
		return m, nil
	}
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		for _, id := range m.ids() {
			m.clampList(id)
		}
		m.sizeEditor()
	case tea.PasteMsg:
		if m.editing != nil && m.overlay == nil && m.palette == nil {
			return m.editKey(msg)
		}
	case refreshedMsg:
		return m.refreshed(msg)
	case refreshDueMsg:
		return m, m.refreshDue(msg)
	case actedMsg:
		return m.acted(msg)
	case editedMsg:
		return m.edited(msg)
	case filesChangedMsg:
		return m.filesChanged(msg)
	case toastExpiredMsg:
		if m.toast.seq == msg.seq {
			m.toast = toast{}
		}
	case tickMsg:
		if msg.seq != m.ticks {
			return m, nil
		}
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

// nextTick schedules one redraw for the soonest change of any live view.
func (m Model) nextTick() tea.Cmd {
	var next time.Time
	for _, w := range m.widgets {
		if t, ok := w.widget.(widget.Ticker); ok && w.setup == "" {
			if n := t.Next(m.now); next.IsZero() || n.Before(next) {
				next = n
			}
		}
	}
	if next.IsZero() {
		return nil
	}
	seq := m.ticks
	return m.opts.Ports.Clock.After(next.Sub(m.now), func(t time.Time) tea.Msg { return tickMsg{at: t, seq: seq} })
}

func (m Model) View() tea.View {
	v := tea.NewView("")
	v.AltScreen = true
	if !m.colors.asking && !m.opts.Theme.FromTerminal() {
		v.BackgroundColor = m.opts.Theme.Color(theme.Bg)
	}
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
				Arrange: m.arranging && t.ID == m.focus,
				Payload: m.payloadIn(t.ID, t.Rect),
				List:    m.lists[t.ID].render(),
			}
			if tile.Focused && !m.arranging {
				tile.Hints = m.hints(t.ID)
			}
			if m.editing != nil && t.ID == m.editing.widget {
				tile.Payload, tile.Hints = widget.Payload{}, []render.Hint{{Key: "esc", Label: "save"}}
				layers = append(layers, m.editorLayer(t.X, t.Y))
			}
			layers = append(layers, lipgloss.NewLayer(render.Draw(m.opts.Theme, tile)).X(t.X).Y(t.Y))
		}
		layers = append(layers, lipgloss.NewLayer(m.footer()).Y(m.h-footerHeight))
	}
	if m.picking {
		layers = append(layers, m.pickerLayer())
	}
	if m.keyMap {
		layers = append(layers, m.keyMapLayer())
	}
	if m.palette != nil {
		layers = append(layers, m.paletteLayer())
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
