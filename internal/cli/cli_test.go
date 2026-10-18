package cli_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/cli"
	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/ports"
)

// hermetic are the real ports with every way out of the machine faked:
// other CLIs (such as gh) aren't installed, the keyring is empty, the system
// opener only records, and every HTTP endpoint is a server that fails the
// test if anything reaches it.
func hermetic(t *testing.T, getenv func(string) string) (*ports.Ports, *hosttest.Commands) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("ghud reached the network: %s %s", r.Method, r.URL)
		http.Error(w, "offline", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	pp := ports.System(getenv)
	cmds := &hosttest.Commands{}
	pp.Commands = cmds
	pp.EventKit = &hosttest.EventKit{}
	pp.Keyring = &hosttest.Keyring{}
	pp.Opener = &hosttest.Opener{}
	e := reflect.ValueOf(&pp.Endpoints).Elem()
	for i := range e.NumField() {
		e.Field(i).SetString(srv.URL)
	}
	return &pp, cmds
}

func system(t *testing.T, stdin string, args ...string) (cli.System, *bytes.Buffer, *bytes.Buffer) {
	home, err := os.MkdirTemp("", "ghud-cli")
	if err != nil {
		t.Fatal(err)
	}
	// A fetch still in flight at exit may write its cache entry after the test ends.
	t.Cleanup(func() { _ = os.RemoveAll(home) })
	getenv := func(k string) string { return map[string]string{"HOME": home}[k] }
	pp, _ := hermetic(t, getenv)
	var stdout, stderr bytes.Buffer
	return cli.System{
		Args:   args,
		Getenv: getenv,
		Ports:  pp,
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		ProgramOptions: []tea.ProgramOption{
			tea.WithWindowSize(60, 16),
			tea.WithoutSignals(),
		},
	}, &stdout, &stderr
}

func run(t *testing.T, s cli.System) int {
	t.Helper()
	done := make(chan int, 1)
	go func() { done <- cli.Run(s) }()
	select {
	case code := <-done:
		return code
	case <-time.After(5 * time.Second):
		t.Fatal("ghud did not exit")
		return -1
	}
}

func TestGhudDrawsTheClockAndQuitsOnQ(t *testing.T) {
	s, stdout, stderr := system(t, "q")

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	out := stdout.String()
	if !strings.Contains(ansi.Strip(out), "Clock") {
		t.Errorf("no Clock tile drawn:\n%q", out)
	}
	enter, leave := ansi.SetModeAltScreenSaveCursor, ansi.ResetModeAltScreenSaveCursor
	if !strings.Contains(out, enter) || strings.LastIndex(out, leave) < strings.LastIndex(out, enter) {
		t.Errorf("alt screen not entered and then restored:\n%q", out)
	}
}

func TestGhudEnablesKittyKeyboardFlagsAndRestoresThemOnExit(t *testing.T) {
	// Shift press and release, then q, all kitty-encoded.
	s, stdout, stderr := system(t, "\x1b[57441;2u\x1b[57441;1:3u\x1b[113u")

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	out := stdout.String()
	flags := ansi.KittyDisambiguateEscapeCodes | ansi.KittyReportEventTypes | ansi.KittyReportAlternateKeys |
		ansi.KittyReportAllKeysAsEscapeCodes | ansi.KittyReportAssociatedKeys
	push, pop := ansi.PushKittyKeyboard(flags), ansi.PopKittyKeyboard(1)
	if !strings.Contains(out, push) || strings.LastIndex(out, pop) < strings.Index(out, push) {
		t.Errorf("kitty flags %d not pushed and then popped:\n%q", flags, out)
	}
}

func TestVersionFlagPrintsTheVersionWithoutStartingTheTUI(t *testing.T) {
	s, stdout, stderr := system(t, "", "--version")

	if code := run(t, s); code != 0 {
		t.Fatalf("exit code %d, stderr: %s", code, stderr)
	}
	if got, want := stdout.String(), "ghud dev\n"; got != want {
		t.Errorf("stdout %q, want %q", got, want)
	}
}

func TestUnknownArgumentIsAUsageError(t *testing.T) {
	s, _, stderr := system(t, "", "--nope")

	if code := run(t, s); code != 2 {
		t.Errorf("exit code %d, want 2", code)
	}
	if !strings.Contains(stderr.String(), "Usage: ghud") {
		t.Errorf("stderr has no usage:\n%s", stderr)
	}
}
