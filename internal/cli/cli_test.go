package cli_test

import (
	"bytes"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/oronbz/ghud/internal/cli"
)

func system(t *testing.T, stdin string, args ...string) (cli.System, *bytes.Buffer, *bytes.Buffer) {
	home := t.TempDir()
	var stdout, stderr bytes.Buffer
	return cli.System{
		Args:   args,
		Getenv: func(k string) string { return map[string]string{"HOME": home}[k] },
		Stdin:  strings.NewReader(stdin),
		Stdout: &stdout,
		Stderr: &stderr,
		ProgramOptions: []tea.ProgramOption{
			tea.WithWindowSize(40, 9),
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
