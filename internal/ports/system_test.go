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
		c := editorCommand(func(k string) string { return tc.env[k] }, "linux", "/notes.md", 0)
		want := []string{"sh", "-c", tc.want + ` "$0"`, "/notes.md"}
		if !slices.Equal(c.Args, want) {
			t.Errorf("env %v: args %q, want %q", tc.env, c.Args, want)
		}
	}
}

func TestTheEditorOpensAtALineInEachEditorsOwnSyntax(t *testing.T) {
	for _, tc := range []struct{ editor, want string }{
		{"nvim", `nvim +12 "$0"`},
		{"code -w", `code -w -g "$0:12"`},
		{"/usr/local/bin/subl -w", `/usr/local/bin/subl -w "$0:12"`},
		{"hx", `hx "$0:12"`},
	} {
		c := editorCommand(func(k string) string { return map[string]string{"EDITOR": tc.editor}[k] }, "darwin", "/config.toml", 12)
		want := []string{"sh", "-c", tc.want, "/config.toml"}
		if !slices.Equal(c.Args, want) {
			t.Errorf("EDITOR=%q: args %q, want %q", tc.editor, c.Args, want)
		}
	}
	c := editorCommand(func(string) string { return "vim" }, "windows", `C:\config.toml`, 3)
	if want := []string{"vim", "+3", `C:\config.toml`}; !slices.Equal(c.Args, want) {
		t.Errorf("windows args %q, want %q", c.Args, want)
	}
}
