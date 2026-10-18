package host

import (
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
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
	switch m.held {
	case tea.ModShift:
		hints = modifierHints("Shift", isShifted)
	case tea.ModCtrl:
		hints = modifierHints("Ctrl", func(k string) bool { return strings.HasPrefix(k, "ctrl+") })
	}
	return ansi.Truncate(badge+" "+th.Fg(theme.Dim).Render(hints), m.w, "…")
}

// modifierHints lists the bindings that have at least one key using the modifier.
func modifierHints(name string, uses func(key string) bool) string {
	var hints []string
	for _, b := range Bindings() {
		for _, k := range b.Keys() {
			if uses(k) {
				hints = append(hints, b.Help().Key+" "+b.Help().Desc)
				break
			}
		}
	}
	if hints == nil {
		return name + ": no keys"
	}
	return name + ": " + strings.Join(hints, " · ")
}

func isShifted(k string) bool {
	r, size := utf8.DecodeRuneInString(k)
	return strings.Contains(k, "shift+") || size == len(k) && unicode.IsUpper(r)
}

// loneModifier reports whether code is a modifier key pressed on its own,
// and the Shift or Ctrl it swaps the footer to (0 for the others).
func loneModifier(code rune) (tea.KeyMod, bool) {
	switch code {
	case tea.KeyLeftShift, tea.KeyRightShift:
		return tea.ModShift, true
	case tea.KeyLeftCtrl, tea.KeyRightCtrl:
		return tea.ModCtrl, true
	case tea.KeyLeftAlt, tea.KeyRightAlt, tea.KeyLeftSuper, tea.KeyRightSuper,
		tea.KeyLeftHyper, tea.KeyRightHyper, tea.KeyLeftMeta, tea.KeyRightMeta,
		tea.KeyIsoLevel3Shift, tea.KeyIsoLevel5Shift:
		return 0, true
	}
	return 0, false
}
