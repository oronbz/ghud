package weather

import (
	"context"
	"net/url"
	"strconv"
	"strings"
)

// forecast is the part of Open-Meteo's answer the Widget shows.
type forecast struct {
	CurrentUnits map[string]string `json:"current_units"`
	Current      struct {
		Temp     float64 `json:"temperature_2m"`
		Apparent float64 `json:"apparent_temperature"`
		Humidity float64 `json:"relative_humidity_2m"`
		Code     int     `json:"weather_code"`
		Wind     float64 `json:"wind_speed_10m"`
		WindDir  float64 `json:"wind_direction_10m"`
		IsDay    int     `json:"is_day"`
	} `json:"current"`
	Hourly struct {
		Time  []string   `json:"time"`
		Temp  []*float64 `json:"temperature_2m"`
		Code  []*int     `json:"weather_code"`
		IsDay []*int     `json:"is_day"`
	} `json:"hourly"`
	Daily struct {
		Max []*float64 `json:"temperature_2m_max"`
		Min []*float64 `json:"temperature_2m_min"`
	} `json:"daily"`
}

const hours = 24

func fetchForecast(ctx context.Context, cfg config, at place, units string) (forecast, error) {
	q := url.Values{
		"latitude":      {strconv.FormatFloat(at.Lat, 'f', -1, 64)},
		"longitude":     {strconv.FormatFloat(at.Lon, 'f', -1, 64)},
		"current":       {"temperature_2m,apparent_temperature,relative_humidity_2m,weather_code,wind_speed_10m,wind_direction_10m,is_day"},
		"hourly":        {"temperature_2m,weather_code,is_day"},
		"daily":         {"temperature_2m_max,temperature_2m_min"},
		"timezone":      {"auto"},
		"forecast_days": {"2"},
		// Hourly slots start at the current hour.
		"forecast_hours": {strconv.Itoa(hours)},
	}
	if units == "imperial" {
		q.Set("temperature_unit", "fahrenheit")
		q.Set("wind_speed_unit", "mph")
		q.Set("precipitation_unit", "inch")
	}
	var f forecast
	_, err := getJSON(ctx, cfg, "Open-Meteo", strings.TrimRight(cfg.forecast, "/")+"/v1/forecast?"+q.Encode(), &f)
	return f, err
}

// conditions are the WMO weather codes Open-Meteo reports.
var conditions = map[int]string{
	0: "Clear sky", 1: "Mainly clear", 2: "Partly cloudy", 3: "Overcast",
	45: "Fog", 48: "Rime fog",
	51: "Light drizzle", 53: "Drizzle", 55: "Dense drizzle",
	56: "Light freezing drizzle", 57: "Freezing drizzle",
	61: "Light rain", 63: "Rain", 65: "Heavy rain",
	66: "Light freezing rain", 67: "Freezing rain",
	71: "Light snow", 73: "Snow", 75: "Heavy snow", 77: "Snow grains",
	80: "Light showers", 81: "Showers", 82: "Violent showers",
	85: "Light snow showers", 86: "Snow showers",
	95: "Thunderstorm", 96: "Thunderstorm, hail", 97: "Heavy thunderstorm", 99: "Thunderstorm, heavy hail",
}

// sky is the glyph for a WMO weather code, a moon for clear nights.
func sky(code int, day bool) span {
	switch {
	case code <= 2:
		if day {
			return span{Text: "☼", Token: "warn"}
		}
		return span{Text: "☾", Token: "info"}
	case code == 3:
		return span{Text: "☁", Token: "dim"}
	case code == 45 || code == 48:
		return span{Text: "≡", Token: "faint"}
	case code >= 71 && code <= 77, code == 85 || code == 86:
		return span{Text: "❄", Token: "bright"}
	case code >= 95:
		return span{Text: "ϟ", Token: "warn"}
	}
	return span{Text: "☂", Token: "accent"}
}
