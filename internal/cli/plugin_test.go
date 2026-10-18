package cli_test

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"

	"github.com/oronbz/ghud/internal/cli"
	"github.com/oronbz/ghud/internal/plugin/plugintest"
)

// installs is a home for `ghud plugin` commands, with GitHub faked.
type installs struct {
	t    *testing.T
	home string
	gh   *plugintest.GitHub
	env  map[string]string
}

func newInstalls(t *testing.T) *installs {
	return &installs{t: t, home: t.TempDir(), gh: plugintest.NewGitHub(t), env: map[string]string{}}
}

// ghud runs ghud with args and stdin, returning the exit code and output.
func (in *installs) ghud(stdin string, args ...string) (int, string, string) {
	in.t.Helper()
	getenv := func(k string) string {
		if k == "HOME" {
			return in.home
		}
		return in.env[k]
	}
	pp, _ := hermetic(in.t, getenv)
	pp.Endpoints.GitHubREST = in.gh.URL
	var stdout, stderr bytes.Buffer
	code := run(in.t, cli.System{Args: args, Getenv: getenv, Ports: pp, Stdin: strings.NewReader(stdin), Stdout: &stdout, Stderr: &stderr})
	return code, stdout.String(), stderr.String()
}

func (in *installs) plugins() string {
	return filepath.Join(in.home, ".local", "share", "ghud", "plugins")
}

// installed lists the entries of the plugins directory.
func (in *installs) installed() []string {
	entries, _ := os.ReadDir(in.plugins())
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	return names
}

// greetPlugin is a Plugin whose build copies the fixture into its
// directory and whose command runs that copy.
func greetPlugin(t *testing.T, dir string, extra map[string]string) map[string]string {
	t.Helper()
	files := map[string]string{
		dir + "ghud-plugin.toml": plugintest.Manifest(t, plugintest.Options{Name: "greet", Command: []string{"./fixture"}, Build: [][]string{{"sh", "build.sh"}}}),
		dir + "build.sh":         fmt.Sprintf("cp %q fixture\n", plugintest.Binary(t)),
	}
	for k, v := range extra {
		files[dir+k] = v
	}
	return files
}

func contains(t *testing.T, what, got string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(got, w) {
			t.Errorf("%s has no %q:\n%s", what, w, got)
		}
	}
}

func TestPluginInstallFetchesBuildsAndMakesTheWidgetTypeAvailable(t *testing.T) {
	in := newInstalls(t)
	sha := in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))

	code, stdout, stderr := in.ghud("", "plugin", "install", "octo/greet", "-y")
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	contains(t, "stdout", stdout, "greet", "sh build.sh", "Installed greet from octo/greet at "+sha[:7])
	if _, err := os.Stat(filepath.Join(in.plugins(), "greet", "fixture")); err != nil {
		t.Errorf("the build didn't run in the Plugin's directory: %v", err)
	}
	_, ref, _ := in.ghud("", "config", "default")
	contains(t, "config default", ref, `type = "greet"`)
}

func TestPluginInstallFromASubdirectoryAtARefAsksForTrust(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/tools", "main", map[string]string{"README.md": "not a Plugin at the root"})
	files := greetPlugin(t, "plugins/greet/", nil)
	files["README.md"] = "a Plugin in a subdirectory"
	sha := in.gh.Push("octo/tools", "v1", files)

	code, stdout, stderr := in.ghud("y\n", "plugin", "install", "octo/tools/plugins/greet", "--ref", "v1")
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	contains(t, "stdout", stdout,
		"Plugin greet from octo/tools/plugins/greet at "+sha[:7]+" (v1)",
		"greet  Fixture",
		"sh build.sh",
		"Trust greet and build it? [y/N]",
		"Installed greet")
	if got := in.installed(); strings.Join(got, " ") != ".staging greet" {
		t.Errorf("plugins directory holds %v, want only greet (and the staging area)", got)
	}
	if _, err := os.Stat(filepath.Join(in.plugins(), "greet", "fixture")); err != nil {
		t.Errorf("not built: %v", err)
	}
	if _, err := os.Stat(filepath.Join(in.plugins(), "greet", "README.md")); err == nil {
		t.Error("files outside the subdirectory were installed")
	}
}

