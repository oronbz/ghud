// Package notes is the Notes Widget type: one markdown scratch note, shown
// as wrapped text.
package notes

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"time"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "notes",
	Title:       "Notes",
	Description: "One markdown scratch note.",
	Settings: []widget.Setting{{
		Name: "file", Kind: widget.String, Default: "",
		Description: "The markdown note; empty means notes.md in the data dir.",
		Why:         "Point it at a synced folder, or give two Notes Widgets their own notes.",
	}},
	New: func(c widget.Config) widget.Widget {
		return notes{file: c.DataFile(c.Settings.String("file"), "notes.md")}
	},
}

type notes struct{ file string }

func (n notes) Files() []string { return []string{n.file} }

func (n notes) Refresh(context.Context, time.Time) (widget.Payload, error) {
	src, err := n.read()
	if err != nil {
		return widget.Payload{}, err
	}
	p := widget.Payload{Wrap: true}
	for _, l := range strings.Split(strings.TrimSuffix(src, "\n"), "\n") {
		l = strings.TrimSuffix(strings.ReplaceAll(l, "\t", "    "), "\r")
		if strings.HasPrefix(l, "#") {
			p.Lines = append(p.Lines, widget.Line{Spans: []widget.Span{{Text: l, Token: theme.Bright, Bold: true}}})
			continue
		}
		p.Lines = append(p.Lines, widget.Text(theme.Fg, l))
	}
	return p, nil
}

func (n notes) read() (string, error) {
	b, err := os.ReadFile(n.file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}
