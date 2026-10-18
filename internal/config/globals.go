package config

import (
	"slices"
	"strings"

	"github.com/oronbz/ghud/internal/widget"
)

// Locale holds the defaults for options that follow the system locale.
type Locale struct {
	TimeFormat string
	WeekStart  string
}

var weekdays = []string{"sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"}

// Globals is the schema of config.toml's top-level options.
func Globals(loc Locale) []widget.Setting {
	if loc.TimeFormat == "" {
		loc.TimeFormat = "24h"
	}
	if loc.WeekStart == "" {
		loc.WeekStart = "monday"
	}
	return []widget.Setting{
		{
			Name: "theme", Kind: widget.String, Default: "ghud", Choices: []string{"ghud", "terminal"},
			Description: `Colour theme: "ghud" is the built-in palette, "terminal" derives colours from your terminal's.`,
			Why:         `Pick "terminal" to match a terminal colour scheme you already like.`,
		},
		{
			Name: "time_format", Kind: widget.String, Default: loc.TimeFormat, Choices: []string{"12h", "24h"},
			Description: "Clock style. Defaults from your locale.",
			Why:         "Set it when your locale's default isn't how you read the time.",
		},
		{
			Name: "week_start", Kind: widget.String, Default: loc.WeekStart, Choices: weekdays,
			Description: "First day of the week in the Calendar. Defaults from your locale.",
			Why:         "Set it when your locale starts the week on a different day than you do.",
		},
		{
			Name: "mouse", Kind: widget.Bool, Default: true,
			Description: "Capture the mouse for clicks, drags and the wheel.",
			Why:         "Turn it off to get the terminal's own text selection back.",
		},
	}
}

// LocaleFromEnv reads the region from LC_ALL, LC_TIME or LANG ("en_US.UTF-8")
// and picks its clock style and first weekday, following CLDR. Unknown
// regions get 24h and Monday.
func LocaleFromEnv(getenv func(string) string) Locale {
	var tag string
	for _, k := range []string{"LC_ALL", "LC_TIME", "LANG"} {
		if tag = getenv(k); tag != "" {
			break
		}
	}
	tag, _, _ = strings.Cut(tag, ".")
	tag, _, _ = strings.Cut(tag, "@")
	_, region, _ := strings.Cut(strings.ReplaceAll(tag, "-", "_"), "_")
	region = strings.ToUpper(region)

	loc := Locale{TimeFormat: "24h", WeekStart: "monday"}
	if slices.Contains(twelveHour, region) {
		loc.TimeFormat = "12h"
	}
	switch {
	case slices.Contains(sundayFirst, region):
		loc.WeekStart = "sunday"
	case slices.Contains(saturdayFirst, region):
		loc.WeekStart = "saturday"
	}
	return loc
}

var twelveHour = []string{"AU", "BD", "CA", "CO", "EG", "IN", "JO", "KR", "MY", "NZ", "PH", "PK", "SA", "TW", "US"}

var sundayFirst = []string{
	"AG", "AS", "BR", "BS", "BT", "BW", "BZ", "CA", "CO", "DM", "DO", "ET", "GT", "GU", "HK", "HN", "ID", "IL",
	"IN", "JM", "JP", "KE", "KH", "KR", "LA", "MH", "MM", "MO", "MT", "MX", "MZ", "NI", "NP", "PA", "PE", "PH",
	"PK", "PR", "PT", "PY", "SA", "SG", "SV", "TH", "TT", "TW", "UM", "US", "VE", "VI", "WS", "YE", "ZA", "ZW",
}

var saturdayFirst = []string{"AE", "AF", "BH", "DJ", "DZ", "EG", "IQ", "IR", "JO", "KW", "LY", "OM", "QA", "SD", "SY"}
