package host

import (
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/theme"
)

// footerHeight is the one line under the grid for the mode badge and hints.
const footerHeight = 1

func (m Model) footer() string {
	th := m.opts.Theme
	badge := th.Fg(theme.Bg).Background(th.Color(theme.Accent)).Bold(true).Render(" NORMAL ")
	hints := "Tab/hl focus · 1–9 jump · ? keys · q quit"
	if m.focus != "" {
		hints = "Tab rows · hl widgets · HJKL any direction · esc unfocus · ? keys"
	}
	return ansi.Truncate(badge+" "+th.Fg(theme.Dim).Render(hints), m.w, "…")
}
