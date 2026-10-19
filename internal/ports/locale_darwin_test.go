//go:build darwin && cgo

package ports

import (
	"slices"
	"testing"
)

func TestMacOSSaysItsClockAndFirstWeekday(t *testing.T) {
	loc := systemLocale()
	if loc.TimeFormat != "12h" && loc.TimeFormat != "24h" {
		t.Errorf("TimeFormat %q, want 12h or 24h", loc.TimeFormat)
	}
	if !slices.Contains(weekdays, loc.WeekStart) {
		t.Errorf("WeekStart %q, want a weekday", loc.WeekStart)
	}
}
