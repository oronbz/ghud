package paths_test

import (
	"path/filepath"
	"testing"

	"github.com/oronbz/visor/internal/paths"
)

func TestResolve(t *testing.T) {
	j := filepath.Join
	tests := []struct {
		name string
		goos string
		env  map[string]string
		want paths.Paths
	}{
		{
			name: "home defaults on macOS, not ~/Library",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/me"},
			want: paths.Paths{Config: j("/Users/me", ".config", "visor"), Data: j("/Users/me", ".local", "share", "visor"), Cache: j("/Users/me", ".cache", "visor")},
		},
		{
			name: "XDG variables win",
			goos: "darwin",
			env:  map[string]string{"HOME": "/Users/me", "XDG_CONFIG_HOME": "/x/conf", "XDG_DATA_HOME": "/x/data", "XDG_CACHE_HOME": "/x/cache"},
			want: paths.Paths{Config: j("/x/conf", "visor"), Data: j("/x/data", "visor"), Cache: j("/x/cache", "visor")},
		},
		{
			name: "relative XDG values are ignored, per the XDG spec",
			goos: "linux",
			env:  map[string]string{"HOME": "/home/me", "XDG_CONFIG_HOME": "rel"},
			want: paths.Paths{Config: j("/home/me", ".config", "visor"), Data: j("/home/me", ".local", "share", "visor"), Cache: j("/home/me", ".cache", "visor")},
		},
		{
			name: "Windows uses AppData and LocalAppData",
			goos: "windows",
			env:  map[string]string{"AppData": "/w/roaming", "LocalAppData": "/w/local"},
			want: paths.Paths{Config: j("/w/roaming", "visor"), Data: j("/w/local", "visor", "data"), Cache: j("/w/local", "visor", "cache")},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := paths.Resolve(func(k string) string { return tt.env[k] }, tt.goos)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got %+v\nwant %+v", got, tt.want)
			}
		})
	}
}

func TestHomeIsHOMEElseUSERPROFILE(t *testing.T) {
	for _, tt := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"HOME": "/home/me", "USERPROFILE": `C:\Users\me`}, "/home/me"},
		{map[string]string{"USERPROFILE": `C:\Users\me`}, `C:\Users\me`},
		{map[string]string{}, ""},
	} {
		if got := paths.Home(func(k string) string { return tt.env[k] }); got != tt.want {
			t.Errorf("Home with %v = %q, want %q", tt.env, got, tt.want)
		}
	}
}

func TestExpandResolvesALeadingTildeOnly(t *testing.T) {
	j := filepath.Join
	for _, tt := range []struct{ p, home, want string }{
		{"~", "/home/me", "/home/me"},
		{"~/notes.md", "/home/me", j("/home/me", "notes.md")},
		{"~other/notes.md", "/home/me", "~other/notes.md"},
		{"a/~/b", "/home/me", "a/~/b"},
		{"/abs", "/home/me", "/abs"},
		{"~/notes.md", "", "~/notes.md"},
	} {
		if got := paths.Expand(tt.p, tt.home); got != tt.want {
			t.Errorf("Expand(%q, %q) = %q, want %q", tt.p, tt.home, got, tt.want)
		}
	}
}

func TestResolveWithoutHomeFails(t *testing.T) {
	if _, err := paths.Resolve(func(string) string { return "" }, "linux"); err == nil {
		t.Error("resolved paths with no $HOME")
	}
}
