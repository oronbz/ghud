package host

import (
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/oronbz/ghud/internal/render"
	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

// health is a Widget's Status, drawn the same way for every Widget.
type health int

const (
	healthy health = iota
	// loading has no data yet.
	loading
	// stale shows data older than twice the interval, or kept after a failed refresh.
	stale
	// failed has no data to show, only why.
	failed
	needsSetup
)

// health is Widget id's Status now.
func (m Model) health(id string) health {
	if m.widgets[id].setup != "" {
		return needsSetup
	}
	s, ok := m.sched[id]
	switch {
	case !ok:
		return healthy
	case s.setup != nil:
		return needsSetup
	case s.fetched.IsZero() && s.err != nil:
		return failed
	case s.fetched.IsZero():
		return loading
	case s.err != nil || s.interval > 0 && m.now.Sub(s.fetched) > 2*s.interval:
		return stale
	}
	return healthy
}

// chrome is the Status drawn in Widget id's top border: a spinner while it
// refreshes, reason and age while it's stale, and the age of data read
// from the cache until a refresh replaces it.
func (m Model) chrome(id string) render.Status {
	var st render.Status
	s := m.sched[id]
	if s.busy {
		st.Spinner = m.spinner()
	}
	switch {
	case m.health(id) == stale:
		reason := "stale"
		if s.err != nil {
			reason = reasonOf(s.err)
		}
		st.Stale = reason + " · " + age(m.now.Sub(s.fetched))
	case s.cached:
		st.Age = age(m.now.Sub(s.fetched))
	}
	return st
}

// statusPayload is what a Widget shows in its body instead of data: needs
// setup with its hint and fix, an error with its hint, or a loading spinner.
func (m Model) statusPayload(id string) (widget.Payload, bool) {
	s := m.sched[id]
	switch m.health(id) {
	case needsSetup:
		hint := m.widgets[id].setup
		lines := []widget.Line{widget.Text(theme.Warn, "needs setup")}
		if s.setup != nil {
			hint = s.setup.Hint
		}
		lines = append(lines, widget.Text(theme.Dim, hint))
		if s.setup != nil && s.setup.Action != nil {
			lines = append(lines, widget.Line{}, widget.Line{Spans: []widget.Span{
				{Text: "⏎ ", Token: theme.Accent, Bold: true}, {Text: s.setup.Action.Label, Token: theme.Dim},
			}})
		}
		return widget.Payload{Center: true, Lines: lines}, true
	case failed:
		lines := []widget.Line{widget.Text(theme.Bad, s.err.Error())}
		if h := hintOf(s.err); h != "" {
			lines = append(lines, widget.Text(theme.Dim, h))
		}
		return widget.Payload{Wrap: true, Lines: lines}, true
	case loading:
		return widget.Payload{Center: true, Lines: []widget.Line{widget.Text(theme.Accent, m.spinner())}}, true
	}
	return widget.Payload{}, false
}

// staleError is the full error behind focused Widget id's stale marker, or "".
func (m Model) staleError(id string) string {
	if s := m.sched[id]; m.health(id) == stale && s.err != nil {
		return m.widgets[id].title + ": " + s.err.Error()
	}
	return ""
}

// reasonOf is the few words on a stale marker: the error's own Reason,
// offline for network failures, else its first line.
func reasonOf(err error) string {
	var r interface{ Reason() string }
	if errors.As(err, &r) {
		return r.Reason()
	}
	var netErr net.Error
	var dns *net.DNSError
	var op *net.OpError
	if errors.As(err, &dns) || errors.As(err, &op) || errors.As(err, &netErr) && netErr.Timeout() {
		return "offline"
	}
	line, _, _ := strings.Cut(err.Error(), "\n")
	return line
}

func hintOf(err error) string {
	var h interface{ Hint() string }
	if errors.As(err, &h) {
		return h.Hint()
	}
	return ""
}

// age is a duration in its largest whole unit, such as 45s, 12m, 3h or 2d.
func age(d time.Duration) string {
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%dm", int(d.Minutes()))
	case d < 24*time.Hour:
		return fmt.Sprintf("%dh", int(d.Hours()))
	}
	return fmt.Sprintf("%dd", int(d.Hours()/24))
}