func TestDecliningTrustInstallsNothing(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", map[string]string{"build.sh": "touch built\n"}))

	for _, answer := range []string{"n\n", "\n", ""} {
		code, stdout, stderr := in.ghud(answer, "plugin", "install", "octo/greet")
		if code != 1 {
			t.Errorf("answer %q: exit %d, want 1", answer, code)
		}
		contains(t, "stdout", stdout, "[y/N]")
		contains(t, "stderr", stderr, "not trusted")
		if _, err := os.Stat(filepath.Join(in.plugins(), "greet")); err == nil {
			t.Errorf("answer %q: greet was installed", answer)
		}
	}
}

func TestAFailingBuildInstallsNothing(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", map[string]string{"build.sh": "touch half-built\necho compiler says no >&2\nexit 3\n"}))

	code, _, stderr := in.ghud("", "plugin", "install", "-y", "octo/greet")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	contains(t, "stderr", stderr, "compiler says no", "build step `sh build.sh` failed: exit status 3")
	if got := in.installed(); strings.Join(got, " ") != ".staging" {
		t.Errorf("plugins directory holds %v after a failed build", got)
	}
	if entries, _ := os.ReadDir(filepath.Join(in.plugins(), ".staging")); len(entries) != 0 {
		t.Errorf("the failed build was left in the staging area: %v", entries)
	}
}

func TestBuildStepsRunWithoutAShellOrStdin(t *testing.T) {
	in := newInstalls(t)
	files := greetPlugin(t, "", map[string]string{"build.sh": fmt.Sprintf("cp %q fixture\nprintf %%s \"$1\" > arg\ncat > stdin\n", plugintest.Binary(t))})
	files["ghud-plugin.toml"] = plugintest.Manifest(t, plugintest.Options{Name: "greet", Command: []string{"./fixture"}, Build: [][]string{{"sh", "build.sh", "$HOME; echo hi"}}})
	in.gh.Push("octo/greet", "main", files)

	if code, _, stderr := in.ghud("y\nmore input\n", "plugin", "install", "octo/greet"); code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	dir := filepath.Join(in.plugins(), "greet")
	if arg, _ := os.ReadFile(filepath.Join(dir, "arg")); string(arg) != "$HOME; echo hi" {
		t.Errorf("the build got argument %q, want it untouched by a shell", arg)
	}
	if stdin, err := os.ReadFile(filepath.Join(dir, "stdin")); err != nil || len(stdin) != 0 {
		t.Errorf("the build read %q (%v) from stdin, want nothing", stdin, err)
	}
}

func TestInstallingAnInstalledPluginPointsToUpdate(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))
	in.ghud("", "plugin", "install", "-y", "octo/greet")

	code, _, stderr := in.ghud("", "plugin", "install", "-y", "octo/greet")
	if code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	contains(t, "stderr", stderr, "already installed", "ghud plugin update greet")
}

// counting is greetPlugin with a build that appends a line to builds.log
// beside the plugins directory, so tests can count builds across updates.
func counting(t *testing.T, in *installs, version string) map[string]string {
	t.Helper()
	log := filepath.Join(in.home, "builds.log")
	return greetPlugin(t, "", map[string]string{
		"build.sh": fmt.Sprintf("cp %q fixture\necho built >> %q\n", plugintest.Binary(t), log),
		"VERSION":  version,
	})
}

func (in *installs) builds() int {
	b, _ := os.ReadFile(filepath.Join(in.home, "builds.log"))
	return strings.Count(string(b), "built")
}

