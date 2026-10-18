// Package cli is ghud's single entry point: the TUI and every subcommand run
// through Run, so tests drive them with a temp home and captured output.
package cli

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/closekey"
	"github.com/oronbz/ghud/internal/config"
	"github.com/oronbz/ghud/internal/host"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/version"
)

// System is everything Run takes from the process.
type System struct {
	Args   []string
	Getenv func(string) string
	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
	// Ports defaults to ports.System(Getenv).
	Ports *ports.Ports
	// ProgramOptions are extra Bubble Tea options, such as a fixed window size in tests.
	ProgramOptions []tea.ProgramOption
}

const usage = `Usage: ghud [--config <path>] [--close-key <keys>]... [--version]
       ghud config default   print the full commented config reference
       ghud config init      write the reference to the config path, once
       ghud auth jira        set up Jira: site, email and an API token,
                             which goes only to the keyring
       ghud plugin schema    print the JSON Schema of Plugin manifests and
                             the protocol
       ghud plugin check [--timeout 10s] <dir>
                             test the Plugin in <dir> against the protocol
       ghud skills install   install the ghud-configure and ghud-make-plugin
                             agent skills to ~/.agents/skills, linked into
                             Claude Code's skills when it's set up
       ghud skills uninstall remove the skills ghud installed

ghud draws a full-screen dashboard of your day. Press q to quit.
The config is $XDG_CONFIG_HOME/ghud/config.toml (default ~/.config/ghud),
or GHUD_CONFIG, or --config.

--close-key quits ghud on a key in herdr's syntax, a chord or a sequence
of two: --close-key cmd+shift+g --close-key 'ctrl+b d'.
`

// Run runs ghud and returns its exit code.
func Run(s System) int {
	fs := flag.NewFlagSet("ghud", flag.ContinueOnError)
	fs.SetOutput(s.Stderr)
	fs.Usage = func() { fmt.Fprint(s.Stderr, usage) }
	showVersion := fs.Bool("version", false, "print the version and exit")
	configFlag := fs.String("config", "", "path to config.toml")
	var closeKeyFlags repeated
	fs.Var(&closeKeyFlags, "close-key", "a key that quits ghud, in herdr's key syntax")
	if err := fs.Parse(s.Args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	var closeKeys []closekey.Seq
	for _, k := range closeKeyFlags {
		seq, err := closekey.Parse(k)
		if err != nil {
			fmt.Fprintf(s.Stderr, "ghud: --close-key %q: %v\n", k, err)
			return 2
		}
		closeKeys = append(closeKeys, seq)
	}
	if *showVersion {
		fmt.Fprintln(s.Stdout, "ghud", version.Version)
		return 0
	}

	p, err := paths.Resolve(s.Getenv, runtime.GOOS)
	if err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	configFile := filepath.Join(p.Config, config.FileName)
	if v := s.Getenv("GHUD_CONFIG"); v != "" {
		configFile = v
	}
	if *configFlag != "" {
		configFile = *configFlag
	}
	locale := config.LocaleFromEnv(s.Getenv)
	pp := ports.System(s.Getenv)
	if s.Ports != nil {
		pp = *s.Ports
	}

	reference := func() string {
		types, problems := host.Types(p, configFile, locale, s.Getenv)
		for _, problem := range problems {
			fmt.Fprintln(s.Stderr, "ghud:", problem)
		}
		return config.Reference(types, locale)
	}
	if args := fs.Args(); len(args) > 0 && args[0] == "plugin" {
		return pluginCommand(s, pp, args[1:])
	}
	if args := fs.Args(); len(args) > 0 && args[0] == "skills" {
		return skillsCommand(s, args[1:])
	}
	switch strings.Join(fs.Args(), " ") {
	case "":
	case "config default":
		fmt.Fprint(s.Stdout, reference())
		return 0
	case "config init":
		return configInit(s, configFile, reference())
	case "auth jira":
		types, _ := host.Types(p, configFile, locale, s.Getenv)
		return authJira(s, pp, config.Editor{Path: configFile, LockDir: filepath.Join(p.Data, "locks"), Types: types, Locale: locale})
	default:
		fmt.Fprintf(s.Stderr, "ghud: unknown command %q\n\n%s", strings.Join(fs.Args(), " "), usage)
		return 2
	}
	// Popups have no HERDR_PANE_ID, and right-click can't reach them anyway.
	if s.Getenv("HERDR_PANE_ID") != "" {
		herdrRightClick(s.Getenv, pp.Commands, "pane")
		// Deferred, so a crash or a signal that ends the program restores herdr's menu too.
		defer herdrRightClick(s.Getenv, pp.Commands, "herdr")
	}
	m := host.New(host.Options{Paths: p, ConfigFile: configFile, Locale: locale, Ports: pp, CloseKeys: closeKeys})
	var opts []tea.ProgramOption
	// Real files get Bubble Tea's defaults, which open /dev/tty when stdin is piped.
	if _, isFile := s.Stdin.(*os.File); !isFile {
		opts = append(opts, tea.WithInput(s.Stdin))
	}
	if _, isFile := s.Stdout.(*os.File); !isFile {
		opts = append(opts, tea.WithOutput(s.Stdout))
	}
	opts = append(opts, s.ProgramOptions...)
	if c, ok := pp.Watcher.(io.Closer); ok {
		defer c.Close()
	}
	// Every copy of the model shares its Plugins, so this stops them however the program ends.
	defer m.Close()
	if _, err := tea.NewProgram(m, opts...).Run(); err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	return 0
}

// repeated collects every value of a repeatable flag.
type repeated []string

func (r *repeated) String() string { return strings.Join(*r, " ") }

func (r *repeated) Set(v string) error {
	*r = append(*r, v)
	return nil
}

// herdrRightClick routes right-clicks in ghud's herdr pane to target: "pane"
// for ghud, "herdr" for herdr's own menu. Failing leaves herdr as it was, so
// errors are ignored.
func herdrRightClick(getenv func(string) string, c ports.Commands, target string) {
	bin := getenv("HERDR_BIN_PATH")
	if bin == "" {
		bin = "herdr"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = c.Output(ctx, bin, "pane", "input", "--current", "--right-click", target)
}

// configInit writes the reference to path, refusing to touch an existing file.
func configInit(s System, path, ref string) int {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if errors.Is(err, fs.ErrExist) {
		fmt.Fprintf(s.Stderr, "ghud: %s already exists; see `ghud config default` for every option\n", path)
		return 1
	}
	if err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	_, werr := f.WriteString(ref)
	if err := errors.Join(werr, f.Close()); err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	fmt.Fprintln(s.Stdout, "wrote", path)
	return 0
}
