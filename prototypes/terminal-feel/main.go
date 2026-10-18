// PROTOTYPE, throwaway. Terminal feel spike for ghud (issue #11).
// Answers: does the approved Panels design hold up in Ghostty (and inside
// herdr) on Go + Bubble Tea v2? Run: go run . (from this directory).
package main

import (
	"fmt"
	"image"
	"image/color"
	"math"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"time"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"
)

var (
	cBg     = lipgloss.Color("#13171d")
	cSel    = lipgloss.Color("#222a36")
	cLine   = lipgloss.Color("#3d4757")
	cFaint  = lipgloss.Color("#4e5766")
	cDim    = lipgloss.Color("#7e8797")
	cFg     = lipgloss.Color("#d3d9e2")
	cBright = lipgloss.Color("#f5f7fa")
	cAccent = lipgloss.Color("#8aa9ff")
	cWarn   = lipgloss.Color("#e4ae5c")
	cOk     = lipgloss.Color("#86c497")
	cBad    = lipgloss.Color("#ef7d7d")
	cInfo   = lipgloss.Color("#b59dff")
	cTeam   = lipgloss.Color("#6cc3d8")
)

func fg(c color.Color) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(c)
}

type cell struct {
	id     string
	weight float64
}

type row struct {
	weight float64
	cells  []cell
}

type rect struct{ x, y, w, h int }

func (r rect) in(x, y int) bool { return x >= r.x && x < r.x+r.w && y >= r.y && y < r.y+r.h }

type pr struct {
	ci, repo, title, author, why, age string
	add, del                          int
	handled                           string
}

var prs = []pr{
	{"✓", "ghud#41", "Host-drawn Items with column priorities", "maya", "@you", "2h", 412, 88, ""},
	{"●", "nag#12", "Complete reminders through go-eventkit", "eli", "team", "5h", 37, 9, ""},
	{"✗", "infra#903", "Bump goreleaser to v3 and sign with quill", "noa", "owner", "1d", 120, 140, ""},
	{"✓", "ghud#44", "Arrange mode: push past an edge opens a new row", "tal", "@you", "1d", 230, 51, ""},
	{"●", "api#2210", "Paginate /search/jql with nextPageToken", "dan", "team", "2d", 64, 12, ""},
	{"✓", "web#77", "Drop the legacy OAuth callback", "ron", "team", "3d", 18, 302, "maya"},
	{"✓", "api#2198", "Retry 429 from Atlassian with backoff", "lior", "team", "4d", 22, 4, "eli"},
}

type dragKind int

const (
	dragNone dragKind = iota
	dragMove
	dragVBorder
	dragHBorder
)

type drag struct {
	kind         dragKind
	id           string
	row, idx     int
	grabX, grabY int
	px, py       int
	orig         []row
	targetID     string
	side         string
}

type evt struct {
	at   time.Time
	text string
}

type model struct {
	w, h        int
	rows        []row
	focus       string
	sel         map[string]int
	scroll      map[string]int
	hover       string
	drag        drag
	arrange     []row
	mode        string
	note        textarea.Model
	noteText    string
	held        map[string]bool
	kbFlags     int
	kbSeen      bool
	log         []evt
	lastMouse   string
	lastHit     string
	toast       string
	toastAt     time.Time
	cpu         []float64
	mem         float64
	frames      int
	fps         float64
	fpsAt       time.Time
	lastClick   time.Time
	lastClickAt string
	lastFocus   string
}

type tickMsg time.Time
type editorDoneMsg struct {
	err  error
	text string
	took time.Duration
}

func initialModel() model {
	ta := textarea.New()
	ta.ShowLineNumbers = false
	ta.Prompt = ""
	m := model{
		rows: []row{
			{weight: 3, cells: []cell{{"prs", 2.2}, {"clock", 1}}},
			{weight: 2, cells: []cell{{"notes", 1}, {"cpu", 1}, {"probe", 1.3}}},
		},
		lastFocus: "prs",
		sel:       map[string]int{},
		scroll:    map[string]int{},
		held:      map[string]bool{},
		mode:      "NORMAL",
		note:      ta,
		noteText:  "# Scratch\n- ask maya about column priorities\n- E opens $EDITOR, i edits inline\n- drag a title, drag a border",
		fpsAt:     time.Now(),
	}
	for range 120 {
		m.cpu = append(m.cpu, 0.3)
	}
	return m
}

func tick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (m model) Init() tea.Cmd { return tick() }

func (m *model) logf(format string, a ...any) {
	m.log = append(m.log, evt{time.Now(), fmt.Sprintf(format, a...)})
	if len(m.log) > 40 {
		m.log = m.log[len(m.log)-40:]
	}
}

func (m *model) say(s string) { m.toast, m.toastAt = s, time.Now() }

