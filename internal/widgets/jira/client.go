package jira

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/oronbz/ghud/internal/widget"
)

// client calls one Jira site's REST API with Basic auth.
type client struct {
	http         *http.Client
	base         string
	email, token string
	// now is when the call is made, for rate limit pauses given in seconds.
	now time.Time
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

func (c client) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	var r io.Reader
	if body != nil {
		r = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return nil, err
	}
	req.SetBasicAuth(c.email, c.token)
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

// retryAt is when Jira says to retry: Retry-After seconds from now, else a minute from now.
func (c client) retryAt(h http.Header) time.Time {
	if s, err := strconv.Atoi(h.Get("Retry-After")); err == nil {
		return c.now.Add(time.Duration(s) * time.Second)
	}
	return c.now.Add(rateLimitWait)
}

// message is Jira's own explanation of a failed request, else its status.
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

// authError is Jira turning down the credentials.
type authError struct{ msg string }

func (e authError) Error() string { return "Jira rejected the token: " + e.msg }
