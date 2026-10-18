package cli

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/oronbz/ghud/internal/config"
	"github.com/oronbz/ghud/internal/host"
	"github.com/oronbz/ghud/internal/paths"
	"github.com/oronbz/ghud/internal/plugin"
	"github.com/oronbz/ghud/internal/plugin/install"
	"github.com/oronbz/ghud/internal/ports"
)

// fetchTimeout bounds one download of a Plugin's repository.
const fetchTimeout = 5 * time.Minute

// pluginManage runs `ghud plugin install|update|uninstall|list <args>`.
func pluginManage(s System, pp ports.Ports, p paths.Paths, configFile string, locale config.Locale, args []string) int {
	c := pluginCLI{s: s, in: bufio.NewReader(s.Stdin), installer: install.Installer{
		Dir:     filepath.Join(p.Data, "plugins"),
		Fetcher: githubFetcher(s.Getenv, pp),
		Log:     s.Stderr,
	}}
	switch args[0] {
	case "install":
		return c.install(args[1:])
	case "update":
		return c.update(args[1:])
	case "uninstall":
		return c.uninstall(args[1:])
	}
	return c.list(host.PluginPaths(configFile, locale, s.Getenv))
}

func githubFetcher(getenv func(string) string, pp ports.Ports) install.GitHub {
	client := &http.Client{Timeout: fetchTimeout}
	if pp.HTTP != nil {
		c := *pp.HTTP
		c.Timeout = fetchTimeout
		client = &c
	}
	return install.GitHub{HTTP: client, API: pp.Endpoints.GitHubREST, Token: githubToken(getenv, pp.Commands)}
}

