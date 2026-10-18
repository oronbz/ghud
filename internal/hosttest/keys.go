package hosttest

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

var namedKeys = map[string]rune{
	"enter": tea.KeyEnter, "esc": tea.KeyEscape, "tab": tea.KeyTab, "space": tea.KeySpace,
	"backspace": tea.KeyBackspace, "delete": tea.KeyDelete,
	"up": tea.KeyUp, "down": tea.KeyDown, "left": tea.KeyLeft, "right": tea.KeyRight,
	"home": tea.KeyHome, "end": tea.KeyEnd, "pgup": tea.KeyPgUp, "pgdown": tea.KeyPgDown,
}

var modifiers = map[string]tea.KeyMod{
	"ctrl": tea.ModCtrl, "alt": tea.ModAlt, "shift": tea.ModShift,
	"meta": tea.ModMeta, "hyper": tea.ModHyper, "super": tea.ModSuper,
}

// loneModifiers are the modifier keys a kitty terminal with report-all-keys
// sends on their own, with the modifier they set.
var loneModifiers = map[string]tea.KeyPressMsg{
	"leftshift": {Code: tea.KeyLeftShift, Mod: tea.ModShift}, "rightshift": {Code: tea.KeyRightShift, Mod: tea.ModShift},
	"leftctrl": {Code: tea.KeyLeftCtrl, Mod: tea.ModCtrl}, "rightctrl": {Code: tea.KeyRightCtrl, Mod: tea.ModCtrl},
	"leftalt": {Code: tea.KeyLeftAlt, Mod: tea.ModAlt}, "rightalt": {Code: tea.KeyRightAlt, Mod: tea.ModAlt},
	"leftsuper": {Code: tea.KeyLeftSuper, Mod: tea.ModSuper}, "rightsuper": {Code: tea.KeyRightSuper, Mod: tea.ModSuper},
}

// Key builds the press a terminal would deliver for s, written the way
// tea.KeyPressMsg.String() prints it: "q", "Q", "enter", "ctrl+k", "shift+tab", "shift+super+g",
// or a lone modifier ("leftshift", "rightctrl").
func Key(s string) tea.KeyPressMsg {
	if s == "+" {
		return tea.KeyPressMsg{Code: '+', Text: "+"}
	}
	if k, ok := loneModifiers[s]; ok {
		return k
	}
	parts := strings.Split(s, "+")
	var k tea.KeyPressMsg
	for _, p := range parts[:len(parts)-1] {
		mod, ok := modifiers[p]
		if !ok {
			panic(fmt.Sprintf("hosttest.Key(%q): unknown modifier %q", s, p))
		}
		k.Mod |= mod
	}
	name := parts[len(parts)-1]
	if code, ok := namedKeys[name]; ok {
		k.Code = code
		if code == tea.KeySpace && k.Mod&^tea.ModShift == 0 {
			k.Text = " "
		}
		return k
	}
	r, size := utf8.DecodeRuneInString(name)
	if size != len(name) {
		panic(fmt.Sprintf("hosttest.Key(%q): unknown key %q", s, name))
	}
	if unicode.IsUpper(r) {
		r = unicode.ToLower(r)
		k.Mod |= tea.ModShift
	}
	k.Code = r
	if k.Mod&^tea.ModShift == 0 {
		if k.Mod&tea.ModShift != 0 {
			k.Text = string(unicode.ToUpper(r))
		} else {
			k.Text = string(r)
		}
	}
	return k
}
