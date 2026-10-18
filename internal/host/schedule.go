package host

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/oronbz/ghud/internal/widget"
)

const (
	// firstBackoff is the wait after a first failure; each further one doubles it, up to maxBackoff.
	firstBackoff = 15 * time.Second
	maxBackoff   = 15 * time.Minute
	// beat is how often the host checks the wall clock for a jump, such as a wake from sleep.
	beat = 10 * time.Second
	// wakeSlack is how late a timer may fire before the host takes it for a wake from sleep.
	wakeSlack = 30 * time.Second
	// stagger spaces out the refreshes of Widgets that fell due during a sleep.
	stagger = 3 * time.Second
	// jitter is the most a fetching Widget's schedule moves either way, as a share of it.
	jitter = 0.1
)

// sched is a Source's refresh schedule and the outcome of its last refresh,
// from which its Status follows.
type sched struct {
	// interval is how often it refreshes; zero means only on demand (and at
	// midnight for types that ask).
	interval time.Duration
	// due is when the next refresh is planned; zero is none.
	due time.Time
	// timer tells the armed timer from ones a newer plan replaced.
	timer int
	// busy is a refresh in flight; again asks for another once it lands.
	busy, again bool
	// fails counts failures since the last success, for the backoff.
	fails int
	// fetched is when the data on show was fetched; zero is no data yet.
	fetched time.Time
	// err is why the last refresh failed, nil after a success.
	err   error
	setup *widget.SetupError
}

type refreshedMsg struct {
	id      string
	payload widget.Payload
	err     error
}

// dueMsg is Widget id's refresh timer firing at at; want is when it was meant to.
type dueMsg struct {
	id       string
	seq      int
	want, at time.Time
}

// beatMsg is the wall-clock check firing.
type beatMsg struct{ want, at time.Time }

// spinMsg advances the refresh spinners.
type spinMsg struct{ at time.Time }

// startMsg starts the first refresh of every visible Source.
type startMsg struct{}

// spinEvery is the spinner's frame time.
const spinEvery = 100 * time.Millisecond

var spinFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

func (m Model) spinner() string {
	return spinFrames[int(m.now.UnixMilli()/spinEvery.Milliseconds())%len(spinFrames)]
}

func (m Model) source(id string) (widget.Source, bool) {
	src, ok := m.widgets[id].widget.(widget.Source)
	return src, ok
}

// start refreshes Source id now, whatever its schedule; a refresh already
// in flight is followed by another.
func (m Model) start(id string) (Model, tea.Cmd) {
	src, ok := m.source(id)
	if !ok {
		return m, nil
	}
	s := m.sched[id]
	if s.busy {
		s.again = true
		m.sched[id] = s
		return m, nil
	}
	s.busy, s.due = true, time.Time{}
	s.timer++
	m.sched[id] = s
	now := m.opts.Ports.Clock.Now()
	fetch := func() tea.Msg {
		p, err := src.Refresh(context.Background(), now)
		return refreshedMsg{id: id, payload: p, err: err}
	}
	if m.spinning {
		return m, fetch
	}
	m.spinning = true
	return m, tea.Batch(fetch, m.opts.Ports.Clock.After(spinEvery, func(t time.Time) tea.Msg { return spinMsg{t} }))
}

// startAll refreshes every visible Source now, except those waiting out a rate limit.
func (m Model) startAll() (Model, tea.Cmd) {
	now := m.opts.Ports.Clock.Now()
	var cmds []tea.Cmd
	for _, id := range m.ids() {
		if _, ok := m.source(id); !ok || m.layout.IsHidden(id) || m.pausedUntil(id).After(now) {
			continue
		}
		var cmd tea.Cmd
		m, cmd = m.start(id)
		cmds = append(cmds, cmd)
	}
	return m, tea.Batch(cmds...)
}

func (m Model) spun(msg spinMsg) (Model, tea.Cmd) {
	m.now = msg.at
	for _, s := range m.sched {
		if s.busy {
			return m, m.opts.Ports.Clock.After(spinEvery, func(t time.Time) tea.Msg { return spinMsg{t} })
		}
	}
	m.spinning = false
	return m, nil
}

