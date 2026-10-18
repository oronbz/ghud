package plugintest

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha1"
	"fmt"
	"maps"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
)

// GitHub is a fake of the GitHub REST endpoints `ghud plugin install`
// uses: resolving a ref to a commit and downloading a commit's tarball.
// Repositories live in memory; Push adds commits.
type GitHub struct {
	*httptest.Server

	mu       sync.Mutex
	repos    map[string]*repo
	tarballs map[string]int
	auth     []string
}

type repo struct {
	branch  string
	refs    map[string]string
	commits map[string]map[string]string
}

func NewGitHub(t testing.TB) *GitHub {
	g := &GitHub{repos: map[string]*repo{}, tarballs: map[string]int{}}
	g.Server = httptest.NewServer(http.HandlerFunc(g.serve))
	t.Cleanup(g.Close)
	return g
}

// Push commits files (path to content) to repo ("owner/name") as ref, a
// branch or tag, and returns the commit's sha. A repository's first push
// names its default branch. Files ending in .sh are executable.
func (g *GitHub) Push(repoName, ref string, files map[string]string) string {
	g.mu.Lock()
	defer g.mu.Unlock()
	r := g.repos[repoName]
	if r == nil {
		r = &repo{branch: ref, refs: map[string]string{}, commits: map[string]map[string]string{}}
		g.repos[repoName] = r
	}
	h := sha1.New()
	fmt.Fprintf(h, "%s %s %d", repoName, ref, len(r.commits))
	for _, name := range slices.Sorted(maps.Keys(files)) {
		fmt.Fprintf(h, "%s\x00%s\x00", name, files[name])
	}
	sha := fmt.Sprintf("%x", h.Sum(nil))
	r.refs[ref] = sha
	r.commits[sha] = maps.Clone(files)
	return sha
}

func (g *GitHub) Tarballs(repoName string) int {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.tarballs[repoName]
}

func (g *GitHub) Auth() []string {
	g.mu.Lock()
	defer g.mu.Unlock()
	return slices.Clone(g.auth)
}

func (g *GitHub) serve(w http.ResponseWriter, req *http.Request) {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.auth = append(g.auth, req.Header.Get("Authorization"))
	parts := strings.SplitN(strings.TrimPrefix(req.URL.Path, "/repos/"), "/", 4)
	if len(parts) != 4 || req.Method != http.MethodGet {
		http.NotFound(w, req)
		return
	}
	name, kind, ref := parts[0]+"/"+parts[1], parts[2], parts[3]
	r := g.repos[name]
	if r == nil {
		http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		return
	}
	sha := r.resolve(ref)
	if sha == "" {
		http.Error(w, `{"message":"No commit found for SHA: `+ref+`"}`, http.StatusUnprocessableEntity)
		return
	}
	switch kind {
	case "commits":
		if req.Header.Get("Accept") != "application/vnd.github.sha" {
			http.Error(w, "fake GitHub answers only Accept: application/vnd.github.sha", http.StatusNotAcceptable)
			return
		}
		fmt.Fprint(w, sha)
	case "tarball":
		g.tarballs[name]++
		w.Header().Set("Content-Type", "application/x-gzip")
		writeTarball(w, fmt.Sprintf("%s-%s-%s", parts[0], parts[1], sha[:7]), sha, r.commits[sha])
	default:
		http.NotFound(w, req)
	}
}

func (r *repo) resolve(ref string) string {
	if ref == "HEAD" {
		ref = r.branch
	}
	if sha, ok := r.refs[ref]; ok {
		return sha
	}
	if _, ok := r.commits[ref]; ok {
		return ref
	}
	return ""
}

// writeTarball writes files the way GitHub's tarballs hold them: a pax
// global header carrying the commit, then everything under one top directory.
func writeTarball(w http.ResponseWriter, top, sha string, files map[string]string) {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeXGlobalHeader, Name: "pax_global_header", PAXRecords: map[string]string{"comment": sha}})
	_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: top + "/", Mode: 0o755})
	dirs := map[string]bool{}
	for _, name := range slices.Sorted(maps.Keys(files)) {
		for d := name; strings.Contains(d, "/"); {
			d = d[:strings.LastIndex(d, "/")]
			if !dirs[d] {
				dirs[d] = true
				_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: top + "/" + d + "/", Mode: 0o755})
			}
		}
		mode := int64(0o644)
		if strings.HasSuffix(name, ".sh") {
			mode = 0o755
		}
		body := files[name]
		_ = tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: top + "/" + name, Mode: mode, Size: int64(len(body))})
		_, _ = tw.Write([]byte(body))
	}
	_ = tw.Close()
	_ = gz.Close()
}
