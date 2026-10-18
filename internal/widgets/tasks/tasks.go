// Package tasks is the Tasks Widget type: a quick local task list kept as a
// markdown checklist, such as `- [ ] Renew passport due:2026-10-20`.
package tasks

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/gofrs/flock"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:        "tasks",
	Title:       "Tasks",
	Description: "A quick task list kept as a markdown checklist.",
	Settings: []widget.Setting{{
		Name: "file", Kind: widget.String, Default: "",
		Description: "The markdown checklist; empty means tasks.md in the data dir.",
		Why:         "Point it at a synced folder, or give two Tasks Widgets their own lists.",
	}},
	Midnight: true,
	New:      newTasks,
}

func newTasks(c widget.Config) widget.Widget {
	file := c.DataFile(c.Settings.String("file"), "tasks.md")
	sum := sha256.Sum256([]byte(file))
	return tasks{file: file, lock: filepath.Join(c.DataDir, "locks", "tasks-"+hex.EncodeToString(sum[:8])+".lock")}
}

type tasks struct {
	file string
	// lock serialises edits across ghud instances; it lives in the data dir
	// so synced folders don't collect lock files.
	lock string
}

func (t tasks) Files() []string { return []string{t.file} }

func (t tasks) Refresh(_ context.Context, now time.Time) (widget.Payload, error) {
	src, err := t.read()
	if err != nil {
		return widget.Payload{}, err
	}
	p := widget.Payload{
		Columns: []widget.Column{{Priority: 0}, {Priority: 1, Flex: true}, {Priority: 2, Right: true}},
		Empty:   "No tasks. Press a to add one.",
		Actions: []widget.Action{{
			ID: "add", Key: "a", Label: "add", Input: widget.TextInput,
			Prompt: "New task (end with “due tue” to set a date)",
		}},
	}
	for _, tk := range parse(src).tasks {
		if tk.done {
			continue
		}
		due, tok := dueLabel(tk.due, now)
		p.Items = append(p.Items, widget.Item{
			ID:     tk.id,
			Fields: []widget.Field{widget.F(theme.Dim, "○"), widget.F(theme.Fg, tk.title), widget.F(tok, due)},
			Actions: []widget.Action{
				{ID: "done", Key: "space", Label: "done", Verb: widget.Complete},
				{ID: "edit", Key: "enter", Label: "edit", Primary: true, Verb: widget.Edit, Input: widget.TextInput, Prompt: "Edit task", Value: tk.editable()},
				{ID: "delete", Key: "d", Label: "delete", Confirm: true, Verb: widget.Delete},
			},
		})
	}
	p.Count = len(p.Items)
	return p, nil
}

func (t tasks) Invoke(_ context.Context, inv widget.Invocation) (widget.Result, error) {
	if err := os.MkdirAll(filepath.Dir(t.lock), 0o755); err != nil {
		return widget.Result{}, err
	}
	l := flock.New(t.lock)
	if err := l.Lock(); err != nil {
		return widget.Result{}, err
	}
	defer l.Unlock()

	src, err := t.read()
	if err != nil {
		return widget.Result{}, err
	}
	doc := parse(src)
	if inv.Action == "add" {
		title, due := parseInput(inv.Text, inv.Now)
		if title == "" {
			return widget.Result{}, errors.New("a task needs a title")
		}
		doc.add(task{title: title, due: due})
		return widget.Result{Toast: fmt.Sprintf("Added “%s”", title)}, t.write(doc)
	}
	i := doc.find(inv.Item)
	if i < 0 {
		return widget.Result{}, errors.New("that task is no longer in " + filepath.Base(t.file))
	}
	tk := doc.tasks[i]
	var toast string
	switch inv.Action {
	case "done":
		doc.lines[tk.line] = tk.prefix + "x" + tk.rest
		toast = fmt.Sprintf("Done “%s”", tk.title)
	case "edit":
		title, due := parseInput(inv.Text, inv.Now)
		if title == "" {
			return widget.Result{}, errors.New("a task needs a title")
		}
		doc.lines[tk.line] = tk.prefix + " ] " + task{title: title, due: due}.text() + tk.eol
		toast = fmt.Sprintf("Saved “%s”", title)
	case "delete":
		doc.lines = append(doc.lines[:tk.line], doc.lines[tk.line+1:]...)
		toast = fmt.Sprintf("Deleted “%s”", tk.title)
	default:
		return widget.Result{}, fmt.Errorf("tasks has no Action %q", inv.Action)
	}
	return widget.Result{Toast: toast}, t.write(doc)
}

func (t tasks) read() (string, error) {
	b, err := os.ReadFile(t.file)
	if errors.Is(err, fs.ErrNotExist) {
		return "", nil
	}
	return string(b), err
}

