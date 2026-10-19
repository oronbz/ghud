package config_test

import (
	"strings"
	"testing"

	"github.com/oronbz/visor/internal/config"
	"github.com/oronbz/visor/internal/widget"
)

func TestTheReferenceWrapsTheDefaultWidgetsWithinTheLine(t *testing.T) {
	var types []widget.Type
	for _, title := range strings.Fields("Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel India Juliett Kilo Lima Mike November Oscar Papa") {
		types = append(types, widget.Type{Name: strings.ToLower(title), Title: title})
	}
	ref := config.Reference(types, config.Locale{})
	_, list, _ := strings.Cut(ref, "the default Widgets when there are none:\n")
	list, _, _ = strings.Cut(list, "# Every block")
	lines := strings.Split(strings.TrimSuffix(list, "\n"), "\n")
	if len(lines) < 2 {
		t.Fatalf("the default Widgets fit one line:\n%s", list)
	}
	for _, l := range lines {
		if len(l) > 80 || !strings.HasPrefix(l, "# ") {
			t.Errorf("line %q isn't prose within 80 columns", l)
		}
	}
	if !strings.Contains(list, "# Alpha, Bravo,") || !strings.HasSuffix(strings.TrimSpace(list), "Papa.") {
		t.Errorf("the default Widgets read:\n%s", list)
	}
}

func TestTheReferenceWithNoDefaultWidgetsHasNoEmptyProseLine(t *testing.T) {
	ref := config.Reference([]widget.Type{{Name: "script", Title: "Script", OptIn: true}}, config.Locale{})
	for _, l := range strings.Split(ref, "\n") {
		if strings.TrimSpace(l) == "#" {
			t.Errorf("the reference has an empty prose line:\n%s", ref)
			break
		}
	}
	if strings.Contains(ref, "when there are none:") {
		t.Errorf("the reference promises a list of default Widgets it doesn't give:\n%s", ref)
	}
}
