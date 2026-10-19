package host_test

import (
	"testing"
	"time"

	"github.com/oronbz/visor/internal/hosttest"
)

func closeKeys(t *testing.T) *hosttest.Host {
	t.Helper()
	return hosttest.New(t, hosttest.Options{Width: 100, Height: 30, Types: liveViews,
		CloseKeys: []string{"cmd+shift+g", "ctrl+b d"}})
}

func TestACloseKeyChordQuits(t *testing.T) {
	h := closeKeys(t)
	// What a kitty terminal reports for cmd+shift+g.
	h.Press("shift+super+g")
	if !h.Quit() {
		t.Error("cmd+shift+g did not quit")
	}
}

func TestACloseKeySequenceQuits(t *testing.T) {
	h := closeKeys(t)
	h.Press("ctrl+b", "d")
	if !h.Quit() {
		t.Error("ctrl+b d did not quit")
	}
}

func TestTheFirstKeyOfASequenceFollowedByAnotherKeyDoesNothing(t *testing.T) {
	h := closeKeys(t)
	before := h.Screen()

	h.Press("ctrl+b", "tab")
	if h.Quit() || h.Screen() != before {
		t.Fatalf("ctrl+b tab did something:\n%s", h.Screen())
	}
	h.Press("d")
	if h.Quit() {
		t.Fatal("d after an abandoned sequence quit")
	}

	h.Press("tab")
	if got := focused(t, h, []string{"Clock", "Calendar"}); got != "Clock" {
		t.Errorf("tab after the sequence focused %q, want Clock", got)
	}
}

func TestAPendingSequenceLapses(t *testing.T) {
	h := closeKeys(t)
	h.Press("ctrl+b")
	h.Advance(3 * time.Second)

	h.Press("d")
	if h.Quit() {
		t.Fatal("d long after ctrl+b quit")
	}
	h.Press("tab")
	if got := focused(t, h, []string{"Clock", "Calendar"}); got != "Clock" {
		t.Errorf("tab focused %q, want Clock", got)
	}
}

func TestCloseKeysQuitFromAnyMode(t *testing.T) {
	for name, open := range map[string][]string{
		"key map": {"?"},
		"palette": {"ctrl+k"},
		"arrange": {"1", "m"},
		"w list":  {"w"},
	} {
		h := closeKeys(t)
		h.Press(open...)
		h.Press("ctrl+b", "d")
		if !h.Quit() {
			t.Errorf("ctrl+b d in the %s did not quit", name)
		}
	}
}

func TestWithoutCloseKeysCtrlBDDoesNotQuit(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 100, Height: 30, Types: liveViews})
	h.Press("ctrl+b", "d", "shift+super+g")
	if h.Quit() {
		t.Error("visor quit with no --close-key given")
	}
}
