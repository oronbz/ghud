package cli_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/cli"
	"github.com/oronbz/ghud/internal/version"
)

var bundled = []string{"ghud-configure", "ghud-make-plugin"}

// skillMD reads the SKILL.md of the skill folder dir.
func skillMD(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func frontmatter(t *testing.T, md string) string {
	t.Helper()
	rest, ok := strings.CutPrefix(md, "---\n")
	end := strings.Index(rest, "\n---\n")
	if !ok || end < 0 {
		t.Fatalf("no frontmatter:\n%s", md)
	}
	return rest[:end+1]
}

func stamped(t *testing.T, dir, v string) {
	t.Helper()
	fm := frontmatter(t, skillMD(t, dir))
	if !strings.Contains(fm, "metadata:\n  ghud-version: \""+v+"\"\n") {
		t.Errorf("%s isn't stamped with ghud-version %q:\n%s", dir, v, fm)
	}
}

func TestSkillsInstallWritesBothSkillsStampedWithTheVersion(t *testing.T) {
	s, home, stdout, stderr := sandbox(t, nil, "", "skills", "install")
	if code := cli.Run(s); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	for _, name := range bundled {
		dir := filepath.Join(home, ".agents", "skills", name)
		fi, err := os.Lstat(dir)
		if err != nil || !fi.IsDir() {
			t.Fatalf("%s isn't a real folder: %v", dir, err)
		}
		stamped(t, dir, version.Version)
		if !strings.Contains(frontmatter(t, skillMD(t, dir)), "name: "+name+"\n") {
			t.Errorf("%s/SKILL.md isn't the %s skill", dir, name)
		}
		if !strings.Contains(stdout.String(), dir) {
			t.Errorf("the output doesn't name %s:\n%s", dir, stdout)
		}
	}
	if _, err := os.Lstat(filepath.Join(home, ".claude")); !os.IsNotExist(err) {
		t.Errorf("install made ~/.claude, which didn't exist: %v", err)
	}
}

func TestSkillsInstallLinksThemIntoClaudesSkillsWhenClaudeIsSetUp(t *testing.T) {
	for name, c := range map[string]struct {
		env    map[string]string
		claude string
	}{
		"~/.claude":         {claude: ".claude"},
		"CLAUDE_CONFIG_DIR": {env: map[string]string{"CLAUDE_CONFIG_DIR": "$HOME/elsewhere/claude"}, claude: "elsewhere/claude"},
	} {
		t.Run(name, func(t *testing.T) {
			s, home, stdout, stderr := sandbox(t, c.env, "", "skills", "install")
			if err := os.MkdirAll(filepath.Join(home, c.claude), 0o755); err != nil {
				t.Fatal(err)
			}
			if code := cli.Run(s); code != 0 {
				t.Fatalf("exit %d: %s", code, stderr)
			}
			for _, name := range bundled {
				link := filepath.Join(home, c.claude, "skills", name)
				target, err := os.Readlink(link)
				if err != nil {
					t.Fatalf("%s isn't a symlink: %v", link, err)
				}
				if want := filepath.Join(home, ".agents", "skills", name); target != want {
					t.Errorf("%s -> %s, want %s", link, target, want)
				}
				stamped(t, link, version.Version)
				if !strings.Contains(stdout.String(), link) {
					t.Errorf("the output doesn't name %s:\n%s", link, stdout)
				}
			}
		})
	}
}

func TestSkillsInstallReplacesTheFoldersWhole(t *testing.T) {
	s, home, _, stderr := sandbox(t, nil, "", "skills", "install")
	dir := filepath.Join(home, ".agents", "skills", "ghud-configure")
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: ghud-configure\ndescription: old\n---\nold\n")
	writeFile(t, filepath.Join(dir, "leftover.md"), "from an older ghud\n")

	if code := cli.Run(s); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "leftover.md")); !os.IsNotExist(err) {
		t.Errorf("a file the bundle doesn't have survived: %v", err)
	}
	stamped(t, dir, version.Version)
}

func TestSkillsUninstallRemovesOnlyStampedFoldersAndTheirLinks(t *testing.T) {
	s, home, _, stderr := sandbox(t, nil, "", "skills", "install")
	claudeSkills := filepath.Join(home, ".claude", "skills")
	if err := os.MkdirAll(claudeSkills, 0o755); err != nil {
		t.Fatal(err)
	}
	if code := cli.Run(s); code != 0 {
		t.Fatalf("install: exit %d: %s", code, stderr)
	}
	agents := filepath.Join(home, ".agents", "skills")
	other := filepath.Join(agents, "someone-elses")
	writeFile(t, filepath.Join(other, "SKILL.md"), "---\nname: someone-elses\ndescription: Not ghud's.\n---\nHello.\n")
	otherLink := filepath.Join(claudeSkills, "someone-elses")
	if err := os.Symlink(other, otherLink); err != nil {
		t.Fatal(err)
	}
	unstamped := filepath.Join(home, "elsewhere", "ghud-configure")
	writeFile(t, filepath.Join(unstamped, "SKILL.md"), "---\nname: ghud-configure\ndescription: A copy from npx skills.\n---\n")

	s.Args = []string{"skills", "uninstall"}
	if code := cli.Run(s); code != 0 {
		t.Fatalf("uninstall: exit %d: %s", code, stderr)
	}
	for _, name := range bundled {
		for _, p := range []string{filepath.Join(agents, name), filepath.Join(claudeSkills, name)} {
			if _, err := os.Lstat(p); !os.IsNotExist(err) {
				t.Errorf("%s survived uninstall: %v", p, err)
			}
		}
	}
	for _, p := range []string{filepath.Join(other, "SKILL.md"), otherLink, unstamped} {
		if _, err := os.Lstat(p); err != nil {
			t.Errorf("uninstall removed %s, which ghud didn't install: %v", p, err)
		}
	}
}

func TestSkillsUninstallLeavesAnUnstampedSkillOfTheSameName(t *testing.T) {
	s, home, _, stderr := sandbox(t, nil, "", "skills", "uninstall")
	dir := filepath.Join(home, ".agents", "skills", "ghud-configure")
	writeFile(t, filepath.Join(dir, "SKILL.md"), "---\nname: ghud-configure\ndescription: From npx skills.\n---\n")

	if code := cli.Run(s); code != 0 {
		t.Fatalf("exit %d: %s", code, stderr)
	}
	if _, err := os.Stat(filepath.Join(dir, "SKILL.md")); err != nil {
		t.Errorf("uninstall removed a skill ghud didn't stamp: %v", err)
	}
}
