package host

import (
	"fmt"
	"slices"

	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/meetings"
	"github.com/oronbz/ghud/internal/widgets/reminders"
)

// macOnly are the built-in Widget types that read EventKit, so they're
// built only on macOS.
var macOnly = []widget.Type{meetings.Type, reminders.Type}

// unavailable are stand-ins for the macOS-only types not in taken,
// so a config.toml naming one works on every machine: its Widgets need
// setup instead of reading as a typo.
func unavailable(taken []string) []widget.Type {
	var out []widget.Type
	for _, t := range macOnly {
		if !slices.Contains(taken, t.Name) {
			out = append(out, widget.Type{Name: t.Name, Title: t.Title, OptIn: true,
				Missing: fmt.Sprintf("`%s` is not available on this platform", t.Name)})
		}
	}
	return out
}
