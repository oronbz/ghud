package config

import (
	"fmt"
	"strings"

	"github.com/oronbz/ghud/internal/widget"
)

// Reference is the full commented config.toml: every global option and
// every Widget type with all its settings at their defaults. Prose lines
// start with "# "; option lines start with a bare "#", so deleting that one
// character turns them on.
func Reference(types []widget.Type, loc Locale) string {
	var b strings.Builder
	prose := func(format string, args ...any) { fmt.Fprintf(&b, "# "+format+"\n", args...) }
	option := func(format string, args ...any) { fmt.Fprintf(&b, "#"+format+"\n", args...) }
	setting := func(s widget.Setting) {
		prose("%s", s.Description)
		if len(s.Choices) > 0 {
			prose("One of: %s.", strings.Join(s.Choices, ", "))
		}
		prose("Why: %s", s.Why)
		option("%s = %s", s.Name, widget.TOMLValue(s.Default))
	}

	prose("ghud config.toml: the full reference, everything commented out.")
	prose("Uncomment a line (delete its leading #) to change an option. ghud reads this")
	prose("file but never rewrites it; new options appear in `ghud config default`.")
	b.WriteString("\n")
	for _, s := range Globals(loc) {
		setting(s)
		b.WriteString("\n")
	}

	prose("Plugins under development, loaded from a local directory.")
	option("[[plugin]]")
	option(`path = "~/code/my-ghud-plugin"`)
	b.WriteString("\n")

	prose("Widgets. Each [[widget]] block places one Widget; the dashboard shows the")
	prose("blocks in this file, or one Widget of every type when there are none.")
	prose("Every block takes:")
	prose(`  type     the Widget type (required)`)
	prose(`  id       defaults to the type name; give each Widget of the same type its own`)
	prose(`  title    drawn in the top border; defaults to the type's name`)
	prose(`  refresh  how often it fetches, such as "5m"; defaults to the type's interval`)
	for _, t := range types {
		b.WriteString("\n")
		prose("%s Widget. %s", t.Title, t.Description)
		option("[[widget]]")
		option("type = %q", t.Name)
		option("id = %q", t.Name)
		option("title = %q", t.Title)
		for _, s := range t.Settings {
			setting(s)
		}
	}
	return b.String()
}
