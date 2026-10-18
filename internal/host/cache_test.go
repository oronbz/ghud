package host_test

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// upstream is an API that several hosts' News Widgets fetch from; every
// fetch gets the next edition.
type upstream struct {
	mu      sync.Mutex
	fetches int
	// during, when set, runs inside a fetch, while it's in flight.
	during func()
}

func (u *upstream) count() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.fetches
}

type news struct {
	up    *upstream
	topic string
}

func (n news) Refresh(context.Context, time.Time) (widget.Payload, error) {
	u := n.up
	u.mu.Lock()
	u.fetches++
	edition, during := u.fetches, u.during
	u.mu.Unlock()
	if during != nil {
		during()
	}
	return widget.Payload{Lines: []widget.Line{widget.Text(theme.Fg, fmt.Sprintf("%s edition %d", n.topic, edition))}}, nil
}

// newsType is a fetching Widget type refreshing every 2m.
func newsType(u *upstream) widget.Type {
	return widget.Type{
		Name: "news", Title: "News", Refresh: 2 * time.Minute, Floor: 30 * time.Second, Fetches: true,
		Settings: []widget.Setting{{Name: "topic", Kind: widget.String, Default: "world", Description: "What the news is about."}},
		New: func(c widget.Config) widget.Widget {
			return news{up: u, topic: c.Settings.String("topic")}
		},
	}
}

// newsHost is a host showing one News Widget, its cache under root.
func newsHost(t *testing.T, u *upstream, root string, now time.Time) *hosttest.Host {
	t.Helper()
	return hosttest.New(t, hosttest.Options{Root: root, Now: now, Width: 60, Height: 8, Types: []widget.Type{newsType(u)}})
}

func TestTheFirstFrameDrawsTheCachedEntryWithItsAgeUntilFreshDataLands(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	newsHost(t, u, root, hosttest.DefaultNow)

	h := newsHost(t, u, root, hosttest.DefaultNow.Add(time.Minute))
	if u.count() != 1 {
		t.Fatalf("fetched %d times, want 1: a second instance a minute later draws the cached entry", u.count())
	}
	if top, body := h.Line(0), h.Screen(); !strings.Contains(body, "world edition 1") || !strings.HasSuffix(top, " 1m ─╮") {
		t.Errorf("first frame:\n%s\nwant the cached edition 1 and its age 1m at the right of the top border", body)
	}

	h.Advance(2 * time.Minute)
	if top, body := h.Line(0), h.Screen(); !strings.Contains(body, "world edition 2") || strings.Contains(top, "m ─╮") {
		t.Errorf("after the next refresh:\n%s\nwant edition 2 and no age", body)
	}
}
