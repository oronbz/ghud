// Package hosttest is the headless host: the full ghud app built in-process
// with a temp root, an injected clock, a fixed terminal size and fake
// outside-world adapters. Tests drive it with key, mouse and resize messages
// and read the rendered screen back as text and styled cells.
package hosttest

import (
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/config"
	"github.com/oronbz/ghud/internal/host"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

type Options struct {
	// Width and Height default to 140×38.
	Width, Height int
	// Now defaults to Sunday 18 October 2026, 14:05:09 UTC.
	Now time.Time
	// Root defaults to a fresh t.TempDir(). Two hosts given the same Root share config, data and cache.
	Root string
	// Endpoints overrides HTTP base URLs, typically with httptest servers. Unset fields keep the real URLs.
	Endpoints ports.Endpoints
	// Theme pins the palette. Left nil, the app picks it from config.toml and
	// the terminal colour answers the test sends.
	Theme *theme.Theme
	// Config, when set, is written to config.toml in the config dir before the app starts.
	Config string
	// Types replaces the built-in Widget types (see host.Options.Types).
	Types []widget.Type
}

var DefaultNow = time.Date(2026, 10, 18, 14, 5, 9, 0, time.UTC)

type Host struct {
	t     testing.TB
	Root  string
	Paths paths.Paths

	Clock     *Clock
	Opener    *Opener
	Clipboard *Clipboard
	Editor    *Editor
	Keyring   *Keyring
	Commands  *Commands
	EventKit  *EventKit

	model tea.Model
	w, h  int
	quit  bool
}

func New(t testing.TB, o Options) *Host {
	t.Helper()
	if o.Width == 0 {
		o.Width = 140
	}
	if o.Height == 0 {
		o.Height = 38
	}
	if o.Now.IsZero() {
		o.Now = DefaultNow
	}
	if o.Root == "" {
		o.Root = t.TempDir()
	}
	var th theme.Theme
	if o.Theme != nil {
		th = *o.Theme
	}
	h := &Host{
		t:         t,
		Root:      o.Root,
		Paths:     paths.Under(o.Root),
		Clock:     newClock(o.Now),
		Opener:    &Opener{},
		Clipboard: &Clipboard{},
		Editor:    &Editor{},
		Keyring:   newKeyring(),
		Commands:  &Commands{},
		EventKit:  &EventKit{},
	}
	if o.Config != "" {
		if err := os.MkdirAll(h.Paths.Config, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(h.Paths.Config, config.FileName), []byte(o.Config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	m := host.New(host.Options{
		Paths: h.Paths,
		Theme: th,
		Types: o.Types,
		Ports: ports.Ports{
			Clock:     h.Clock,
			Opener:    h.Opener,
			Clipboard: h.Clipboard,
			Editor:    h.Editor,
			Keyring:   h.Keyring,
			Commands:  h.Commands,
			EventKit:  h.EventKit,
			HTTP:      &http.Client{Timeout: 5 * time.Second},
			Endpoints: mergeEndpoints(ports.DefaultEndpoints(), o.Endpoints),
		},
	})
	h.model = m
	h.run(m.Init())
	h.Resize(o.Width, o.Height)
	return h
}

func mergeEndpoints(base, over ports.Endpoints) ports.Endpoints {
	b, o := reflect.ValueOf(&base).Elem(), reflect.ValueOf(over)
	for i := range b.NumField() {
		if s := o.Field(i).String(); s != "" {
			b.Field(i).SetString(s)
		}
	}
	return base
}

// Send delivers msg to the app and runs every Cmd it returns inline, feeding
// their messages back in until the app is idle.
func (h *Host) Send(msg tea.Msg) {
	h.t.Helper()
	if h.quit {
		h.t.Fatalf("hosttest: Send(%T) after the app quit", msg)
	}
	var cmd tea.Cmd
	h.model, cmd = h.model.Update(msg)
	h.run(cmd)
}

// Press sends one key press per argument, in Key syntax ("q", "ctrl+k", "H", "shift+tab").
func (h *Host) Press(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		h.Send(Key(k))
	}
}

// Release sends one key release per argument, in Key syntax, the way a
// terminal reporting event types does. A released modifier no longer sets its own Mod.
func (h *Host) Release(keys ...string) {
	h.t.Helper()
	for _, k := range keys {
		r := tea.KeyReleaseMsg(Key(k))
		if m, ok := loneModifiers[k]; ok {
			r.Mod &^= m.Mod
		}
		h.Send(r)
	}
}

func (h *Host) Resize(w, hh int) {
	h.t.Helper()
	h.w, h.h = w, hh
	h.Send(tea.WindowSizeMsg{Width: w, Height: hh})
}

// Advance moves the fake clock forward, firing every timer that falls due on the way, in order.
func (h *Host) Advance(d time.Duration) {
	h.t.Helper()
	target := h.Clock.Now().Add(d)
	for {
		msg, ok := h.Clock.fireNext(target)
		if !ok {
			break
		}
		h.Send(msg)
	}
	h.Clock.set(target)
}

// Quit reports whether the app has asked Bubble Tea to quit.
func (h *Host) Quit() bool { return h.quit }

var cmdType = reflect.TypeFor[tea.Cmd]()

func (h *Host) run(cmd tea.Cmd) {
	if cmd == nil || h.quit {
		return
	}
	switch msg := cmd().(type) {
	case nil:
	case tea.QuitMsg:
		h.quit = true
	case tea.BatchMsg:
		for _, c := range msg {
			h.run(c)
		}
	default:
		// tea.Sequence returns an unexported []tea.Cmd; unpack it by reflection.
		if v := reflect.ValueOf(msg); v.Kind() == reflect.Slice && v.Type().Elem() == cmdType {
			for i := range v.Len() {
				h.run(v.Index(i).Interface().(tea.Cmd))
			}
			return
		}
		h.Send(msg)
	}
}
