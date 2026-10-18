// Package script is the script Widget type: a shell command run on an
// interval whose output is the Widget, for one-liners that don't deserve a Plugin.
package script

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

var Type = widget.Type{
	Name:  "script",
	Title: "Script",
	Description: "A command run on an interval. It prints a JSON array of objects (one Item each, " +
		"one column per key) or plain text lines; {\"lines\": [...]} gives styled lines.",
	Settings: []widget.Setting{{
		Name: "command", Kind: widget.String, Default: "",
		Description: "The command, run by sh (cmd on Windows).",
		Why:         "Show anything a one-liner can print, such as `brew outdated --json=v2 | jq '[.formulae[] | {name, current: .current_version}]'`.",
	}, {
		Name: "interval", Kind: widget.Duration, Default: time.Minute,
		Description: "How often the command runs; `refresh` does the same and wins.",
		Why:         "Slow or rate-limited commands want a longer one.",
	}, {
		Name: "actions", Kind: widget.Tables, Default: []widget.Settings{},
		Description: "Actions, each a command template: {{key}} is the selected Item's value for key, " +
			"shell-quoted; a template without one acts on the whole Widget.",
		Why: `Act on what the command lists, such as [{ key = "u", label = "upgrade", run = "brew upgrade {{name}}", confirm = true }].`,
		Fields: []widget.Setting{
			{Name: "run", Kind: widget.String, Default: "", Required: true, Description: "The command template."},
			{Name: "label", Kind: widget.String, Default: "", Required: true, Description: "What the Action is called in hints and the palette."},
			{Name: "key", Kind: widget.String, Default: "", Description: "The key that runs it while the Widget is focused."},
			{Name: "confirm", Kind: widget.Bool, Default: false, Description: "Ask before running it."},
			{Name: "primary", Kind: widget.Bool, Default: false, Description: "Run it on enter."},
		},
	}},
	Refresh: time.Minute,
	Floor:   time.Second,
	Fetches: true,
	OptIn:   true,
	New:     newScript,
}

// timeout stops a hung command so the Widget can show why.
const timeout = time.Minute

func newScript(c widget.Config) widget.Widget {
	s := &script{command: c.Settings.String("command"), interval: c.Settings.Duration("interval")}
	for i, a := range c.Settings.Tables("actions") {
		s.actions = append(s.actions, template{
			Action: widget.Action{
				ID: fmt.Sprint(i), Key: a.String("key"), Label: a.String("label"),
				Confirm: a.Bool("confirm"), Primary: a.Bool("primary"),
			},
			run: a.String("run"),
		})
	}
	return s
}

type script struct {
	command  string
	interval time.Duration
	actions  []template
	mu       sync.Mutex
	// items are the last Items' values by Item id, for filling templates.
	items map[string]map[string]string
}

// template is an Action whose command is filled from the selected Item.
type template struct {
	widget.Action
	run string
}

var placeholder = regexp.MustCompile(`\{\{\s*([^{}\s]+)\s*\}\}`)

func (t template) perItem() bool { return placeholder.MatchString(t.run) }

// command is the template filled with an Item's values, each shell-quoted
// so a value can't run as code.
func (t template) command(values map[string]string) string {
	return placeholder.ReplaceAllStringFunc(t.run, func(m string) string {
		return quote(values[placeholder.FindStringSubmatch(m)[1]])
	})
}

func quote(s string) string {
	if runtime.GOOS == "windows" {
		return `"` + strings.ReplaceAll(s, `"`, `""`) + `"`
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func (s *script) Interval() time.Duration { return s.interval }

func (s *script) Refresh(ctx context.Context, _ time.Time) (widget.Payload, error) {
	if strings.TrimSpace(s.command) == "" {
		return widget.Payload{}, &widget.SetupError{Hint: "Set `command` in this Widget's [[widget]] block"}
	}
	out, err := run(ctx, s.command)
	if err != nil {
		return widget.Payload{}, err
	}
	p, values, err := parse(out)
	if err != nil {
		return widget.Payload{}, err
	}
	for _, t := range s.actions {
		if !t.perItem() {
			p.Actions = append(p.Actions, t.Action)
			continue
		}
		for i := range p.Items {
			p.Items[i].Actions = append(p.Items[i].Actions, t.Action)
		}
	}
	s.mu.Lock()
	s.items = values
	s.mu.Unlock()
	return p, nil
}

func (s *script) Invoke(ctx context.Context, inv widget.Invocation) (widget.Result, error) {
	i, err := strconv.Atoi(inv.Action)
	if err != nil || i < 0 || i >= len(s.actions) {
		return widget.Result{}, fmt.Errorf("no Action %q", inv.Action)
	}
	t := s.actions[i]
	s.mu.Lock()
	values := s.items[inv.Item]
	s.mu.Unlock()
	out, err := run(ctx, t.command(values))
	if err != nil {
		return widget.Result{}, err
	}
	lines := strings.Split(strings.TrimSpace(ansi.Strip(string(out))), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return widget.Result{Toast: last}, nil
	}
	return widget.Result{Toast: t.Label + " done"}, nil
}

// run runs command through the shell and returns its stdout; a failure
// carries stderr.
func run(ctx context.Context, command string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	c := exec.CommandContext(ctx, "sh", "-c", command)
	if runtime.GOOS == "windows" {
		c = exec.CommandContext(ctx, "cmd", "/C", command)
	}
	var stdout, stderr bytes.Buffer
	c.Stdout, c.Stderr = &stdout, &stderr
	err := c.Run()
	if errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return nil, failure{reason: "timed out", msg: fmt.Sprintf("the command ran over %s", timeout)}
	}
	if err != nil {
		return nil, commandFailure(err, stderr.String())
	}
	return stdout.Bytes(), nil
}

// failure is a command that failed; reason is the short form for the stale marker.
type failure struct{ reason, msg string }

func (f failure) Error() string  { return f.msg }
func (f failure) Reason() string { return f.reason }
func (f failure) Hint() string   { return "Check `command` by running it in a shell" }

func commandFailure(err error, stderr string) failure {
	reason := err.Error()
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		reason = fmt.Sprintf("exit %d", exit.ExitCode())
	}
	msg := reason
	if s := strings.TrimSpace(ansi.Strip(stderr)); s != "" {
		lines := strings.Split(s, "\n")
		reason = strings.TrimSpace(lines[len(lines)-1])
		msg += ": " + strings.Join(lines, " ")
	}
	return failure{reason: reason, msg: msg}
}

