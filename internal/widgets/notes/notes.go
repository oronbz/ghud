// Package notes is the Notes Widget type: one markdown scratch note, shown
// as wrapped text.
package notes

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
	p := widget.Payload{Wrap: true, Actions: []widget.Action{
		{ID: "edit", Key: "i", Label: "edit", Verb: widget.Edit, Input: widget.InlineInput, Value: src},
	}}
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

func (n notes) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	switch inv.Action {
	case "edit":
		text := inv.Text
		if text != "" && !strings.HasSuffix(text, "\n") {
			text += "\n"
		}
		return widget.Result{Toast: "Saved " + filepath.Base(n.file)}, widget.WriteFile(n.file, []byte(text))
	}
	return widget.Result{}, fmt.Errorf("notes has no Action %q", inv.Action)
}

func (n notes) read() (string, error) {
	b, err := os.ReadFile(n.file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}