// githubToken is the token gh and GitHub's tools use, if any; Plugins
// from public repositories need none.
func githubToken(getenv func(string) string, c ports.Commands) string {
	for _, k := range []string{"GH_TOKEN", "GITHUB_TOKEN"} {
		if v := getenv(k); v != "" {
			return v
		}
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := c.Output(ctx, "gh", "auth", "token")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

type pluginCLI struct {
	s         System
	in        *bufio.Reader
	installer install.Installer
	yes       bool
}

// parse reads flags wherever they sit among the positional arguments.
func (c *pluginCLI) parse(fs *flag.FlagSet, args []string) ([]string, bool) {
	fs.SetOutput(c.s.Stderr)
	fs.BoolVar(&c.yes, "y", false, "trust the Plugin without asking")
	var rest []string
	for {
		if err := fs.Parse(args); err != nil {
			return nil, false
		}
		args = fs.Args()
		if len(args) == 0 {
			return rest, true
		}
		rest, args = append(rest, args[0]), args[1:]
	}
}

func (c *pluginCLI) fail(err error) int {
	fmt.Fprintln(c.s.Stderr, "ghud:", err)
	return 1
}

func (c *pluginCLI) install(args []string) int {
	fs := flag.NewFlagSet("ghud plugin install", flag.ContinueOnError)
	ref := fs.String("ref", "", "the branch, tag or commit to install")
	rest, ok := c.parse(fs, args)
	if !ok || len(rest) != 1 {
		fmt.Fprintln(c.s.Stderr, "Usage: ghud plugin install <owner/repo[/subdir]> [--ref <ref>] [-y]")
		return 2
	}
	src, err := install.ParseSource(rest[0])
	if err != nil {
		return c.fail(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
	defer cancel()
	plan, err := c.installer.Install(ctx, src, *ref, c.trust)
	if err != nil {
		return c.fail(err)
	}
	fmt.Fprintf(c.s.Stdout, "Installed %s from %s\n", plan.Manifest.Name, at(plan.Record))
	return 0
}

func (c *pluginCLI) update(args []string) int {
	rest, ok := c.parse(flag.NewFlagSet("ghud plugin update", flag.ContinueOnError), args)
	if !ok {
		fmt.Fprintln(c.s.Stderr, "Usage: ghud plugin update [name...] [-y]")
		return 2
	}
	if len(rest) == 0 {
		for _, p := range c.installer.List() {
			if p.Record.Source != "" {
				rest = append(rest, filepath.Base(p.Dir))
			}
		}
	}
	code := 0
	for _, name := range rest {
		ctx, cancel := context.WithTimeout(context.Background(), fetchTimeout)
		was, now, err := c.installer.Update(ctx, name, c.trust)
		cancel()
		switch {
		case err != nil:
			fmt.Fprintf(c.s.Stderr, "ghud: %s: %v\n", name, err)
			code = 1
		case was == now:
			fmt.Fprintf(c.s.Stdout, "%s is up to date at %s\n", name, short(now.Commit))
		default:
			fmt.Fprintf(c.s.Stdout, "Updated %s from %s to %s\n", name, short(was.Commit), short(now.Commit))
		}
	}
	return code
}

func (c *pluginCLI) uninstall(args []string) int {
	if len(args) != 1 {
		fmt.Fprintln(c.s.Stderr, "Usage: ghud plugin uninstall <name>")
		return 2
	}
	if _, err := c.installer.Uninstall(args[0]); err != nil {
		return c.fail(err)
	}
	fmt.Fprintln(c.s.Stdout, "Uninstalled", args[0])
	return 0
}

// list prints the installed Plugins, then those config.toml lists by path.
func (c *pluginCLI) list(listed []string) int {
	w := tabwriter.NewWriter(c.s.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "PLUGIN\tSOURCE\tCOMMIT\tWIDGET TYPES")
	row := func(name, source, commit string, m plugin.Manifest, err error) {
		types := "-"
		if err != nil {
			types = "can't read its manifest: " + err.Error()
		} else if len(m.Widgets) > 0 {
			var names []string
			for _, wt := range m.Widgets {
				names = append(names, wt.Type)
			}
			types = strings.Join(names, ", ")
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", name, source, commit, types)
	}
	for _, p := range c.installer.List() {
		source, commit := "-", "-"
		if p.Record.Source != "" {
			source, commit = p.Record.Source, short(p.Record.Commit)
			if p.Record.Ref != "" {
				source += "@" + p.Record.Ref
			}
		}
		row(filepath.Base(p.Dir), source, commit, p.Manifest, p.Err)
	}
	for _, dir := range listed {
		m, err := plugin.ReadManifest(dir)
		name := m.Name
		if name == "" {
			name = filepath.Base(dir)
		}
		row(name, dir, "-", m, err)
	}
	if err := w.Flush(); err != nil {
		return c.fail(err)
	}
	return 0
}

// trust shows what a Plugin provides and runs, and asks whether to go on.
func (c *pluginCLI) trust(plan install.Plan) bool {
	out := c.s.Stdout
	m := plan.Manifest
	fmt.Fprintf(out, "Plugin %s from %s\n", m.Name, at(plan.Record))
	if m.Description != "" {
		fmt.Fprintf(out, "  %s\n", m.Description)
	}
	fmt.Fprintln(out, "  Widget types:")
	for _, wt := range m.Widgets {
		fmt.Fprintf(out, "    %s  %s\n", wt.Type, wt.Title)
	}
	if len(m.Build) == 0 {
		fmt.Fprintln(out, "  No build steps.")
	} else {
		fmt.Fprintf(out, "  Build steps, run in %s without a shell:\n", plan.Dir)
		for _, b := range m.Build {
			fmt.Fprintf(out, "    %s\n", strings.Join(b.Run, " "))
		}
	}
	fmt.Fprintf(out, "  Runs: %s\n", strings.Join(m.Command, " "))
	if c.yes {
		return true
	}
	fmt.Fprintf(out, "A Plugin runs with your permissions. Trust %s and build it? [y/N] ", m.Name)
	line, err := c.in.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return false
	}
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}

// at is a Record as `owner/repo at abc1234 (ref)`.
func at(r plugin.Record) string {
	s := r.Source + " at " + short(r.Commit)
	if r.Ref != "" {
		s += " (" + r.Ref + ")"
	}
	return s
}

func short(commit string) string { return commit[:min(7, len(commit))] }
