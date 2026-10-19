package closekey_test

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	uv "github.com/charmbracelet/ultraviolet"

	"github.com/oronbz/visor/internal/closekey"
)

// press decodes the bytes a terminal sends for one key, as Bubble Tea does.
func press(t *testing.T, raw string) tea.KeyPressMsg {
	t.Helper()
	var d uv.EventDecoder
	n, ev := d.Decode([]byte(raw))
	k, ok := ev.(uv.KeyPressEvent)
	if !ok || n != len(raw) {
		t.Fatalf("%q decodes to %#v (%d bytes)", raw, ev, n)
	}
	return tea.KeyPressMsg{Text: k.Text, Mod: k.Mod, Code: k.Code, ShiftedCode: k.ShiftedCode, BaseCode: k.BaseCode}
}

func TestCombosMatchWhatTerminalsSend(t *testing.T) {
	for _, tc := range []struct {
		combo string
		raw   []string
	}{
		{"cmd+shift+g", []string{"\x1b[103;10u", "\x1b[103:71;10u"}},
		{"super+shift+g", []string{"\x1b[103;10u"}},
		{"command+G", []string{"\x1b[103:71;10u"}},
		{"ctrl+b", []string{"\x02", "\x1b[98;5u"}},
		{"Control+B", []string{"\x1b[98;6u"}},
		{"d", []string{"d", "\x1b[100u"}},
		{"G", []string{"G", "\x1b[71;2u", "\x1b[103:71;2u"}},
		{"shift+g", []string{"G"}},
		{"meta+d", []string{"\x1bd"}},
		{"alt+d", []string{"\x1b[100;3u"}},
		{"option+d", []string{"\x1bd"}},
		{"?", []string{"?", "\x1b[47:63;2u"}},
		{"slash", []string{"/"}},
		{"minus", []string{"-"}},
		{"+", []string{"+"}},
		{"plus", []string{"+"}},
		{"ctrl++", []string{"\x1b[43;5u"}},
		{"space", []string{" "}},
		{"enter", []string{"\r"}},
		{"return", []string{"\r"}},
		{"esc", []string{"\x1b[27u"}},
		{"shift+tab", []string{"\x1b[Z"}},
		{"f5", []string{"\x1b[15~"}},
		{"hyper+x", []string{"\x1b[120;17u"}},
		{"ctrl+left", []string{"\x1b[1;5D"}},
	} {
		seq, err := closekey.Parse(tc.combo)
		if err != nil {
			t.Errorf("Parse(%q): %v", tc.combo, err)
			continue
		}
		for _, raw := range tc.raw {
			if k := press(t, raw); !seq[0].Matches(k) {
				t.Errorf("%q does not match %q (%s)", tc.combo, raw, k)
			}
		}
	}
}

func TestCombosDoNotMatchOtherKeys(t *testing.T) {
	for _, tc := range []struct{ combo, raw string }{
		{"cmd+shift+g", "\x1b[103;9u"},
		{"cmd+shift+g", "G"},
		{"ctrl+b", "b"},
		{"ctrl+b", "\x1b[98;6u"},
		{"d", "D"},
		{"G", "g"},
		{"meta+d", "d"},
		{"cmd+d", "\x1bd"},
		{"?", "/"},
	} {
		seq, err := closekey.Parse(tc.combo)
		if err != nil {
			t.Fatalf("Parse(%q): %v", tc.combo, err)
		}
		if k := press(t, tc.raw); seq[0].Matches(k) {
			t.Errorf("%q matches %q (%s)", tc.combo, tc.raw, k)
		}
	}
}

func TestLockKeysDoNotStopAMatch(t *testing.T) {
	seq, _ := closekey.Parse("ctrl+b")
	if !seq[0].Matches(tea.KeyPressMsg{Code: 'b', Mod: tea.ModCtrl | tea.ModNumLock | tea.ModCapsLock}) {
		t.Error("ctrl+b with num lock and caps lock on does not match")
	}
}

func TestASequenceIsSpaceSeparatedCombos(t *testing.T) {
	seq, err := closekey.Parse("  ctrl+b   d ")
	if err != nil {
		t.Fatal(err)
	}
	if len(seq) != 2 || !seq[0].Matches(press(t, "\x02")) || !seq[1].Matches(press(t, "d")) {
		t.Errorf("Parse(%q) = %v", "ctrl+b d", seq)
	}
}

func TestInvalidSyntaxIsAnError(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"", "no key"},
		{"   ", "no key"},
		{"ctrl", `"ctrl" is not a key`},
		{"ctrl+", "no key"},
		{"cntrl+b", `"cntrl" is not a modifier`},
		{"ctrl+a+b", `"a" is not a modifier`},
		{"ctrl+bee", `"bee" is not a key`},
		{"f13", `"f13" is not a key`},
		{"ctrl+b d x", "at most two keys"},
	} {
		_, err := closekey.Parse(tc.in)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%q) error %v, want one mentioning %q", tc.in, err, tc.want)
		}
	}
}
