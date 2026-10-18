package weather

import (
	"fmt"
	"math"
	"strings"
	"unicode/utf8"
)

// report is a forecast for a place, ready to draw at any size.
type report struct {
	place place
	f     forecast
}

type span struct {
	Text  string `json:"text"`
	Token string `json:"token"`
	Bold  bool   `json:"bold,omitempty"`
}

type line struct {
	Spans []span `json:"spans"`
}

// block is lines that show or go together; lower rank stays longest
// when the Widget is too short for everything.
type block struct {
	rank  int
	lines []line
}

// hourWidth is the cells one hour takes in the hourly rows.
const hourWidth = 5

// payload draws r to fit sz, the Widget's inner size; a zero size is unknown.
func (r *report) payload(sz size) map[string]any {
	f := r.f
	deg := "°"
	unit := f.CurrentUnits["temperature_2m"]
	if unit == "" {
		unit = "°C"
	}
	header := span{Text: r.place.header(), Token: "dim"}
	if r.place.Source == fromZone {
		header.Token = "warn"
	}
	blocks := []block{
		{0, []line{{[]span{header}}}},
		{0, []line{{[]span{
			{Text: temp(f.Current.Temp) + unit, Token: "bright", Bold: true},
			{Text: " " + condition(f.Current.Code), Token: "fg"},
		}}}},
	}
	var details []string
	if hi, lo := first(f.Daily.Max), first(f.Daily.Min); hi != nil && lo != nil {
		details = append(details, "high "+temp(*hi)+deg, "low "+temp(*lo)+deg)
	}
	details = append(details, "feels "+temp(f.Current.Apparent)+deg)
	blocks = append(blocks, block{1, fit(sz.Width, details...)})
	blocks = append(blocks, block{2, fit(sz.Width,
		fmt.Sprintf("wind %s %s %s", temp(f.Current.Wind), windUnit(f.CurrentUnits["wind_speed_10m"]), compass(f.Current.WindDir)),
		fmt.Sprintf("humidity %s%%", temp(f.Current.Humidity)),
	)})
	if hourly := r.hourly(sz.Width); len(hourly) > 0 {
		blocks = append(blocks, block{3, append([]line{{}}, hourly...)})
	}
	credit := "Weather data by Open-Meteo.com"
	if sz.Width > 0 && sz.Width < utf8.RuneCountInString(credit) {
		credit = "Open-Meteo.com"
	}
	blocks = append(blocks, block{0, []line{{[]span{{Text: credit, Token: "faint"}}}}})
	return map[string]any{
		"lines": shown(blocks, sz.Height),
		"actions": []any{
			map[string]any{"id": refreshLocation, "key": "r", "label": "refresh location"},
			map[string]any{"id": "open-meteo", "key": "o", "label": "open Open-Meteo.com", "verb": "open", "target": "https://open-meteo.com/"},
		},
	}
}

// hourly is two rows, hours over temperatures, as many hours as fit width.
func (r *report) hourly(width int) []line {
	n := 6
	if width > 0 {
		n = min((width+hourWidth-3)/hourWidth, 12)
	}
	h := r.f.Hourly
	n = min(n, len(h.Time), len(h.Temp))
	if n <= 0 {
		return nil
	}
	var when, temps strings.Builder
	for i := range n {
		label := "now"
		if i > 0 {
			label = h.Time[i][min(11, len(h.Time[i])):min(13, len(h.Time[i]))]
		}
		t := "–"
		if h.Temp[i] != nil {
			t = temp(*h.Temp[i]) + "°"
		}
		when.WriteString(pad(label))
		temps.WriteString(pad(t))
	}
	return []line{
		{[]span{{Text: strings.TrimRight(when.String(), " "), Token: "dim"}}},
		{[]span{{Text: strings.TrimRight(temps.String(), " "), Token: "fg"}}},
	}
}

func pad(s string) string {
	return s + strings.Repeat(" ", max(0, hourWidth-utf8.RuneCountInString(s)))
}

// shown keeps the blocks that fit height lines, dropping the highest
// ranks first, in their order; height 0 keeps them all.
func shown(blocks []block, height int) []line {
	keep := make([]bool, len(blocks))
	used := 0
	for rank := 0; rank <= 3; rank++ {
		for i, b := range blocks {
			if b.rank == rank && (height <= 0 || used+len(b.lines) <= height) {
				keep[i] = true
				used += len(b.lines)
			}
		}
	}
	lines := []line{}
	for i, b := range blocks {
		if keep[i] {
			lines = append(lines, b.lines...)
		}
	}
	return lines
}

// fit joins parts with " · " on one line when it fits width, else gives
// each its own line.
func fit(width int, parts ...string) []line {
	joined := strings.Join(parts, " · ")
	if width <= 0 || utf8.RuneCountInString(joined) <= width {
		return []line{{[]span{{Text: joined, Token: "fg"}}}}
	}
	lines := make([]line, len(parts))
	for i, p := range parts {
		lines[i] = line{[]span{{Text: p, Token: "fg"}}}
	}
	return lines
}

func temp(v float64) string {
	r := math.Round(v)
	if r == 0 {
		r = 0 // no "-0"
	}
	return fmt.Sprintf("%.0f", r)
}

func first(vs []*float64) *float64 {
	if len(vs) == 0 {
		return nil
	}
	return vs[0]
}

func condition(code int) string {
	if c, ok := conditions[code]; ok {
		return c
	}
	return fmt.Sprintf("Weather code %d", code)
}

func windUnit(u string) string {
	switch u {
	case "", "km/h":
		return "km/h"
	case "mp/h":
		return "mph"
	}
	return u
}

func compass(deg float64) string {
	points := []string{"N", "NE", "E", "SE", "S", "SW", "W", "NW"}
	i := int(math.Round(math.Mod(math.Mod(deg, 360)+360, 360)/45)) % 8
	return points[i]
}
