package weather_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/ghud/internal/paths"
)

func TestACityIsGeocodedOnceAcrossRefreshesAndInstances(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	root := t.TempDir()
	a := withWeatherAt(t, s, root, weatherWidget+"city = \"Petah Tikva\"\n")
	a.Press("ctrl+r")
	b := withWeatherAt(t, s, root, weatherWidget+"city = \"Petah Tikva\"\n")
	b.Press("ctrl+r")
	shows(t, b, "Petah Tikva", "21°C Partly cloudy")
	if n := len(s.calls("geocoding")); n != 1 {
		t.Errorf("geocoded %d times, want once", n)
	}
	if n := len(s.calls("forecast")); n != 3 {
		t.Errorf("fetched the forecast %d times, want 3 (start and two ^R)", n)
	}
}

func TestWithNoCityTheIPLocationIsUsedAndNamedInTheHeader(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := withWeather(t, s, weatherWidget)
	shows(t, h, "Tel Aviv · from IP", "21°C Partly cloudy")
	f := s.calls("forecast")
	if len(f) != 1 || f[0].Get("latitude") != "32.0809" || f[0].Get("longitude") != "34.7806" {
		t.Errorf("forecast requests = %v, want one for the IP's coordinates", f)
	}
}

func TestASecondInstanceReusesTheIPLocationFromTheSharedCache(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	root := t.TempDir()
	withWeatherAt(t, s, root, weatherWidget)
	b := withWeatherAt(t, s, root, weatherWidget)
	b.Press("ctrl+r")
	shows(t, b, "Tel Aviv · from IP")
	if n := len(s.calls("ipapi")); n != 1 {
		t.Errorf("ipapi.is was asked %d times, want once", n)
	}
	if n := len(s.calls("forecast")); n != 2 {
		t.Errorf("fetched the forecast %d times, want twice", n)
	}
	if _, err := os.Stat(filepath.Join(paths.Under(root).Cache, "weather", "location.json")); err != nil {
		t.Errorf("no shared location cache: %v", err)
	}
}

func TestAnIPLocationOlderThanThreeHoursIsLookedUpAgain(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	root := t.TempDir()
	writeLocation(t, root, "Haifa", "Asia/Jerusalem", time.Now().Add(-3*time.Hour-time.Minute))
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "Tel Aviv · from IP")
	if n := len(s.calls("ipapi")); n != 1 {
		t.Errorf("ipapi.is was asked %d times, want once", n)
	}
}

func TestAFreshCachedIPLocationIsUsedWithoutALookup(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	root := t.TempDir()
	writeLocation(t, root, "Haifa", "Asia/Jerusalem", time.Now().Add(-2*time.Hour))
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "Haifa · from IP")
	if n := len(s.calls("ipapi")); n != 0 {
		t.Errorf("ipapi.is was asked %d times, want none", n)
	}
}

func TestATimeZoneChangeMakesTheCachedLocationStale(t *testing.T) {
	t.Setenv("TZ", "Europe/Berlin")
	s := newSky(t)
	root := t.TempDir()
	writeLocation(t, root, "Haifa", "Asia/Jerusalem", time.Now().Add(-time.Minute))
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "Tel Aviv · from IP")
	hides(t, h, "Haifa")
	if n := len(s.calls("ipapi")); n != 1 {
		t.Errorf("ipapi.is was asked %d times, want once", n)
	}
}

func TestTheLastGoodIPLocationIsKeptWhenALookupFails(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	s.ipStatus = http.StatusInternalServerError
	root := t.TempDir()
	writeLocation(t, root, "Haifa", "Asia/Jerusalem", time.Now().Add(-5*time.Hour))
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "Haifa · from IP", "21°C Partly cloudy")
}

func TestWithIPLookupFailingTheTimeZonesCityIsUsedAndMarkedApproximate(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	s.ip = map[string]any{"ip": "10.0.0.1", "is_bogon": true, "city": nil, "region": nil, "country": nil, "lat": nil, "lon": nil, "timezone": nil}
	h := withWeather(t, s, weatherWidget)
	shows(t, h, "~Jerusalem · from timezone", "21°C Partly cloudy")
	f := s.calls("forecast")
	if len(f) != 1 || !strings.HasPrefix(f[0].Get("latitude"), "31.7") || !strings.HasPrefix(f[0].Get("longitude"), "35.2") {
		t.Errorf("forecast requests = %v, want one near Jerusalem (31.78, 35.22)", f)
	}
}

