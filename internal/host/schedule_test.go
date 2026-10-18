package host_test

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// feed is a Source that records when it was refreshed and answers with the
// next scripted error, or with one Item when none is left.
type feed struct{ log *feedLog }

type feedLog struct {
	mu   sync.Mutex
	runs []time.Time
	errs []error
	// during, when set, runs inside Refresh, while the refresh is in flight.
	during func()
}

func (f feed) Refresh(_ context.Context, now time.Time) (widget.Payload, error) {
	f.log.mu.Lock()
	f.log.runs = append(f.log.runs, now)
	var err error
	if len(f.log.errs) > 0 {
		err, f.log.errs = f.log.errs[0], f.log.errs[1:]
	}
	during := f.log.during
	f.log.mu.Unlock()
	if during != nil {
		during()
	}
	if err != nil {
		return widget.Payload{}, err
	}
	return widget.Payload{Columns: []widget.Column{{Flex: true}}, Items: items("", "headline"), Count: 1}, nil
}

func (l *feedLog) count() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.runs)
}

// fail makes the next refreshes fail with errs, in order.
func (l *feedLog) fail(errs ...error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.errs = append(l.errs, errs...)
}

// feedType is a fetching Widget type refreshing every 2m, never more often than every 30s.
func feedType(name, title string, log *feedLog) widget.Type {
	return widget.Type{
		Name: name, Title: title, Refresh: 2 * time.Minute, Floor: 30 * time.Second,
		New: func(widget.Config) widget.Widget { return feed{log: log} },
	}
}

func withFeed(t *testing.T, o hosttest.Options) (*hosttest.Host, *feedLog) {
	t.Helper()
	log := &feedLog{}
	o.Types = append(o.Types, feedType("feed", "Feed", log))
	if o.Width == 0 {
		o.Width, o.Height = 90, 10
	}
	return hosttest.New(t, o), log
}

func TestRefreshBelowTheFloorIsClampedWithAToast(t *testing.T) {
	h, log := withFeed(t, hosttest.Options{Config: "[[widget]]\ntype = \"feed\"\nrefresh = \"5s\"\n"})

	if footer := h.Line(9); !strings.Contains(footer, "Feed: refresh 5s is below the feed minimum; using 30s") {
		t.Errorf("footer %q, want the clamping toast", footer)
	}
	h.Advance(29 * time.Second)
	if log.count() != 1 {
		t.Errorf("refreshed %d times in the first 29s, want 1 (at start)", log.count())
	}
	h.Advance(time.Second)
	if log.count() != 2 {
		t.Errorf("refreshed %d times by 30s, want 2", log.count())
	}
}

func TestRefreshSettingOverridesTheTypesInterval(t *testing.T) {
	h, log := withFeed(t, hosttest.Options{Config: "[[widget]]\ntype = \"feed\"\nrefresh = \"5m\"\n"})

	h.Advance(4 * time.Minute)
	if log.count() != 1 {
		t.Errorf("refreshed %d times in 4m with refresh = 5m, want 1", log.count())
	}
	h.Advance(time.Minute)
	if log.count() != 2 {
		t.Errorf("refreshed %d times by 5m, want 2", log.count())
	}
}

func TestHiddenWidgetIsNotRefreshedUntilShown(t *testing.T) {
	root := savedLayout(t, `{"default": {"rows": [{"weight": 1, "widgets": [{"id": "clock", "weight": 1}, {"id": "feed", "weight": 1}]}], "hidden": ["feed"]}}`)
	clockType := liveViews[0]
	h, log := withFeed(t, hosttest.Options{Root: root, Types: []widget.Type{clockType}})

	h.Advance(10 * time.Minute)
	if log.count() != 0 {
		t.Fatalf("hidden Widget refreshed %d times, want none", log.count())
	}

	h.Press("w", "j", "space", "esc")
	if log.count() != 1 {
		t.Errorf("showing the Widget refreshed it %d times, want once", log.count())
	}
	if _, _, ok := h.Find("headline"); !ok {
		t.Errorf("shown Widget lacks its Item:\n%s", h.Screen())
	}
	h.Advance(2 * time.Minute)
	if log.count() != 2 {
		t.Errorf("refreshed %d times 2m after showing, want 2", log.count())
	}
}

var _ = theme.Warn
