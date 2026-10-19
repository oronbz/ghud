package host

import (
	"fmt"
	"image/color"
	"strconv"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/theme"
)

// colorWait is how long ghud waits for the terminal to answer its colour
// queries before settling on what it has.
const colorWait = 500 * time.Millisecond

// colors tracks the theme against the terminal's colour answers.
type colors struct {
	// pinned themes come from Options.Theme and ignore the terminal.
	pinned bool
	name   string
	over   theme.Overrides
	term   theme.Terminal
	// asking is true until the terminal has answered or colorWait passed;
	// ghud leaves the terminal background alone meanwhile so the query
	// doesn't read back ghud's own colour.
	asking bool
}

type colorWaitMsg struct{}

// Theme is the palette in use.
func (m Model) Theme() theme.Theme { return m.opts.Theme }

// queryColors asks for the terminal's colours, then for device attributes,
// whose answer comes after every colour answer the terminal will give.
func (m Model) queryColors() tea.Cmd {
	if m.colors.pinned {
		return nil
	}
	queries := []tea.Cmd{tea.RequestBackgroundColor}
	if m.colors.name == theme.TerminalTheme {
		var osc4 strings.Builder
		for n := 1; n <= 6; n++ {
			fmt.Fprintf(&osc4, "\x1b]4;%d;?\x07", n)
		}
		queries = append(queries, tea.RequestForegroundColor, tea.Raw(osc4.String()))
	}
	queries = append(queries, tea.Raw(ansi.RequestPrimaryDeviceAttributes))
	return tea.Batch(
		tea.Sequence(queries...),
		m.opts.Ports.Clock.After(colorWait, func(time.Time) tea.Msg { return colorWaitMsg{} }),
	)
}

// background is the colour ghud sets as the terminal background, nil while
// it leaves the terminal's own alone.
func (m Model) background() color.Color {
	if m.colors.asking || m.opts.Theme.FromTerminal() {
		return nil
	}
	return m.opts.Theme.Color(theme.Bg)
}

// colorAnswer handles a reply to queryColors; ok is false for other messages.
// When the answer changes the terminal background it clears the screen:
// herdr applies a new default background only to cells drawn after it, so
// without a full redraw the old colour lingers until each cell repaints.
func (m Model) colorAnswer(msg tea.Msg) (Model, tea.Cmd, bool) {
	before := m.background()
	next, ok := m.applyColorAnswer(msg)
	if !ok {
		return m, nil, false
	}
	if sameColor(before, next.background()) {
		return next, nil, true
	}
	return next, tea.ClearScreen, true
}

func (m Model) applyColorAnswer(msg tea.Msg) (Model, bool) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.colors.term.Bg = msg.Color
	case tea.ForegroundColorMsg:
		m.colors.term.Fg = msg.Color
	case uv.UnknownOscEvent:
		if !paletteAnswer(string(msg), &m.colors.term) {
			return m, false
		}
	case uv.PrimaryDeviceAttributesEvent, colorWaitMsg:
		m.colors.asking = false
		return m, true
	default:
		return m, false
	}
	return m.resolveTheme(), true
}

func sameColor(a, b color.Color) bool {
	if a == nil || b == nil {
		return a == b
	}
	ar, ag, ab, aa := a.RGBA()
	br, bg, bb, ba := b.RGBA()
	return ar == br && ag == bg && ab == bb && aa == ba
}

func (m Model) resolveTheme() Model {
	if !m.colors.pinned {
		m.opts.Theme = theme.Resolve(m.colors.name, m.colors.term, m.colors.over)
	}
	return m
}

// paletteAnswer reads an OSC 4 answer ("ESC ]4;1;rgb:cccc/0000/0000 ST"),
// which may carry several number;colour pairs, into term.
func paletteAnswer(s string, term *theme.Terminal) bool {
	body, ok := strings.CutPrefix(s, "\x1b]4;")
	if !ok {
		return false
	}
	body = strings.TrimSuffix(strings.TrimSuffix(body, "\x07"), "\x1b\\")
	parts := strings.Split(body, ";")
	for i := 0; i+1 < len(parts); i += 2 {
		n, err := strconv.Atoi(parts[i])
		if err != nil || n < 0 || n >= len(term.ANSI) {
			continue
		}
		if c := ansi.XParseColor(parts[i+1]); c != nil {
			term.ANSI[n] = c
		}
	}
	return true
}
