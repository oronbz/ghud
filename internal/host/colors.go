package host

import (
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
	term   theme.Terminal
	// asking is true until the terminal has answered or colorWait passed;
	// ghud leaves the terminal background alone meanwhile so the query
	// doesn't read back ghud's own colour.
	asking bool
}

type colorWaitMsg struct{}

// Theme is the palette in use.
func (m Model) Theme() theme.Theme { return m.opts.Theme }

// queryColors asks for the terminal background, then for device attributes,
// whose answer comes after every colour answer the terminal will give.
func (m Model) queryColors() tea.Cmd {
	if m.colors.pinned {
		return nil
	}
	return tea.Batch(
		tea.Sequence(tea.RequestBackgroundColor, tea.Raw(ansi.RequestPrimaryDeviceAttributes)),
		m.opts.Ports.Clock.After(colorWait, func(time.Time) tea.Msg { return colorWaitMsg{} }),
	)
}

// colorAnswer handles a reply to queryColors; ok is false for other messages.
func (m Model) colorAnswer(msg tea.Msg) (Model, bool) {
	switch msg := msg.(type) {
	case tea.BackgroundColorMsg:
		m.colors.term.Bg = msg.Color
	case uv.PrimaryDeviceAttributesEvent, colorWaitMsg:
		m.colors.asking = false
		return m, true
	default:
		return m, false
	}
	if !m.colors.pinned {
		m.opts.Theme = theme.Resolve(m.colors.name, m.colors.term)
	}
	return m, true
}