func TestARefusedIPLookupIsNotRetriedUntilUTCMidnight(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	s.ipStatus = http.StatusTooManyRequests
	root := t.TempDir()
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "~Jerusalem · from timezone")
	h.Press("ctrl+r")
	h.Press("1", "r")
	if n := len(s.calls("ipapi")); n != 1 {
		t.Errorf("ipapi.is was asked %d times after refusing, want once", n)
	}
	b, err := os.ReadFile(filepath.Join(paths.Under(root).Cache, "weather", "location.json"))
	if err != nil {
		t.Fatal(err)
	}
	var c struct {
		IPRetry time.Time `json:"ip_retry"`
	}
	if err := json.Unmarshal(b, &c); err != nil {
		t.Fatal(err)
	}
	y, m, d := time.Now().UTC().Date()
	if want := time.Date(y, m, d+1, 0, 0, 0, 0, time.UTC); !c.IPRetry.Equal(want) {
		t.Errorf("ip_retry = %v, want the next UTC midnight %v", c.IPRetry, want)
	}
}

func TestWithNoWayToLocateWeatherNeedsSetupAndSettingACityFixesIt(t *testing.T) {
	t.Setenv("TZ", "UTC")
	s := newSky(t)
	s.ipStatus = http.StatusInternalServerError
	root := t.TempDir()
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "needs setup", "Set city")
	h.Press("1", "enter")
	h.Type("Paris")
	h.Press("enter")
	shows(t, h, "Paris", "21°C Partly cloudy")
	cfg, err := os.ReadFile(filepath.Join(paths.Under(root).Config, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(cfg), `city = "Paris"`) {
		t.Errorf("config.toml doesn't set the city:\n%s", cfg)
	}
}

func TestAnUnknownCityNeedsSetup(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := withWeather(t, s, weatherWidget+"city = \"Atlantis\"\n")
	shows(t, h, "needs setup", "no place called")
	if n := len(s.calls("ipapi")); n != 0 {
		t.Errorf("ipapi.is was asked %d times with a city set", n)
	}
}

func TestLatAndLonWinOverCityWhichNamesThePlace(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := withWeather(t, s, weatherWidget+"city = \"Home\"\nlat = \"32.1\"\nlon = \"34.9\"\n")
	shows(t, h, "Home", "21°C Partly cloudy")
	f := s.calls("forecast")
	if len(f) != 1 || f[0].Get("latitude") != "32.1" || f[0].Get("longitude") != "34.9" {
		t.Errorf("forecast requests = %v, want one at 32.1, 34.9", f)
	}
	if n := len(s.calls("geocoding")) + len(s.calls("ipapi")); n != 0 {
		t.Errorf("located %d times with lat and lon set", n)
	}
}

func TestRefreshLocationBypassesTheCache(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	root := t.TempDir()
	writeLocation(t, root, "Haifa", "Asia/Jerusalem", time.Now().Add(-time.Minute))
	h := withWeatherAt(t, s, root, weatherWidget)
	shows(t, h, "Haifa · from IP")
	h.Press("1", "r")
	shows(t, h, "Tel Aviv · from IP")
	if n := len(s.calls("ipapi")); n != 1 {
		t.Errorf("ipapi.is was asked %d times, want once", n)
	}
}

// writeLocation leaves an IP location in the shared cache, found at `at`
// while the machine was in time zone zone.
func writeLocation(t *testing.T, root, name, zone string, at time.Time) {
	t.Helper()
	dir := filepath.Join(paths.Under(root).Cache, "weather")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(map[string]any{"ip": map[string]any{
		"lat": 32.794, "lon": 34.9896, "place": name, "source": "ip", "zone": zone, "at": at.UTC().Format(time.RFC3339),
	}})
	if err := os.WriteFile(filepath.Join(dir, "location.json"), b, 0o600); err != nil {
		t.Fatal(err)
	}
}
