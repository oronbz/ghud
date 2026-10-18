package weather_test

import (
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/weather/weathertest"
	"github.com/oronbz/ghud/internal/widget"
)

// withWeather starts ghud with the Weather Plugin next to its executable, the
// way an install ships it, and only the Widgets config places.
func withWeather(t *testing.T, s *sky, config string) *hosttest.Host {
	t.Helper()
	return withWeatherAt(t, s, t.TempDir(), config)
}

func withWeatherAt(t *testing.T, s *sky, root, config string) *hosttest.Host {
	t.Helper()
	return hosttest.New(t, hosttest.Options{
		Root:      root,
		Width:     48,
		Height:    16,
		Types:     []widget.Type{},
		Config:    config,
		BundleDir: weathertest.Dir(t),
		Endpoints: s.endpoints(),
	})
}

func shows(t *testing.T, h *hosttest.Host, want ...string) {
	t.Helper()
	screen := h.Screen()
	for _, w := range want {
		if !strings.Contains(screen, w) {
			t.Errorf("no %q on screen:\n%s", w, screen)
		}
	}
}

func hides(t *testing.T, h *hosttest.Host, unwanted ...string) {
	t.Helper()
	screen := h.Screen()
	for _, w := range unwanted {
		if strings.Contains(screen, w) {
			t.Errorf("%q on screen:\n%s", w, screen)
		}
	}
}

const weatherWidget = "[[widget]]\ntype = \"weather\"\n"

func TestWithACityWeatherShowsTheForecastCreditedToOpenMeteo(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := withWeather(t, s, weatherWidget+"city = \"Petah Tikva\"\n")
	shows(t, h,
		"Petah Tikva",
		"21°C Partly cloudy",
		"high 26° · low 18° · feels 23°",
		"wind 2 km/h E · humidity 74%",
		"now  23   00   01",
		"22°  21°  20°  19°",
		"Weather data by Open-Meteo.com",
	)
	hides(t, h, "from IP")
	f := s.calls("forecast")
	if len(f) != 1 || f[0].Get("latitude") != "32.0888" || f[0].Get("longitude") != "34.88666" {
		t.Errorf("forecast requests = %v, want one for Petah Tikva's coordinates", f)
	}
	if n := len(s.calls("ipapi")); n != 0 {
		t.Errorf("ipapi.is was asked %d times with a city set", n)
	}
}
