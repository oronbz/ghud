package host

// PROTOTYPE for #75, throwaway: where and how the Upgrade marker sits in the
// footer. F9 cycles the variants; VISOR_PROTOTYPE_MARKER=A|B|C|off picks the
// first one. Every variant pretends v0.7.0 is out.

import (
	"os"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/visor/internal/theme"
)

var protoVariants = []string{"A", "B", "C", "off"}

var protoNames = map[string]string{
	"A":   "accent text",
	"B":   "badge, like the mode badge",
	"C":   "key hint, shrinks when narrow",
	"off": "no marker",
}

var protoVariant = func() int {
	for i, v := range protoVariants {
		if strings.EqualFold(v, os.Getenv("VISOR_PROTOTYPE_MARKER")) {
			return i
		}
	}
	return 0
}()

func (m Model) protoCycle() (Model, tea.Cmd) {
	protoVariant = (protoVariant + 1) % len(protoVariants)
	v := protoVariants[protoVariant]
	return m.say("prototype marker "+v+": "+protoNames[v], theme.Fg)
}

func (m Model) protoMarker() string {
	th := m.opts.Theme
	switch protoVariants[protoVariant] {
	case "A":
		return th.Fg(theme.Accent).Bold(true).Render("↑ v0.7.0")
	case "B":
		return th.Fg(theme.Bg).Background(th.Color(theme.Accent)).Bold(true).Render(" ↑ v0.7.0 ")
	case "C":
		if m.w < 90 {
			return th.Fg(theme.Accent).Bold(true).Render("U") + th.Fg(theme.Accent).Render(" ↑ v0.7.0")
		}
		return th.Fg(theme.Accent).Bold(true).Render("U") + " " + th.Fg(theme.Dim).Render("upgrade to") + " " + th.Fg(theme.Accent).Bold(true).Render("v0.7.0")
	}
	return ""
}

// protoFooter pins the marker to the right edge and cuts the rest before it.
func (m Model) protoFooter(left string) string {
	marker := m.protoMarker()
	if marker == "" {
		return ansi.Truncate(left, m.w, "…")
	}
	mw := ansi.StringWidth(marker)
	room := m.w - mw - 1
	if room < 1 {
		return ansi.Truncate(marker, m.w, "")
	}
	left = ansi.Truncate(left, room, "…")
	return left + strings.Repeat(" ", m.w-ansi.StringWidth(left)-mw) + marker
}
