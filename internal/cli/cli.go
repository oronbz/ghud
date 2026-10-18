// Package cli is ghud's single entry point: the TUI and every subcommand run
// through Run, so tests drive them with a temp home and captured output.
package cli

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"runtime"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/host"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/theme"
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

const usage = `Usage: ghud

ghud draws a full-screen dashboard of your day. Press q to quit.
`

// Run runs ghud and returns its exit code.
func Run(s System) int {
	fs := flag.NewFlagSet("ghud", flag.ContinueOnError)
	fs.SetOutput(s.Stderr)
	fs.Usage = func() { fmt.Fprint(s.Stderr, usage) }
	if err := fs.Parse(s.Args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(s.Stderr, "ghud: unknown command %q\n\n%s", fs.Arg(0), usage)
		return 2
	}

	p, err := paths.Resolve(s.Getenv, runtime.GOOS)
	if err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	pp := ports.System(s.Getenv)
	if s.Ports != nil {
		pp = *s.Ports
	}
	m := host.New(host.Options{Paths: p, Ports: pp, Theme: theme.Dark()})
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