// plan arms Source id's next refresh for at.
func (m Model) plan(id string, at time.Time) tea.Cmd {
	s := m.sched[id]
	s.due = at
	s.timer++
	m.sched[id] = s
	seq := s.timer
	return m.opts.Ports.Clock.After(at.Sub(m.opts.Ports.Clock.Now()), func(t time.Time) tea.Msg {
		return dueMsg{id: id, seq: seq, want: at, at: t}
	})
}

func (m Model) due(msg dueMsg) (Model, tea.Cmd) {
	m.now = msg.at
	if m.sched[msg.id].timer != msg.seq {
		return m, nil
	}
	if msg.at.Sub(msg.want) > wakeSlack {
		return m.woke()
	}
	// A hidden Widget stays overdue, and refreshes once it's shown.
	if m.layout.IsHidden(msg.id) {
		return m, nil
	}
	return m.start(msg.id)
}

// shown refreshes Source id if it fell due while hidden.
func (m Model) shown(id string) (Model, tea.Cmd) {
	s, ok := m.sched[id]
	if !ok || s.busy || !s.fetched.IsZero() && (s.due.IsZero() || s.due.After(m.opts.Ports.Clock.Now())) {
		return m, nil
	}
	return m.start(id)
}

// beat arms the next wall-clock check, when anything is scheduled.
func (m Model) beat(now time.Time) tea.Cmd {
	for id, s := range m.sched {
		if s.interval > 0 || m.widgets[id].typ.Midnight {
			want := now.Add(beat)
			return m.opts.Ports.Clock.After(beat, func(t time.Time) tea.Msg { return beatMsg{want: want, at: t} })
		}
	}
	return nil
}

func (m Model) beaten(msg beatMsg) (Model, tea.Cmd) {
	m.now = msg.at
	if msg.at.Sub(msg.want) <= wakeSlack {
		return m, m.beat(msg.at)
	}
	m, cmd := m.woke()
	return m, tea.Batch(cmd, m.beat(msg.at))
}

// woke handles a jump of the wall clock, such as a wake from sleep: every
// Widget that fell due meanwhile refreshes once, a few seconds apart, and
// the others keep their plans.
func (m Model) woke() (Model, tea.Cmd) {
	now := m.opts.Ports.Clock.Now()
	var overdue []string
	var cmds []tea.Cmd
	for _, id := range m.ids() {
		s := m.sched[id]
		switch {
		case s.busy || s.due.IsZero():
		case !s.due.After(now) && !m.layout.IsHidden(id):
			overdue = append(overdue, id)
		default:
			cmds = append(cmds, m.plan(id, s.due))
		}
	}
	slices.SortStableFunc(overdue, func(a, b string) int { return m.sched[a].due.Compare(m.sched[b].due) })
	for i, id := range overdue {
		if i == 0 {
			var cmd tea.Cmd
			m, cmd = m.start(id)
			cmds = append(cmds, cmd)
			continue
		}
		cmds = append(cmds, m.plan(id, now.Add(time.Duration(i)*stagger)))
	}
	return m, tea.Batch(cmds...)
}

