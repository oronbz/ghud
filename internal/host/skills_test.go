package host_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/skills"
)

// withHome starts ghud at version v with HOME at a fresh temp folder.
func withHome(t *testing.T, home, v string) *hosttest.Host {
	t.Helper()
	return hosttest.New(t, hosttest.Options{Types: liveViews, Width: 80, Height: 20, Version: v, Env: map[string]string{"HOME": home}})
}

func readSkill(t *testing.T, dir string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(dir, "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// olderGhud installs skills as ghud 1.0.0 did, with content that differs from today's bundle.
func olderGhud(t *testing.T, home string) skills.Installer {
	t.Helper()
	in := skills.Installer{Agents: filepath.Join(home, ".agents", "skills"), Version: "1.0.0", Bundle: fstest.MapFS{
		"ghud-configure/SKILL.md":   {Data: []byte("---\nname: ghud-configure\ndescription: The 1.0 wording.\n---\nOld body.\n")},
		"ghud-configure/options.md": {Data: []byte("A reference file 1.0 had.\n")},
	}}
	if _, err := in.Install(); err != nil {
		t.Fatal(err)
	}
	return in
}

func TestInstallSkillsFromThePaletteWritesStampedFoldersAndClaudeLinks(t *testing.T) {
	home := t.TempDir()
	if err := os.Mkdir(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	h := withHome(t, home, "2.3.4")

	runFromPalette(t, h, "install agent skills")

	for _, name := range []string{"ghud-configure", "ghud-make-plugin"} {
		dir := filepath.Join(home, ".agents", "skills", name)
		if md := readSkill(t, dir); !strings.Contains(md, "metadata:\n  ghud-version: \"2.3.4\"\n") {
			t.Errorf("%s isn't stamped with this ghud's version:\n%s", dir, md)
		}
		link := filepath.Join(home, ".claude", "skills", name)
		if target, err := os.Readlink(link); err != nil || target != dir {
			t.Errorf("%s -> %q (%v), want a symlink to %s", link, target, err, dir)
		}
	}
	if footer := h.Line(19); !strings.Contains(footer, "Installed 2 agent skills") {
		t.Errorf("footer %q, want a toast saying the skills were installed", footer)
	}
}

func TestAnUntouchedSkillFromAnOlderGhudIsRewrittenSilentlyAtStart(t *testing.T) {
	home := t.TempDir()
	in := olderGhud(t, home)
	dir := filepath.Join(in.Agents, "ghud-configure")

	h := withHome(t, home, "1.1.0")

	md := readSkill(t, dir)
	if strings.Contains(md, "Old body.") || !strings.Contains(md, "metadata:\n  ghud-version: \"1.1.0\"\n") {
		t.Errorf("SKILL.md wasn't rewritten as 1.1.0's:\n%s", md)
	}
	if _, err := os.Stat(filepath.Join(dir, "options.md")); !os.IsNotExist(err) {
		t.Errorf("a file only 1.0.0 wrote survived the rewrite: %v", err)
	}
	if footer := h.Line(19); strings.Contains(footer, "skills") {
		t.Errorf("footer %q, want the rewrite to be silent", footer)
	}
}

func TestAnEditedSkillFromAnOlderGhudIsLeftAloneWithAHint(t *testing.T) {
	home := t.TempDir()
	in := olderGhud(t, home)
	dir := filepath.Join(in.Agents, "ghud-configure")
	edited := readSkill(t, dir) + "My own note.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	h := withHome(t, home, "1.1.0")

	if got := readSkill(t, dir); got != edited {
		t.Errorf("the edited SKILL.md changed:\n%s", got)
	}
	if footer := h.Line(19); !strings.Contains(footer, "ghud skills install") {
		t.Errorf("footer %q, want a hint to run ghud skills install", footer)
	}
}

func TestSkillsFromTheSameOrANewerGhudAreLeftAloneQuietly(t *testing.T) {
	for name, v := range map[string]string{"same": "1.0.0", "newer stamp": "0.9.0", "dev build": "dev"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			in := olderGhud(t, home)
			before := readSkill(t, filepath.Join(in.Agents, "ghud-configure"))

			h := withHome(t, home, v)

			if got := readSkill(t, filepath.Join(in.Agents, "ghud-configure")); got != before {
				t.Errorf("ghud %s rewrote a 1.0.0 skill:\n%s", v, got)
			}
			if footer := h.Line(19); strings.Contains(footer, "skills") {
				t.Errorf("footer %q, want nothing said", footer)
			}
		})
	}
}