// split distributes total cells across weights with largest-remainder rounding.
func split(total int, weights []float64, min int) []int {
	sum := 0.0
	for _, w := range weights {
		sum += w
	}
	out := make([]int, len(weights))
	rem := make([]float64, len(weights))
	used := 0
	for i, w := range weights {
		f := float64(total) * w / sum
		out[i] = int(f)
		rem[i] = f - float64(out[i])
		used += out[i]
	}
	for used < total {
		best := 0
		for i := range rem {
			if rem[i] > rem[best] {
				best = i
			}
		}
		out[best]++
		rem[best] = -1
		used++
	}
	for i := range out {
		if out[i] < min {
			d := min - out[i]
			out[i] = min
			for j := range out {
				if j != i && out[j]-d >= min {
					out[j] -= d
					break
				}
			}
		}
	}
	return out
}

func layout(rows []row, w, h int) map[string]rect {
	rects := map[string]rect{}
	var rw []float64
	for _, r := range rows {
		rw = append(rw, r.weight)
	}
	hs := split(h, rw, 4)
	y := 0
	for i, r := range rows {
		var cw []float64
		for _, c := range r.cells {
			cw = append(cw, c.weight)
		}
		ws := split(w, cw, 12)
		x := 0
		for j, c := range r.cells {
			rects[c.id] = rect{x, y, ws[j], hs[i]}
			x += ws[j]
		}
		y += hs[i]
	}
	return rects
}

func blank(w, h int) string {
	line := strings.Repeat(" ", max(w, 1))
	return strings.TrimSuffix(strings.Repeat(line+"\n", max(h, 1)), "\n")
}

// hitZones builds a hit-test-only compositor: zones are never drawn, they
// exist so Compositor.Hit can resolve titles and shared borders above panels.
func (m model) hitZones() *lipgloss.Compositor {
	rects := layout(m.rows, m.w, m.h-1)
	var layers []*lipgloss.Layer
	for ri, r := range m.rows {
		for ci, c := range r.cells {
			rc := rects[c.id]
			layers = append(layers,
				lipgloss.NewLayer(blank(rc.w, rc.h)).X(rc.x).Y(rc.y).Z(1).ID("panel:"+c.id),
				lipgloss.NewLayer(blank(min(len(title(c.id))+6, rc.w-2), 1)).X(rc.x+1).Y(rc.y).Z(3).ID("title:"+c.id),
			)
			if ci < len(r.cells)-1 {
				layers = append(layers, lipgloss.NewLayer(blank(2, rc.h-2)).X(rc.x+rc.w-1).Y(rc.y+1).Z(4).ID(fmt.Sprintf("vb:%d:%d", ri, ci)))
			}
		}
		if ri < len(m.rows)-1 {
			rc := rects[r.cells[0].id]
			layers = append(layers, lipgloss.NewLayer(blank(m.w, 2)).X(0).Y(rc.y+rc.h-1).Z(2).ID(fmt.Sprintf("hb:%d", ri)))
		}
	}
	return lipgloss.NewCompositor(layers...)
}

func title(id string) string {
	switch id {
	case "prs":
		return "Review requests"
	case "clock":
		return "Clock"
	case "notes":
		return "Notes"
	case "cpu":
		return "CPU + memory"
	case "probe":
		return "Probe"
	}
	return id
}

func (m model) order() []string {
	var ids []string
	for _, r := range m.rows {
		for _, c := range r.cells {
			ids = append(ids, c.id)
		}
	}
	return ids
}

func (m model) find(id string) (int, int) {
	for ri, r := range m.rows {
		for ci, c := range r.cells {
			if c.id == id {
				return ri, ci
			}
		}
	}
	return -1, -1
}

func cloneRows(rs []row) []row {
	out := make([]row, len(rs))
	for i, r := range rs {
		out[i] = row{r.weight, append([]cell(nil), r.cells...)}
	}
	return out
}

func (m *model) remove(id string) cell {
	ri, ci := m.find(id)
	c := m.rows[ri].cells[ci]
	m.rows[ri].cells = append(m.rows[ri].cells[:ci], m.rows[ri].cells[ci+1:]...)
	if len(m.rows[ri].cells) == 0 {
		m.rows = append(m.rows[:ri], m.rows[ri+1:]...)
	}
	return c
}

func (m *model) drop(id, target, side string) {
	if id == target {
		return
	}
	c := m.remove(id)
	ri, ci := m.find(target)
	switch side {
	case "left", "right":
		if side == "right" {
			ci++
		}
		cs := m.rows[ri].cells
		cs = append(cs[:ci], append([]cell{c}, cs[ci:]...)...)
		m.rows[ri].cells = cs
	case "top", "bottom":
		if side == "bottom" {
			ri++
		}
		nr := row{weight: 1.5, cells: []cell{{c.id, 1}}}
		m.rows = append(m.rows[:ri], append([]row{nr}, m.rows[ri:]...)...)
	}
}

