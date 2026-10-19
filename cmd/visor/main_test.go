package main_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The release config, not this test, owns the -X flag, so a moved version
// variable fails here instead of shipping a binary that reports "dev".
func TestReleaseLdflagsStampTheVersion(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a binary")
	}
	cfg, err := os.ReadFile("../../.goreleaser.yaml")
	if err != nil {
		t.Fatal(err)
	}
	stamp := regexp.MustCompile(`-X (\S+)=\{\{ ?\.Version ?\}\}`).FindSubmatch(cfg)
	if stamp == nil {
		t.Fatal(".goreleaser.yaml has no -X <var>={{.Version}} ldflag")
	}

	bin := filepath.Join(t.TempDir(), "visor")
	build := exec.Command("go", "build", "-ldflags", "-X "+string(stamp[1])+"=1.2.3", "-o", bin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}
	out, err := exec.Command(bin, "--version").Output()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(string(out)); got != "visor 1.2.3" {
		t.Errorf("--version printed %q, want %q", got, "visor 1.2.3")
	}
}
