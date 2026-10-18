// Package plugin is the external plugin host (ADR 0001): it reads Plugin
// manifests, runs one long-lived process per Plugin speaking JSON-RPC 2.0
// over stdio, and adapts that protocol to the Widget contract, so the host
// app treats a Plugin's Widgets like built-in ones.
package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/version"
	"github.com/oronbz/ghud/internal/widget"
)

// Protocol is the protocol version this ghud speaks.
const Protocol = 1

const (
	// firstRestart is the wait before restarting a crashed Plugin; each
	// crash in a row doubles it, up to maxRestart.
	firstRestart = time.Second
	maxRestart   = time.Minute
	// steady is how long a process must run for its crash to count as a first one again.
	steady           = time.Minute
	handshakeTimeout = 10 * time.Second
	invokeTimeout    = time.Minute
	// exitWait is how long ghud waits for a Plugin to exit once told to.
	exitWait = time.Second
)

// Registry is every Plugin ghud found, keyed by directory, so finding them
// again after a config change keeps the processes already running.
type Registry struct {
	mu      sync.Mutex
	plugins map[string]*Plugin
}

func NewRegistry() *Registry { return &Registry{plugins: map[string]*Plugin{}} }

// Dirs are the Plugin directories to read: each directory under
// pluginsDir holding a manifest, then paths as listed.
func Dirs(pluginsDir string, paths []string) []string {
	var dirs []string
	entries, _ := os.ReadDir(pluginsDir)
	for _, e := range entries {
		dir := filepath.Join(pluginsDir, e.Name())
		if _, err := os.Stat(filepath.Join(dir, ManifestFile)); err == nil {
			dirs = append(dirs, dir)
		}
	}
	return append(dirs, paths...)
}

// Discover reads the manifests in dirs and returns the Widget types they
// provide, leaving out type names in taken (the built-ins) and any a Plugin
// read earlier provides. Problems name the manifest and what's wrong.
func (r *Registry) Discover(dirs []string, taken []string) ([]widget.Type, []string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var types []widget.Type
	var problems, names []string
	taken = slices.Clone(taken)
	for _, dir := range dirs {
		m, err := ReadManifest(dir)
		if err != nil {
			problems = append(problems, err.Error())
			continue
		}
		file := filepath.Join(dir, ManifestFile)
		if slices.Contains(names, m.Name) {
			problems = append(problems, fmt.Sprintf("%s: another Plugin is already called `%s`", file, m.Name))
			continue
		}
		names = append(names, m.Name)
		p := r.plugins[dir]
		if p == nil || !reflect.DeepEqual(p.m, m) {
			p = &Plugin{m: m, widgets: map[string]*pluginWidget{}}
			r.plugins[dir] = p
		}
		for _, wt := range m.Widgets {
			if slices.Contains(taken, wt.Type) {
				problems = append(problems, fmt.Sprintf("%s: Widget type `%s` is taken; it's left out", file, wt.Type))
				continue
			}
			taken = append(taken, wt.Type)
			t, _ := wt.widgetType()
			t.New = func(c widget.Config) widget.Widget { return p.widget(t.Name, t.Settings, c) }
			types = append(types, t)
		}
	}
	return types, problems
}

// Close stops every Plugin's process and waits a moment for them to exit.
func (r *Registry) Close() {
	r.mu.Lock()
	plugins := slices.Collect(maps.Values(r.plugins))
	r.mu.Unlock()
	for _, p := range plugins {
		p.shutdown()
	}
}

// Plugin is one Plugin and its process, started when a Widget first needs
// it and restarted with backoff after a crash.
type Plugin struct {
	m Manifest

	mu    sync.Mutex
	clock ports.Clock
	proc  *process
	// fails counts crashes in a row; retry is when the process may start again.
	fails int
	retry time.Time
	// stopped is why the last process crashed.
	stopped error
	closed  bool

	wmu     sync.Mutex
	widgets map[string]*pluginWidget
}