// parse reads a JSON array of objects as Items, {"lines": [...]} as styled
// lines, and anything else as plain text lines. values are each Item's
// values by key, under its id.
func parse(out []byte) (p widget.Payload, values map[string]map[string]string, err error) {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return widget.Payload{Lines: []widget.Line{widget.Text(theme.Faint, "no output")}}, nil, nil
	}
	switch trimmed[0] {
	case '[':
		return items(trimmed)
	case '{':
		var doc struct {
			Lines []line `json:"lines"`
		}
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return widget.Payload{}, nil, badJSON(err)
		}
		for _, l := range doc.Lines {
			p.Lines = append(p.Lines, widget.Line(l))
		}
		return p, nil, nil
	}
	for _, l := range strings.Split(strings.TrimRight(ansi.Strip(string(out)), "\n"), "\n") {
		p.Lines = append(p.Lines, widget.Text(theme.Fg, strings.ReplaceAll(l, "\t", "    ")))
	}
	return p, nil, nil
}

func badJSON(err error) error {
	return failure{reason: "bad JSON", msg: "the command printed bad JSON: " + err.Error()}
}

// line is a styled line, or a plain string for a line of fg text.
type line widget.Line

func (l *line) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		*l = line(widget.Text(theme.Fg, s))
		return nil
	}
	return json.Unmarshal(b, (*widget.Line)(l))
}

// object is one JSON object with its keys in the order printed.
type object struct {
	keys   []string
	values map[string]string
}

func items(b []byte) (widget.Payload, map[string]map[string]string, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if _, err := dec.Token(); err != nil {
		return widget.Payload{}, nil, badJSON(err)
	}
	var objs []object
	var keys []string
	for dec.More() {
		o, err := readObject(dec)
		if err != nil {
			return widget.Payload{}, nil, badJSON(err)
		}
		for _, k := range o.keys {
			if k != "id" && !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
		objs = append(objs, o)
	}
	if _, err := dec.Token(); err != nil {
		return widget.Payload{}, nil, badJSON(err)
	}
	values := map[string]map[string]string{}
	p := widget.Payload{Count: len(objs), Empty: "Nothing to show."}
	for i := range keys {
		p.Columns = append(p.Columns, widget.Column{Priority: i, Flex: i == 0})
	}
	if len(p.Columns) == 0 {
		p.Columns = []widget.Column{{Flex: true}}
	}
	seen := map[string]int{}
	for _, o := range objs {
		it := widget.Item{ID: o.values["id"]}
		for i, k := range keys {
			tok := theme.Dim
			if i == 0 {
				tok = theme.Fg
			}
			it.Fields = append(it.Fields, widget.F(tok, o.values[k]))
		}
		if it.ID == "" && len(keys) > 0 {
			it.ID = o.values[keys[0]]
		}
		seen[it.ID]++
		if n := seen[it.ID]; n > 1 {
			it.ID = fmt.Sprintf("%s#%d", it.ID, n)
		}
		p.Items = append(p.Items, it)
		values[it.ID] = o.values
	}
	return p, values, nil
}

func readObject(dec *json.Decoder) (object, error) {
	o := object{values: map[string]string{}}
	t, err := dec.Token()
	if err != nil {
		return o, err
	}
	if d, ok := t.(json.Delim); !ok || d != '{' {
		return o, errors.New("the array must hold objects")
	}
	for dec.More() {
		t, err := dec.Token()
		if err != nil {
			return o, err
		}
		k, _ := t.(string)
		var v any
		if err := dec.Decode(&v); err != nil {
			return o, err
		}
		if _, dup := o.values[k]; !dup {
			o.keys = append(o.keys, k)
		}
		o.values[k] = text(v)
	}
	_, err = dec.Token()
	return o, err
}

// text is how a JSON value shows in a column and fills a template.
func text(v any) string {
	switch v := v.(type) {
	case nil:
		return ""
	case string:
		return v
	case json.Number:
		return v.String()
	case bool:
		return fmt.Sprint(v)
	}
	b, _ := json.Marshal(v)
	return string(b)
}
