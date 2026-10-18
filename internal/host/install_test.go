package host_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/plugin/install"
	"github.com/oronbz/ghud/internal/plugin/plugintest"
	"github.com/oronbz/ghud/internal/widget"
)

// installs is ghud running from root while `ghud plugin` commands change
// its plugins directory, with GitHub faked.
type installs struct {
	t   *testing.T
	gh  *plugintest.GitHub
	ins install.Installer
	h   *hosttest.Host
}

func newInstalls(t *testing.T) *installs {
	t.Helper()
	root := t.TempDir()
	gh := plugintest.NewGitHub(t)
	gh.Push("octo/greet", "main", greetRepo(t, "1"))
	return &installs{t: t, gh: gh, ins: install.Installer{
		Dir:     filepath.Join(paths.Under(root).Data, "plugins"),
		Fetcher: install.GitHub{API: gh.URL},
	}, h: hosttest.New(t, hosttest.Options{Root: root, Width: 100, Height: 12, Types: []widget.Type{}, Config: "[[widget]]\ntype = \"greet\"\n"})}
}

func greetRepo(t *testing.T, version string) map[string]string {
	return map[string]string{
		"ghud-plugin.toml": plugintest.Manifest(t, plugintest.Options{Name: "greet"}),
		"VERSION":          version,
	}
}

func trusted(install.Plan) bool { return true }

// changed is what the running instance's watcher reports after a command.
func (in *installs) changed() { in.h.Changed(in.ins.Dir) }

func (in *installs) install() {
	in.t.Helper()
	if _, err := in.ins.Install(context.Background(), install.Source{Owner: "octo", Repo: "greet"}, "", trusted); err != nil {
		in.t.Fatal(err)
	}
	in.changed()
}

func TestARunningInstancePicksUpAnInstalledPlugin(t *testing.T) {
	in := newInstalls(t)
	shows(t, in.h, "needs setup", "unknown Widget type `greet`")

	in.install()
	shows(t, in.h, "hello alpha", "refresh 1")
	hides(t, in.h, "needs setup")
}

func TestUninstallingPutsTheWidgetsInNeedsSetupNamingTheInstallCommand(t *testing.T) {
	in := newInstalls(t)
	in.install()

	if _, err := in.ins.Uninstall("greet"); err != nil {
		t.Fatal(err)
	}
	in.changed()
	shows(t, in.h, "needs setup", "Plugin `greet` isn't installed", "ghud plugin install octo/greet")
	hides(t, in.h, "hello alpha", "unknown Widget type")
}

func TestARunningInstanceRestartsThePluginAfterAnUpdate(t *testing.T) {
	in := newInstalls(t)
	in.install()
	in.h.Press("ctrl+r")
	shows(t, in.h, "refresh 2")

	in.gh.Push("octo/greet", "main", greetRepo(t, "2"))
	if _, _, err := in.ins.Update(context.Background(), "greet", trusted); err != nil {
		t.Fatal(err)
	}
	in.changed()
	shows(t, in.h, "hello alpha", "refresh 1")
}
