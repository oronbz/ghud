package plugin_test

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/plugin"
	"github.com/oronbz/ghud/internal/plugin/plugintest"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/widget"
)

var t0 = time.Date(2026, 10, 18, 14, 5, 9, 0, time.UTC)

// clock is a settable clock; the plugin host only reads it.
type clock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *clock) After(time.Duration, func(time.Time) tea.Msg) tea.Cmd { return nil }

func (c *clock) add(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

// fixture discovers a fixture Plugin written with o and returns its Widget type.
func fixture(t *testing.T, o plugintest.Options) (*plugin.Registry, widget.Type) {
	t.Helper()
	dir := plugintest.Write(t, filepath.Join(t.TempDir(), "fixture"), o)
	r := plugin.NewRegistry()
	t.Cleanup(r.Close)
	types, problems := r.Discover([]string{dir}, nil)
	if len(problems) > 0 {
		t.Fatalf("problems: %v", problems)
	}
	if len(types) != 1 {
		t.Fatalf("types = %v, want the fixture's one", types)
	}
	return r, types[0]
}

// open builds Widget id of typ with settings over its defaults.
func open(t *testing.T, typ widget.Type, id string, c *clock, settings widget.Settings) widget.Widget {
	t.Helper()
	s := widget.Defaults(typ.Settings)
	for k, v := range settings {
		s[k] = v
	}
	w := typ.New(widget.Config{ID: id, Settings: s, Ports: ports.Ports{Clock: c}})
	t.Cleanup(w.(widget.Closer).Close)
	return w
}

func refresh(t *testing.T, w widget.Widget) (widget.Payload, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return w.(widget.Source).Refresh(ctx, t0)
}

func text(f widget.Field) string {
	var s string
	for _, sp := range f {
		s += sp.Text
	}
	return s
}

func TestAPluginWidgetTypeComesFromItsManifest(t *testing.T) {
	_, typ := fixture(t, plugintest.Options{Refresh: "2m"})
	if typ.Name != "fixture" || typ.Title != "Fixture" || typ.Refresh != 2*time.Minute || typ.Floor != 10*time.Second {
		t.Fatalf("type = %+v", typ)
	}
	if !typ.Fetches || !typ.OptIn {
		t.Errorf("Fetches, OptIn = %t, %t; want a cached, opt-in type", typ.Fetches, typ.OptIn)
	}
	if len(typ.Settings) != 2 || typ.Settings[0].Name != "greeting" || typ.Settings[0].Default != "hello" || typ.Settings[1].Choices[1] != "lines" {
		t.Errorf("settings = %+v", typ.Settings)
	}
}

func TestAPluginWidgetRefreshesItsItemsFromTheProcess(t *testing.T) {
	_, typ := fixture(t, plugintest.Options{})
	w := open(t, typ, "fx", &clock{now: t0}, widget.Settings{"greeting": "hi"})
	p, err := refresh(t, w)
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Items) != 2 || p.Items[0].ID != "alpha" || text(p.Items[0].Fields[0]) != "hi alpha" || text(p.Items[1].Fields[1]) != "refresh 1" {
		t.Fatalf("payload = %+v", p)
	}
	if a := p.Items[0].Actions[1]; a.Verb != widget.Open || a.Target != "https://example.com/alpha" || !a.Primary {
		t.Errorf("open action = %+v", a)
	}
	if a := p.Actions[1]; a.Input != widget.SecretInput {
		t.Errorf("token action = %+v, want secret input", a)
	}
	if p, _ = refresh(t, w); text(p.Items[0].Fields[1]) != "refresh 2" {
		t.Errorf("second refresh = %v, want refresh 2", text(p.Items[0].Fields[1]))
	}
}
