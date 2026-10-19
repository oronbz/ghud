package weather_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"github.com/oronbz/ghud/internal/ports"
)

// sky fakes Open-Meteo's forecast and geocoding APIs and ipapi.is, in the
// shapes the real ones answered with (notes/open-meteo.md).
type sky struct {
	mu       sync.Mutex
	requests map[string][]url.Values
	// places are the cities geocoding knows, by lower-case name.
	places map[string]geo
	// ip is ipapi.is's answer; ipStatus, when set, replaces it with that status.
	ip       map[string]any
	ipStatus int
	// forecastStatus, when set, makes the forecast fail with Open-Meteo's error shape.
	forecastStatus int

	forecast, geocoding, ipapi *httptest.Server
}

type geo struct {
	name     string
	lat, lon float64
}

func newSky(t *testing.T) *sky {
	t.Helper()
	s := &sky{
		requests: map[string][]url.Values{},
		places:   map[string]geo{"petah tikva": {"Petah Tikva", 32.0888, 34.88666}, "paris": {"Paris", 48.85341, 2.3488}},
		ip: map[string]any{
			"ip": "203.0.113.7", "is_bogon": false, "company": "Example", "asn": "AS64500 Example",
			"city": "Tel Aviv", "region": "Tel Aviv", "country": "Israel",
			"lat": 32.0809, "lon": 34.7806, "timezone": "Asia/Jerusalem", "docs": "https://ipapi.is/free-tier.html?ref=api",
		},
	}
	s.forecast = httptest.NewServer(http.HandlerFunc(s.serveForecast))
	s.geocoding = httptest.NewServer(http.HandlerFunc(s.serveGeocoding))
	s.ipapi = httptest.NewServer(http.HandlerFunc(s.serveIP))
	t.Cleanup(func() {
		s.forecast.Close()
		s.geocoding.Close()
		s.ipapi.Close()
	})
	return s
}

func (s *sky) endpoints() ports.Endpoints {
	return ports.Endpoints{OpenMeteo: s.forecast.URL, Geocoding: s.geocoding.URL, IPAPI: s.ipapi.URL}
}

func (s *sky) record(api string, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.requests[api] = append(s.requests[api], r.URL.Query())
}

// calls are the queries api ("forecast", "geocoding", "ipapi") was asked so far.
func (s *sky) calls(api string) []url.Values {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]url.Values(nil), s.requests[api]...)
}

func (s *sky) serveIP(w http.ResponseWriter, r *http.Request) {
	s.record("ipapi", r)
	s.mu.Lock()
	status, body := s.ipStatus, s.ip
	s.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"error": "Too many requests"})
		return
	}
	json.NewEncoder(w).Encode(body)
}

func (s *sky) serveGeocoding(w http.ResponseWriter, r *http.Request) {
	s.record("geocoding", r)
	if r.URL.Path != "/v1/search" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	p, ok := s.places[strings.ToLower(r.URL.Query().Get("name"))]
	s.mu.Unlock()
	if !ok {
		json.NewEncoder(w).Encode(map[string]any{"generationtime_ms": 0.09})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{"results": []any{map[string]any{
		"id": 293918, "name": p.name, "latitude": p.lat, "longitude": p.lon,
		"country_code": "IL", "timezone": "Asia/Jerusalem", "country": "Israel", "admin1": "Central District",
	}}, "generationtime_ms": 0.11})
}

func (s *sky) serveForecast(w http.ResponseWriter, r *http.Request) {
	s.record("forecast", r)
	if r.URL.Path != "/v1/forecast" {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	status := s.forecastStatus
	s.mu.Unlock()
	if status != 0 {
		w.WriteHeader(status)
		json.NewEncoder(w).Encode(map[string]any{"error": true, "reason": "Latitude must be in range of -90 to 90°. Given: 132.0."})
		return
	}
	q := r.URL.Query()
	tempUnit, windUnit := "°C", "km/h"
	temps := []float64{21.5, 20.7, 19.9, 19.4, 18.8, 18.1, 17.9, 17.6, 17.5, 18.2, 19.6, 21.3}
	current, feels, high, low, wind := 20.9, 22.6, 25.8, 17.5, 2.2
	if q.Get("temperature_unit") == "fahrenheit" {
		tempUnit = "°F"
		temps = []float64{70.7, 69.3, 67.8, 66.9, 65.8, 64.6, 64.2, 63.7, 63.5, 64.8, 67.3, 70.3}
		current, feels, high, low = 69.6, 72.7, 78.4, 63.5
	}
	if q.Get("wind_speed_unit") == "mph" {
		windUnit, wind = "mp/h", 1.4
	}
	var times []string
	var precip, codes []int
	for i := range temps {
		times = append(times, "2026-10-18T"+twoDigits((22+i)%24)+":00")
		if i >= 2 {
			times[i] = "2026-10-19T" + twoDigits((22+i)%24) + ":00"
		}
		precip = append(precip, 0)
		codes = append(codes, 2)
	}
	json.NewEncoder(w).Encode(map[string]any{
		"latitude": 32.06273, "longitude": 34.85472, "utc_offset_seconds": 10800,
		"timezone": "Asia/Jerusalem", "timezone_abbreviation": "GMT+3", "elevation": 61.0,
		"current_units": map[string]any{"time": "iso8601", "interval": "seconds", "temperature_2m": tempUnit,
			"apparent_temperature": tempUnit, "relative_humidity_2m": "%", "weather_code": "wmo code",
			"wind_speed_10m": windUnit, "wind_direction_10m": "°", "is_day": ""},
		"current": map[string]any{"time": "2026-10-18T22:45", "interval": 900, "temperature_2m": current,
			"apparent_temperature": feels, "relative_humidity_2m": 74, "weather_code": 2,
			"wind_speed_10m": wind, "wind_direction_10m": 90, "is_day": 0},
		"hourly_units": map[string]any{"time": "iso8601", "temperature_2m": tempUnit, "precipitation_probability": "%", "weather_code": "wmo code"},
		"hourly":       map[string]any{"time": times, "temperature_2m": temps, "precipitation_probability": precip, "weather_code": codes},
		"daily_units":  map[string]any{"time": "iso8601", "temperature_2m_max": tempUnit},
		"daily": map[string]any{"time": []string{"2026-10-18"}, "weather_code": []int{3},
			"temperature_2m_max": []float64{high}, "temperature_2m_min": []float64{low}},
	})
}

func twoDigits(n int) string {
	return string(rune('0'+n/10)) + string(rune('0'+n%10))
}