type process struct {
	cmd     *exec.Cmd
	conn    *conn
	stdin   io.Closer
	started time.Time
	stderr  *tail
	// stopping marks a process ghud stopped on purpose, which isn't a crash.
	stopping atomic.Bool
	exited   chan struct{}
}

func (p *Plugin) widget(typ string, schema []widget.Setting, c widget.Config) *pluginWidget {
	w := &pluginWidget{p: p, id: c.ID, typ: typ, schema: schema, settings: settingsJSON(c.Settings)}
	p.mu.Lock()
	if p.clock == nil {
		p.clock = c.Ports.Clock
	}
	p.mu.Unlock()
	p.wmu.Lock()
	p.widgets[w.id] = w
	p.wmu.Unlock()
	return w
}

// connect returns the running process, starting it if it isn't, with
// Widget w opened on it.
func (p *Plugin) connect(ctx context.Context, w *pluginWidget) (*process, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.m.Protocol != Protocol {
		return nil, p.versionErr("declares", p.m.Protocol)
	}
	if p.proc == nil {
		if p.closed {
			return nil, p.named(errClosed)
		}
		if now := p.clock.Now(); now.Before(p.retry) {
			return nil, p.down(now)
		}
		proc, err := p.start(ctx)
		if err != nil {
			return nil, err
		}
		p.proc = proc
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.on != p.proc {
		open := openParams{frameParams: frameOf(w.id, w.shown), Type: w.typ, Settings: w.settings}
		if err := p.proc.conn.Notify("widget/open", open); err != nil {
			return nil, p.callErr(err)
		}
		w.on = p.proc
	}
	return p.proc, nil
}

// start runs the Plugin's command and shakes hands with it.
func (p *Plugin) start(ctx context.Context) (*process, error) {
	name := p.m.Command[0]
	if strings.ContainsAny(name, `/\`) && !filepath.IsAbs(name) {
		name = filepath.Join(p.m.Dir, name)
	}
	cmd := exec.Command(name, p.m.Command[1:]...)
	cmd.Dir = p.m.Dir
	cmd.WaitDelay = exitWait
	proc := &process{cmd: cmd, started: p.clock.Now(), stderr: &tail{}, exited: make(chan struct{})}
	cmd.Stderr = proc.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, p.named(fmt.Errorf("didn't start: %w", err))
	}
	proc.stdin = stdin
	proc.conn = newConn(stdout, stdin, p.notified, func(err error) { p.ended(proc, err) })
	ctx, cancel := context.WithTimeout(ctx, handshakeTimeout)
	defer cancel()
	var res initializeResult
	if err := proc.conn.Call(ctx, "initialize", initializeParams{Protocol: Protocol, Ghud: version.Version}, &res); err != nil {
		proc.stop()
		<-proc.exited
		return nil, p.named(fmt.Errorf("failed its handshake: %w", proc.why(err)))
	}
	if res.Protocol != Protocol {
		proc.stop()
		return nil, p.versionErr("speaks", res.Protocol)
	}
	return proc, nil
}

func (p *Plugin) versionErr(verb string, v int) error {
	return &widget.SetupError{Hint: fmt.Sprintf("Plugin `%s` %s protocol %d, but this ghud speaks %d. Update ghud or the Plugin.", p.m.Name, verb, v, Protocol)}
}

// ended handles the end of proc's session: a crash fails its Widgets and
// sets when the process may start again.
func (p *Plugin) ended(proc *process, err error) {
	if proc.cmd.Process != nil {
		_ = proc.cmd.Process.Kill()
	}
	waitErr := proc.cmd.Wait()
	close(proc.exited)
	p.mu.Lock()
	if p.proc != proc || proc.stopping.Load() {
		p.mu.Unlock()
		return
	}
	p.proc = nil
	now := p.clock.Now()
	if now.Sub(proc.started) >= steady {
		p.fails = 0
	}
	p.fails++
	p.retry = now.Add(restartDelay(p.fails))
	if errors.Is(err, io.EOF) {
		err = waitErr
	}
	if err == nil {
		err = errors.New("exited")
	}
	p.stopped = proc.stderr.with(err)
	retry, crash := p.retry, p.down(now)
	p.mu.Unlock()
	for _, w := range p.listed() {
		w.deliver(widget.Push{Err: crash, Lost: true, Retry: retry})
	}
}

func restartDelay(fails int) time.Duration {
	d := firstRestart
	for i := 1; i < fails && d < maxRestart; i++ {
		d *= 2
	}
	return min(d, maxRestart)
}

// down is the error a stopped Plugin's Widgets show until it restarts.
func (p *Plugin) down(now time.Time) error {
	hint := "Restarting it in " + widget.ShortDuration(p.retry.Sub(now).Round(time.Second))
	return failure{msg: fmt.Sprintf("Plugin `%s` stopped: %v", p.m.Name, p.stopped), reason: "plugin stopped", hint: hint}
}

func (p *Plugin) listed() []*pluginWidget {
	p.wmu.Lock()
	defer p.wmu.Unlock()
	ws := make([]*pluginWidget, 0, len(p.widgets))
	for _, w := range p.widgets {
		ws = append(ws, w)
	}
	return ws
}

// notified handles a notification from the Plugin.
func (p *Plugin) notified(method string, params json.RawMessage) {
	if method != "widget/update" {
		return
	}
	var u update
	if err := json.Unmarshal(params, &u); err != nil {
		return
	}
	p.wmu.Lock()
	w := p.widgets[u.Widget]
	p.wmu.Unlock()
	if w == nil {
		return
	}
	payload, err := u.read()
	w.deliver(widget.Push{Payload: payload, Err: p.named(err)})
}

// named puts the Plugin's name in front of errors ghud found, leaving the
// ones the Plugin reported as they are.
func (p *Plugin) named(err error) error {
	var f failure
	var setup *widget.SetupError
	var limited *widget.RateLimitError
	var rpc *RPCError
	if err == nil || errors.As(err, &f) || errors.As(err, &setup) || errors.As(err, &limited) || errors.As(err, &rpc) {
		return err
	}
	return fmt.Errorf("Plugin `%s` %w", p.m.Name, err)
}

// callErr is a failed call's error: a stopped process says so.
func (p *Plugin) callErr(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, errClosed) {
		return p.named(errors.New("stopped"))
	}
	return p.named(err)
}

// release lets go of Widget w, stopping the process once no Widget needs it.
func (p *Plugin) release(w *pluginWidget, on *process) {
	p.wmu.Lock()
	// A Widget rebuilt under the same id has taken w's place, so the Plugin keeps it.
	replaced := p.widgets[w.id] != w
	if !replaced {
		delete(p.widgets, w.id)
	}
	left := len(p.widgets)
	p.wmu.Unlock()
	p.mu.Lock()
	defer p.mu.Unlock()
	if on != nil && on == p.proc && !replaced {
		_ = on.conn.Notify("widget/close", widgetParams{Widget: w.id})
	}
	if left == 0 && p.proc != nil {
		p.proc.stop()
		p.proc = nil
	}
}

func (p *Plugin) shutdown() {
	p.mu.Lock()
	proc := p.proc
	p.proc, p.closed = nil, true
	p.mu.Unlock()
	if proc == nil {
		return
	}
	proc.stop()
	select {
	case <-proc.exited:
	case <-time.After(exitWait):
	}
}

// stop ends a process on purpose: closing stdin asks it to quit, and the
// kill that follows makes sure it does.
func (proc *process) stop() {
	proc.stopping.Store(true)
	proc.conn.Close()
	_ = proc.stdin.Close()
	_ = proc.cmd.Process.Kill()
}

// why is the reason a call failed: how the process ended, when it did.
func (proc *process) why(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, errClosed) {
		<-proc.exited
		return proc.stderr.with(proc.cmd.ProcessState)
	}
	return err
}

// tail keeps the end of a Plugin's stderr, to say why it stopped.
type tail struct {
	mu  sync.Mutex
	buf []byte
}

func (t *tail) Write(b []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, b...)
	if len(t.buf) > 4096 {
		t.buf = t.buf[len(t.buf)-4096:]
	}
	return len(b), nil
}

// with is err followed by the last line the Plugin wrote to stderr.
func (t *tail) with(err any) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	lines := strings.Split(strings.TrimSpace(string(t.buf)), "\n")
	if last := strings.TrimSpace(lines[len(lines)-1]); last != "" {
		return fmt.Errorf("%v (%s)", err, last)
	}
	return fmt.Errorf("%v", err)
}

// pluginWidget adapts one Widget of a Plugin's type to the Widget contract.
type pluginWidget struct {
	p        *Plugin
	id, typ  string
	schema   []widget.Setting
	settings map[string]any

	mu     sync.Mutex
	shown  widget.Shown
	push   func(widget.Push)
	closed bool
	// on is the process the Widget is open on.
	on *process
}

func (w *pluginWidget) Refresh(ctx context.Context, _ time.Time) (widget.Payload, error) {
	proc, err := w.p.connect(ctx, w)
	if err != nil {
		return widget.Payload{}, err
	}
	var u update
	if err := proc.conn.Call(ctx, "widget/refresh", widgetParams{Widget: w.id}, &u); err != nil {
		return widget.Payload{}, w.p.callErr(err)
	}
	p, err := u.read()
	switch {
	case err != nil:
		return widget.Payload{}, w.p.named(err)
	case p == nil:
		return widget.Payload{}, w.p.named(errors.New("answered widget/refresh with no payload"))
	}
	return *p, nil
}

func (w *pluginWidget) Invoke(ctx context.Context, inv widget.Invocation) (widget.Result, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, invokeTimeout)
		defer cancel()
	}
	proc, err := w.p.connect(ctx, w)
	if err != nil {
		return widget.Result{}, err
	}
	var res invokeResult
	err = proc.conn.Call(ctx, "action/invoke", invokeParams{Widget: w.id, Action: inv.Action, Item: inv.Item, Text: inv.Text, Choice: inv.Choice, Base: inv.Base}, &res)
	if err != nil {
		return widget.Result{}, w.p.callErr(err)
	}
	settings, err := res.settings(w.schema)
	if err != nil {
		return widget.Result{}, w.p.named(err)
	}
	res.Result.Settings = settings
	return res.Result, nil
}

func (w *pluginWidget) Choices(ctx context.Context, inv widget.Invocation) ([]widget.Choice, error) {
	proc, err := w.p.connect(ctx, w)
	if err != nil {
		return nil, err
	}
	var res choicesResult
	if err := proc.conn.Call(ctx, "action/choices", choicesParams{Widget: w.id, Action: inv.Action, Item: inv.Item}, &res); err != nil {
		return nil, w.p.callErr(err)
	}
	choices := make([]widget.Choice, len(res.Choices))
	for i, c := range res.Choices {
		choices[i] = widget.Choice{ID: c.ID, Label: c.Label, Target: c.Target}
	}
	return choices, nil
}

func (w *pluginWidget) Listen(push func(widget.Push)) {
	w.mu.Lock()
	if w.closed {
		w.mu.Unlock()
		return
	}
	w.push = push
	w.mu.Unlock()
	if _, err := w.p.connect(context.Background(), w); err != nil {
		w.deliver(widget.Push{Err: err})
	}
}

func (w *pluginWidget) Observe(s widget.Shown) {
	w.mu.Lock()
	defer w.mu.Unlock()
	w.shown = s
	if w.on != nil {
		_ = w.on.conn.Notify("widget/frame", frameOf(w.id, s))
	}
}

func (w *pluginWidget) Close() {
	w.mu.Lock()
	w.closed, w.push = true, nil
	on := w.on
	w.mu.Unlock()
	go w.p.release(w, on)
}

func (w *pluginWidget) deliver(push widget.Push) {
	w.mu.Lock()
	fn := w.push
	w.mu.Unlock()
	if fn != nil {
		fn(push)
	}
}
