package widget_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/theme"
	"github.com/oronbz/ghud/internal/widget"
)

func TestLiveViewLinesTakeThemeTokens(t *testing.T) {
	var l widget.Line
	in := `{"spans": [{"text": "cpu ", "token": "dim"}, {"token": "info", "bar": {"value": 0.5}}],
		"chart": {"values": [0.1, 0.9], "token": "accent"}}`
	if err := json.Unmarshal([]byte(in), &l); err != nil {
		t.Fatal(err)
	}
	if l.Spans[0].Token != theme.Dim || l.Spans[1].Token != theme.Info || l.Spans[1].Bar.Value != 0.5 || l.Chart.Token != theme.Accent {
		t.Errorf("decoded %+v", l)
	}

	var d widget.Line
	if err := json.Unmarshal([]byte(`{"spans": [{"text": "12:30"}], "digits": "half-block"}`), &d); err != nil || d.Digits != widget.HalfBlock {
		t.Errorf("digits: %v, %v", d.Digits, err)
	}
}

func TestLiveViewLinesRejectRawColours(t *testing.T) {
	for _, in := range []string{
		`{"spans": [{"text": "hot", "token": "#ff0000"}]}`,
		`{"spans": [{"text": "hot", "token": "accent", "bg": "rgb(255,0,0)"}]}`,
		`{"chart": {"values": [1], "token": "red"}}`,
	} {
		var l widget.Line
		err := json.Unmarshal([]byte(in), &l)
		if err == nil {
			t.Errorf("%s: accepted", in)
			continue
		}
		if !strings.Contains(err.Error(), "not a theme token") {
			t.Errorf("%s: error %q doesn't say why", in, err)
		}
	}
}
