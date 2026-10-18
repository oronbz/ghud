package herdr_test

import (
	"bytes"
	"os"
	"regexp"
	"testing"

	"github.com/pelletier/go-toml/v2"
)

// The rules herdr 0.9.5 applies when it installs a plugin (manifest.rs), so a
// bad manifest fails here rather than on a user's herdr plugin install.
func TestManifestIsAValidHerdrBuildOnlyPlugin(t *testing.T) {
	raw, err := os.ReadFile("herdr-plugin.toml")
	if err != nil {
		t.Fatal(err)
	}
	var m struct {
		ID, Name, Version, Description string
		MinHerdrVersion                string `toml:"min_herdr_version"`
		Platforms                      []string
		Build                          []struct{ Command []string }
	}
	if err := toml.NewDecoder(bytes.NewReader(raw)).DisallowUnknownFields().Decode(&m); err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^[A-Za-z0-9.:_-]+$`).MatchString(m.ID) {
		t.Errorf("id %q has characters herdr rejects", m.ID)
	}
	if m.Name == "" {
		t.Error("name is empty")
	}
	semver := regexp.MustCompile(`^\d+\.\d+\.\d+$`)
	if !semver.MatchString(m.Version) || !semver.MatchString(m.MinHerdrVersion) {
		t.Errorf("version %q and min_herdr_version %q must be semver", m.Version, m.MinHerdrVersion)
	}
	for _, p := range m.Platforms {
		if p != "macos" && p != "linux" {
			t.Errorf("platform %q: the build step needs Homebrew, which only runs on macos and linux", p)
		}
	}
	if len(m.Platforms) == 0 {
		t.Error("platforms is empty")
	}
	if len(m.Build) != 1 {
		t.Fatalf("want one [[build]] step, got %d", len(m.Build))
	}
	cmd := m.Build[0].Command
	if len(cmd) != 2 || cmd[0] != "bash" {
		t.Fatalf("build command = %q, want [\"bash\", <script>] (herdr runs it without a shell)", cmd)
	}
	if _, err := os.Stat(cmd[1]); err != nil {
		t.Errorf("build script: %v", err)
	}
}
