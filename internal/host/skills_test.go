package host_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/skills"
)

// withHome starts visor at version v with HOME at a fresh temp folder.
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

// olderVisor installs skills as visor 1.0.0 did, with content that differs from today's bundle.
func olderVisor(t *testing.T, home string) skills.Installer {
	t.Helper()
	in := skills.Installer{Agents: filepath.Join(home, ".agents", "skills"), Version: "1.0.0", Bundle: fstest.MapFS{
		"visor-configure/SKILL.md":   {Data: []byte("---\nname: visor-configure\ndescription: The 1.0 wording.\n---\nOld body.\n")},
		"visor-configure/options.md": {Data: []byte("A reference file 1.0 had.\n")},
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

	for _, name := range []string{"visor-configure", "visor-make-plugin"} {
		dir := filepath.Join(home, ".agents", "skills", name)
		if md := readSkill(t, dir); !strings.Contains(md, "metadata:\n  visor-version: \"2.3.4\"\n") {
			t.Errorf("%s isn't stamped with this visor's version:\n%s", dir, md)
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

func TestAnUntouchedSkillFromAnOlderVisorIsRewrittenSilentlyAtStart(t *testing.T) {
	home := t.TempDir()
	in := olderVisor(t, home)
	dir := filepath.Join(in.Agents, "visor-configure")

	h := withHome(t, home, "1.1.0")

	md := readSkill(t, dir)
	if strings.Contains(md, "Old body.") || !strings.Contains(md, "metadata:\n  visor-version: \"1.1.0\"\n") {
		t.Errorf("SKILL.md wasn't rewritten as 1.1.0's:\n%s", md)
	}
	if _, err := os.Stat(filepath.Join(dir, "options.md")); !os.IsNotExist(err) {
		t.Errorf("a file only 1.0.0 wrote survived the rewrite: %v", err)
	}
	if footer := h.Line(19); strings.Contains(footer, "skills") {
		t.Errorf("footer %q, want the rewrite to be silent", footer)
	}
}

func TestAnEditedSkillFromAnOlderVisorIsLeftAloneWithAHint(t *testing.T) {
	home := t.TempDir()
	in := olderVisor(t, home)
	dir := filepath.Join(in.Agents, "visor-configure")
	edited := readSkill(t, dir) + "My own note.\n"
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}

	h := withHome(t, home, "1.1.0")

	if got := readSkill(t, dir); got != edited {
		t.Errorf("the edited SKILL.md changed:\n%s", got)
	}
	if footer := h.Line(19); !strings.Contains(footer, "visor skills install") {
		t.Errorf("footer %q, want a hint to run visor skills install", footer)
	}
}

func TestSkillsFromTheSameOrANewerVisorAreLeftAloneQuietly(t *testing.T) {
	for name, v := range map[string]string{"same": "1.0.0", "newer stamp": "0.9.0", "dev build": "dev"} {
		t.Run(name, func(t *testing.T) {
			home := t.TempDir()
			in := olderVisor(t, home)
			before := readSkill(t, filepath.Join(in.Agents, "visor-configure"))

			h := withHome(t, home, v)

			if got := readSkill(t, filepath.Join(in.Agents, "visor-configure")); got != before {
				t.Errorf("visor %s rewrote a 1.0.0 skill:\n%s", v, got)
			}
			if footer := h.Line(19); strings.Contains(footer, "skills") {
				t.Errorf("footer %q, want nothing said", footer)
			}
		})
	}
}
