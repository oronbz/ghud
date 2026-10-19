package install_test

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/oronbz/ghud/internal/plugin/install"
)

// serving is a GitHub whose every tarball holds entries.
func serving(t *testing.T, entries ...*tar.Header) install.GitHub {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gz := gzip.NewWriter(w)
		tw := tar.NewWriter(gz)
		for _, h := range entries {
			_ = tw.WriteHeader(h)
			if h.Size > 0 {
				_, _ = tw.Write([]byte(strings.Repeat("x", int(h.Size))))
			}
		}
		_ = tw.Close()
		_ = gz.Close()
	}))
	t.Cleanup(srv.Close)
	return install.GitHub{API: srv.URL}
}

func TestFetchRefusesEntriesThatEscapeThePlugin(t *testing.T) {
	for name, entry := range map[string]*tar.Header{
		"dot-dot path":  {Typeflag: tar.TypeReg, Name: "top/../../evil", Size: 1, Mode: 0o644},
		"link outside":  {Typeflag: tar.TypeSymlink, Name: "top/link", Linkname: "../../evil"},
		"absolute link": {Typeflag: tar.TypeSymlink, Name: "top/link", Linkname: "/etc/passwd"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "a", "b")
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			err := serving(t, entry).Fetch(context.Background(), "octo", "evil", "abc", "", dir)
			if err == nil || !strings.Contains(err.Error(), "outside the Plugin") {
				t.Errorf("Fetch returned %v, want a refusal", err)
			}
			if _, err := os.Lstat(filepath.Join(root, "evil")); err == nil {
				t.Error("a file was written outside the Plugin")
			}
		})
	}
}
