package meetings_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/oronbz/visor/internal/hosttest"
	"github.com/oronbz/visor/internal/ports"
	"github.com/oronbz/visor/internal/theme"
	"github.com/oronbz/visor/internal/widget"
	"github.com/oronbz/visor/internal/widgets/meetings"
)

// at is a time on the harness's day, Sunday 18 October 2026, in UTC, which
// is the fake clock's zone.
func at(day, hour, minute int) time.Time {
	return time.Date(2026, 10, 18+day, hour, minute, 0, 0, time.UTC)
}

func event(id, title string, start time.Time, d time.Duration) ports.Event {
	return ports.Event{ID: id, Title: title, Start: start, End: start.Add(d)}
}

// day is a calendar around the harness's now, 14:05:09 on Sunday.
func day() []ports.Event {
	return []ports.Event{
		event("standup", "Standup", at(0, 9, 30), 15*time.Minute),
		event("review", "Design review", at(0, 13, 50), 45*time.Minute),
		event("dana", "1:1 with Dana", at(0, 14, 45), 30*time.Minute),
		event("planning", "Sprint planning", at(0, 16, 0), time.Hour),
		event("chapter", "iOS chapter", at(1, 11, 0), time.Hour),
		event("roadmap", "Roadmap review", at(2, 13, 0), 90*time.Minute),
	}
}

func granted(events []ports.Event) *hosttest.EventKit {
	return &hosttest.EventKit{
		Access:   map[ports.Entity]ports.Access{ports.Events: ports.AccessGranted},
		Calendar: events,
	}
}

