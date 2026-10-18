package plugin

import (
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"time"

	"github.com/oronbz/ghud/internal/widget"
)

// The protocol's messages, as schema/protocol.json describes them.

type initializeParams struct {
	Protocol int    `json:"protocol"`
	Ghud     string `json:"ghud"`
}

type initializeResult struct {
	Protocol int `json:"protocol"`
}

type size struct {
	Width  int `json:"width"`
	Height int `json:"height"`
}

type frameParams struct {
	Widget  string `json:"widget"`
	Size    size   `json:"size"`
	Focused bool   `json:"focused"`
	Visible bool   `json:"visible"`
}

func frameOf(id string, s widget.Shown) frameParams {
	return frameParams{Widget: id, Size: size{s.Size.W, s.Size.H}, Focused: s.Focused, Visible: s.Visible}
}

type openParams struct {
	frameParams
	Type     string         `json:"type"`
	Settings map[string]any `json:"settings"`
}

type widgetParams struct {
	Widget string `json:"widget"`
}

type invokeParams struct {
	Widget string `json:"widget"`
	Action string `json:"action"`
	Item   string `json:"item,omitempty"`
	Text   string `json:"text,omitempty"`
	Choice string `json:"choice,omitempty"`
	Base   string `json:"base,omitempty"`
}

type choicesParams struct {
	Widget string `json:"widget"`
	Action string `json:"action"`
	Item   string `json:"item,omitempty"`
}

type choicesResult struct {
	Choices []struct {
		ID     string `json:"id"`
		Label  string `json:"label"`
		Target string `json:"target"`
	} `json:"choices"`
}

// invokeResult is widget.Result as JSON carries it: settings arrive as
// JSON values, to be read against the Widget type's schema.
type invokeResult struct {
	widget.Result
	Settings map[string]any `json:"settings"`
}

// settings reads a Plugin's settings against schema, the way config.toml's are.
func (r invokeResult) settings(schema []widget.Setting) (widget.Settings, error) {
	if len(r.Settings) == 0 {
		return nil, nil
	}
	out := make(widget.Settings, len(r.Settings))
	for k, v := range r.Settings {
		i := slices.IndexFunc(schema, func(s widget.Setting) bool { return s.Name == k })
		if i < 0 {
			return nil, fmt.Errorf("sent an unknown setting `%s`", k)
		}
		if f, ok := v.(float64); ok && schema[i].Kind == widget.Int && f == math.Trunc(f) {
			v = int64(f)
		}
		val, err := schema[i].Check(v)
		if err != nil {
			return nil, fmt.Errorf("sent setting `%s` that %w", k, err)
		}
		out[k] = val
	}
	return out, nil
}

// update is a refresh's result and, with Widget, a pushed widget/update.
type update struct {
	Widget  string          `json:"widget,omitempty"`
	Payload json.RawMessage `json:"payload,omitempty"`
	Status  *status         `json:"status,omitempty"`
}

type status struct {
	State   string         `json:"state"`
	Message string         `json:"message,omitempty"`
	Reason  string         `json:"reason,omitempty"`
	Hint    string         `json:"hint,omitempty"`
	Action  *widget.Action `json:"action,omitempty"`
	Until   time.Time      `json:"until,omitzero"`
}

// read is what an update says in Widget-contract terms: its payload, when
// it has one, and its Status as Refresh would return it. A payload that
// doesn't decode or that the schema rejects, such as one with a raw colour
// or an unknown field, is an error naming why.
func (u update) read() (*widget.Payload, error) {
	var p *widget.Payload
	if len(u.Payload) > 0 && string(u.Payload) != "null" {
		p = new(widget.Payload)
		err := json.Unmarshal(u.Payload, p)
		if err == nil {
			err = validateJSON("protocol.json#/$defs/payload", u.Payload)
		}
		if err != nil {
			return nil, fmt.Errorf("sent a payload ghud can't show: %w", err)
		}
	}
	if u.Status == nil {
		return p, nil
	}
	switch st := u.Status; st.State {
	case "", "ok":
		return p, nil
	case "setup":
		return nil, &widget.SetupError{Hint: st.Message, Action: st.Action}
	case "rate_limited":
		return nil, &widget.RateLimitError{Until: st.Until, Err: failure{msg: st.Message, reason: st.Reason, hint: st.Hint}}
	case "error":
		return nil, failure{msg: st.Message, reason: st.Reason, hint: st.Hint}
	default:
		return nil, fmt.Errorf("sent an unknown status %q", st.State)
	}
}

// failure is an error a Plugin reported, with its stale-marker reason and fix.
type failure struct{ msg, reason, hint string }

func (f failure) Error() string {
	if f.msg == "" {
		return "failed"
	}
	return f.msg
}

func (f failure) Reason() string {
	if f.reason == "" {
		return f.Error()
	}
	return f.reason
}

func (f failure) Hint() string { return f.hint }

// settingsJSON is a Widget's settings as a Plugin receives them: durations
// written the way config.toml writes them.
func settingsJSON(s widget.Settings) map[string]any {
	out := make(map[string]any, len(s))
	for k, v := range s {
		if d, ok := v.(time.Duration); ok {
			v = widget.ShortDuration(d)
		}
		out[k] = v
	}
	return out
}
