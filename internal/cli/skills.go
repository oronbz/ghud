package cli

import (
	"fmt"

	"github.com/oronbz/ghud/internal/skills"
	"github.com/oronbz/ghud/internal/version"
)

func skillsCommand(s System, args []string) int {
	if len(args) != 1 || args[0] != "install" && args[0] != "uninstall" {
		fmt.Fprint(s.Stderr, "ghud: skills needs install or uninstall\n\n", usage)
		return 2
	}
	in, err := skills.For(s.Getenv, version.Version)
	if err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	var r skills.Report
	if args[0] == "install" {
		r, err = in.Install()
	} else {
		r, err = in.Uninstall()
	}
	for _, p := range r.Installed {
		fmt.Fprintln(s.Stdout, "installed", p)
	}
	for _, p := range r.Linked {
		fmt.Fprintln(s.Stdout, "linked", p)
	}
	for _, p := range r.Removed {
		fmt.Fprintln(s.Stdout, "removed", p)
	}
	for _, note := range r.Kept {
		fmt.Fprintln(s.Stdout, note)
	}
	if err != nil {
		fmt.Fprintln(s.Stderr, "ghud:", err)
		return 1
	}
	if args[0] == "uninstall" && len(r.Removed) == 0 {
		fmt.Fprintln(s.Stdout, "no skills installed by ghud in", in.Agents)
	}
	return 0
}