func withMeetings(t *testing.T, ek *hosttest.EventKit, config string, w, h int) *hosttest.Host {
	t.Helper()
	if config == "" {
		config = "[[widget]]\ntype = \"meetings\"\n"
	}
	return hosttest.New(t, hosttest.Options{
		Width: w, Height: h,
		Types:    []widget.Type{meetings.Type},
		Config:   config,
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

func TestMeetingsGroupsTodayByDayWithANowLineLiveMarkerAndCountdowns(t *testing.T) {
	h := withMeetings(t, granted(day()), "", 60, 12)

	want := []string{
		"Today ──────────────────────────────────────────────────",
		"13:50 ● Design review                                now",
		"14:05 ──────────────────────────────────────────────────",
		"14:45   1:1 with Dana                             in 40m",
		"16:00   Sprint planning                        in 1h 55m",
		"Tomorrow ───────────────────────────────────────────────",
		"11:00   iOS chapter                                   1h",
		"Tue 20 Oct ─────────────────────────────────────────────",
		"13:00   Roadmap review                            1h 30m",
	}
	got := body(h)
	for i, line := range want {
		if i >= len(got) || got[i] != line {
			t.Fatalf("body line %d = %q, want %q\nscreen:\n%s", i, at2(got, i), line, h.Screen())
		}
	}
	if _, _, ok := h.Find("Standup"); ok {
		t.Errorf("a meeting that already ended is shown:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("Meetings 3"); !ok {
		t.Errorf("title doesn't count today's meetings still to come:\n%s", h.Screen())
	}

	x, y, _ := h.Find("14:05 ─")
	if tok, _ := h.Fg(x, y); tok != theme.Bad {
		t.Errorf("now-line is %v, want bad", tok)
	}
	x, y, _ = h.Find("●")
	if tok, _ := h.Fg(x, y); tok != theme.Ok {
		t.Errorf("live marker is %v, want ok", tok)
	}
}

func TestAShortWidgetShowsTodayAndATallerOneTheFollowingDays(t *testing.T) {
	h := withMeetings(t, granted(day()), "", 60, 8)
	if _, _, ok := h.Find("Tomorrow"); ok {
		t.Errorf("a 5-row Widget shows following days before today's meetings:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("1:1 with Dana"); !ok {
		t.Errorf("a 5-row Widget misses today's next meeting:\n%s", h.Screen())
	}
	h.Resize(60, 14)
	for _, s := range []string{"Tomorrow", "iOS chapter", "Tue 20 Oct", "Roadmap review"} {
		if _, _, ok := h.Find(s); !ok {
			t.Errorf("a taller Widget doesn't show %q:\n%s", s, h.Screen())
		}
	}
}

func TestDaysOneShowsOnlyToday(t *testing.T) {
	h := withMeetings(t, granted(day()), "[[widget]]\ntype = \"meetings\"\ndays = 1\n", 60, 20)
	for _, s := range []string{"Tomorrow", "iOS chapter", "Roadmap review"} {
		if _, _, ok := h.Find(s); ok {
			t.Errorf("days = 1 shows %q:\n%s", s, h.Screen())
		}
	}
	if _, _, ok := h.Find("Sprint planning"); !ok {
		t.Errorf("days = 1 misses today's meetings:\n%s", h.Screen())
	}
	if _, _, ok := h.Find("14:05 ─"); !ok {
		t.Errorf("the now-line goes missing after today's last meeting:\n%s", h.Screen())
	}
}

func TestEnterOpensTheSelectedEventInCalendarAndJSkipsTheNowLine(t *testing.T) {
	h := withMeetings(t, granted(day()), "", 60, 14)
	h.Press("1", "enter")
	h.Press("j", "enter")
	want := []string{
		"ical://ekevent/review?method=show&options=more",
		"ical://ekevent/dana?method=show&options=more",
	}
	if !slices.Equal(h.Opener.Opened, want) {
		t.Errorf("opened %q, want %q", h.Opener.Opened, want)
	}
	if _, _, ok := h.Find("⏎ open in Calendar"); !ok {
		t.Errorf("focused Meetings doesn't hint ⏎:\n%s", h.Screen())
	}
}

func TestUndeterminedAccessNeedsSetupAndEnterAsksForIt(t *testing.T) {
	ek := &hosttest.EventKit{
		Access:   map[ports.Entity]ports.Access{ports.Events: ports.AccessUndetermined},
		Granted:  true,
		Calendar: day(),
	}
	h := withMeetings(t, ek, "", 60, 14)
	for _, s := range []string{"needs setup", "Allow access to your calendars", "⏎ allow access"} {
		if _, _, ok := h.Find(s); !ok {
			t.Errorf("undetermined access doesn't show %q:\n%s", s, h.Screen())
		}
	}
	if len(ek.Requests) != 0 {
		t.Fatalf("access was requested before the user asked: %v", ek.Requests)
	}

	h.Press("1", "enter")
	if !slices.Equal(ek.Requests, []ports.Entity{ports.Events}) {
		t.Errorf("enter requested %v, want calendar access", ek.Requests)
	}
	if _, _, ok := h.Find("Design review"); !ok {
		t.Errorf("granting access doesn't show the meetings:\n%s", h.Screen())
	}
}

func TestDeniedAccessOpensTheCalendarsPrivacyPane(t *testing.T) {
	ek := &hosttest.EventKit{
		Access:   map[ports.Entity]ports.Access{ports.Events: ports.AccessUndetermined},
		Calendar: day(),
	}
	h := withMeetings(t, ek, "", 80, 14)
	h.Press("1", "enter")
	if _, _, ok := h.Find("Calendar access is off for this terminal"); !ok {
		t.Errorf("a denial doesn't point at the Privacy pane:\n%s", h.Screen())
	}

	h.Press("enter")
	want := []string{"x-apple.systempreferences:com.apple.preference.security?Privacy_Calendars"}
	if !slices.Equal(h.Opener.Opened, want) {
		t.Errorf("opened %q, want %q", h.Opener.Opened, want)
	}
	if len(ek.Requests) != 1 {
		t.Errorf("after a denial enter asked again: %v", ek.Requests)
	}
}

func TestMeetingsWithoutEventKitIsNotAvailable(t *testing.T) {
	h := withMeetings(t, &hosttest.EventKit{}, "", 60, 10)
	if _, _, ok := h.Find("Not available on this platform"); !ok {
		t.Errorf("no EventKit doesn't say so:\n%s", h.Screen())
	}
}

func TestAConfigNamingMeetingsWhereTheTypeIsMissingNeedsSetup(t *testing.T) {
	h := hosttest.New(t, hosttest.Options{
		Width: 60, Height: 10,
		Types:  []widget.Type{},
		Config: "[[widget]]\ntype = \"meetings\"\n",
	})
	for _, s := range []string{"needs setup", "not available on this platform"} {
		if _, _, ok := h.Find(s); !ok {
			t.Errorf("a config naming meetings without the type doesn't show %q:\n%s", s, h.Screen())
		}
	}
	if _, _, ok := h.Find("unknown Widget type"); ok {
		t.Errorf("meetings reads as a typo off macOS:\n%s", h.Screen())
	}
}

func TestAllDayEventsLeadTheDayAndDeclinedOrCanceledOnesAreLeftOut(t *testing.T) {
	evs := []ports.Event{
		event("review", "Design review", at(0, 13, 50), 45*time.Minute),
		{ID: "holiday", Title: "Sukkot", Start: at(0, 0, 0), End: at(1, 0, 0), AllDay: true},
		{ID: "golf", Title: "Golf", Start: at(0, 15, 0), End: at(0, 16, 0), Declined: true},
		{ID: "sync", Title: "Old sync", Start: at(0, 15, 30), End: at(0, 16, 0), Canceled: true},
		event("soon", "Coffee", at(0, 14, 15), 15*time.Minute),
	}
	h := withMeetings(t, granted(evs), "[[widget]]\ntype = \"meetings\"\ndays = 1\n", 50, 10)
	want := []string{
		"Today ────────────────────────────────────────",
		"all-day Sukkot",
		"13:50 ● Design review                      now",
		"14:05 ────────────────────────────────────────",
		"14:15   Coffee                          in 10m",
	}
	got := body(h)
	for i, line := range want {
		if i >= len(got) || got[i] != line {
			t.Fatalf("body line %d = %q, want %q\nscreen:\n%s", i, at2(got, i), line, h.Screen())
		}
	}
	if _, _, ok := h.Find("Meetings 2"); !ok {
		t.Errorf("the count includes all-day events:\n%s", h.Screen())
	}
	x, y, _ := h.Find("in 10m")
	if tok, _ := h.Fg(x, y); tok != theme.Warn {
		t.Errorf("a meeting 10 minutes away is %v, want warn", tok)
	}
}

func TestTwelveHourClockTimes(t *testing.T) {
	h := withMeetings(t, granted(day()), "time_format = \"12h\"\n[[widget]]\ntype = \"meetings\"\n", 60, 12)
	for _, s := range []string{"1:50 PM ● Design review", "2:05 PM ─", "4:00 PM   Sprint planning"} {
		if _, _, ok := h.Find(s); !ok {
			t.Errorf("12h clock doesn't show %q:\n%s", s, h.Screen())
		}
	}
}

func at2(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return "<missing>"
}
