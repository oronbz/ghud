package cli_test

import (
	"context"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/cli"
	"github.com/oronbz/ghud/internal/hosttest"
)

// inHerdr is a ghud run with env set and herdr faked.
func inHerdr(t *testing.T, stdin string, env map[string]string, args ...string) (cli.System, *hosttest.Commands) {
	s, _, _ := system(t, stdin, args...)
	home := s.Getenv("HOME")
	s.Getenv = func(k string) string {
		if k == "HOME" {
			return home
		}
		return env[k]
	}
	var cmds *hosttest.Commands
	s.Ports, cmds = hermetic(t, s.Getenv)
	return s, cmds
}

// herdrCalls are the herdr command lines ghud ran, leaving out the ones the
// default Layout's Widgets run, such as `gh auth token`.
func herdrCalls(cmds *hosttest.Commands) [][]string {
	var calls [][]string
	for _, c := range cmds.Ran() {
		if filepath.Base(c[0]) == "herdr" {
			calls = append(calls, c)
		}
	}
	return calls
}

func TestCloseKeysQuitGhud(t *testing.T) {
	for _, tc := range []struct{ name, stdin string }{
		{"kitty cmd+shift+g", "\x1b[103;10u"},
		{"ctrl+b d", "\x02d"},
	} {
		s, _ := inHerdr(t, tc.stdin, nil, "--close-key", "cmd+shift+g", "--close-key", "ctrl+b d")
		if code := run(t, s); code != 0 {
			t.Errorf("%s: exit code %d", tc.name, code)
		}
	}
}

func TestInvalidCloseKeyFailsBeforeTheTUIStarts(t *testing.T) {
	s, stdout, stderr := system(t, "q", "--close-key", "cntrl+b d")

	if code := run(t, s); code != 2 {
		t.Errorf("exit code %d, want 2", code)
	}
	if got := stderr.String(); !strings.Contains(got, `--close-key "cntrl+b d"`) || !strings.Contains(got, `"cntrl" is not a modifier`) {
		t.Errorf("stderr doesn't explain the bad key:\n%s", got)
	}
	if stdout.Len() != 0 {
		t.Errorf("the TUI started:\n%q", stdout)
	}
}

func TestInAHerdrPaneRightClickIsOptedInAndRestored(t *testing.T) {
	s, cmds := inHerdr(t, "q", map[string]string{"HERDR_PANE_ID": "w1-3"})

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	want := [][]string{
		{"herdr", "pane", "input", "--current", "--right-click", "pane"},
		{"herdr", "pane", "input", "--current", "--right-click", "herdr"},
	}
	if got := herdrCalls(cmds); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("herdr calls %q, want %q", got, want)
	}
}

func TestHerdrsOwnBinaryIsPreferred(t *testing.T) {
	s, cmds := inHerdr(t, "q", map[string]string{"HERDR_PANE_ID": "w1-3", "HERDR_BIN_PATH": "/opt/herdr/bin/herdr"})

	run(t, s)
	calls := herdrCalls(cmds)
	for _, c := range calls {
		if c[0] != "/opt/herdr/bin/herdr" {
			t.Errorf("ran %q, not HERDR_BIN_PATH", c)
		}
	}
	if len(calls) != 2 {
		t.Errorf("herdr calls %q, want the opt-in and the restore", calls)
	}
}

func TestOutsideAHerdrPaneHerdrIsNeverRun(t *testing.T) {
	// A herdr popup has HERDR_ACTIVE_PANE_ID but no HERDR_PANE_ID.
	s, cmds := inHerdr(t, "q", map[string]string{"HERDR_ACTIVE_PANE_ID": "w1-3", "HERDR_BIN_PATH": "/opt/herdr/bin/herdr"})

	run(t, s)
	if calls := herdrCalls(cmds); len(calls) != 0 {
		t.Errorf("herdr calls %q, want none", calls)
	}
}

func TestRightClickIsRestoredWhenGhudIsKilled(t *testing.T) {
	s, cmds := inHerdr(t, "", map[string]string{"HERDR_PANE_ID": "w1-3"})
	// A cancelled context kills the program, as a signal does.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s.ProgramOptions = append(s.ProgramOptions, tea.WithContext(ctx))

	if code := run(t, s); code == 0 {
		t.Error("a killed ghud exited 0")
	}
	if calls := herdrCalls(cmds); len(calls) != 2 || calls[1][5] != "herdr" {
		t.Errorf("herdr calls %q, want the restore last", calls)
	}
}
