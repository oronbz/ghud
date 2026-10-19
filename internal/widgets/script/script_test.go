package script_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/visor/internal/hosttest"
)

// fixture is a script Widget whose command logs each run to a file in a temp
// dir before running cmd; runs counts them.
type fixture struct {
	*hosttest.Host
	dir string
}

func withScript(t *testing.T, block, cmd string) fixture {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cmd.sh"), []byte(cmd), 0o644); err != nil {
		t.Fatal(err)
	}
	config := fmt.Sprintf("[[widget]]\ntype = \"script\"\ntitle = \"Outdated\"\ncommand = 'cd %s && echo run >> runs && . ./cmd.sh'\n%s", dir, block)
	return fixture{hosttest.New(t, hosttest.Options{Width: 60, Height: 10, Config: config}), dir}
}

func (f fixture) runs() int {
	b, _ := os.ReadFile(filepath.Join(f.dir, "runs"))
	return strings.Count(string(b), "run")
}

// setCmd replaces what the command does from its next run on.
func (f fixture) setCmd(t *testing.T, cmd string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, "cmd.sh"), []byte(cmd), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFailingCommandShowsItsStderrAsAnErrorWithNoDataAndAsTheStaleReasonWithData(t *testing.T) {
	f := withScript(t, `interval = "10m"`, `echo "brew: command not found" >&2; exit 127`)
	if _, _, ok := f.Find("exit 127: brew: command not found"); !ok {
		t.Errorf("failing command with no data doesn't show the error:\n%s", f.Screen())
	}

	f.setCmd(t, outdated)
	f.Advance(17 * time.Second)
	if _, _, ok := f.Find("git"); !ok {
		t.Fatalf("retry 15s (±10%%) after the failure didn't show the Items:\n%s", f.Screen())
	}

	f.setCmd(t, `echo "Error: no network" >&2; exit 1`)
	f.Advance(12 * time.Minute)
	top := f.Line(0)
	if !strings.Contains(top, "Error: no network · 1") {
		t.Errorf("top border %q, want stderr as the stale reason with the age", top)
	}
	if _, _, ok := f.Find("git"); !ok {
		t.Errorf("stale Widget hides its Items:\n%s", f.Screen())
	}
}

func TestPlainTextOutputShowsAsLinesAndBadJSONIsAnError(t *testing.T) {
	f := withScript(t, "", `printf 'disk 81%%\nswap 2%%\n'`)
	if _, _, ok := f.Find("disk 81%"); !ok {
		t.Errorf("screen lacks the first line:\n%s", f.Screen())
	}
	if _, _, ok := f.Find("swap 2%"); !ok {
		t.Errorf("screen lacks the second line:\n%s", f.Screen())
	}

	g := withScript(t, "", `printf '[{"name": "git",'`)
	if _, _, ok := g.Find("bad JSON"); !ok {
		t.Errorf("bad JSON isn't reported:\n%s", g.Screen())
	}
}

func TestStyledLinesUseThemeTokens(t *testing.T) {
	f := withScript(t, "", `printf '{"lines": ["plain", {"spans": [{"text": "warm", "token": "warn"}]}]}'`)
	x, y, ok := f.Find("warm")
	if !ok {
		t.Fatalf("screen lacks the styled line:\n%s", f.Screen())
	}
	if tok, _ := f.Fg(x, y); tok.String() != "warn" {
		t.Errorf("styled span drawn in %v, want warn", tok)
	}
	if _, _, ok := f.Find("plain"); !ok {
		t.Errorf("screen lacks the plain line:\n%s", f.Screen())
	}

	g := withScript(t, "", `printf '{"lines": [{"spans": [{"text": "x", "token": "#ff0000"}]}]}'`)
	if _, _, ok := g.Find("is not a theme"); !ok {
		t.Errorf("a raw colour isn't rejected:\n%s", g.Screen())
	}
}

func TestScriptWithoutACommandNeedsSetup(t *testing.T) {
	g := hosttest.New(t, hosttest.Options{Width: 60, Height: 10, Config: "[[widget]]\ntype = \"script\"\n"})
	if _, _, ok := g.Find("needs setup"); !ok {
		t.Errorf("script without a command isn't in needs setup:\n%s", g.Screen())
	}
}

