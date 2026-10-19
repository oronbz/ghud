package config_test

import (
	"testing"

	"github.com/oronbz/visor/internal/config"
)

func TestDetectLocalePrefersLCTimeThenTheSystemThenLANG(t *testing.T) {
	israel := config.Locale{TimeFormat: "24h", WeekStart: "sunday"}
	for _, tc := range []struct {
		name   string
		env    map[string]string
		system config.Locale
		want   config.Locale
	}{
		{"LANG alone", map[string]string{"LANG": "en_US.UTF-8"}, config.Locale{}, config.Locale{TimeFormat: "12h", WeekStart: "sunday"}},
		{"the system beats a terminal's LANG", map[string]string{"LANG": "en_US.UTF-8"}, israel, israel},
		{"LC_TIME beats the system", map[string]string{"LANG": "en_US.UTF-8", "LC_TIME": "de_DE.UTF-8"}, israel, config.Locale{TimeFormat: "24h", WeekStart: "monday"}},
		{"LC_ALL beats the system", map[string]string{"LC_ALL": "en_US.UTF-8"}, israel, config.Locale{TimeFormat: "12h", WeekStart: "sunday"}},
		{"a system that says only the clock", map[string]string{"LANG": "de_DE.UTF-8"}, config.Locale{TimeFormat: "12h"}, config.Locale{TimeFormat: "12h", WeekStart: "monday"}},
		{"nothing set", nil, config.Locale{}, config.Locale{TimeFormat: "24h", WeekStart: "monday"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := config.DetectLocale(func(k string) string { return tc.env[k] }, tc.system); got != tc.want {
				t.Errorf("DetectLocale = %+v, want %+v", got, tc.want)
			}
		})
	}
}