func (t tasks) write(d doc) error {
	return widget.WriteFile(t.file, []byte(strings.Join(d.lines, "")))
}

// doc is a checklist file as lines, each with its own line ending, so lines
// a change doesn't touch are written back byte for byte.
type doc struct {
	lines []string
	tasks []task
}

type task struct {
	// line indexes doc.lines.
	line int
	id   string
	done bool
	// prefix is everything up to the checkbox's mark ("  - ["); rest is
	// everything after it, line ending included; eol is just the line ending.
	prefix, rest, eol string
	title             string
	due               time.Time
}

var (
	taskLine = regexp.MustCompile(`^(\s*[-*+] \[)([ xX])(\] (.*?))(\r?\n)?$`)
	dueTag   = regexp.MustCompile(`\s+due:(\d{4}-\d{2}-\d{2})$`)
)

func parse(src string) doc {
	var d doc
	d.lines = strings.SplitAfter(src, "\n")
	if d.lines[len(d.lines)-1] == "" {
		d.lines = d.lines[:len(d.lines)-1]
	}
	seen := map[string]int{}
	for i, l := range d.lines {
		m := taskLine.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		tk := task{line: i, done: m[2] != " ", prefix: m[1], rest: m[3] + m[5], eol: m[5], title: m[4]}
		if dm := dueTag.FindStringSubmatchIndex(tk.title); dm != nil {
			if due, err := time.Parse(time.DateOnly, tk.title[dm[2]:dm[3]]); err == nil {
				tk.due = due
				tk.title = tk.title[:dm[0]]
			}
		}
		// Ids come from the text, numbered among identical lines, so they
		// survive other lines being added, removed or reordered.
		seen[m[4]]++
		tk.id = fmt.Sprintf("%d:%s", seen[m[4]], m[4])
		d.tasks = append(d.tasks, tk)
	}
	return d
}

func (d doc) find(id string) int {
	for i, tk := range d.tasks {
		if tk.id == id {
			return i
		}
	}
	return -1
}

func (d *doc) add(tk task) {
	if n := len(d.lines); n > 0 && !strings.HasSuffix(d.lines[n-1], "\n") {
		d.lines[n-1] += "\n"
	}
	d.lines = append(d.lines, "- [ ] "+tk.text()+"\n")
}

// text is the task as written after its checkbox.
func (tk task) text() string {
	if tk.due.IsZero() {
		return tk.title
	}
	return tk.title + " due:" + tk.due.Format(time.DateOnly)
}

// editable is the task as the edit prompt starts: the way you'd type it.
func (tk task) editable() string {
	if tk.due.IsZero() {
		return tk.title
	}
	return tk.title + " due " + tk.due.Format(time.DateOnly)
}

var dueSuffix = regexp.MustCompile(`(?i)\s+due[ :](\S+)$`)

// parseInput splits typed text into a title and a due date from a trailing
// "due <day>": today, tomorrow, a weekday (the next one after today) or a date.
func parseInput(text string, now time.Time) (string, time.Time) {
	text = strings.TrimSpace(text)
	m := dueSuffix.FindStringSubmatchIndex(text)
	if m == nil {
		return text, time.Time{}
	}
	day := strings.ToLower(text[m[2]:m[3]])
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	var due time.Time
	switch {
	case day == "today":
		due = today
	case day == "tomorrow":
		due = today.AddDate(0, 0, 1)
	default:
		if d, err := time.Parse(time.DateOnly, day); err == nil {
			due = d
			break
		}
		for wd := time.Sunday; wd <= time.Saturday; wd++ {
			name := strings.ToLower(wd.String())
			if len(day) >= 3 && strings.HasPrefix(name, day) {
				ahead := (int(wd)-int(now.Weekday())+6)%7 + 1
				due = today.AddDate(0, 0, ahead)
			}
		}
	}
	if due.IsZero() {
		return text, time.Time{}
	}
	return strings.TrimSpace(text[:m[0]]), due
}

// dueLabel says when a task is due relative to now.
func dueLabel(due, now time.Time) (string, theme.Token) {
	if due.IsZero() {
		return "", theme.Dim
	}
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	days := int(due.Sub(today).Hours() / 24)
	switch {
	case days < 0:
		return "overdue", theme.Bad
	case days == 0:
		return "today", theme.Warn
	case days == 1:
		return "tomorrow", theme.Dim
	case days < 7:
		return due.Format("Mon"), theme.Dim
	case due.Year() == now.Year():
		return due.Format("2 Jan"), theme.Dim
	}
	return due.Format("2 Jan 2006"), theme.Dim
}
