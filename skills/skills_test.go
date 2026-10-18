package skills_test

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/skills"
)

var want = []string{"ghud-configure", "ghud-make-plugin"}

// TestTheRepoHoldsExactlyTheBundledSkills is what `npx skills add <repo>`
// relies on: no root SKILL.md, and one folder per skill under skills/.
func TestTheRepoHoldsExactlyTheBundledSkills(t *testing.T) {
	var found []string
	err := filepath.WalkDir("..", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		if d.Name() == "SKILL.md" {
			found = append(found, filepath.ToSlash(p))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, name := range want {
		paths = append(paths, "../skills/"+name+"/SKILL.md")
	}
	if !slices.Equal(found, paths) {
		t.Errorf("SKILL.md files in the repo: %q, want exactly %q", found, paths)
	}

	var embedded []string
	entries, err := fs.ReadDir(skills.FS, ".")
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		embedded = append(embedded, e.Name())
	}
	if !slices.Equal(embedded, want) {
		t.Errorf("the binary bundles %q, want %q", embedded, want)
	}
}

var (
	nameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)
	// plainRe is a one-line YAML plain scalar that can't be read as anything else.
	plainRe = regexp.MustCompile(`^[^-?:,\[\]{}#&*!|>'"%@` + "`" + `\s][^\n]*$`)
)

// TestEverySkillHasValidAgentSkillsFrontmatter checks the Agent Skills
// specification's rules, and leaves metadata to the installer's stamp.
func TestEverySkillHasValidAgentSkillsFrontmatter(t *testing.T) {
	for _, name := range want {
		t.Run(name, func(t *testing.T) {
			md, err := os.ReadFile(filepath.Join(name, "SKILL.md"))
			if err != nil {
				t.Fatal(err)
			}
			rest, ok := strings.CutPrefix(string(md), "---\n")
			front, body, ok2 := strings.Cut(rest, "\n---\n")
			if !ok || !ok2 {
				t.Fatalf("no frontmatter")
			}
			fields := map[string]string{}
			for _, l := range strings.Split(front, "\n") {
				k, v, ok := strings.Cut(l, ": ")
				if !ok {
					t.Fatalf("frontmatter line %q isn't `key: value`", l)
				}
				if !slices.Contains([]string{"name", "description", "license", "compatibility", "allowed-tools"}, k) {
					t.Errorf("frontmatter key %q isn't in the spec (or is metadata, which the installer writes)", k)
				}
				if !plainRe.MatchString(v) || strings.Contains(v, ": ") || strings.Contains(v, " #") || strings.HasSuffix(v, ":") {
					t.Errorf("%s: %q isn't a safe plain YAML scalar", k, v)
				}
				fields[k] = v
			}
			if n := fields["name"]; n != name || len(n) > 64 || !nameRe.MatchString(n) {
				t.Errorf("name %q, want %q (lowercase letters, digits and single hyphens, ≤64)", n, name)
			}
			if d := fields["description"]; len(d) == 0 || len(d) > 1024 {
				t.Errorf("description is %d characters, want 1–1024", len(d))
			}
			if lines := strings.Count(body, "\n"); lines > 500 {
				t.Errorf("SKILL.md body is %d lines; keep it under 500", lines)
			}
		})
	}
}

func TestTheMakePluginSkillListsEveryThemeToken(t *testing.T) {
	md, err := os.ReadFile(filepath.Join("ghud-make-plugin", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for tok := theme.Bg; tok <= theme.Alt; tok++ {
		if !strings.Contains(string(md), "`"+tok.String()+"`") {
			t.Errorf("SKILL.md doesn't list the token `%s`", tok)
		}
	}
}
