package herdr_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// toggleMachine runs toggle-visor.sh against a stub herdr whose live panes are
// "pane_id tab_id" lines in a file, and which logs every call it gets.
type toggleMachine struct {
	*machine
	state, panes, herdr string
}

func newToggleMachine(t *testing.T, opened string) *toggleMachine {
	m := &toggleMachine{machine: newMachine(t), state: t.TempDir()}
	m.panes = filepath.Join(m.state, "live-panes")
	m.herdr = m.stub(m.path, "herdr", `echo "$*" >> `+m.log+`
case "$1 $2 $3" in
  "pane get "*)
    tab=$(awk -v p="$3" '$1 == p { print $2 }' `+m.panes+` 2> /dev/null)
    [ -n "$tab" ] || { echo '{"error":{"code":"pane_not_found"}}'; exit 1; }
    echo '{"result":{"pane":{"pane_id":"'"$3"'","tab_id":"'"$tab"'"}}}' ;;
  "plugin pane open") echo '`+opened+`' ;;
esac`)
	return m
}

func (m *toggleMachine) live(pane, tab string) {
	m.t.Helper()
	f, err := os.OpenFile(m.panes, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		m.t.Fatal(err)
	}
	defer f.Close()
	if _, err := f.WriteString(pane + " " + tab + "\n"); err != nil {
		m.t.Fatal(err)
	}
}

func (m *toggleMachine) remember(pane string) {
	m.t.Helper()
	if err := os.WriteFile(filepath.Join(m.state, "pane"), []byte(pane+"\n"), 0o644); err != nil {
		m.t.Fatal(err)
	}
}

func (m *toggleMachine) remembered() string {
	raw, err := os.ReadFile(filepath.Join(m.state, "pane"))
	if os.IsNotExist(err) {
		return ""
	} else if err != nil {
		m.t.Fatal(err)
	}
	return strings.TrimSpace(string(raw))
}

// press runs the action the way herdr does for a key pressed in tab.
func (m *toggleMachine) press(tab string) result {
	m.t.Helper()
	cmd := exec.Command("/bin/bash", "toggle-visor.sh")
	cmd.Env = []string{
		"PATH=" + m.path + ":/usr/bin:/bin",
		"HERDR_BIN_PATH=" + m.herdr,
		"HERDR_PLUGIN_ID=visor",
		"HERDR_PLUGIN_STATE_DIR=" + m.state,
		"HERDR_TAB_ID=" + tab,
	}
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	r := result{stdout: out.String(), stderr: errb.String()}
	if exit, ok := err.(*exec.ExitError); ok {
		r.code = exit.ExitCode()
	} else if err != nil {
		m.t.Fatal(err)
	}
	if log, err := os.ReadFile(m.log); err == nil {
		r.calls = strings.Split(strings.TrimSpace(string(log)), "\n")
	}
	return r
}

const openedP9 = `{"result":{"plugin_pane":{"entrypoint":"visor","pane":{"pane_id":"w1:p9","tab_id":"w1:t1"}},"type":"plugin_pane_opened"}}`

func wantCalls(t *testing.T, r result, want ...string) {
	t.Helper()
	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	if strings.Join(r.calls, "|") != strings.Join(want, "|") {
		t.Errorf("herdr calls = %q, want %q", r.calls, want)
	}
}

func TestToggleOpensVisorWhenNoneIsOpen(t *testing.T) {
	m := newToggleMachine(t, openedP9)

	r := m.press("w1:t1")

	wantCalls(t, r, "plugin pane open --plugin visor --entrypoint visor")
	if got := m.remembered(); got != "w1:p9" {
		t.Errorf("remembered pane %q, want w1:p9", got)
	}
}

func TestToggleClosesVisorOpenInThisTab(t *testing.T) {
	m := newToggleMachine(t, openedP9)
	m.remember("w1:p4")
	m.live("w1:p4", "w1:t1")

	r := m.press("w1:t1")

	wantCalls(t, r, "pane get w1:p4", "plugin pane close w1:p4")
	if got := m.remembered(); got != "" {
		t.Errorf("still remembers pane %q after closing it", got)
	}
}

func TestToggleBringsVisorFromAnotherTab(t *testing.T) {
	m := newToggleMachine(t, openedP9)
	m.remember("w2:p4")
	m.live("w2:p4", "w2:t3")

	r := m.press("w1:t1")

	wantCalls(t, r, "pane get w2:p4", "plugin pane close w2:p4", "plugin pane open --plugin visor --entrypoint visor")
	if got := m.remembered(); got != "w1:p9" {
		t.Errorf("remembered pane %q, want w1:p9", got)
	}
}

// Quitting visor with q closes its pane without the action knowing.
func TestToggleOpensVisorAfterItQuitByItself(t *testing.T) {
	m := newToggleMachine(t, openedP9)
	m.remember("w1:p4")

	r := m.press("w1:t1")

	wantCalls(t, r, "pane get w1:p4", "plugin pane open --plugin visor --entrypoint visor")
	if got := m.remembered(); got != "w1:p9" {
		t.Errorf("remembered pane %q, want w1:p9", got)
	}
}

func TestToggleFailsWhenHerdrOpensNoPane(t *testing.T) {
	m := newToggleMachine(t, `{"error":{"code":"ui_busy"}}`)

	r := m.press("w1:t1")

	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	if !strings.Contains(r.stderr, "ui_busy") {
		t.Errorf("stderr lacks herdr's reply: %q", r.stderr)
	}
	if got := m.remembered(); got != "" {
		t.Errorf("remembered pane %q, want none", got)
	}
}
