package host

import (
	"os"
	"path/filepath"
	"runtime"

	"github.com/oronbz/ghud/internal/plugin"
	"github.com/oronbz/ghud/internal/weather"
)

// bundled are the manifests of the Plugins ghud ships with whose
// executables sit in BundleDir. Weather is the only one; it gets ghud's
// cache dir and the HTTP endpoints through its environment.
func (m Model) bundled() []plugin.Manifest {
	if m.opts.BundleDir == "" {
		return nil
	}
	exe := filepath.Join(m.opts.BundleDir, weather.Binary)
	if runtime.GOOS == "windows" {
		exe += ".exe"
	}
	if _, err := os.Stat(exe); err != nil {
		return nil
	}
	e := m.opts.Ports.Endpoints
	mf, err := plugin.Shipped(weather.Manifest, exe, []string{
		weather.EnvCache + "=" + m.opts.Paths.Cache,
		weather.EnvForecast + "=" + e.OpenMeteo,
		weather.EnvGeocoding + "=" + e.Geocoding,
		weather.EnvIPAPI + "=" + e.IPAPI,
	})
	if err != nil {
		return nil
	}
	return []plugin.Manifest{mf}
}
