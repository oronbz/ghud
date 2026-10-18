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
	"runtime"
	"slices"
	"strings"
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
		Description: "How often the command runs.",
		Why:         "Slow or rate-limited commands want a longer one.",
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
	return script{command: c.Settings.String("command"), interval: c.Settings.Duration("interval")}
}

type script struct {
	command  string
	interval time.Duration
}

func (s script) Interval() time.Duration { return s.interval }

func (s script) Refresh(ctx context.Context, _ time.Time) (widget.Payload, error) {
	if strings.TrimSpace(s.command) == "" {
		return widget.Payload{}, &widget.SetupError{Hint: "Set `command` in this Widget's [[widget]] block"}
	}
	out, err := run(ctx, s.command)
	if err != nil {
		return widget.Payload{}, err
	}
	return parse(out)
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
// lines, and anything else as plain text lines.
func parse(out []byte) (widget.Payload, error) {
	trimmed := bytes.TrimSpace(out)
	if len(trimmed) == 0 {
		return widget.Payload{Lines: []widget.Line{widget.Text(theme.Faint, "no output")}}, nil
	}
	switch trimmed[0] {
	case '[':
		return items(trimmed)
	case '{':
		var doc struct {
			Lines []line `json:"lines"`
		}
		if err := json.Unmarshal(trimmed, &doc); err != nil {
			return widget.Payload{}, badJSON(err)
		}
		p := widget.Payload{}
		for _, l := range doc.Lines {
			p.Lines = append(p.Lines, widget.Line(l))
		}
		return p, nil
	}
	var p widget.Payload
	for _, l := range strings.Split(strings.TrimRight(ansi.Strip(string(out)), "\n"), "\n") {
		p.Lines = append(p.Lines, widget.Text(theme.Fg, strings.ReplaceAll(l, "\t", "    ")))
	}
	return p, nil
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

func items(b []byte) (widget.Payload, error) {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	if _, err := dec.Token(); err != nil {
		return widget.Payload{}, badJSON(err)
	}
	var objs []object
	var keys []string
	for dec.More() {
		o, err := readObject(dec)
		if err != nil {
			return widget.Payload{}, badJSON(err)
		}
		for _, k := range o.keys {
			if k != "id" && !slices.Contains(keys, k) {
				keys = append(keys, k)
			}
		}
		objs = append(objs, o)
	}
	if _, err := dec.Token(); err != nil {
		return widget.Payload{}, badJSON(err)
	}
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
	}
	return p, nil
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
