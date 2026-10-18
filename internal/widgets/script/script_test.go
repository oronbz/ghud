package script_test

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/hosttest"
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
