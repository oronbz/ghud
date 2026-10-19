package host_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/hosttest"
)

func TestBelowTheMinimumTileSizeACentredNoticeReplacesTheGrid(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 32, Height: 6})
	if _, _, ok := h.Find("too small"); ok {
		t.Fatalf("notice shown at exactly the minimum size:\n%s", h.Screen())
	}
	if got := cornersOn(h.Line(0)); !slices.Equal(got, []int{0, 16}) {
		t.Errorf("at the minimum size tiles start at %v, want [0 16]", got)
	}

	for _, size := range [][2]int{{31, 20}, {80, 5}} {
		h.Resize(size[0], size[1])
		const notice = "too small (needs 32×6)"
		x, y, ok := h.Find(notice)
		if !ok {
			t.Fatalf("at %dx%d no notice:\n%s", size[0], size[1], h.Screen())
		}
		if wantX, wantY := (size[0]-len([]rune(notice)))/2, (size[1]-1)/2; x != wantX || y != wantY {
			t.Errorf("at %dx%d notice at %d,%d, want centred at %d,%d", size[0], size[1], x, y, wantX, wantY)
		}
		if strings.ContainsAny(h.Screen(), "╭╰│") {
			t.Errorf("at %dx%d the grid still draws behind the notice:\n%s", size[0], size[1], h.Screen())
		}
		if zones := h.Zones(); len(zones) != 0 {
			t.Errorf("at %dx%d the notice still has %d hit zones", size[0], size[1], len(zones))
		}
	}
}

func TestTheRequiredSizeFollowsTheLayout(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 40, Height: 9, Root: savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "clock", "weight": 1}]},
		{"weight": 1, "widgets": [{"id": "calendar", "weight": 1}]}
	]}}`)})
	if _, _, ok := h.Find("too small (needs 16×11)"); !ok {
		t.Errorf("two stacked Widgets at 40x9, want the notice to ask for 16×11, footer included:\n%s", h.Screen())
	}
}

func TestASmallWeightStillGetsTheMinimumTileSize(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Types: liveViews, Width: 60, Height: 12, Root: savedLayout(t, `{"default": {"rows": [
		{"weight": 1, "widgets": [{"id": "clock", "weight": 1}, {"id": "calendar", "weight": 9}]}
	]}}`)})
	if got := cornersOn(h.Line(0)); !slices.Equal(got, []int{0, 16}) {
		t.Errorf("a 1:9 row at width 60 has tiles at %v, want the small one at its 16-cell minimum: [0 16]", got)
	}
}
