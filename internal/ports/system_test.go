package ports

import (
	"slices"
	"testing"
)

func TestTheEditorIsVisualThenEditorThenVi(t *testing.T) {
	for _, tc := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{}, "vi"},
		{map[string]string{"EDITOR": "code -w"}, "code -w"},
		{map[string]string{"EDITOR": "nano", "VISUAL": "hx"}, "hx"},
	} {
		c := editorCommand(func(k string) string { return tc.env[k] }, "linux", "/notes.md")
		want := []string{"sh", "-c", tc.want + ` "$0"`, "/notes.md"}
		if !slices.Equal(c.Args, want) {
			t.Errorf("env %v: args %q, want %q", tc.env, c.Args, want)
		}
	}
}
