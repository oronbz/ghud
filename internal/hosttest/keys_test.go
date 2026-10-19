package hosttest_test

import (
	"testing"

	"github.com/oronbz/visor/internal/hosttest"
)

func TestKeyPrintsTheWayBubbleTeaDecodesIt(t *testing.T) {
	for _, s := range []string{
		"q", "Q", "H", ":", "?", "1", "+",
		"enter", "esc", "tab", "shift+tab", "space", "backspace",
		"up", "shift+left", "ctrl+k", "ctrl+r", "alt+x", "shift+super+g",
		"leftshift", "rightshift", "leftctrl", "rightctrl", "leftalt", "leftsuper",
	} {
		if got := hosttest.Key(s).String(); got != s {
			t.Errorf("Key(%q).String() = %q", s, got)
		}
	}
}
