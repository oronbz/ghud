// Package weather is the Weather Plugin visor ships with. It runs as its own
// process (cmd/visor-weather) and speaks the plugin protocol over stdio like
// any external Plugin, so it depends on nothing else in visor: it only
// describes what to draw, from Open-Meteo, at a place found by the `city`
// or `lat`/`lon` settings, else by IP address, else by time zone.
package weather

import (
	_ "embed"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/oronbz/visor/internal/paths"
)

// Manifest is the Plugin's visor-plugin.toml, built into visor, which reads
// it when it finds Binary next to its own executable.
//
//go:embed visor-plugin.toml
var Manifest []byte

// Binary is the Plugin's executable name (plus .exe on Windows).
const Binary = "visor-weather"

// The environment visor sets for the Plugin's process. Empty ones fall back
// to the real services and the usual cache dir.
const (
	EnvCache     = "VISOR_CACHE_DIR"
	EnvForecast  = "VISOR_OPEN_METEO_URL"
	EnvGeocoding = "VISOR_GEOCODING_URL"
	EnvIPAPI     = "VISOR_IPAPI_URL"
)

// Main serves the protocol on stdin and stdout until stdin closes.
func Main() int {
	s := newServer(os.Stdout, config{
		cacheDir:  cacheDir(os.Getenv),
		forecast:  or(os.Getenv(EnvForecast), "https://api.open-meteo.com"),
		geocoding: or(os.Getenv(EnvGeocoding), "https://geocoding-api.open-meteo.com"),
		ipapi:     or(os.Getenv(EnvIPAPI), "https://api.ipapi.is"),
		http:      &http.Client{Timeout: 15 * time.Second},
		zone:      localZone,
		now:       time.Now,
	})
	return s.serve(os.Stdin)
}

// config is everything the Plugin takes from its process.
type config struct {
	cacheDir                   string
	forecast, geocoding, ipapi string
	http                       *http.Client
	// zone is the machine's IANA time zone name, "" when unknown.
	zone func() string
	now  func() time.Time
}

func or(v, fallback string) string {
	if v == "" {
		return fallback
	}
	return v
}

// cacheDir is visor's cache dir: the one visor passes, else where visor keeps it.
func cacheDir(getenv func(string) string) string {
	if d := getenv(EnvCache); d != "" {
		return d
	}
	if d := getenv("XDG_CACHE_HOME"); filepath.IsAbs(d) {
		return filepath.Join(d, "visor")
	}
	if home := paths.Home(getenv); home != "" {
		return filepath.Join(home, ".cache", "visor")
	}
	return filepath.Join(os.TempDir(), "visor")
}
