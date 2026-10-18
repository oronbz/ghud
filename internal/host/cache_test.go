package host_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/cache"
	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
	"github.com/oronbz/ghud/internal/widgets/meetings"
	"github.com/oronbz/ghud/internal/widgets/prs"
	"github.com/oronbz/ghud/internal/widgets/reminders"
)

// upstream is an API that several hosts' News Widgets fetch from; every
// fetch gets the next edition.
type upstream struct {
	mu      sync.Mutex
	fetches int
	// during, when set, runs inside a fetch, while it's in flight.
	during func()
	// down fails every fetch with it.
	down error
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
	edition, during, down := u.fetches, u.during, u.down
	u.mu.Unlock()
	if during != nil {
		during()
	}
	if down != nil {
		return widget.Payload{}, down
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

// entryFile is the one cache entry under root.
func entryFile(t *testing.T, root string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(paths.Under(root).Cache, "widgets", "*.json"))
	if len(files) != 1 {
		t.Fatalf("cache entries %v, want one", files)
	}
	return files[0]
}

func TestTwoInstancesOnOneCacheFetchADueWidgetOnceAndBothShowIt(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	a := newsHost(t, u, root, hosttest.DefaultNow)
	b := newsHost(t, u, root, hosttest.DefaultNow)
	if u.count() != 1 {
		t.Fatalf("two instances starting together fetched %d times, want 1", u.count())
	}
	for name, h := range map[string]*hosttest.Host{"a": a, "b": b} {
		if !strings.Contains(h.Screen(), "world edition 1") {
			t.Errorf("%s shows:\n%s\nwant edition 1", name, h.Screen())
		}
	}

	a.Advance(3 * time.Minute)
	b.Changed(entryFile(t, root))
	if !strings.Contains(b.Screen(), "world edition 2") {
		t.Errorf("b shows:\n%s\nwant edition 2 adopted from a's fetch", b.Screen())
	}
	b.Advance(3 * time.Minute)
	if u.count() != 2 {
		t.Errorf("fetched %d times once due again, want 2", u.count())
	}
	if !strings.Contains(a.Screen(), "world edition 2") {
		t.Errorf("a shows:\n%s\nwant edition 2", a.Screen())
	}
}

func TestAFailedFetchInOneInstanceMarksTheWidgetStaleInBoth(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	a := newsHost(t, u, root, hosttest.DefaultNow)
	b := newsHost(t, u, root, hosttest.DefaultNow)
	u.down = offline

	a.Advance(3 * time.Minute)
	tries := u.count()
	b.Changed(entryFile(t, root))
	b.Advance(3 * time.Minute)
	if u.count() != tries {
		t.Errorf("b fetched %d times, want none: it takes a's failure and backs off with it", u.count()-tries)
	}
	for name, h := range map[string]*hosttest.Host{"a": a, "b": b} {
		if top := h.Line(0); !strings.Contains(top, "offline · 3m") || !strings.Contains(h.Screen(), "world edition 1") {
			t.Errorf("%s shows:\n%s\nwant edition 1 marked offline · 3m", name, h.Screen())
		}
	}
}

func TestCtrlRFetchesEvenWhenTheCachedEntryIsFresh(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	newsHost(t, u, root, hosttest.DefaultNow)
	h := newsHost(t, u, root, hosttest.DefaultNow.Add(time.Minute))

	h.Press("ctrl+r")
	if u.count() != 2 || !strings.Contains(h.Screen(), "world edition 2") {
		t.Errorf("fetched %d times after ^R, want 2; screen:\n%s", u.count(), h.Screen())
	}
}

func TestAnInstanceWhoseTimerFiresFirstFetchesAndTheOtherTakesItsEntry(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	a := newsHost(t, u, root, hosttest.DefaultNow)
	b := newsHost(t, u, root, hosttest.DefaultNow)

	// b fetches first; a's timer then finds the entry fresh without being told it changed.
	b.Advance(3 * time.Minute)
	a.Advance(3 * time.Minute)
	if u.count() != 2 {
		t.Errorf("fetched %d times, want 2", u.count())
	}
	if !strings.Contains(a.Screen(), "world edition 2") {
		t.Errorf("a shows:\n%s\nwant edition 2", a.Screen())
	}
}

func TestChangingAWidgetsSettingsUsesANewCacheEntry(t *testing.T) {
	u := &upstream{}
	h := hosttest.New(t, hosttest.Options{Width: 60, Height: 8, Types: []widget.Type{newsType(u)},
		Config: "[[widget]]\ntype = \"news\"\ntopic = \"tech\"\n"})

	writeConfig(t, h, "[[widget]]\ntype = \"news\"\ntopic = \"sport\"\n")
	if u.count() != 2 || !strings.Contains(h.Screen(), "sport edition 2") {
		t.Errorf("fetched %d times after the topic changed, want 2; screen:\n%s", u.count(), h.Screen())
	}
	if files, _ := filepath.Glob(filepath.Join(h.Paths.Cache, "widgets", "*.json")); len(files) != 2 {
		t.Errorf("cache entries %v, want one per topic", files)
	}

	writeConfig(t, h, "[[widget]]\ntype = \"news\"\ntopic = \"tech\"\n")
	if u.count() != 2 || !strings.Contains(h.Screen(), "tech edition 1") {
		t.Errorf("fetched %d times after the topic changed back, want 2: the tech entry is fresh; screen:\n%s", u.count(), h.Screen())
	}
}

func TestCacheEntriesAreReadableOnlyByTheUser(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows has no permission bits")
	}
	root := t.TempDir()
	newsHost(t, &upstream{}, root, hosttest.DefaultNow)

	info, err := os.Stat(entryFile(t, root))
	if err != nil {
		t.Fatal(err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("entry permissions %v, want 0600", perm)
	}
}

func TestStartRemovesEntriesOfNoConfiguredWidgetOnceTheyreOver30DaysOld(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	newsHost(t, u, root, hosttest.DefaultNow)
	configured := entryFile(t, root)
	dir := filepath.Dir(configured)
	old, recent := filepath.Join(dir, "gone-0123456789ab.json"), filepath.Join(dir, "moved-0123456789ab.json")
	ages := map[string]time.Duration{configured: 40 * 24 * time.Hour, old: 31 * 24 * time.Hour, recent: 29 * 24 * time.Hour}
	for f, age := range ages {
		if f != configured {
			if err := os.WriteFile(f, []byte("{}"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.Chtimes(f, hosttest.DefaultNow.Add(-age), hosttest.DefaultNow.Add(-age)); err != nil {
			t.Fatal(err)
		}
	}

	newsHost(t, u, root, hosttest.DefaultNow.Add(time.Minute))
	for f, want := range map[string]bool{configured: true, old: false, recent: true} {
		if _, err := os.Stat(f); (err == nil) != want {
			t.Errorf("%s exists: %v, want %v", filepath.Base(f), err == nil, want)
		}
	}
}

// lockEntry takes root's one cache entry's fetch lock at at, as an instance that then died would.
func lockEntry(t *testing.T, root string, at time.Time) {
	t.Helper()
	file := entryFile(t, root)
	store := cache.Store{Dir: filepath.Dir(file)}
	if _, ok := store.Lock(strings.TrimSuffix(filepath.Base(file), ".json"), at, time.Minute); !ok {
		t.Fatal("the entry's lock is taken")
	}
}

func TestALockHeldByAnotherInstanceHoldsOffTheFetchUntilItsAbandoned(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	h := newsHost(t, u, root, hosttest.DefaultNow)
	lockEntry(t, root, hosttest.DefaultNow.Add(90*time.Second))

	h.Advance(2*time.Minute + 30*time.Second)
	if u.count() != 1 {
		t.Errorf("fetched %d times while another instance held the lock, want 1", u.count())
	}
	h.Advance(2 * time.Minute)
	if u.count() != 2 || !strings.Contains(h.Screen(), "world edition 2") {
		t.Errorf("fetched %d times once the lock was over a minute old, want 2; screen:\n%s", u.count(), h.Screen())
	}
}

func TestAnAbandonedLockDoesntBlockFetching(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	h := newsHost(t, u, root, hosttest.DefaultNow)
	lockEntry(t, root, hosttest.DefaultNow)

	h.Advance(2*time.Minute + 30*time.Second)
	if u.count() != 2 || !strings.Contains(h.Screen(), "world edition 2") {
		t.Errorf("fetched %d times past a lock older than the fetch timeout, want 2; screen:\n%s", u.count(), h.Screen())
	}
}

func TestAnInstanceSkipsTheFetchAnotherIsMakingAndShowsItsResult(t *testing.T) {
	root, u := t.TempDir(), &upstream{}
	var b *hosttest.Host
	u.during = func() {
		u.during = nil
		b = newsHost(t, u, root, hosttest.DefaultNow)
	}
	newsHost(t, u, root, hosttest.DefaultNow)
	if u.count() != 1 {
		t.Fatalf("fetched %d times, want 1: b started while a's fetch held the lock", u.count())
	}
	b.Changed(entryFile(t, root))
	if !strings.Contains(b.Screen(), "world edition 1") {
		t.Errorf("b shows:\n%s\nwant edition 1 from a's fetch", b.Screen())
	}
}

func TestPRsMeetingsAndRemindersAreCachedWidgets(t *testing.T) {
	for _, typ := range []widget.Type{prs.Type, meetings.Type, reminders.Type} {
		t.Run(typ.Name, func(t *testing.T) {
			h := hosttest.New(t, hosttest.Options{Width: 60, Height: 10, Types: []widget.Type{typ},
				Config: "[[widget]]\ntype = \"" + typ.Name + "\"\n"})
			entryFile(t, h.Root)
		})
	}
}
