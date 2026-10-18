package weather

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sync"
)

// The protocol's messages, kept to what this Plugin reads and sends.
type message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type params struct {
	Widget   string         `json:"widget"`
	Settings map[string]any `json:"settings"`
	Size     size           `json:"size"`
	Action   string         `json:"action"`
	Text     string         `json:"text"`
}

// widget is one placed Weather Widget and what it last showed, so a
// resize can redraw it without fetching.
type widget struct {
	id       string
	settings settings
	size     size
	shown    *report
}

type server struct {
	cfg  config
	locs *locations

	mu      sync.Mutex
	out     *json.Encoder
	widgets map[string]*widget
	jobs    sync.WaitGroup
}

func newServer(out io.Writer, cfg config) *server {
	return &server{cfg: cfg, locs: &locations{cfg: cfg}, out: json.NewEncoder(out), widgets: map[string]*widget{}}
}

func (s *server) serve(in io.Reader) int {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 1<<20), 1<<24)
	for sc.Scan() {
		var msg message
		if err := json.Unmarshal(sc.Bytes(), &msg); err != nil {
			fmt.Fprintln(os.Stderr, "ghud-weather: bad message:", err)
			continue
		}
		s.handle(msg)
	}
	s.jobs.Wait()
	return 0
}

func (s *server) send(msg message) {
	msg.JSONRPC = "2.0"
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = s.out.Encode(msg)
}

func (s *server) reply(id json.RawMessage, result any) { s.send(message{ID: id, Result: result}) }

func (s *server) fail(id json.RawMessage, code int, text string) {
	s.send(message{ID: id, Error: &rpcError{Code: code, Message: text}})
}

func (s *server) handle(msg message) {
	var p params
	_ = json.Unmarshal(msg.Params, &p)
	switch msg.Method {
	case "initialize":
		s.reply(msg.ID, map[string]any{"protocol": 1})
	case "widget/open":
		s.mu.Lock()
		s.widgets[p.Widget] = &widget{id: p.Widget, settings: settingsOf(p.Settings), size: p.Size}
		s.mu.Unlock()
	case "widget/close":
		s.mu.Lock()
		delete(s.widgets, p.Widget)
		s.mu.Unlock()
	case "widget/frame":
		s.framed(p)
	case "widget/refresh":
		s.async(func() { s.refresh(msg.ID, p.Widget) })
	case "action/invoke":
		s.async(func() { s.invoke(msg.ID, p) })
	default:
		if msg.ID != nil {
			s.fail(msg.ID, -32601, "no method "+msg.Method)
		}
	}
}

// async runs a request off the read loop, so a slow fetch never holds up
// another Widget's request.
func (s *server) async(fn func()) {
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		fn()
	}()
}

func (s *server) widget(id string) (widget, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	w, ok := s.widgets[id]
	if !ok {
		return widget{}, false
	}
	return *w, true
}

// framed redraws a resized Widget's last report to fit its new size.
func (s *server) framed(p params) {
	s.mu.Lock()
	w, ok := s.widgets[p.Widget]
	if !ok || w.size == p.Size {
		s.mu.Unlock()
		return
	}
	w.size = p.Size
	shown := w.shown
	s.mu.Unlock()
	if shown != nil {
		s.send(message{Method: "widget/update", Params: raw(map[string]any{"widget": p.Widget, "payload": shown.payload(p.Size)})})
	}
}

func (s *server) refresh(id json.RawMessage, wid string) {
	w, ok := s.widget(wid)
	if !ok {
		s.fail(id, -32602, "no widget "+wid)
		return
	}
	ctx := context.Background()
	at, err := s.locs.locate(ctx, w.settings, false)
	if err != nil {
		s.reply(id, map[string]any{"status": statusOf(err)})
		return
	}
	f, err := fetchForecast(ctx, s.cfg, at, w.settings.units)
	if err != nil {
		s.reply(id, map[string]any{"status": statusOf(err)})
		return
	}
	r := &report{place: at, f: f}
	s.mu.Lock()
	if cur, ok := s.widgets[wid]; ok {
		cur.shown = r
		w.size = cur.size
	}
	s.mu.Unlock()
	s.reply(id, map[string]any{"payload": r.payload(w.size)})
}

func (s *server) invoke(id json.RawMessage, p params) {
	w, ok := s.widget(p.Widget)
	if !ok {
		s.fail(id, -32602, "no widget "+p.Widget)
		return
	}
	ctx := context.Background()
	switch p.Action {
	case refreshLocation:
		at, err := s.locs.locate(ctx, w.settings, true)
		if err != nil {
			s.fail(id, 1, failText(err))
			return
		}
		s.reply(id, map[string]any{"toast": "Location: " + at.header()})
	case setCity:
		at, err := s.locs.city(ctx, p.Text, true)
		if err != nil {
			s.fail(id, 1, failText(err))
			return
		}
		s.reply(id, map[string]any{"toast": "Weather for " + at.Name, "settings": map[string]any{"city": p.Text}})
	default:
		s.fail(id, -32601, "no action "+p.Action)
	}
}

// statusOf is a failed refresh's status: needs setup when only a setting
// can fix it, else an error with a short reason for the stale marker.
func statusOf(err error) map[string]any {
	var setup *setupError
	if errors.As(err, &setup) {
		return map[string]any{"state": "setup", "message": setup.hint, "action": setCityAction}
	}
	var f *failure
	if errors.As(err, &f) {
		return map[string]any{"state": "error", "message": f.msg, "reason": f.reason, "hint": f.hint}
	}
	return map[string]any{"state": "error", "message": err.Error()}
}

func failText(err error) string {
	var setup *setupError
	if errors.As(err, &setup) {
		return setup.hint
	}
	return err.Error()
}

// setupError is a place no setting names and nothing could find.
type setupError struct{ hint string }

func (e *setupError) Error() string { return e.hint }

// failure is a failed fetch, with the stale marker's reason and a fix.
type failure struct{ msg, reason, hint string }

func (f *failure) Error() string { return f.msg }

const (
	refreshLocation = "refresh-location"
	setCity         = "set-city"
)

var setCityAction = map[string]any{"id": setCity, "label": "set city", "input": "text", "prompt": "City"}

func raw(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}