// refreshed records a refresh's outcome and plans the next one.
func (m Model) refreshed(msg refreshedMsg) (Model, tea.Cmd) {
	now := m.opts.Ports.Clock.Now()
	m.now = now
	s := m.sched[msg.id]
	s.busy = false
	var setup *widget.SetupError
	var limited *widget.RateLimitError
	switch {
	case msg.err == nil:
		s.fetched, s.err, s.fails, s.setup = now, nil, 0, nil
		m.fetched[msg.id] = msg.payload
		st := m.lists[msg.id]
		if i := slices.IndexFunc(msg.payload.Items, func(it widget.Item) bool { return it.ID == st.sel }); i >= 0 {
			st.idx = i
		}
		m.lists[msg.id] = st
	case errors.As(msg.err, &setup):
		s.err, s.fails, s.setup = nil, 0, setup
	case errors.As(msg.err, &limited):
		s.err, s.setup = msg.err, nil
		if c, ok := m.widgets[msg.id].widget.(widget.Credentialed); ok && limited.Until.After(m.paused[c.Credential()]) {
			m.paused[c.Credential()] = limited.Until
		}
	default:
		s.err, s.setup = msg.err, nil
		s.fails++
	}
	m.sched[msg.id] = s
	m.clampList(msg.id)
	if s.again {
		s.again = false
		m.sched[msg.id] = s
		return m.start(msg.id)
	}
	var cmds []tea.Cmd
	if at := m.next(msg.id, now); !at.IsZero() {
		cmds = append(cmds, m.plan(msg.id, at))
	}
	if limited != nil {
		// The pause covers every Widget on the same credential.
		for _, id := range m.ids() {
			if o := m.sched[id]; id != msg.id && !o.due.IsZero() && o.due.Before(m.pausedUntil(id)) {
				cmds = append(cmds, m.plan(id, m.pausedUntil(id)))
			}
		}
	}
	return m, tea.Batch(cmds...)
}

// next is when Source id refreshes after a refresh at now: an interval on,
// sooner after a failure, never before a rate limit lifts, and at midnight
// for types that ask.
func (m Model) next(id string, now time.Time) time.Time {
	s, typ := m.sched[id], m.widgets[id].typ
	var at time.Time
	switch {
	case s.err != nil:
		at = now.Add(m.jittered(typ, backoff(s.fails)))
	case s.interval > 0:
		at = now.Add(m.jittered(typ, s.interval))
	}
	if typ.Midnight {
		y, mo, d := now.Date()
		if mid := time.Date(y, mo, d+1, 0, 0, 0, 0, now.Location()); at.IsZero() || mid.Before(at) {
			at = mid
		}
	}
	if p := m.pausedUntil(id); !at.IsZero() && p.After(at) {
		at = p
	}
	return at
}

// backoff is the wait after the nth failure in a row: 15s doubling up to 15 minutes.
func backoff(fails int) time.Duration {
	d := firstBackoff
	for i := 1; i < fails && d < maxBackoff; i++ {
		d *= 2
	}
	return min(d, maxBackoff)
}

// jittered moves d by up to ±10% for fetching Widgets, so instances and
// Widgets started together drift apart.
func (m Model) jittered(typ widget.Type, d time.Duration) time.Duration {
	if !typ.Fetches {
		return d
	}
	return time.Duration(float64(d) * (1 - jitter + 2*jitter*m.rng.Float64()))
}

// pausedUntil is when the rate limit on Source id's credential lifts.
func (m Model) pausedUntil(id string) time.Time {
	if c, ok := m.widgets[id].widget.(widget.Credentialed); ok {
		return m.paused[c.Credential()]
	}
	return time.Time{}
}

func newRand(now time.Time) *rand.Rand {
	return rand.New(rand.NewPCG(uint64(now.UnixNano()), 0x9e3779b97f4a7c15))
}

// interval is how often Source p refreshes: its `refresh` setting, else its
// own interval, else its type's, clamped to the type's floor; problem says
// when it was clamped.
func interval(p placed, refresh time.Duration) (d time.Duration, problem string) {
	d = p.typ.Refresh
	if s, ok := p.widget.(widget.Scheduled); ok && s.Interval() > 0 {
		d = s.Interval()
	}
	if refresh > 0 {
		d = refresh
	}
	if d > 0 && d < p.typ.Floor {
		return p.typ.Floor, fmt.Sprintf("%s: refresh %s is below the %s minimum; using %s", p.title, short(d), p.typ.Name, short(p.typ.Floor))
	}
	return d, ""
}

// short writes a duration the way config.toml does, such as "30s" or "1h30m".
func short(d time.Duration) string {
	s := d.String()
	if strings.HasSuffix(s, "m0s") {
		s = s[:len(s)-2]
	}
	if strings.HasSuffix(s, "h0m") {
		s = s[:len(s)-2]
	}
	return s
}