func (m *model) moveKey(dir string) {
	ri, ci := m.find(m.focus)
	switch dir {
	case "h":
		if ci > 0 {
			cs := m.rows[ri].cells
			cs[ci], cs[ci-1] = cs[ci-1], cs[ci]
		}
	case "l":
		if ci < len(m.rows[ri].cells)-1 {
			cs := m.rows[ri].cells
			cs[ci], cs[ci+1] = cs[ci+1], cs[ci]
		}
	case "k", "j":
		alone := len(m.rows[ri].cells) == 1
		target := ri - 1
		if dir == "j" {
			target = ri + 1
		}
		if target < 0 || target >= len(m.rows) {
			if alone {
				return
			}
			c := m.remove(m.focus)
			nr := row{weight: 1.5, cells: []cell{c}}
			if target < 0 {
				m.rows = append([]row{nr}, m.rows...)
			} else {
				m.rows = append(m.rows, nr)
			}
			return
		}
		c := m.remove(m.focus)
		if alone && dir == "j" {
			target--
		}
		m.rows[target].cells = append(m.rows[target].cells, c)
	}
}

func (m *model) resizeKey(dir string) {
	ri, ci := m.find(m.focus)
	switch dir {
	case "H":
		m.rows[ri].cells[ci].weight = math.Max(0.2, m.rows[ri].cells[ci].weight*0.85)
	case "L":
		m.rows[ri].cells[ci].weight *= 1.15
	case "K":
		m.rows[ri].weight = math.Max(0.3, m.rows[ri].weight*0.85)
	case "J":
		m.rows[ri].weight *= 1.15
	}
}

func modName(code rune) string {
	switch code {
	case tea.KeyLeftShift, tea.KeyRightShift:
		return "shift"
	case tea.KeyLeftCtrl, tea.KeyRightCtrl:
		return "ctrl"
	case tea.KeyLeftAlt, tea.KeyRightAlt:
		return "alt"
	case tea.KeyLeftSuper, tea.KeyRightSuper:
		return "super"
	}
	return ""
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.logf("resize %dx%d", m.w, m.h)
		m.clampScroll()
		return m, nil

	case tea.KeyboardEnhancementsMsg:
		m.kbFlags, m.kbSeen = msg.Flags, true
		m.logf("kitty flags=%d events=%v all-esc=%v text=%v", msg.Flags, msg.SupportsEventTypes(), msg.SupportsAllKeysAsEscapeCodes(), msg.SupportsAssociatedText())
		return m, nil

	case tickMsg:
		m.frames++
		if d := time.Since(m.fpsAt); d >= time.Second {
			m.fps = float64(m.frames) / d.Seconds()
			m.frames, m.fpsAt = 0, time.Now()
		}
		last := m.cpu[len(m.cpu)-1]
		next := math.Min(0.98, math.Max(0.03, last+(rand.Float64()-0.5)*0.18))
		m.cpu = append(m.cpu[1:], next)
		m.mem = 0.62 + 0.04*math.Sin(float64(time.Now().UnixMilli())/4000)
		return m, tick()

	case editorDoneMsg:
		if msg.err != nil {
			m.say("editor: " + msg.err.Error())
		} else {
			m.noteText = msg.text
			m.say(fmt.Sprintf("note saved from $EDITOR (%s)", msg.took.Round(time.Millisecond)))
		}
		m.logf("$EDITOR returned after %s err=%v", msg.took.Round(time.Millisecond), msg.err)
		return m, nil

	case tea.KeyReleaseMsg:
		k := tea.Key(msg)
		if n := modName(k.Code); n != "" {
			delete(m.held, n)
		}
		m.logf("release %s", msg.String())
		return m, nil

	case tea.KeyPressMsg:
		k := tea.Key(msg)
		if n := modName(k.Code); n != "" {
			if !k.IsRepeat {
				m.held[n] = true
				m.logf("press   %s (lone modifier)", msg.String())
			}
			return m, nil
		}
		if !k.IsRepeat {
			m.logf("press   %s text=%q", msg.String(), k.Text)
		}
		return m.key(msg)

	case tea.MouseMsg:
		return m.mouse(msg)
	}
	return m, nil
}

func (m model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	s := msg.String()
	if m.mode == "INSERT" {
		if s == "esc" {
			m.noteText = m.note.Value()
			m.note.Blur()
			m.mode = "NORMAL"
			m.say("note saved")
			return m, nil
		}
		var cmd tea.Cmd
		m.note, cmd = m.note.Update(msg)
		return m, cmd
	}
	if m.mode == "ARRANGE" {
		switch s {
		case "h", "j", "k", "l", "left", "down", "up", "right":
			if d, ok := map[string]string{"left": "h", "down": "j", "up": "k", "right": "l"}[s]; ok {
				s = d
			}
			m.moveKey(s)
		case "H", "J", "K", "L", "shift+left", "shift+down", "shift+up", "shift+right":
			d := map[string]string{"shift+left": "H", "shift+down": "J", "shift+up": "K", "shift+right": "L"}[s]
			if d == "" {
				d = s
			}
			m.resizeKey(d)
		case "enter":
			m.mode, m.arrange = "NORMAL", nil
			m.say("layout kept")
		case "esc":
			m.rows, m.mode, m.arrange = m.arrange, "NORMAL", nil
			m.say("layout reverted")
		}
		return m, nil
	}
	ids := m.order()
	switch s {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "esc":
		if m.focus != "" {
			m.lastFocus, m.focus = m.focus, ""
		}
	case "tab", "shift+tab", "h", "l", "left", "right":
		if m.focus == "" {
			m.focus = m.lastFocus
			return m, nil
		}
		m.focus = m.neighbour(s)
	case "j", "down":
		if m.focus == "prs" {
			m.sel["prs"] = min(m.sel["prs"]+1, len(prs)-1)
			m.reveal()
		}
	case "k", "up":
		if m.focus == "prs" {
			m.sel["prs"] = max(m.sel["prs"]-1, 0)
			m.reveal()
		}
	case "m":
		if m.focus == "" {
			m.focus = m.lastFocus
		}
		m.mode, m.arrange = "ARRANGE", cloneRows(m.rows)
	case "enter":
		if m.focus == "prs" {
			m.say("open " + prs[m.sel["prs"]].repo + " in browser")
		}
	case "a":
		if m.focus == "prs" {
			m.say("approved " + prs[m.sel["prs"]].repo + "   u undo")
		}
	case "i":
		if m.focus == "notes" {
			m.mode = "INSERT"
			m.note.SetValue(m.noteText)
			return m, m.note.Focus()
		}
	case "E":
		if m.focus == "notes" {
			return m, m.openEditor()
		}
	default:
		if len(s) == 1 && s[0] >= '1' && s[0] <= '9' && int(s[0]-'1') < len(ids) {
			m.focus = ids[s[0]-'1']
		}
	}
	return m, nil
}

