package weather_test

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/plugin"
	"github.com/oronbz/ghud/internal/ports"
	"github.com/oronbz/ghud/internal/weather"
	"github.com/oronbz/ghud/internal/weather/weathertest"
)

// Weather dogfoods the protocol, so it must pass `ghud plugin check` like
// any Plugin, run from a directory holding its own manifest.
func TestWeatherPassesThePluginConformanceCheck(t *testing.T) {
	s := newSky(t)
	t.Setenv("TZ", "Asia/Jerusalem")
	t.Setenv(weather.EnvCache, t.TempDir())
	t.Setenv(weather.EnvForecast, s.forecast.URL)
	t.Setenv(weather.EnvGeocoding, s.geocoding.URL)
	t.Setenv(weather.EnvIPAPI, s.ipapi.URL)
	dir := t.TempDir()
	exe := filepath.Join(weathertest.Dir(t), weather.Binary)
	manifest := strings.Replace(string(weather.Manifest), `command = "ghud-weather"`, "command = "+strconv.Quote(exe), 1)
	if err := os.WriteFile(filepath.Join(dir, plugin.ManifestFile), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if !plugin.Check(dir, plugin.CheckOptions{Clock: ports.System(os.Getenv).Clock}, &out) {
		t.Errorf("ghud plugin check failed:\n%s", out.String())
	}
	if len(s.calls("forecast")) == 0 {
		t.Errorf("the check's refresh fetched no forecast:\n%s", out.String())
	}
}
