package reminders_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
	"github.com/oronbz/visor/internal/widgets/reminders"
)

// on is a day around the harness's now, 14:05:09 on Sunday 18 October 2026
// UTC, the fake clock's zone.
func on(day, hour, minute int) time.Time {
	return time.Date(2026, 10, 18+day, hour, minute, 0, 0, time.UTC)
}

func list() []ports.Reminder {
	return []ports.Reminder{
		{ID: "milk", Title: "Buy milk", List: "Groceries"},
		{ID: "passport", Title: "Renew passport", List: "Personal", Due: on(2, 0, 0), DateOnly: true},
		{ID: "dentist", Title: "Call the dentist", List: "Personal", Due: on(0, 16, 30)},
		{ID: "taxes", Title: "File taxes", List: "Personal", Due: on(-3, 0, 0), DateOnly: true},
		{ID: "plants", Title: "Water the plants", List: "Home", Due: on(0, 0, 0), DateOnly: true},
		{ID: "bins", Title: "Take the bins out", List: "Home", Due: on(0, 9, 0)},
	}
}

func granted(open []ports.Reminder) *hosttest.EventKit {
	return &hosttest.EventKit{
		Access: map[ports.Entity]ports.Access{ports.Reminders: ports.AccessGranted},
		Open:   open,
	}
}

func withReminders(t *testing.T, ek *hosttest.EventKit, w, h int) *hosttest.Host {
	t.Helper()
	return hosttest.New(t, hosttest.Options{
		Width: w, Height: h,
		Types:    []widget.Type{reminders.Type},
		Config:   "[[widget]]\ntype = \"reminders\"\n",
		EventKit: ek,
	})
}

// body is the tile's inner lines, border and footer left out.
func body(h *hosttest.Host) []string {
	lines := strings.Split(h.Screen(), "\n")
	var out []string
	for _, l := range lines[1 : len(lines)-2] {
		l = strings.TrimSuffix(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(l), "│")), "│")
		out = append(out, strings.TrimSpace(l))
	}
	return out
}

func TestRemindersDueTodayOrOverdueAreListedByDueDate(t *testing.T) {
	h := withReminders(t, granted(list()), 60, 10)
	want := []string{
		"○ File taxes                            Personal overdue",
		"○ Water the plants                      Home       today",
		"○ Take the bins out                     Home     overdue",
		"○ Call the dentist                      Personal   16:30",
		"",
	}
	got := body(h)
	for i, line := range want {
		if i >= len(got) || got[i] != line {
			t.Fatalf("body line %d = %q, want %q\nscreen:\n%s", i, at(got, i), line, h.Screen())
		}
	}
	for _, later := range []string{"Renew passport", "Buy milk"} {
		if _, _, ok := h.Find(later); ok {
			t.Errorf("%q isn't due today but is listed:\n%s", later, h.Screen())
		}
	}
	if _, _, ok := h.Find("Reminders 4"); !ok {
		t.Errorf("title doesn't count the reminders due today:\n%s", h.Screen())
	}
	x, y, _ := h.Find("overdue")
	if tok, _ := h.Fg(x, y); tok != theme.Bad {
		t.Errorf("an overdue reminder is %v, want bad", tok)
	}
	x, y, _ = h.Find("16:30")
	if tok, _ := h.Fg(x, y); tok != theme.Warn {
		t.Errorf("a reminder due later today is %v, want warn", tok)
	}
}