// neighbour: h/l walk the focused row and stop at its edges; Tab/S-Tab jump to
// the next/previous row (wrapping), landing on the Widget nearest horizontally.
func (m model) neighbour(s string) string {
	ri, ci := m.find(m.focus)
	switch s {
	case "h", "left":
		return m.rows[ri].cells[max(ci-1, 0)].id
	case "l", "right":
		return m.rows[ri].cells[min(ci+1, len(m.rows[ri].cells)-1)].id
	}
	step := 1
	if s == "shift+tab" {
		step = -1
	}
	target := (ri + step + len(m.rows)) % len(m.rows)
	rects := layout(m.rows, m.w, m.h-1)
	cur := rects[m.focus]
	cx := cur.x + cur.w/2
	best, bestD := m.rows[target].cells[0].id, math.MaxInt
	for _, c := range m.rows[target].cells {
		rc := rects[c.id]
		d := 0
		if cx < rc.x {
			d = rc.x - cx
		} else if cx >= rc.x+rc.w {
			d = cx - rc.x - rc.w + 1
		}
		if d < bestD {
			best, bestD = c.id, d
		}
	}
	return best
}

// prRows returns the PR list's lines and, per line, the PR index it shows (-1 for the divider).
func prRows() []int {
	var idx []int
	shown := false
	for i, p := range prs {
		if p.handled != "" && !shown {
			idx = append(idx, -1)
			shown = true
		}
		idx = append(idx, i)
	}
	return idx
}

func (m model) prHeight() int {
	return layout(m.rows, m.w, m.h-1)["prs"].h - 2
}

func (m *model) clampScroll() {
	m.scroll["prs"] = min(max(m.scroll["prs"], 0), max(0, len(prRows())-m.prHeight()))
}

func (m *model) reveal() {
	line := 0
	for l, i := range prRows() {
		if i == m.sel["prs"] {
			line = l
		}
	}
	h := m.prHeight()
	if line < m.scroll["prs"] {
		m.scroll["prs"] = line
	} else if line >= m.scroll["prs"]+h {
		m.scroll["prs"] = line - h + 1
	}
	m.clampScroll()
}

func (m model) openEditor() tea.Cmd {
	f, err := os.CreateTemp("", "ghud-note-*.md")
	if err != nil {
		return func() tea.Msg { return editorDoneMsg{err: err} }
	}
	_, _ = f.WriteString(m.noteText)
	_ = f.Close()
	ed := os.Getenv("EDITOR")
	if ed == "" {
		ed = "vi"
	}
	start := time.Now()
	c := exec.Command("sh", "-c", ed+` "$0"`, f.Name())
	return tea.ExecProcess(c, func(err error) tea.Msg {
		b, rerr := os.ReadFile(f.Name())
		_ = os.Remove(f.Name())
		if err == nil {
			err = rerr
		}
		return editorDoneMsg{err: err, text: strings.TrimRight(string(b), "\n"), took: time.Since(start)}
	})
}

