// Package cli implements cmd/repose: the API client, project resolution,
// certificate management, and every command in docs/workstreams/07-cli.md.
package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// APIError is the decoded {error: {code, message, detail}} envelope from
// docs/interfaces/api.md.
type APIError struct {
	Code    string         `json:"code"`
	Message string         `json:"message"`
	Detail  map[string]any `json:"detail,omitempty"`
	Status  int            `json:"-"`
	// RetryAfter is a 429's Retry-After header, zero when absent.
	RetryAfter time.Duration `json:"-"`
}

func (e *APIError) Error() string { return fmt.Sprintf("%s: %s", e.Code, e.Message) }

// Is lets errors.Is(err, apiCode(...)) match on the API error code.
func (e *APIError) Is(target error) bool {
	t, ok := target.(codeError)
	return ok && e.Code == string(t)
}

type codeError string

func (c codeError) Error() string { return string(c) }

// TokenSource supplies and refreshes the bearer token. The CLI's real
// implementation is the OIDC login state; tests use a static token.
type TokenSource interface {
	// AccessToken returns a token to try. forceRefresh asks it to refresh
	// first (used after a 401).
	AccessToken(ctx context.Context, forceRefresh bool) (string, error)
}

// staticToken is a TokenSource that never refreshes, for tests and
// internal routes.
type staticToken string

func (s staticToken) AccessToken(context.Context, bool) (string, error) { return string(s), nil }

// Client is the HTTP API client (docs/interfaces/api.md).
type Client struct {
	BaseURL string
	Tokens  TokenSource
	HTTP    *http.Client
}

func newClient(baseURL string, tokens TokenSource) *Client {
	return &Client{BaseURL: strings.TrimSuffix(baseURL, "/"), Tokens: tokens, HTTP: &http.Client{Timeout: 30 * time.Second}}
}

// rateLimitBudget is how long one request waits out the api's per-user
// rate limit before the refusal reaches the caller (DECISIONS I-187). The
// api refuses before any handler runs, so a refused request of any method
// did nothing and is safe to send again.
var rateLimitBudget = 60 * time.Second

// rateLimitWait is the pause after a 429: its Retry-After, else 2 s, and
// never more than 15 s at a time.
func rateLimitWait(e *APIError) time.Duration {
	d := e.RetryAfter
	if d <= 0 {
		d = 2 * time.Second
	}
	if d > 15*time.Second {
		d = 15 * time.Second
	}
	return d
}

// do sends one request, retrying once on a 401 unauthenticated after a
// forced token refresh (07-cli.md §5.2: "on 401 with unauthenticated,
// refresh once, retry once"), and waiting out a rate_limited refusal for
// up to rateLimitBudget.
func (c *Client) do(ctx context.Context, method, path string, body any, out any) error {
	var refreshed bool
	var limitedSince time.Time
	for {
		var reader io.Reader
		if body != nil {
			b, err := json.Marshal(body)
			if err != nil {
				return err
			}
			reader = bytes.NewReader(b)
		}
		req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, reader)
		if err != nil {
			return err
		}
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		if c.Tokens != nil {
			tok, err := c.Tokens.AccessToken(ctx, refreshed)
			if err != nil {
				return &notLoggedInError{cause: err}
			}
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return &unreachableError{cause: err}
		}
		apiErr, decodeErr := readResponse(resp, out)
		if apiErr != nil && apiErr.Code == "unauthenticated" && !refreshed && c.Tokens != nil {
			refreshed = true
			continue
		}
		if apiErr != nil && apiErr.Code == "rate_limited" {
			if limitedSince.IsZero() {
				limitedSince = time.Now()
			}
			if time.Since(limitedSince) < rateLimitBudget {
				if err := sleepOrDone(ctx, rateLimitWait(apiErr)); err != nil {
					return err
				}
				continue
			}
		}
		if decodeErr != nil {
			return decodeErr
		}
		if apiErr != nil {
			return apiErr
		}
		return nil
	}
}

func readResponse(resp *http.Response, out any) (*APIError, error) {
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		var env struct {
			Error APIError `json:"error"`
		}
		if err := json.Unmarshal(b, &env); err != nil || env.Error.Code == "" {
			return &APIError{Code: "internal", Message: string(b), Status: resp.StatusCode}, nil
		}
		env.Error.Status = resp.StatusCode
		if s, err := strconv.Atoi(strings.TrimSpace(resp.Header.Get("Retry-After"))); err == nil && s > 0 {
			env.Error.RetryAfter = time.Duration(s) * time.Second
		}
		return &env.Error, nil
	}
	if out == nil || len(b) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(b, out); err != nil {
		return nil, err
	}
	return nil, nil
}

// unreachableError wraps a transport failure (07-cli.md §6: "API
// unreachable").
type unreachableError struct{ cause error }

func (e *unreachableError) Error() string { return e.cause.Error() }
func (e *unreachableError) Unwrap() error { return e.cause }

// notLoggedInError wraps a token-source failure (refresh token invalid or
// absent).
type notLoggedInError struct{ cause error }

func (e *notLoggedInError) Error() string { return e.cause.Error() }
func (e *notLoggedInError) Unwrap() error { return e.cause }

func (c *Client) get(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodGet, path, nil, out)
}
func (c *Client) post(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPost, path, body, out)
}
func (c *Client) put(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPut, path, body, out)
}
func (c *Client) patch(ctx context.Context, path string, body, out any) error {
	return c.do(ctx, http.MethodPatch, path, body, out)
}
func (c *Client) delete(ctx context.Context, path string, out any) error {
	return c.do(ctx, http.MethodDelete, path, nil, out)
}

// getNDJSON is GET /projects/:id/logs's shape (api.md: "JSON lines", the
// fake serves it as application/x-ndjson): a stream of concatenated JSON
// values rather than one JSON array, decoded one at a time into a slice
// built from a zero-value template via a factory so callers keep type
// safety without generics duplicating this per type.
func (c *Client) getNDJSON(ctx context.Context, path string, decodeLine func(dec *json.Decoder) error) error {
	var refreshed bool
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.BaseURL+path, nil)
		if err != nil {
			return err
		}
		if c.Tokens != nil {
			tok, err := c.Tokens.AccessToken(ctx, refreshed)
			if err != nil {
				return &notLoggedInError{cause: err}
			}
			req.Header.Set("Authorization", "Bearer "+tok)
		}
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return &unreachableError{cause: err}
		}
		if resp.StatusCode >= 400 {
			apiErr, decodeErr := readResponse(resp, nil)
			if decodeErr != nil {
				return decodeErr
			}
			if apiErr.Code == "unauthenticated" && !refreshed && c.Tokens != nil {
				refreshed = true
				continue
			}
			return apiErr
		}
		defer func() { _ = resp.Body.Close() }()
		dec := json.NewDecoder(resp.Body)
		for dec.More() {
			if err := decodeLine(dec); err != nil {
				return err
			}
		}
		return nil
	}
}