func TestSpaceCompletesTheSelectedReminderToastsAndRefreshes(t *testing.T) {
	ek := granted(list())
	h := withReminders(t, ek, 60, 10)
	h.Press("1", "space")
	if !slices.Equal(ek.Completed, []string{"taxes"}) {
		t.Fatalf("completed %q, want the selected reminder", ek.Completed)
	}
	if _, _, ok := h.Find("Completed “File taxes”"); !ok {
		t.Errorf("completing doesn't toast:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("○ File taxes"); ok {
		t.Errorf("a completed reminder is still listed:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("Reminders 3"); !ok {
		t.Errorf("the count doesn't drop after completing:\n%s", h.Screen())
	}
}

func TestEnterOpensTheSelectedReminderInRemindersApp(t *testing.T) {
	h := withReminders(t, granted(list()), 60, 10)
	h.Press("1", "j", "j", "j", "enter")
	want := []string{"x-apple-reminderkit://REMCDReminder/dentist"}
	if !slices.Equal(h.Opener.Opened, want) {
		t.Errorf("opened %q, want %q", h.Opener.Opened, want)
	}
	for _, s := range []string{"␣ complete", "⏎ open in Reminders"} {
		if _, _, ok := h.Find(s); !ok {
			t.Errorf("focused Reminders doesn't hint %q:\n%s", s, h.Screen())
		}
	}
}

func TestOneListLeavesTheListColumnOut(t *testing.T) {
	h := withReminders(t, granted([]ports.Reminder{
		{ID: "a", Title: "Water the plants", List: "Home", Due: on(0, 0, 0), DateOnly: true},
		{ID: "b", Title: "Fix the tap", List: "Home", Due: on(-1, 0, 0), DateOnly: true},
	}), 50, 8)
	want := []string{
		"○ Fix the tap                          overdue",
		"○ Water the plants                       today",
	}
	got := body(h)
	for i, line := range want {
		if i >= len(got) || got[i] != line {
			t.Fatalf("body line %d = %q, want %q\nscreen:\n%s", i, at(got, i), line, h.Screen())
		}
	}
}

func TestUndeterminedAccessNeedsSetupAndEnterAsksForIt(t *testing.T) {
	ek := &hosttest.EventKit{
		Access:  map[ports.Entity]ports.Access{ports.Reminders: ports.AccessUndetermined},
		Granted: true,
		Open:    list(),
	}
	h := withReminders(t, ek, 60, 10)
	for _, s := range []string{"needs setup", "Allow access to your reminders", "⏎ allow access"} {
		if _, _, ok := h.Find(s); !ok {
			t.Errorf("undetermined access doesn't show %q:\n%s", s, h.Screen())
		}
	}
	if len(ek.Requests) != 0 {
		t.Fatalf("access was requested before the user asked: %v", ek.Requests)
	}

	h.Press("1", "enter")
	if !slices.Equal(ek.Requests, []ports.Entity{ports.Reminders}) {
		t.Errorf("enter requested %v, want reminders access", ek.Requests)
	}
	if _, _, ok := h.Find("File taxes"); !ok {
		t.Errorf("granting access doesn't show the reminders:\n%s", h.Screen())
	}
}

func TestDeniedAccessOpensTheRemindersPrivacyPane(t *testing.T) {
	ek := &hosttest.EventKit{
		Access: map[ports.Entity]ports.Access{ports.Reminders: ports.AccessDenied},
		Open:   list(),
	}
	h := withReminders(t, ek, 80, 10)
	if _, _, ok := h.Find("Reminders access is off for this terminal"); !ok {
		t.Errorf("a denial doesn't point at the Privacy pane:\n%s", h.Screen())
	}
	h.Press("1", "enter")
	want := []string{"x-apple.systempreferences:com.apple.preference.security?Privacy_Reminders"}
	if !slices.Equal(h.Opener.Opened, want) {
		t.Errorf("opened %q, want %q", h.Opener.Opened, want)
	}
	if len(ek.Requests) != 0 {
		t.Errorf("a denial asked again: %v", ek.Requests)
	}
}

func at(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<missing>"
}

func TestAMultilineReminderStaysOnItsRow(t *testing.T) {
	h := withReminders(t, granted([]ports.Reminder{
		{ID: "a", Title: "ClaudeDevs (@ClaudeDevs)\n1.1K likes · 71 replies", List: "Home", Due: on(0, 0, 0), DateOnly: true},
		{ID: "b", Title: "Fix the tap", List: "Home", Due: on(0, 0, 0), DateOnly: true},
	}), 70, 8)
	want := []string{
		"○ ClaudeDevs (@ClaudeDevs) 1.1K likes · 71 replies           today",
		"○ Fix the tap                                                today",
	}
	got := body(h)
	for i, line := range want {
		if i >= len(got) || got[i] != line {
			t.Fatalf("body line %d = %q, want %q\nscreen:\n%s", i, at(got, i), line, h.Screen())
		}
	}
}

func TestNothingDueTodaySaysSo(t *testing.T) {
	h := withReminders(t, granted([]ports.Reminder{
		{ID: "a", Title: "Buy milk", List: "Home"},
		{ID: "b", Title: "Renew passport", List: "Home", Due: on(3, 0, 0), DateOnly: true},
	}), 50, 8)
	if _, _, ok := h.Find("Nothing due today"); !ok {
		t.Errorf("no empty message:\n%s", h.Screen())
	}
}
