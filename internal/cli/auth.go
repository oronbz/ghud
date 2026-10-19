package cli

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/oronbz/visor/internal/config"
	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/widget"
	"github.com/oronbz/visor/internal/widgets/jira"
)

// authJira runs the Set up Jira flow, the same one as the TUI's Action, on
// the terminal: for the config's first Jira Widget, or a new one.
func authJira(s System, pp ports.Ports, edits config.Editor) int {
	cfg, err := config.Load(edits.Path, edits.Types, edits.Locale)
	if err != nil {
		fmt.Fprintln(s.Stderr, "visor:", err)
		return 1
	}
	var ref config.Ref
	var site, email string
	seen := map[string]int{}
	for _, w := range cfg.Widgets {
		if w.Type == jira.Type.Name && w.Setup == "" {
			ref = config.Ref{ID: w.ID, N: seen[w.ID]}
			site, email = w.Settings.String("site"), w.Settings.String("email")
			break
		}
		seen[w.ID]++
	}

	in := bufio.NewReader(s.Stdin)
	flow := jira.Setup{Keyring: pp.Keyring}
	a := flow.Start(site, email)
	for {
		if a.Target != "" {
			fmt.Fprintln(s.Stdout, "Opening", a.Target)
			if err := pp.Opener.Open(a.Target); err != nil {
				fmt.Fprintln(s.Stdout, "Couldn't open it, so open it yourself:", err)
			}
		}
		text, err := ask(s, in, a)
		if err != nil {
			fmt.Fprintln(s.Stderr, "visor:", err)
			return 1
		}
		res, err := flow.Invoke(context.Background(), widget.Invocation{Action: a.ID, Text: text, Base: a.Value, Now: pp.Clock.Now()})
		if err != nil {
			fmt.Fprintln(s.Stderr, "visor:", err)
			return 1
		}
		if len(res.Settings) > 0 {
			if _, err := edits.SetSettings(jira.Type.Name, ref, res.Settings); err != nil {
				fmt.Fprintln(s.Stderr, "visor:", err)
				return 1
			}
		}
		if res.Toast != "" {
			fmt.Fprintln(s.Stdout, res.Toast)
		}
		if res.Then == nil {
			return 0
		}
		a = *res.Then
	}
}

// ask prompts for an Action's input on one line. Empty text keeps the
// Action's Value; a secret is read without echo from a terminal.
func ask(s System, in *bufio.Reader, a widget.Action) (string, error) {
	prompt := a.Prompt
	if a.Input == widget.TextInput && a.Value != "" {
		prompt += " [" + a.Value + "]"
	}
	fmt.Fprint(s.Stdout, prompt+": ")
	if a.Input == widget.SecretInput {
		defer fmt.Fprintln(s.Stdout)
		if f, ok := s.Stdin.(*os.File); ok && term.IsTerminal(f.Fd()) {
			b, err := term.ReadPassword(f.Fd())
			return string(b), err
		}
	}
	line, err := in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" && a.Input == widget.TextInput {
		return a.Value, nil
	}
	return line, nil
}
