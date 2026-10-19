package herdr_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// machine is a fake PATH with stub executables. Stubs record their arguments
// and environment, so no test ever reaches a real brew.
type machine struct {
	t        *testing.T
	path     string
	fallback string
	log      string
}

func newMachine(t *testing.T) *machine {
	if runtime.GOOS == "windows" {
		t.Skip("the herdr plugin runs only on macos and linux")
	}
	dir := t.TempDir()
	m := &machine{
		t:        t,
		path:     filepath.Join(dir, "bin"),
		fallback: filepath.Join(dir, "elsewhere"),
		log:      filepath.Join(dir, "calls.log"),
	}
	for _, d := range []string{m.path, m.fallback} {
		if err := os.Mkdir(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return m
}

func (m *machine) stub(dir, name, body string) string {
	m.t.Helper()
	file := filepath.Join(dir, name)
	script := "#!/bin/sh\n" + body + "\n"
	if err := os.WriteFile(file, []byte(script), 0o755); err != nil {
		m.t.Fatal(err)
	}
	return file
}

// brew installs a stub brew in dir whose prefix is prefix and whose install
// exits with code.
func (m *machine) brew(dir, prefix string, code int) {
	m.stub(dir, "brew", `if [ "$1" = --prefix ]; then echo `+prefix+`; exit 0; fi
echo "brew $* HOMEBREW_NO_INSTALL_UPGRADE=$HOMEBREW_NO_INSTALL_UPGRADE" >> `+m.log+`
echo "==> Installing visor"
exit `+strconv.Itoa(code))
}

type result struct {
	code           int
	stdout, stderr string
	calls          []string
}

func (m *machine) run(fallbacks string) result {
	m.t.Helper()
	cmd := exec.Command("/bin/bash", "install-visor.sh")
	cmd.Env = []string{"PATH=" + m.path, "HOME=" + m.t.TempDir(), "VISOR_BREW_FALLBACKS=" + fallbacks}
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

func TestVisorOnPathIsLeftAlone(t *testing.T) {
	m := newMachine(t)
	m.stub(m.path, "visor", "exit 0")
	m.brew(m.path, t.TempDir(), 0)

	r := m.run("")

	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	if len(r.calls) != 0 {
		t.Errorf("brew was called: %q", r.calls)
	}
}

func TestMissingVisorIsInstalledQuietlyWithBrewWithoutUpgrading(t *testing.T) {
	m := newMachine(t)
	m.brew(m.path, t.TempDir(), 0)

	r := m.run("")

	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	want := []string{"brew install oronbz/tap/visor HOMEBREW_NO_INSTALL_UPGRADE=1"}
	if strings.Join(r.calls, "|") != strings.Join(want, "|") {
		t.Errorf("brew calls = %q, want %q", r.calls, want)
	}
	if strings.Contains(r.stdout+r.stderr, "==> Installing visor") {
		t.Errorf("brew's own output was shown after it succeeded: %q", r.stdout+r.stderr)
	}
	if !strings.Contains(r.stdout, "visor is installed") {
		t.Errorf("stdout lacks the success line: %q", r.stdout)
	}
}

func TestBrewOffPathIsFoundInItsStandardLocations(t *testing.T) {
	m := newMachine(t)
	m.brew(m.fallback, t.TempDir(), 0)

	r := m.run("/nonexistent/brew:" + filepath.Join(m.fallback, "brew"))

	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	if len(r.calls) != 1 || !strings.HasPrefix(r.calls[0], "brew install oronbz/tap/visor ") {
		t.Errorf("brew calls = %q, want one install", r.calls)
	}
}

// herdr's build environment may lack brew's bin on PATH even when brew
// already installed visor there.
func TestVisorInstalledByBrewButOffPathIsLeftAlone(t *testing.T) {
	m := newMachine(t)
	prefix := t.TempDir()
	if err := os.Mkdir(filepath.Join(prefix, "bin"), 0o755); err != nil {
		t.Fatal(err)
	}
	m.stub(filepath.Join(prefix, "bin"), "visor", "exit 0")
	m.brew(m.fallback, prefix, 0)

	r := m.run(filepath.Join(m.fallback, "brew"))

	if r.code != 0 {
		t.Fatalf("exit %d, stderr: %s", r.code, r.stderr)
	}
	if len(r.calls) != 0 {
		t.Errorf("brew was called: %q", r.calls)
	}
}

func TestWithoutBrewTheInstallOptionsArePrinted(t *testing.T) {
	m := newMachine(t)

	r := m.run("/nonexistent/brew")

	if r.code != 1 {
		t.Fatalf("exit %d, want 1", r.code)
	}
	for _, want := range []string{"https://brew.sh", "brew install oronbz/tap/visor", "https://github.com/oronbz/visor/releases", "herdr plugin install oronbz/visor/herdr"} {
		if !strings.Contains(r.stderr, want) {
			t.Errorf("stderr lacks %q:\n%s", want, r.stderr)
		}
	}
}

func TestAFailedBrewInstallFailsTheBuildAndShowsBrewsOutput(t *testing.T) {
	m := newMachine(t)
	m.brew(m.path, t.TempDir(), 3)

	r := m.run("")

	if r.code == 0 {
		t.Fatal("exit 0 after brew failed")
	}
	if !strings.Contains(r.stderr, "==> Installing visor") {
		t.Errorf("brew's output wasn't shown after it failed: %q", r.stderr)
	}
}