func TestPluginUpdateRebuildsOnlyAfterANewCommit(t *testing.T) {
	in := newInstalls(t)
	first := in.gh.Push("octo/greet", "main", counting(t, in, "1"))
	in.ghud("", "plugin", "install", "-y", "octo/greet")

	code, stdout, stderr := in.ghud("", "plugin", "update", "greet")
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	contains(t, "stdout", stdout, "greet is up to date at "+first[:7])
	if n := in.builds(); n != 1 {
		t.Errorf("%d builds after an update with no new commit, want 1", n)
	}

	second := in.gh.Push("octo/greet", "main", counting(t, in, "2"))
	code, stdout, stderr = in.ghud("y\n", "plugin", "update")
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	contains(t, "stdout", stdout, "Trust greet and build it?", "Updated greet from "+first[:7]+" to "+second[:7])
	if n := in.builds(); n != 2 {
		t.Errorf("%d builds after an update to a new commit, want 2", n)
	}
	if v, _ := os.ReadFile(filepath.Join(in.plugins(), "greet", "VERSION")); string(v) != "2" {
		t.Errorf("VERSION is %q after the update, want 2", v)
	}
	if entries, _ := os.ReadDir(filepath.Join(in.plugins(), ".staging")); len(entries) != 0 {
		t.Errorf("the old version was left in the staging area: %v", entries)
	}
}

func TestAFailedUpdateKeepsThePluginAsItWas(t *testing.T) {
	in := newInstalls(t)
	first := in.gh.Push("octo/greet", "main", counting(t, in, "1"))
	in.ghud("", "plugin", "install", "-y", "octo/greet")
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", map[string]string{"build.sh": "exit 1\n", "VERSION": "2"}))

	if code, _, _ := in.ghud("", "plugin", "update", "-y", "greet"); code != 1 {
		t.Errorf("exit %d, want 1", code)
	}
	if code, _, _ := in.ghud("n\n", "plugin", "update", "greet"); code != 1 {
		t.Errorf("declined update: exit %d, want 1", code)
	}
	dir := filepath.Join(in.plugins(), "greet")
	if v, _ := os.ReadFile(filepath.Join(dir, "VERSION")); string(v) != "1" {
		t.Errorf("VERSION is %q after failed updates, want 1", v)
	}
	if _, stdout, _ := in.ghud("", "plugin", "list"); !strings.Contains(stdout, first[:7]) {
		t.Errorf("list doesn't show the commit still installed:\n%s", stdout)
	}
}

func TestPluginListShowsSourceCommitAndWidgetTypes(t *testing.T) {
	in := newInstalls(t)
	sha := in.gh.Push("octo/greet", "v2", greetPlugin(t, "", nil))
	in.ghud("", "plugin", "install", "-y", "octo/greet", "--ref", "v2")
	plugintest.Write(t, filepath.Join(in.plugins(), "byhand"), plugintest.Options{Name: "byhand"})
	dev := plugintest.Write(t, filepath.Join(in.home, "dev", "wip"), plugintest.Options{Name: "wip"})
	writeFile(t, filepath.Join(in.home, ".config", "ghud", "config.toml"), "[[plugin]]\npath = \"~/dev/wip\"\n")

	code, stdout, stderr := in.ghud("", "plugin", "list")
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	lines := strings.Split(strings.TrimSpace(stdout), "\n")
	want := [][]string{
		{"PLUGIN", "SOURCE", "COMMIT", "WIDGET", "TYPES"},
		{"byhand", "-", "-", "byhand"},
		{"greet", "octo/greet@v2", sha[:7], "greet"},
		{"wip", dev, "-", "wip"},
	}
	if len(lines) != len(want) {
		t.Fatalf("list:\n%s\nwant %d lines", stdout, len(want))
	}
	for i, w := range want {
		if got := strings.Fields(lines[i]); strings.Join(got, " ") != strings.Join(w, " ") {
			t.Errorf("line %d is %q, want %q", i, got, w)
		}
	}
}

