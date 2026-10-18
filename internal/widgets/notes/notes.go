// Package notes is the Notes Widget type: one markdown scratch note, shown
// as wrapped text.
package notes

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gofrs/flock"

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
		file := c.DataFile(c.Settings.String("file"), "notes.md")
		sum := sha256.Sum256([]byte(file))
		return notes{file: file, lock: filepath.Join(c.DataDir, "locks", "notes-"+hex.EncodeToString(sum[:8])+".lock")}
	},
}

type notes struct {
	file string
	// lock makes a save's check for a newer version and its write one step
	// across ghud instances.
	lock string
}

func (n notes) Files() []string { return []string{n.file} }

func (n notes) Refresh(context.Context, time.Time) (widget.Payload, error) {
	src, err := n.read()
	if err != nil {
		return widget.Payload{}, err
	}
	p := widget.Payload{Wrap: true, Actions: []widget.Action{
		{ID: "edit", Key: "i", Label: "edit", Verb: widget.Edit, Input: widget.InlineInput, Value: src},
		{ID: "editor", Key: "E", Label: "$EDITOR", Verb: widget.Edit},
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
		return n.save(inv.Text, inv.Base, inv.Now)
	case "editor":
		// The editor may be one that can't create the note's directory.
		return widget.Result{Edit: n.file}, os.MkdirAll(filepath.Dir(n.file), 0o755)
	}
	return widget.Result{}, fmt.Errorf("notes has no Action %q", inv.Action)
}

// save writes text over the note. When the note changed on disk since base
// was read, the user's text still lands and the other version moves to a
// conflict file next to the note.
func (n notes) save(text, base string, now time.Time) (widget.Result, error) {
	if text != "" && !strings.HasSuffix(text, "\n") {
		text += "\n"
	}
	if err := os.MkdirAll(filepath.Dir(n.lock), 0o755); err != nil {
		return widget.Result{}, err
	}
	l := flock.New(n.lock)
	if err := l.Lock(); err != nil {
		return widget.Result{}, err
	}
	defer l.Unlock()

	disk, err := n.read()
	if err != nil {
		return widget.Result{}, err
	}
	res := widget.Result{Toast: "Saved " + filepath.Base(n.file)}
	if disk != base && disk != text && disk != "" {
		conflict, err := n.keep(disk, now)
		if err != nil {
			return widget.Result{}, err
		}
		res.Toast += "; the other version is in " + filepath.Base(conflict)
	}
	return res, widget.WriteFile(n.file, []byte(text))
}

// keep writes the other version of the note to <name>.conflict-<time>.md
// beside it, never over an earlier conflict file.
func (n notes) keep(other string, now time.Time) (string, error) {
	ext := filepath.Ext(n.file)
	stem := strings.TrimSuffix(n.file, ext) + ".conflict-" + now.Format("20060102-150405")
	for i := 1; ; i++ {
		path := stem + ext
		if i > 1 {
			path = fmt.Sprintf("%s-%d%s", stem, i, ext)
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if errors.Is(err, fs.ErrExist) {
			continue
		}
		if err != nil {
			return "", err
		}
		_, werr := f.WriteString(other)
		return path, errors.Join(werr, f.Close())
	}
}

func (n notes) read() (string, error) {
	b, err := os.ReadFile(n.file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}
