// Package host is the ghud Bubble Tea program: it owns the Widgets, input
// modes and focus, and draws the dashboard. Everything outside the process
// goes through Options.Ports.
package host

import (
	"fmt"
	"image"
	"math/rand/v2"
	"path/filepath"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/cache"
	"github.com/oronbz/ghud/internal/closekey"
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
	"github.com/oronbz/ghud/internal/widgets/jira"
	"github.com/oronbz/ghud/internal/widgets/notes"
	"github.com/oronbz/ghud/internal/widgets/prs"
	"github.com/oronbz/ghud/internal/widgets/script"
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
	// CloseKeys quit ghud from any mode (--close-key).
	CloseKeys []closekey.Seq
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
	// sched is each Source's refresh schedule and Status; paused, when each
	// rate-limited credential may be used again.
	sched  map[string]sched
	paused map[string]time.Time
	// cache keeps fetching Widgets' data across runs and instances.
	cache cache.Store
	rng   *rand.Rand
	// timers counts the refresh timers armed so far; copies share it.
	timers *int
	// spinning and beating are the spinner's and the wall-clock check's
	// timer chains, each kept to one.
	spinning, beating bool
	// overlay is the open confirm or text prompt, which takes every key.
	overlay *overlay
	// editing is INSERT mode's editor, which takes every key but insertHostKeys.
	editing *editing
	// palette is the open ^K Action palette; menu the open Action menu;
	// menus counts the choice menus opened so far.
	palette *palette
	menu    *menu
	menus   int
	// held is the modifier (Shift or Ctrl) held on its own, which swaps the
	// footer to that modifier's keys; only kitty-protocol terminals report it.
	held   tea.KeyMod
	colors colors
	// mouse is config.toml's mouse option: capture clicks, drags and the wheel.
	mouse bool
	// hover is the title or seam under the pointer while no button is held.
	hover *layout.Zone
	drag  *drag
	click click
	// edits is how host Actions change config.toml.
	edits config.Editor
	// base is what every Widget is built with, besides its settings.
	base widget.Config
	// sticky is the config.toml problem shown whenever no toast is, until the file is fixed.
	sticky string
	// ticks tells the current redraw timer from ones a config reload replaced.
	ticks   int
	closing closekey.Matcher
}

type placed struct {
	typ    widget.Type
	title  string
	widget widget.Widget
	// setup, when set, is the needs-setup hint drawn instead of the Widget.
	setup string
	// ref finds the Widget's block in config.toml; block is what it said
	// when the Widget was built.
	ref   config.Ref
	block config.Widget
}

type tickMsg struct {
	at  time.Time
	seq int
}

// Builtins are the Widget types compiled into ghud, in reference order.
var Builtins = slices.Concat([]widget.Type{clock.Type, calendar.Type}, macTypes,
	[]widget.Type{tasks.Type, cpu.Type, notes.Type, prs.Type, jira.Type, script.Type})

func New(o Options) Model {
	m := Model{opts: o, now: o.Ports.Clock.Now(), widgets: map[string]placed{},
		fetched: map[string]widget.Payload{}, lists: map[string]listState{},
		sched: map[string]sched{}, paused: map[string]time.Time{}, cache: cacheStore(o), rng: newRand(o.Ports.Clock.Now()), timers: new(int),
		closing: closekey.NewMatcher(o.CloseKeys)}
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
	m.colors = colors{pinned: !o.Theme.IsZero(), name: cfg.Theme, over: cfg.Colors, asking: o.Theme.IsZero()}
	m = m.resolveTheme()
	m.edits = config.Editor{Path: file, LockDir: filepath.Join(o.Paths.Data, "locks"), Types: types, Locale: o.Locale}
	m, ids, _, clamped := m.placeAll(cfg)
	m.sticky = toastOf(append(append(problems, cfg.Problems...), clamped...))
	// An unreadable layout.json falls back to the default rather than blocking launch.
	saved, err := layout.Load(o.Paths.Config)
	if err != nil {
		saved = layout.Default()
	}
	m.layout = saved.Join(ids)
	return m.watch()
}

// placeAll builds cfg's Widgets and returns their ids in config order, the
// ids of those it built afresh, and toasts for refresh settings it clamped.
// A Widget whose block, and the globals Widgets are built from, haven't
// changed is kept as it was, state and all.
func (m Model) placeAll(cfg config.Config) (Model, []string, []string, []string) {
	old := m.widgets
	sameGlobals := cfg.TimeFormat == m.base.TimeFormat && cfg.WeekStart == m.base.WeekStart
	m.mouse = cfg.Mouse
	m.base = widget.Config{TimeFormat: cfg.TimeFormat, WeekStart: cfg.WeekStart, DataDir: m.opts.Paths.Data, Ports: m.opts.Ports}
	m.widgets = make(map[string]placed, len(cfg.Widgets))
	var ids, fresh, clamped []string
	for _, wc := range cfg.Widgets {
		id, ref := wc.ID, config.Ref{ID: wc.ID}
		// Widgets sharing an id are all in needs setup; suffixes keep them apart on screen only.
		for n := 2; m.widgets[id].title != ""; n++ {
			id, ref.N = fmt.Sprintf("%s~%d", wc.ID, n), n-1
		}
		ids = append(ids, id)
		if p, ok := old[id]; ok && sameGlobals && p.block.Equal(wc) {
			p.ref = ref
			m.widgets[id] = p
			continue
		}
		m.widgets[id] = m.place(wc, ref)
		fresh = append(fresh, id)
		if problem := m.schedule(id, wc.Refresh); problem != "" {
			clamped = append(clamped, problem)
		}
	}
	for id := range m.sched {
		if _, ok := m.widgets[id]; !ok {
			delete(m.sched, id)
		}
	}
	return m, ids, fresh, clamped
}

