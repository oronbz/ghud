package config

import (
	"fmt"
	"strings"

	"github.com/pelletier/go-toml/v2/unstable"
)

// keyLines maps every key to its 1-based line, so problems can name
// file:line. Keys are "theme", "widget.0" (the block header), "widget.0.id",
// "plugin.1", "colors.dark" (a table header), "colors.dark.accent" and so on.
func keyLines(src []byte) map[string]int {
	lines := map[string]int{}
	var p unstable.Parser
	p.Reset(src)
	prefix := ""
	count := map[string]int{}
	for p.NextExpression() {
		e := p.Expression()
		switch e.Kind {
		case unstable.ArrayTable:
			name, line := keyPath(&p, e)
			prefix = ""
			if _, seen := lines[name]; !seen {
				lines[name] = line
			}
			if name == "widget" || name == "plugin" {
				prefix = fmt.Sprintf("%s.%d", name, count[name])
				count[name]++
				lines[prefix] = line
				prefix += "."
			}
		case unstable.Table:
			name, line := keyPath(&p, e)
			for k := name; ; k = k[:strings.LastIndex(k, ".")] {
				if _, seen := lines[k]; !seen {
					lines[k] = line
				}
				if !strings.Contains(k, ".") {
					break
				}
			}
			prefix = name + "."
		case unstable.KeyValue:
			if prefix == "\x00" {
				continue
			}
			name, line := keyPath(&p, e)
			first, _, _ := strings.Cut(name, ".")
			if _, seen := lines[prefix+first]; !seen {
				lines[prefix+first] = line
			}
		}
	}
	return lines
}

func keyPath(p *unstable.Parser, n *unstable.Node) (string, int) {
	var parts []string
	line := 0
	it := n.Key()
	for it.Next() {
		k := it.Node()
		if line == 0 {
			line = p.Shape(k.Raw).Start.Line
		}
		parts = append(parts, string(k.Data))
	}
	return strings.Join(parts, "."), line
}

func headerLines(src []byte) []int {
	var lines []int
	var p unstable.Parser
	p.Reset(src)
	for p.NextExpression() {
		e := p.Expression()
		if e.Kind == unstable.Table || e.Kind == unstable.ArrayTable {
			_, line := keyPath(&p, e)
			lines = append(lines, line)
		}
	}
	return lines
}
