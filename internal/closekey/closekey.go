// Package closekey parses --close-key values, written in herdr's key syntax,
// and follows key presses through them.
package closekey

import (
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	tea "charm.land/bubbletea/v2"
)

// Key is one combo: modifiers and exactly one key.
type Key struct {
	Code rune
	Mod  tea.KeyMod
}

// Seq is one close key: a chord, or a sequence of two combos pressed in turn.
type Seq []Key

// herdr's meta is alt, and cmd is super.
var modifiers = map[string]tea.KeyMod{
	"ctrl": tea.ModCtrl, "control": tea.ModCtrl,
	"shift": tea.ModShift,
	"alt":   tea.ModAlt, "option": tea.ModAlt, "meta": tea.ModAlt,
	"cmd": tea.ModSuper, "command": tea.ModSuper, "super": tea.ModSuper,
	"hyper": tea.ModHyper,
}

var named = map[string]rune{
	"space": tea.KeySpace, "enter": tea.KeyEnter, "return": tea.KeyEnter,
	"esc": tea.KeyEscape, "escape": tea.KeyEscape, "tab": tea.KeyTab,
	"backspace": tea.KeyBackspace, "bs": tea.KeyBackspace,
	"left": tea.KeyLeft, "right": tea.KeyRight, "up": tea.KeyUp, "down": tea.KeyDown,
	"minus": '-', "comma": ',', "period": '.', "slash": '/', "backslash": '\\',
	"quote": '\'', "double_quote": '"', "double-quote": '"', "semicolon": ';',
	"colon": ':', "percent": '%', "ampersand": '&', "backtick": '`', "plus": '+',
}

func init() {
	for n := range 12 {
		named[fmt.Sprintf("f%d", n+1)] = tea.KeyF1 + rune(n)
	}
}

const locks = tea.ModCapsLock | tea.ModNumLock | tea.ModScrollLock

// Parse reads a chord ("cmd+shift+g") or a space-separated sequence of two ("ctrl+b d").
func Parse(s string) (Seq, error) {
	fields := strings.Fields(s)
	if len(fields) == 0 {
		return nil, errors.New("no key given")
	}
	if len(fields) > 2 {
		return nil, fmt.Errorf("%q: a sequence has at most two keys", s)
	}
	var seq Seq
	for _, f := range fields {
		k, err := parseCombo(f)
		if err != nil {
			return nil, err
		}
		seq = append(seq, k)
	}
	return seq, nil
}

func parseCombo(s string) (Key, error) {
	var k Key
	var tokens []string
	switch {
	case s == "+":
		tokens = []string{"+"}
	case strings.HasSuffix(s, "++"):
		tokens = append(strings.Split(strings.TrimSuffix(s, "++"), "+"), "+")
	default:
		tokens = strings.Split(s, "+")
	}
	for _, t := range tokens[:len(tokens)-1] {
		mod, ok := modifiers[strings.ToLower(t)]
		if !ok {
			return k, fmt.Errorf("%q: %q is not a modifier (use ctrl, shift, alt, cmd or hyper)", s, t)
		}
		k.Mod |= mod
	}
	name := tokens[len(tokens)-1]
	if name == "" {
		return k, fmt.Errorf("%q: no key after the modifiers", s)
	}
	if r, size := utf8.DecodeRuneInString(name); size == len(name) {
		k.Code = r
		return k.normal(), nil
	}
	code, ok := named[strings.ToLower(name)]
	if !ok {
		return k, fmt.Errorf("%q: %q is not a key", s, name)
	}
	k.Code = code
	return k, nil
}

// normal writes an uppercase letter as shift and the lowercase letter, as herdr does.
func (k Key) normal() Key {
	if unicode.IsUpper(k.Code) {
		k.Code = unicode.ToLower(k.Code)
		k.Mod |= tea.ModShift
	}
	return k
}

// Matches reports whether msg is this combo. A shifted key arrives either as
// the base key with shift or as the shifted character, depending on the terminal.
func (k Key) Matches(msg tea.KeyPressMsg) bool {
	mod := msg.Mod &^ locks
	forms := []Key{{msg.Code, mod}}
	if msg.ShiftedCode != 0 && mod&tea.ModShift != 0 {
		forms = append(forms, Key{msg.ShiftedCode, mod &^ tea.ModShift})
	}
	if r, size := utf8.DecodeRuneInString(msg.Text); size > 0 && size == len(msg.Text) && mod&^tea.ModShift == 0 {
		forms = append(forms, Key{Code: r})
	}
	for _, f := range forms {
		if f.normal() == k {
			return true
		}
	}
	return false
}

// Lapse is how long a sequence waits for its second key.
const Lapse = 2 * time.Second

type Outcome int

const (
	// Pass means the press has nothing to do with a close key.
	Pass Outcome = iota
	// Swallow means the press started a sequence, or ended one that went
	// nowhere, and must do nothing else.
	Swallow
	Close
)

// Matcher follows presses through the close keys. It's a value: Press
// returns the next state.
type Matcher struct {
	seqs []Seq
	// pending are the sequences whose first key was pressed at at.
	pending []Seq
	at      time.Time
}

func NewMatcher(seqs []Seq) Matcher { return Matcher{seqs: seqs} }

func (m Matcher) Press(msg tea.KeyPressMsg, now time.Time) (Matcher, Outcome) {
	if len(m.pending) > 0 && now.Sub(m.at) <= Lapse {
		for _, s := range m.pending {
			if s[1].Matches(msg) {
				return Matcher{seqs: m.seqs}, Close
			}
		}
		return Matcher{seqs: m.seqs}, Swallow
	}
	m.pending = nil
	for _, s := range m.seqs {
		if !s[0].Matches(msg) {
			continue
		}
		if len(s) == 1 {
			return m, Close
		}
		m.pending, m.at = append(m.pending, s), now
	}
	if len(m.pending) > 0 {
		return m, Swallow
	}
	return m, Pass
}