// place builds the Widget a [[widget]] block configures, unless it needs setup.
func (m Model) place(wc config.Widget, ref config.Ref) placed {
	p := placed{title: wc.Title, setup: wc.Setup, ref: ref, block: wc}
	if i := slices.IndexFunc(m.edits.Types, func(t widget.Type) bool { return t.Name == wc.Type }); i >= 0 {
		p.typ = m.edits.Types[i]
	}
	if p.setup == "" {
		c := m.base
		c.Settings = wc.Settings
		p.widget = p.typ.New(c)
	}
	return p
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(m.nextTick(), m.queryColors(), m.nextChange(), func() tea.Msg { return startMsg{} }, m.cleanCache())
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
		if m.overlay != nil {
			return m.overlayPaste(msg), nil
		}
		if m.editing != nil && m.palette == nil {
			return m.editKey(msg)
		}
	case refreshedMsg:
		return m.refreshed(msg)
	case startMsg:
		m, fetch := m.startAll(false)
		m, beat := m.beat()
		return m, tea.Batch(fetch, beat)
	case dueMsg:
		return m.due(msg)
	case beatMsg:
		return m.beaten(msg)
	case spinMsg:
		return m.spun(msg)
	case actedMsg:
		return m.acted(msg)
	case choicesMsg:
		return m.chose(msg)
	case editedMsg:
		return m.edited(msg)
	case filesChangedMsg:
		return m.filesChanged(msg)
	case configEditedMsg:
		return m.configEdited(msg)
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
	case tea.MouseMsg:
		return m.mouseMsg(msg)
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
	if m.mouse {
		v.MouseMode = tea.MouseModeAllMotion
	}
	if m.w == 0 || m.h == 0 {
		return v
	}
	var layers []*lipgloss.Layer
	s := m.scene()
	if s.TooSmall {
		layers = append(layers, lipgloss.NewLayer(m.tooSmall(s.Need)))
	} else {
		for i, t := range s.Tiles {
			if m.editing != nil && t.ID == m.editing.widget {
				layers = append(layers, m.editorLayer(t.X, t.Y))
			}
			layers = append(layers, lipgloss.NewLayer(render.Draw(m.opts.Theme, s.tiles[i])).X(t.X).Y(t.Y))
		}
		layers = append(layers, m.dragLayers(s)...)
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
	if m.menu != nil {
		layers = append(layers, m.menuLayer())
	}
	if m.overlay != nil {
		layers = append(layers, m.overlayLayer())
	}
	canvas := lipgloss.NewCanvas(m.w, m.h)
	lipgloss.NewCompositor(layers...).Draw(canvas, image.Rect(0, 0, m.w, m.h))
	m.tint(canvas, s.Tint)
	v.SetContent(canvas.Render())
	return v
}

// payloadIn is what Widget id shows in a tile at r, border included.
func (m Model) payloadIn(id string, r layout.Rect) widget.Payload {
	w := m.widgets[id]
	if p, ok := m.statusPayload(id); ok {
		return p
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
func (m Model) Zones() []layout.Zone { return m.scene().Zones }

// scene is the one layout pass View draws and the mouse hits: the frame's
// geometry, each tile's content, and the zones of the Item rows and key
// hints placed in it.
type scene struct {
	layout.Frame
	// tiles are drawn at Frame.Tiles' places.
	tiles []render.Tile
}

func (m Model) scene() scene {
	s := scene{Frame: m.frame()}
	if s.TooSmall {
		return s
	}
	for _, t := range s.Tiles {
		tile := render.Tile{
			Title:   t.Title,
			W:       t.W,
			H:       t.H,
			Focused: t.ID == m.focus,
			Arrange: m.arranging && t.ID == m.focus,
			Handle:  t.Handle,
			Payload: m.payloadIn(t.ID, t.Rect),
			List:    m.lists[t.ID].render(),
			Status:  m.chrome(t.ID),
		}
		switch {
		case m.editing != nil && t.ID == m.editing.widget:
			tile.Payload, tile.Hints = widget.Payload{}, []render.Hint{{Key: "esc", Label: "save"}}
		case tile.Focused && !m.arranging:
			tile.Hints = m.hints(t.ID)
			for i, at := range render.HintSlots(tile.Hints, t.W) {
				s.Zones = append(s.Zones, layout.Zone{Kind: layout.Hint, ID: t.ID, Index: i, Rect: layout.Rect{X: t.X + at.X, Y: t.Y + t.H - 1, W: at.W, H: 1}})
			}
		}
		if tile.Payload.IsList() {
			rows := render.Rows(tile.Payload)
			for i := range t.H - 2 {
				if j := tile.List.Top + i; j < len(rows) && rows[j] >= 0 {
					s.Zones = append(s.Zones, layout.Zone{Kind: layout.Item, ID: t.ID, Index: rows[j], Rect: layout.Rect{X: t.X + 1, Y: t.Y + 1 + i, W: t.W - 2, H: 1}})
				}
			}
		}
		s.tiles = append(s.tiles, tile)
	}
	return s
}

func (m Model) frame() layout.Frame {
	f := layout.Compute(m.layout, m.titles(), m.w, m.h-footerHeight, m.pointer())
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
