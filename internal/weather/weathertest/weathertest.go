// Package weathertest builds the Weather Plugin's executable for tests, so
// they can hand ghud a directory laid out like an install.
package weathertest

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/oronbz/ghud/internal/weather"
)

var (
	build    sync.Once
	dir      string
	buildErr error
)

// Dir is a directory holding a freshly built ghud-weather, built once per
// test process: what ghud's own directory holds once installed.
func Dir(t testing.TB) string {
	t.Helper()
	build.Do(func() {
		_, here, _, _ := runtime.Caller(0)
		dir, buildErr = os.MkdirTemp("", "ghud-bundle-")
		if buildErr != nil {
			return
		}
		bin := filepath.Join(dir, weather.Binary)
		if runtime.GOOS == "windows" {
			bin += ".exe"
		}
		root := filepath.Join(filepath.Dir(here), "..", "..", "..")
		// The test binary links little of the Plugin, so reading its sources
		// is what tells go test's cache that a change to them matters.
		for _, src := range []string{"internal/weather", "cmd/ghud-weather"} {
			files, _ := os.ReadDir(filepath.Join(root, src))
			for _, f := range files {
				if !f.IsDir() {
					_, _ = os.ReadFile(filepath.Join(root, src, f.Name()))
				}
			}
		}
		cmd := exec.Command("go", "build", "-o", bin, "./cmd/ghud-weather")
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			buildErr = fmt.Errorf("building ghud-weather: %v\n%s", err, out)
		}
	})
	if buildErr != nil {
		t.Fatal(buildErr)
	}
	return dir
}