func (m model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	ms := msg.Mouse()
	m.clampScroll()
	hit := m.hitZones().Hit(ms.X, ms.Y)
	m.lastHit = hit.ID()
	m.lastMouse = fmt.Sprintf("%T %d,%d %s", msg, ms.X, ms.Y, ms.Button)
	rects := layout(m.rows, m.w, m.h-1)

	switch msg := msg.(type) {
	case tea.MouseWheelMsg:
		kind, id, _ := strings.Cut(hit.ID(), ":")
		if (kind == "panel" || kind == "title") && id == "prs" {
			if msg.Button == tea.MouseWheelDown {
				m.scroll[id]++
			} else {
				m.scroll[id]--
			}
			m.clampScroll()
		}
		return m, nil

	case tea.MouseClickMsg:
		if msg.Button != tea.MouseLeft {
			if msg.Button == tea.MouseRight && strings.HasPrefix(hit.ID(), "panel:prs") {
				m.say("item menu: open · approve · remove me · copy branch")
			}
			return m, nil
		}
		kind, rest, _ := strings.Cut(hit.ID(), ":")
		switch kind {
		case "title":
			rc := rects[rest]
			m.focus = rest
			m.drag = drag{kind: dragMove, id: rest, grabX: ms.X - rc.x, grabY: ms.Y - rc.y, px: ms.X, py: ms.Y, orig: cloneRows(m.rows)}
			m.logf("drag start move %s", rest)
		case "vb":
			var ri, ci int
			fmt.Sscanf(rest, "%d:%d", &ri, &ci)
			m.drag = drag{kind: dragVBorder, row: ri, idx: ci, orig: cloneRows(m.rows)}
			m.logf("drag start vborder %s", rest)
		case "hb":
			var ri int
			fmt.Sscanf(rest, "%d", &ri)
			m.drag = drag{kind: dragHBorder, row: ri, orig: cloneRows(m.rows)}
			m.logf("drag start hborder %s", rest)
		case "panel":
			m.focus = rest
			rc := rects[rest]
			if rest == "prs" {
				rows := prRows()
				if l := ms.Y - rc.y - 1 + m.scroll["prs"]; l >= 0 && l < len(rows) && rows[l] >= 0 {
					m.sel["prs"] = rows[l]
				}
			}
			if time.Since(m.lastClick) < 400*time.Millisecond && m.lastClickAt == hit.ID() {
				m.say("double-click: primary Action on " + title(rest))
			}
			m.lastClick, m.lastClickAt = time.Now(), hit.ID()
		}
		return m, nil

	case tea.MouseMotionMsg:
		switch m.drag.kind {
		case dragNone:
			m.hover = hit.ID()
		case dragMove:
			m.drag.px, m.drag.py = ms.X, ms.Y
			m.drag.targetID, m.drag.side = "", ""
			for id, rc := range rects {
				if id == m.drag.id || !rc.in(ms.X, ms.Y) {
					continue
				}
				dx := float64(ms.X-rc.x)/float64(rc.w) - 0.5
				dy := float64(ms.Y-rc.y)/float64(rc.h) - 0.5
				m.drag.targetID = id
				switch {
				case math.Abs(dx) > math.Abs(dy) && dx < 0:
					m.drag.side = "left"
				case math.Abs(dx) > math.Abs(dy):
					m.drag.side = "right"
				case dy < 0:
					m.drag.side = "top"
				default:
					m.drag.side = "bottom"
				}
			}
		case dragVBorder:
			r := &m.rows[m.drag.row]
			a, b := r.cells[m.drag.idx], r.cells[m.drag.idx+1]
			ra, rb := rects[a.id], rects[b.id]
			span := ra.w + rb.w
			left := min(max(ms.X-ra.x+1, 12), span-12)
			tot := a.weight + b.weight
			r.cells[m.drag.idx].weight = tot * float64(left) / float64(span)
			r.cells[m.drag.idx+1].weight = tot * float64(span-left) / float64(span)
		case dragHBorder:
			ra := rects[m.rows[m.drag.row].cells[0].id]
			rb := rects[m.rows[m.drag.row+1].cells[0].id]
			span := ra.h + rb.h
			top := min(max(ms.Y-ra.y+1, 4), span-4)
			tot := m.rows[m.drag.row].weight + m.rows[m.drag.row+1].weight
			m.rows[m.drag.row].weight = tot * float64(top) / float64(span)
			m.rows[m.drag.row+1].weight = tot * float64(span-top) / float64(span)
		}
		return m, nil

	case tea.MouseReleaseMsg:
		if m.drag.kind == dragMove && m.drag.targetID != "" {
			m.drop(m.drag.id, m.drag.targetID, m.drag.side)
			m.say(fmt.Sprintf("moved %s to %s of %s   u undo", title(m.drag.id), m.drag.side, title(m.drag.targetID)))
		}
		if m.drag.kind != dragNone {
			m.logf("drag end")
		}
		m.drag = drag{}
		return m, nil
	}
	return m, nil
}

// ---- drawing ----

func pad(s string, w int) string {
	s = ansi.Truncate(s, w, "…")
	if d := w - lipgloss.Width(s); d > 0 {
		s += strings.Repeat(" ", d)
	}
	return s
}