func TestTemplatedActionRunsWithTheSelectedItemsFieldsQuotedThenRefreshes(t *testing.T) {
	dir := t.TempDir()
	block := fmt.Sprintf(`actions = [
  { key = "u", label = "upgrade", run = "cd %s && printf '%%s %%s\n' {{name}} {{current}} >> upgraded" },
  { label = "upgrade all", run = "cd %s && echo all >> upgraded", confirm = true },
]`, dir, dir)
	f := withScript(t, block, `printf '[{"name":"git","current":"2.47"},{"name":"x; touch pwned","current":"1"}]'`)

	f.Press("tab", "j")
	if bottom := f.Line(8); !strings.Contains(bottom, "u upgrade") {
		t.Errorf("bottom border %q, want the u upgrade hint", bottom)
	}
	f.Press("u")
	f.Press("k", "u")
	got, _ := os.ReadFile(filepath.Join(dir, "upgraded"))
	if want := "x; touch pwned 1\ngit 2.47\n"; string(got) != want {
		t.Errorf("the Action wrote %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "pwned")); err == nil {
		t.Error("an Item field ran as shell code")
	}
	if f.runs() != 3 {
		t.Errorf("command ran %d times, want 3 (start, then a refresh after each Action)", f.runs())
	}

	f.Press("ctrl+k", "a", "l", "l", "enter")
	if _, _, ok := f.Find("Upgrade all?"); !ok {
		t.Fatalf("a confirm Action didn't ask:\n%s", f.Screen())
	}
	f.Press("y")
	got, _ = os.ReadFile(filepath.Join(dir, "upgraded"))
	if !strings.HasSuffix(string(got), "all\n") {
		t.Errorf("the Widget Action wrote %q, want it to end in all", got)
	}
}

func TestFailingActionToastsItsStderr(t *testing.T) {
	f := withScript(t, `actions = [{ key = "u", label = "upgrade", run = "echo 'no such formula: {{name}}' >&2; exit 1" }]`, outdated)
	f.Press("tab", "u")
	if footer := f.Line(9); !strings.Contains(footer, "Upgrade failed: exit 1: no such formula: git") {
		t.Errorf("footer %q, want the failure with stderr", footer)
	}
}

func TestBadActionTemplateNeedsSetup(t *testing.T) {
	f := withScript(t, `actions = [{ key = "u", label = "upgrade" }]`, outdated)
	if _, _, ok := f.Find("needs setup"); !ok {
		t.Errorf("an Action without run isn't needs setup:\n%s", f.Screen())
	}
}

func TestAddWidgetPlacesAScriptThatAsksForItsCommand(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{Width: 80, Height: 20, Config: "[[widget]]\ntype = \"clock\"\n"})
	h.Press("ctrl+k", "a", "d", "d", " ", "s", "c", "r", "i", "p", "t", "enter")

	if _, _, ok := h.Find("Set `command`"); !ok {
		t.Errorf("an added script Widget doesn't ask for its command:\n%s", h.Screen())
	}
}

const outdated = `printf '[{"name":"git","installed":"2.46","current":"2.47"},{"name":"go","installed":"1.27.0","current":"1.27.1"}]'`

func TestScriptRunsItsCommandOnItsIntervalAndShowsTheItemsItPrints(t *testing.T) {
	f := withScript(t, `interval = "10m"`, outdated)

	if f.runs() != 1 {
		t.Fatalf("command ran %d times at start, want 1", f.runs())
	}
	for _, want := range []string{"git", "2.46", "2.47", "go", "1.27.1", "Outdated 2"} {
		if _, _, ok := f.Find(want); !ok {
			t.Errorf("screen lacks %q:\n%s", want, f.Screen())
		}
	}

	f.Advance(9 * time.Minute)
	if f.runs() != 1 {
		t.Errorf("command ran %d times 9m in, want 1 (10m interval, ±10%%)", f.runs())
	}
	f.Advance(2*time.Minute + time.Second)
	if f.runs() != 2 {
		t.Errorf("command ran %d times 11m in, want 2", f.runs())
	}
	f.Advance(24 * time.Minute)
	if f.runs() != 4 {
		t.Errorf("command ran %d times 35m in, want 4", f.runs())
	}
}