func TestPluginUninstallDeletesTheFolder(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))
	in.ghud("", "plugin", "install", "-y", "octo/greet")

	code, stdout, stderr := in.ghud("", "plugin", "uninstall", "greet")
	if code != 0 {
		t.Fatalf("exit %d, stderr:\n%s", code, stderr)
	}
	contains(t, "stdout", stdout, "Uninstalled greet")
	if got := in.installed(); strings.Join(got, " ") != ".staging" {
		t.Errorf("plugins directory holds %v after uninstall", got)
	}
	code, _, stderr = in.ghud("", "plugin", "uninstall", "greet")
	if code != 1 {
		t.Errorf("uninstalling again: exit %d, want 1", code)
	}
	contains(t, "stderr", stderr, "Plugin `greet` isn't installed")
	if _, _, stderr := in.ghud("", "plugin", "uninstall", "../../.."); !strings.Contains(stderr, "isn't a Plugin name") {
		t.Errorf("a path as the name: %s", stderr)
	}
}

func TestPluginCommandsRefuseToRunWhileAnotherHoldsThePluginsLock(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))
	in.ghud("", "plugin", "install", "-y", "octo/greet")

	// Another `ghud plugin` command, in another process, holds the lock.
	locks := filepath.Join(in.home, ".local", "share", "ghud", "locks")
	if err := os.MkdirAll(locks, 0o755); err != nil {
		t.Fatal(err)
	}
	other := flock.New(filepath.Join(locks, "plugins.lock"))
	if ok, err := other.TryLock(); !ok || err != nil {
		t.Fatalf("couldn't take the lock: %v", err)
	}
	for _, args := range [][]string{
		{"plugin", "install", "-y", "octo/greet"},
		{"plugin", "update", "-y"},
		{"plugin", "uninstall", "greet"},
	} {
		code, _, stderr := in.ghud("", args...)
		if code != 1 {
			t.Errorf("%v: exit %d, want 1", args, code)
		}
		contains(t, strings.Join(args, " ")+" stderr", stderr, "another `ghud plugin` command is running")
	}
	if got := in.installed(); !strings.Contains(strings.Join(got, " "), "greet") {
		t.Errorf("plugins directory holds %v, want greet untouched", got)
	}
	if code, stdout, _ := in.ghud("", "plugin", "list"); code != 0 || !strings.Contains(stdout, "greet") {
		t.Errorf("plugin list: exit %d, stdout %q; listing needs no lock", code, stdout)
	}

	if err := other.Unlock(); err != nil {
		t.Fatal(err)
	}
	if code, _, stderr := in.ghud("", "plugin", "uninstall", "greet"); code != 0 {
		t.Errorf("once the lock is free: exit %d, stderr:\n%s", code, stderr)
	}
}

func TestInstallSendsTheGitHubTokenWhenThereIsOne(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))
	in.ghud("", "plugin", "install", "-y", "octo/greet")
	for _, a := range in.gh.Auth() {
		if a != "" {
			t.Errorf("sent Authorization %q with no token around", a)
		}
	}

	in = newInstalls(t)
	in.env["GH_TOKEN"] = "ghp_secret"
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))
	in.ghud("", "plugin", "install", "-y", "octo/greet")
	if auth := in.gh.Auth(); len(auth) == 0 || auth[0] != "Bearer ghp_secret" {
		t.Errorf("Authorization headers %q, want the GH_TOKEN", auth)
	}
}

func TestInstallingFromAMissingRepositoryOrRefSaysSo(t *testing.T) {
	in := newInstalls(t)
	in.gh.Push("octo/greet", "main", greetPlugin(t, "", nil))

	_, _, stderr := in.ghud("", "plugin", "install", "-y", "octo/nope")
	contains(t, "stderr", stderr, "GitHub has no repository octo/nope")
	_, _, stderr = in.ghud("", "plugin", "install", "-y", "octo/greet", "--ref", "v9")
	contains(t, "stderr", stderr, "octo/greet has no branch, tag or commit `v9`")
	_, _, stderr = in.ghud("", "plugin", "install", "-y", "octo/greet/nested")
	contains(t, "stderr", stderr, "octo/greet has no directory `nested`")
}
