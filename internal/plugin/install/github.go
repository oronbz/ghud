package install

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
)

// maxExtract caps the bytes a repository may unpack to.
const maxExtract = 512 << 20

// GitHub fetches repositories through GitHub's REST API: a ref resolves
// to a commit, and a commit's files come as a tarball.
type GitHub struct {
	HTTP *http.Client
	// API is the REST base URL, https://api.github.com in real use.
	API string
	// Token, when set, reaches private repositories and lifts rate limits.
	Token string
}

func (g GitHub) Resolve(ctx context.Context, owner, repo, ref string) (string, error) {
	if ref == "" {
		ref = "HEAD"
	}
	res, err := g.get(ctx, owner, repo, "commits", ref, "application/vnd.github.sha")
	if err != nil {
		return "", err
	}
	defer res.Body.Close()
	switch res.StatusCode {
	case http.StatusOK:
		b, err := io.ReadAll(io.LimitReader(res.Body, 1024))
		return strings.TrimSpace(string(b)), err
	case http.StatusNotFound:
		return "", fmt.Errorf("GitHub has no repository %s/%s, or it's private and no token reached it (set GH_TOKEN or run `gh auth login`)", owner, repo)
	case http.StatusUnprocessableEntity:
		return "", fmt.Errorf("%s/%s has no branch, tag or commit `%s`", owner, repo, ref)
	}
	return "", fmt.Errorf("GitHub answered %s for %s/%s at %s", res.Status, owner, repo, ref)
}

func (g GitHub) Fetch(ctx context.Context, owner, repo, commit, subdir, dir string) error {
	res, err := g.get(ctx, owner, repo, "tarball", commit, "")
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode != http.StatusOK {
		return fmt.Errorf("GitHub answered %s for the files of %s/%s at %s", res.Status, owner, repo, commit)
	}
	n, err := extract(res.Body, subdir, dir)
	if err != nil {
		return fmt.Errorf("unpacking %s/%s: %w", owner, repo, err)
	}
	if n == 0 && subdir != "" {
		return fmt.Errorf("%s/%s has no directory `%s` at %s", owner, repo, subdir, commit)
	}
	return nil
}

func (g GitHub) get(ctx context.Context, owner, repo, kind, ref, accept string) (*http.Response, error) {
	var segs []string
	for _, s := range strings.Split(ref, "/") {
		segs = append(segs, url.PathEscape(s))
	}
	u := fmt.Sprintf("%s/repos/%s/%s/%s/%s", strings.TrimSuffix(g.API, "/"), url.PathEscape(owner), url.PathEscape(repo), kind, strings.Join(segs, "/"))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ghud")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	if g.Token != "" {
		req.Header.Set("Authorization", "Bearer "+g.Token)
	}
	c := g.HTTP
	if c == nil {
		c = http.DefaultClient
	}
	return c.Do(req)
}

// extract unpacks a GitHub tarball's files under subdir ("" for all) into
// dir, dropping the tarball's top directory. It returns how many entries
// it wrote.
func extract(r io.Reader, subdir, dir string) (int, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return 0, err
	}
	tr := tar.NewReader(gz)
	prefix := ""
	if subdir != "" {
		prefix = path.Clean(subdir) + "/"
	}
	var n int
	var budget int64 = maxExtract
	for {
		hdr, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return n, nil
		}
		if err != nil {
			return n, err
		}
		_, rel, ok := strings.Cut(hdr.Name, "/")
		if !ok {
			continue
		}
		if !strings.HasPrefix(rel+"/", prefix) {
			continue
		}
		rel = strings.TrimPrefix(rel, prefix)
		if rel == "" || rel == "/" || hdr.Typeflag == tar.TypeXGlobalHeader {
			continue
		}
		rel = path.Clean(rel)
		if !filepath.IsLocal(filepath.FromSlash(rel)) {
			return n, fmt.Errorf("%s points outside the Plugin", hdr.Name)
		}
		target := filepath.Join(dir, filepath.FromSlash(rel))
		switch hdr.Typeflag {
		case tar.TypeDir:
			err = os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			if budget -= hdr.Size; budget < 0 {
				return n, errors.New("the repository is too large")
			}
			err = writeFile(target, tr, hdr.FileInfo().Mode())
		case tar.TypeSymlink:
			link := filepath.FromSlash(hdr.Linkname)
			if filepath.IsAbs(link) || !filepath.IsLocal(filepath.Join(filepath.Dir(filepath.FromSlash(rel)), link)) {
				return n, fmt.Errorf("link %s points outside the Plugin", hdr.Name)
			}
			if err = os.MkdirAll(filepath.Dir(target), 0o755); err == nil {
				err = os.Symlink(hdr.Linkname, target)
			}
		default:
			continue
		}
		if err != nil {
			return n, err
		}
		n++
	}
}

func writeFile(target string, r io.Reader, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return err
	}
	perm := os.FileMode(0o644)
	if mode&0o111 != 0 {
		perm = 0o755
	}
	f, err := os.OpenFile(target, os.O_CREATE|os.O_EXCL|os.O_WRONLY, perm)
	if err != nil {
		return err
	}
	_, err = io.Copy(f, r)
	return errors.Join(err, f.Close())
}
