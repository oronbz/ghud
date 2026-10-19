package cache_test

import (
	"reflect"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/cache"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

func TestAnEntryReadsBackAsItWasSaved(t *testing.T) {
	store := cache.Store{Dir: t.TempDir()}
	at := time.Date(2026, 10, 18, 14, 5, 9, 0, time.UTC)
	want := cache.Entry{
		Payload: widget.Payload{
			Lines:   []widget.Line{{Spans: []widget.Span{{Text: "42%", Token: theme.Accent, Bold: true, Bar: &widget.Bar{Value: 0.42}}}}, {Chart: &widget.Chart{Values: []float64{0.1, 0.5}, Token: theme.Warn}}},
			Columns: []widget.Column{{Flex: true}, {Priority: 1, Right: true}},
			Items: []widget.Item{{ID: "#12", Group: "Review requested", Fields: []widget.Field{widget.F(theme.Fg, "Fix it"), widget.F(theme.Dim, "2h")},
				Actions: []widget.Action{{ID: "open", Key: "o", Label: "open", Primary: true, Verb: widget.Open, Target: "https://github.com/o/r/pull/12"}}}},
			Count:   1,
			Actions: []widget.Action{{ID: "add", Key: "a", Label: "add", Input: widget.TextInput, Prompt: "New"}},
		},
		FetchedAt: at,
		CheckedAt: at.Add(time.Minute),
		Fails:     2,
		Status:    cache.Status{Error: "dial tcp: network is unreachable", Reason: "offline", Hint: "Check your connection"},
	}
	key := cache.Key("prs", "github-prs", "0.1.0", widget.Settings{"query": "is:open"})
	if err := store.Save(key, want); err != nil {
		t.Fatal(err)
	}
	got, ok := store.Load(key)
	if !ok || !reflect.DeepEqual(got, want) {
		t.Errorf("read back %+v, want %+v", got, want)
	}
}

func TestAnUpgradeStartsANewEntry(t *testing.T) {
	settings := widget.Settings{"query": "is:open"}
	if cache.Key("prs", "prs", "0.1.0", settings) == cache.Key("prs", "prs", "0.1.1", settings) {
		t.Error("two ghud versions share a cache entry")
	}
}