func (m model) panel(id string, rc rect) string {
	focused := id == m.focus
	arranging := focused && m.mode == "ARRANGE"
	bc := cLine
	if focused {
		bc = cAccent
	}
	if arranging {
		bc = cWarn
	}
	hoverBorder := false
	if m.drag.kind == dragNone && (strings.HasPrefix(m.hover, "vb:") || strings.HasPrefix(m.hover, "hb:")) {
		hoverBorder = m.borderTouches(m.hover, id)
	}
	if m.drag.kind == dragVBorder || m.drag.kind == dragHBorder {
		hoverBorder = m.dragTouches(id)
	}
	if hoverBorder {
		bc = cAccent
	}
	b := fg(bc)
	tl, tr, bl, br, hz, vt := "╭", "╮", "╰", "╯", "─", "│"
	if arranging {
		tl, tr, bl, br, hz, vt = "┏", "┓", "┗", "┛", "━", "┃"
	}
	inner := rc.w - 2
	tStyle := fg(cFg)
	if focused {
		tStyle = fg(cBright).Bold(true)
	}
	handle := ""
	if m.hover == "title:"+id || m.drag.id == id {
		handle = fg(cAccent).Render("⠿ ")
	}
	tt := " " + handle + tStyle.Render(title(id)) + m.count(id) + " "
	top := b.Render(tl+hz) + tt
	top += b.Render(strings.Repeat(hz, max(0, rc.w-3-lipgloss.Width(tt)))) + b.Render(tr)
	top = ansi.Truncate(top, rc.w, "")

	body := m.content(id, inner, rc.h-2)
	lines := strings.Split(body, "\n")
	var out []string
	out = append(out, top)
	for i := 0; i < rc.h-2; i++ {
		l := ""
		if i < len(lines) {
			l = lines[i]
		}
		out = append(out, b.Render(vt)+pad(l, inner)+b.Render(vt))
	}
	keys := ""
	if focused {
		keys = " " + fg(cDim).Render(m.keysFor(id)) + " "
	}
	bot := b.Render(bl+hz) + keys
	bot += b.Render(strings.Repeat(hz, max(0, rc.w-3-lipgloss.Width(keys)))) + b.Render(br)
	out = append(out, ansi.Truncate(bot, rc.w, ""))
	return strings.Join(out, "\n")
}

func (m model) borderTouches(zone, id string) bool {
	ri, ci := m.find(id)
	kind, rest, _ := strings.Cut(zone, ":")
	if kind == "vb" {
		var r, c int
		fmt.Sscanf(rest, "%d:%d", &r, &c)
		return r == ri && (c == ci || c+1 == ci)
	}
	var r int
	fmt.Sscanf(rest, "%d", &r)
	return r == ri || r+1 == ri
}

func (m model) dragTouches(id string) bool {
	if m.drag.kind == dragVBorder {
		return m.borderTouches(fmt.Sprintf("vb:%d:%d", m.drag.row, m.drag.idx), id)
	}
	return m.borderTouches(fmt.Sprintf("hb:%d", m.drag.row), id)
}

func (m model) count(id string) string {
	if id == "prs" {
		return fg(cDim).Render(fmt.Sprintf(" %d", len(prs)))
	}
	return ""
}

func (m model) keysFor(id string) string {
	switch id {
	case "prs":
		return "⏎ open · a approve · r remove me · y copy"
	case "notes":
		if m.mode == "INSERT" {
			return "esc save"
		}
		return "i edit · E $EDITOR"
	}
	return "m arrange"
}

func (m model) content(id string, w, h int) string {
	switch id {
	case "prs":
		return m.prList(w, h)
	case "clock":
		return clock(w, h)
	case "notes":
		if m.mode == "INSERT" {
			m.note.SetWidth(w)
			m.note.SetHeight(h)
			return m.note.View()
		}
		return fg(cFg).Render(m.noteText)
	case "cpu":
		return m.cpuView(w, h)
	case "probe":
		return m.probe(w, h)
	}
	return ""
}

func (m model) prList(w, h int) string {
	var lines []string
	shown := false
	for i, p := range prs {
		if p.handled != "" && !shown {
			lines = append(lines, fg(cFaint).Render(pad("── Handled by a teammate ", w)))
			shown = true
		}
		ci := map[string]lipgloss.Style{"✓": fg(cOk), "●": fg(cWarn), "✗": fg(cBad)}[p.ci]
		why := map[string]lipgloss.Style{"@you": fg(cAccent), "team": fg(cTeam), "owner": fg(cInfo)}[p.why]
		right := fg(cOk).Render(fmt.Sprintf("+%d", p.add)) + " " + fg(cBad).Render(fmt.Sprintf("−%d", p.del))
		if w > 70 {
			right += " " + fg(cDim).Render(fmt.Sprintf("%-5s", p.author))
		}
		if w > 58 {
			right += " " + why.Render(fmt.Sprintf("%-5s", p.why))
		}
		if p.handled != "" {
			right = fg(cOk).Render("✓ "+p.handled) + " " + right
		}
		right += " " + fg(cDim).Render(fmt.Sprintf("%3s", p.age))
		leftW := w - lipgloss.Width(right) - 1
		left := ci.Render(p.ci) + " " + fg(cAccent).Render(fmt.Sprintf("%-10s", p.repo)) + fg(cFg).Render(p.title)
		line := pad(left, leftW) + " " + right
		if i == m.sel["prs"] && m.focus == "prs" {
			line = lipgloss.NewStyle().Background(cSel).Render(pad(ansi.Strip(line), w))
		}
		lines = append(lines, line)
	}
	off := min(max(m.scroll["prs"], 0), max(0, len(lines)-h))
	return strings.Join(lines[off:], "\n")
}

