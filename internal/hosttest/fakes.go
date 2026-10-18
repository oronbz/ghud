package hosttest

import (
	"context"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/ports"
)

// Clock is a fake clock. Timers fire only when the test calls Host.Advance.
type Clock struct {
	mu     sync.Mutex
	now    time.Time
	timers []timer
	seq    int
}

type timer struct {
	due time.Time
	seq int
	msg func(time.Time) tea.Msg
}

func newClock(now time.Time) *Clock { return &Clock{now: now} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

// After arms its timer when the Cmd runs, as tea.Tick does.
func (c *Clock) After(d time.Duration, msg func(time.Time) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		c.mu.Lock()
		defer c.mu.Unlock()
		c.seq++
		c.timers = append(c.timers, timer{due: c.now.Add(d), seq: c.seq, msg: msg})
		return nil
	}
}

// Pending is the number of armed timers.
func (c *Clock) Pending() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timers)
}

func (c *Clock) set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = t
}

func (c *Clock) fireNext(limit time.Time) (tea.Msg, bool) {
	c.mu.Lock()
	if len(c.timers) == 0 {
		c.mu.Unlock()
		return nil, false
	}
	i := 0
	for j, t := range c.timers {
		if t.due.Before(c.timers[i].due) || (t.due.Equal(c.timers[i].due) && t.seq < c.timers[i].seq) {
			i = j
		}
	}
	t := c.timers[i]
	if t.due.After(limit) {
		c.mu.Unlock()
		return nil, false
	}
	c.timers = slices.Delete(c.timers, i, i+1)
	c.now = t.due
	c.mu.Unlock()
	return t.msg(t.due), true
}

// Machine reports the Stats the test last Set, zero until then.
type Machine struct {
	mu    sync.Mutex
	stats ports.Stats
	Err   error
}

func (m *Machine) Set(s ports.Stats) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stats = s
}

func (m *Machine) Stats() (ports.Stats, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.stats, m.Err
}

// Opener records every target handed to the system opener.
type Opener struct {
	Opened []string
	Err    error
}

func (o *Opener) Open(target string) error {
	o.Opened = append(o.Opened, target)
	return o.Err
}

// Clipboard records every OSC 52 copy.
type Clipboard struct{ Copied []string }

func (c *Clipboard) Copy(text string) tea.Cmd {
	return func() tea.Msg {
		c.Copied = append(c.Copied, text)
		return nil
	}
}

// Editor records every $EDITOR handoff. OnEdit, when set, plays the user's
// edit of the file before the TUI resumes.
type Editor struct {
	Edited []string
	OnEdit func(path string) error
}

func (e *Editor) Edit(path string, done func(error) tea.Msg) tea.Cmd {
	return func() tea.Msg {
		e.Edited = append(e.Edited, path)
		var err error
		if e.OnEdit != nil {
			err = e.OnEdit(path)
		}
		return done(err)
	}
}

// Keyring is an in-memory keyring. A non-nil Err makes every call fail, as when no keyring exists.
type Keyring struct {
	mu      sync.Mutex
	secrets map[string]string
	Err     error
}

func newKeyring() *Keyring { return &Keyring{secrets: map[string]string{}} }

func (k *Keyring) Get(service, account string) (string, error) {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.Err != nil {
		return "", k.Err
	}
	s, ok := k.secrets[service+"\x00"+account]
	if !ok {
		return "", ports.ErrSecretNotFound
	}
	return s, nil
}

func (k *Keyring) Set(service, account, secret string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.Err != nil {
		return k.Err
	}
	k.secrets[service+"\x00"+account] = secret
	return nil
}

func (k *Keyring) Delete(service, account string) error {
	k.mu.Lock()
	defer k.mu.Unlock()
	if k.Err != nil {
		return k.Err
	}
	if _, ok := k.secrets[service+"\x00"+account]; !ok {
		return ports.ErrSecretNotFound
	}
	delete(k.secrets, service+"\x00"+account)
	return nil
}

// Commands fakes other CLIs. Unscripted commands fail as if not installed.
type Commands struct {
	mu      sync.Mutex
	Calls   [][]string
	scripts map[string]result
}

type result struct {
	out string
	err error
}

// Script makes the command line `name args...` print out and return err.
func (c *Commands) Script(out string, err error, name string, args ...string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.scripts == nil {
		c.scripts = map[string]result{}
	}
	c.scripts[strings.Join(append([]string{name}, args...), " ")] = result{out, err}
}

func (c *Commands) Output(_ context.Context, name string, args ...string) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	line := append([]string{name}, args...)
	c.Calls = append(c.Calls, line)
	r, ok := c.scripts[strings.Join(line, " ")]
	if !ok {
		return nil, &exec.Error{Name: name, Err: exec.ErrNotFound}
	}
	return []byte(r.out), r.err
}

// EventKit fakes macOS calendar access. Access defaults to AccessUnsupported.
type EventKit struct {
	Access   map[ports.Entity]ports.Access
	Granted  bool // what the user answers to a Request prompt
	Requests []ports.Entity
}

func (e *EventKit) Status(en ports.Entity) ports.Access { return e.Access[en] }

func (e *EventKit) Request(_ context.Context, en ports.Entity) (ports.Access, error) {
	e.Requests = append(e.Requests, en)
	if e.Access[en] == ports.AccessUndetermined {
		e.Access[en] = ports.AccessDenied
		if e.Granted {
			e.Access[en] = ports.AccessGranted
		}
	}
	return e.Access[en], nil
}
