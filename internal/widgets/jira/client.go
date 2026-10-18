package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/oronbz/ghud/internal/widget"
)

// client calls one Jira site's REST API: with Basic auth, or with a Data
// Center personal access token as Bearer when there's no email. A Cloud
// site that turns the token down may still take it through
// api.atlassian.com, as scoped tokens need; route remembers which worked.
type client struct {
	http *http.Client
	// site is the site's base URL; api is api.atlassian.com's.
	site, api    string
	route        *route
	email, token string
	// now is when the call is made, for rate limit pauses given in seconds.
	now time.Time
}

// route is how one Widget reaches its site: the site's cloudId once looked
// up, and whether calls go through api.atlassian.com.
type route struct {
	mu      sync.Mutex
	cloudID string
	viaAPI  bool
}

func (r *route) get() (cloudID string, viaAPI bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.cloudID, r.viaAPI
}

func (r *route) set(cloudID string, viaAPI bool) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cloudID, r.viaAPI = cloudID, viaAPI
}

// rateLimitWait is the pause when Jira limits a request without saying for how long.
const rateLimitWait = time.Minute

func (c client) get(ctx context.Context, path string, out any) error {
	b, err := c.do(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(b, out); err != nil {
		return fmt.Errorf("Jira sent a reply ghud can't read: %w", err)
	}
	return nil
}

func (c client) send(ctx context.Context, method, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	_, err = c.do(ctx, method, path, body)
	return err
}

// do calls the route that worked last. When the site turns the token
// down, it looks up the site's cloudId (once) and tries api.atlassian.com,
// keeping that route if it takes the token.
func (c client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	cloudID, viaAPI := c.route.get()
	if viaAPI {
		b, err := c.call(ctx, c.viaAPI(cloudID)+path, method, body)
		if !errors.As(err, new(authError)) {
			return b, err
		}
		c.route.set(cloudID, false)
	}
	b, err := c.call(ctx, c.site+path, method, body)
	if c.email == "" || viaAPI || !errors.As(err, new(authError)) {
		return b, err
	}
	if cloudID == "" {
		if cloudID = c.cloudID(ctx); cloudID == "" {
			return b, err
		}
		c.route.set(cloudID, false)
	}
	b, apiErr := c.call(ctx, c.viaAPI(cloudID)+path, method, body)
	if errors.As(apiErr, new(authError)) {
		return b, err
	}
	c.route.set(cloudID, true)
	return b, apiErr
}

func (c client) viaAPI(cloudID string) string { return c.api + "/ex/jira/" + url.PathEscape(cloudID) }

// cloudID is the site's cloudId from its unauthenticated tenant info, or "".
func (c client) cloudID(ctx context.Context) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.site+"/_edge/tenant_info", nil)
	if err != nil {
		return ""
	}
	res, err := c.http.Do(req)
	if err != nil {
		return ""
	}
	defer res.Body.Close()
	var info struct{ CloudID string }
	if res.StatusCode != http.StatusOK || json.NewDecoder(res.Body).Decode(&info) != nil {
		return ""
	}
	return info.CloudID
}

func (c client) call(ctx context.Context, target, method string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, r)
	if err != nil {
		return nil, err
	}
	if c.email == "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	} else {
		req.SetBasicAuth(c.email, c.token)
	}
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	res, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode < 300 {
		return b, nil
	}
	msg := message(b, res.Status)
	switch res.StatusCode {
	case http.StatusUnauthorized:
		return nil, authError{msg}
	case http.StatusTooManyRequests:
		return nil, &widget.RateLimitError{Until: c.retryAt(res.Header), Err: errors.New("Jira: " + msg)}
	}
	return nil, errors.New("Jira: " + msg)
}

func (c client) retryAt(h http.Header) time.Time {
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil {
		return c.now.Add(time.Duration(s) * time.Second)
	}
	return c.now.Add(rateLimitWait)
}

func message(body []byte, status string) string {
	var e struct {
		ErrorMessages []string          `json:"errorMessages"`
		Errors        map[string]string `json:"errors"`
	}
	if json.Unmarshal(body, &e) != nil {
		return status
	}
	msgs := e.ErrorMessages
	fields := make([]string, 0, len(e.Errors))
	for f := range e.Errors {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		msgs = append(msgs, f+": "+e.Errors[f])
	}
	if len(msgs) == 0 {
		return status
	}
	return strings.Join(msgs, "; ")
}

type authError struct{ msg string }

func (e authError) Error() string { return "Jira rejected the token: " + e.msg }