var digits = map[rune][3]string{
	'0': {"█▀█", "█ █", "▀▀▀"}, '1': {" ▀█", "  █", "  ▀"}, '2': {"▀▀█", "█▀▀", "▀▀▀"},
	'3': {"▀▀█", " ▀█", "▀▀▀"}, '4': {"█ █", "▀▀█", "  ▀"}, '5': {"█▀▀", "▀▀█", "▀▀▀"},
	'6': {"█▀▀", "█▀█", "▀▀▀"}, '7': {"▀▀█", "  █", "  ▀"}, '8': {"█▀█", "█▀█", "▀▀▀"},
	'9': {"█▀█", "▀▀█", "▀▀▀"}, ':': {" ", "▀", "▀"},
}

func clock(w, h int) string {
	now := time.Now()
	t := now.Format("15:04:05")
	if w < 30 || h < 5 {
		return lipgloss.PlaceHorizontal(w, lipgloss.Center, fg(cBright).Bold(true).Render(t)) + "\n" +
			lipgloss.PlaceHorizontal(w, lipgloss.Center, fg(cDim).Render(now.Format("Mon 2 Jan")))
	}
	var rows [3]string
	for _, r := range t {
		for i := range 3 {
			rows[i] += digits[r][i] + " "
		}
	}
	big := fg(cBright).Render(strings.Join(rows[:], "\n"))
	s := lipgloss.JoinVertical(lipgloss.Center, big, "", fg(cDim).Render(now.Format("Monday 2 January")))
	return lipgloss.Place(w, h, lipgloss.Center, lipgloss.Center, s)
}

// braille area chart: each cell covers 2 samples × 4 vertical dots.
func (m model) cpuView(w, h int) string {
	ch := max(1, h-2)
	samples := m.cpu[max(0, len(m.cpu)-w*2):]
	grid := make([][]rune, ch)
	for i := range grid {
		grid[i] = []rune(strings.Repeat("⠀", w))
	}
	left := [4]rune{0x40, 0x04, 0x02, 0x01}
	right := [4]rune{0x80, 0x20, 0x10, 0x08}
	for i, v := range samples {
		col := i / 2
		if col >= w {
			break
		}
		dots := int(math.Round(v * float64(ch*4)))
		for d := 0; d < dots; d++ {
			r := ch - 1 - d/4
			bit := left[d%4]
			if i%2 == 1 {
				bit = right[d%4]
			}
			grid[r][col] |= bit
		}
	}
	var lines []string
	for _, g := range grid {
		lines = append(lines, fg(cAccent).Render(string(g)))
	}
	cur := m.cpu[len(m.cpu)-1]
	lines[0] = pad(fg(cBright).Render(fmt.Sprintf("cpu %3.0f%%", cur*100)), w)
	barW := max(1, w-10)
	full := m.mem * float64(barW)
	eighths := []string{"", "▏", "▎", "▍", "▌", "▋", "▊", "▉"}
	bar := strings.Repeat("█", int(full)) + eighths[int((full-math.Floor(full))*8)]
	lines = append(lines, fg(cDim).Render("mem ")+fg(cTeam).Render(pad(bar, barW))+fg(cDim).Render(fmt.Sprintf(" %2.0f%%", m.mem*100)))
	lines = append(lines, fg(cFaint).Render("load 2.41 2.10 1.88"))
	return strings.Join(lines, "\n")
}

func (m model) probe(w, h int) string {
	kb := fg(cBad).Render("no reply")
	if m.kbSeen {
		kb = fg(cOk).Render(fmt.Sprintf("flags %d", m.kbFlags))
	}
	held := []string{}
	for _, k := range []string{"shift", "ctrl", "alt", "super"} {
		if m.held[k] {
			held = append(held, k)
		}
	}
	lines := []string{
		fg(cDim).Render("TERM ") + os.Getenv("TERM") + fg(cDim).Render(" prog ") + os.Getenv("TERM_PROGRAM"),
		fg(cDim).Render("kitty ") + kb + fg(cDim).Render(fmt.Sprintf("  fps %.0f  view %s", m.fps, viewCost.Round(10*time.Microsecond))),
		fg(cDim).Render("held  ") + fg(cWarn).Render(strings.Join(held, "+")),
		fg(cDim).Render("hit   ") + fg(cAccent).Render(m.lastHit),
		fg(cDim).Render("mouse ") + m.lastMouse,
	}
	n := max(0, h-len(lines))
	start := max(0, len(m.log)-n)
	for i := len(m.log) - 1; i >= start; i-- {
		e := m.log[i]
		lines = append(lines, fg(cFaint).Render(e.at.Format("05.000 "))+fg(cFg).Render(e.text))
	}
	return strings.Join(lines, "\n")
}

