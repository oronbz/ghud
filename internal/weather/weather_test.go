package weather_test

import (
	"net/http"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/hosttest"
	"github.com/oronbz/ghud/internal/theme"
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
		"☾ 21°C Partly cloudy",
		"high 26° · low 18° · feels 23°",
		"wind 2 km/h E · humidity 74%",
		"now  23   00   01",
		"☾22° ☾21° ☂20° ☁19°",
		"Weather data by Open-Meteo.com",
	)
	hides(t, h, "from IP")
	for _, c := range []struct {
		glyph string
		want  theme.Token
	}{{"☾", theme.Info}, {"☂", theme.Accent}, {"☁", theme.Dim}} {
		x, y, ok := h.Find(c.glyph)
		if !ok {
			t.Errorf("%q not on screen", c.glyph)
			continue
		}
		if got, _ := h.Fg(x, y); got != c.want {
			t.Errorf("%q drawn in %v, want %v", c.glyph, got, c.want)
		}
	}
	f := s.calls("forecast")
	if len(f) != 1 || f[0].Get("latitude") != "32.0888" || f[0].Get("longitude") != "34.88666" {
		t.Errorf("forecast requests = %v, want one for Petah Tikva's coordinates", f)
	}
	if n := len(s.calls("ipapi")); n != 0 {
		t.Errorf("ipapi.is was asked %d times with a city set", n)
	}
}

func TestAShortWidgetDropsTheHourlyRowsAndKeepsTheCredit(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := hosttest.New(t, hosttest.Options{
		Width: 48, Height: 9, Types: []widget.Type{}, Config: weatherWidget + "city = \"Petah Tikva\"\n",
		BundleDir: weathertest.Dir(t), Endpoints: s.endpoints(),
	})
	h.Receive()
	shows(t, h, "Petah Tikva", "21°C Partly cloudy", "humidity 74%", "Weather data by Open-Meteo.com")
	hides(t, h, "now  23")
	if n := len(s.calls("forecast")); n != 1 {
		t.Errorf("fetched the forecast %d times; fitting a new size needs no fetch", n)
	}
}

func TestImperialUnitsAskForFahrenheitAndMph(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := withWeather(t, s, weatherWidget+"city = \"Petah Tikva\"\nunits = \"imperial\"\n")
	shows(t, h, "70°F Partly cloudy", "high 78° · low 64° · feels 73°", "wind 1 mph E")
	if q := s.calls("forecast")[0]; q.Get("temperature_unit") != "fahrenheit" || q.Get("wind_speed_unit") != "mph" {
		t.Errorf("forecast query = %v, want fahrenheit and mph", q)
	}
}

func TestAnOpenMeteoErrorShowsItsReason(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	s.forecastStatus = http.StatusBadRequest
	h := withWeather(t, s, weatherWidget+"city = \"Petah Tikva\"\n")
	shows(t, h, "Latitude must be in range")
}

func TestWeatherIsInTheDefaultLayout(t *testing.T) {
	t.Setenv("TZ", "Asia/Jerusalem")
	s := newSky(t)
	h := withWeather(t, s, "")
	shows(t, h, "─ Weather ", "Tel Aviv · from IP")
}
