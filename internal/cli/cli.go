// Package cli is ghud's single entry point: the TUI and every subcommand run
// through Run, so tests drive them with a temp home and captured output.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	tea "charm.land/bubbletea/v2"

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

const usage = `Usage: ghud [--config <path>] [--version]
       ghud config default   print the full commented config reference
       ghud config init      write the reference to the config path, once

ghud draws a full-screen dashboard of your day. Press q to quit.
The config is $XDG_CONFIG_HOME/ghud/config.toml (default ~/.config/ghud),
or GHUD_CONFIG, or --config.
`

// Run runs ghud and returns its exit code.
func Run(s System) int {
	fs := flag.NewFlagSet("ghud", flag.ContinueOnError)
	fs.SetOutput(s.Stderr)
	fs.Usage = func() { fmt.Fprint(s.Stderr, usage) }
	showVersion := fs.Bool("version", false, "print the version and exit")
	configFlag := fs.String("config", "", "path to config.toml")
	if err := fs.Parse(s.Args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
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

	switch strings.Join(fs.Args(), " ") {
	case "":
	case "config default":
		fmt.Fprint(s.Stdout, config.Reference(host.Builtins, locale))
		return 0
	case "config init":
		return configInit(s, configFile, config.Reference(host.Builtins, locale))
	default:
		fmt.Fprintf(s.Stderr, "ghud: unknown command %q\n\n%s", strings.Join(fs.Args(), " "), usage)
		return 2
	}
	pp := ports.System(s.Getenv)
	if s.Ports != nil {
		pp = *s.Ports
	}
	m := host.New(host.Options{Paths: p, ConfigFile: configFile, Locale: locale, Ports: pp})
	var opts []tea.ProgramOption
	// Real files get Bubble Tea's defaults, which open /dev/tty when stdin is piped.
	if _, isFile := s.Stdin.(*os.File); !isFile {
		opts = append(opts, tea.WithInput(s.Stdin))
	}
	if _, isFile := s.Stdout.(*os.File); !isFile {
		opts = append(opts, tea.WithOutput(s.Stdout))
	}
	opts = append(opts, s.ProgramOptions...)
	if _, err := tea.NewProgram(m, opts...).Run(); err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	return 0
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