func (m model) footer() string {
	badge := map[string]color.Color{"NORMAL": cAccent, "ARRANGE": cWarn, "INSERT": cOk}[m.mode]
	out := lipgloss.NewStyle().Background(badge).Foreground(cBg).Bold(true).Render(" "+m.mode+" ") + " "
	hints := "Tab/hl focus · 1–9 jump · ^K palette · ? keys · q quit"
	if m.focus != "" {
		hints = "Tab rows · hl widgets · m arrange · esc unfocus · ^K palette · ? keys"
	}
	switch {
	case m.held["ctrl"]:
		hints = fg(cWarn).Render("CTRL ") + "^K palette · ^C quit · ^D page down · ^U page up"
	case m.held["shift"]:
		hints = fg(cWarn).Render("SHIFT ") + "HJKL focus any direction · E $EDITOR · S-Tab previous"
	case m.mode == "ARRANGE":
		hints = "hjkl move · HJKL resize · ⏎ keep · esc revert"
	}
	if m.toast != "" && time.Since(m.toastAt) < 3*time.Second {
		hints = fg(cBright).Render(m.toast)
	}
	return pad(out+fg(cDim).Render(hints), m.w)
}

func ghost(rc rect) []*lipgloss.Layer {
	s := fg(cAccent)
	h := s.Render("╭" + strings.Repeat("┄", max(0, rc.w-2)) + "╮")
	b := s.Render("╰" + strings.Repeat("┄", max(0, rc.w-2)) + "╯")
	v := s.Render(strings.TrimSuffix(strings.Repeat("┆\n", max(1, rc.h-2)), "\n"))
	return []*lipgloss.Layer{
		lipgloss.NewLayer(h).X(rc.x).Y(rc.y).Z(5),
		lipgloss.NewLayer(b).X(rc.x).Y(rc.y + rc.h - 1).Z(5),
		lipgloss.NewLayer(v).X(rc.x).Y(rc.y + 1).Z(5),
		lipgloss.NewLayer(v).X(rc.x + rc.w - 1).Y(rc.y + 1).Z(5),
	}
}

func dropBar(rc rect, side string) *lipgloss.Layer {
	s := fg(cAccent).Bold(true)
	switch side {
	case "left", "right":
		x := rc.x
		if side == "right" {
			x = rc.x + rc.w - 1
		}
		return lipgloss.NewLayer(s.Render(strings.TrimSuffix(strings.Repeat("┃\n", rc.h), "\n"))).X(x).Y(rc.y).Z(6)
	}
	y := rc.y
	if side == "bottom" {
		y = rc.y + rc.h - 1
	}
	return lipgloss.NewLayer(s.Render(strings.Repeat("━", rc.w))).X(rc.x).Y(y).Z(6)
}

func (m model) View() tea.View {
	start := time.Now()
	v := tea.NewView("")
	v.AltScreen = true
	v.MouseMode = tea.MouseModeAllMotion
	v.KeyboardEnhancements = tea.KeyboardEnhancements{
		ReportEventTypes:           true,
		ReportAlternateKeys:        true,
		ReportAllKeysAsEscapeCodes: true,
		ReportAssociatedText:       true,
	}
	v.WindowTitle = "ghud spike"
	v.BackgroundColor = cBg
	if m.w == 0 {
		return v
	}
	rects := layout(m.rows, m.w, m.h-1)
	var layers []*lipgloss.Layer
	for _, id := range m.order() {
		rc := rects[id]
		layers = append(layers, lipgloss.NewLayer(m.panel(id, rc)).X(rc.x).Y(rc.y).Z(1).ID(id))
	}
	if m.drag.kind == dragMove {
		o := rects[m.drag.id]
		gw, gh := min(o.w, 34), min(o.h, 6)
		g := rect{m.drag.px - min(m.drag.grabX, gw-2), m.drag.py - min(m.drag.grabY, gh-1), gw, gh}
		g.x = min(max(g.x, 0), m.w-g.w)
		g.y = min(max(g.y, 0), m.h-1-g.h)
		layers = append(layers, ghost(g)...)
		layers = append(layers, lipgloss.NewLayer(fg(cAccent).Bold(true).Render(ansi.Truncate(" "+title(m.drag.id)+" ", g.w-4, "…"))).X(g.x+2).Y(g.y).Z(6))
		if m.drag.targetID != "" {
			layers = append(layers, dropBar(rects[m.drag.targetID], m.drag.side))
		}
	}
	layers = append(layers, lipgloss.NewLayer(m.footer()).X(0).Y(m.h-1).Z(1))
	comp := lipgloss.NewCompositor(layers...)
	canvas := lipgloss.NewCanvas(m.w, m.h)
	comp.Draw(canvas, image.Rect(0, 0, m.w, m.h))
	v.SetContent(canvas.Render())
	viewCost = time.Since(start)
	return v
}

// viewCost is written from View (value receiver) and surfaced on the next frame.
var viewCost time.Duration

func main() {
	if len(os.Args) > 1 && os.Args[1] == "-snapshot" {
		m := initialModel()
		m.w, m.h = 140, 38
		fmt.Println(m.View().Content)
		return
	}
	m := initialModel()
	// herdr routes right-click to its own pane menu unless the pane opts in.
	if os.Getenv("HERDR_PANE_ID") != "" {
		out, err := exec.Command("herdr", "pane", "input", "--current", "--right-click", "pane").CombinedOutput()
		m.logf("herdr right-click → pane: err=%v %s", err, strings.TrimSpace(string(out)))
		defer exec.Command("herdr", "pane", "input", "--current", "--right-click", "herdr").Run()
	}
	p := tea.NewProgram(m)
	if _, err := p.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
