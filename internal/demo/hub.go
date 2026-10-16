package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"time"

	"lotse.sh/internal/hub"
)

// The hub runs in this process, exactly as lotse hub runs it. Setup is
// done over its API, as the browser would do it; the one-time setup token
// is read from the hub's log, where a person would find it too.

const (
	adminEmail    = "jana@example.com"
	adminPassword = "demo-password-1234"
	adminName     = "Jana Petersen"
)

// tokenCatcher passes log records on and picks the setup link out of them.
type tokenCatcher struct {
	slog.Handler
	tokens chan string
}

func (c tokenCatcher) Handle(ctx context.Context, r slog.Record) error {
	if _, token, ok := strings.Cut(r.Message, "setup?token="); ok {
		select {
		case c.tokens <- strings.TrimSpace(token):
		default:
		}
	}
	return c.Handler.Handle(ctx, r)
}

func (c tokenCatcher) WithAttrs(attrs []slog.Attr) slog.Handler {
	return tokenCatcher{c.Handler.WithAttrs(attrs), c.tokens}
}

func (c tokenCatcher) WithGroup(name string) slog.Handler {
	return tokenCatcher{c.Handler.WithGroup(name), c.tokens}
}

// startHub runs a hub until ctx ends and sets it up: SQLite, the admin and
// its address. It returns a client logged in as the admin.
// It closes done when the hub has stopped, and says on failed why, if it
// failed.
func startHub(ctx context.Context, cfg hub.Config, log *slog.Logger, failed chan<- error, done chan<- struct{}) (*client, error) {
	tokens := make(chan string, 1)
	hubLog := slog.New(tokenCatcher{log.Handler(), tokens})
	go func() {
		defer close(done)
		if err := hub.Run(ctx, cfg, hubLog); err != nil {
			failed <- fmt.Errorf("the hub stopped: %w", err)
		}
	}()
	var token string
	select {
	case token = <-tokens:
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-time.After(30 * time.Second):
		return nil, errors.New("the hub did not offer setup")
	}
	c := newClient("http://" + cfg.Listen)
	if err := c.do(ctx, "POST", "/api/setup/database", map[string]any{"token": token, "kind": "sqlite"}, nil); err != nil {
		return nil, fmt.Errorf("setup, database: %w", err)
	}
	// The hub opens the database, and then asks for the admin.
	if err := waitFor(ctx, 30*time.Second, func() bool {
		var h struct{ Setup string }
		return c.do(ctx, "GET", "/api/health", nil, &h) == nil && h.Setup == "admin"
	}); err != nil {
		return nil, fmt.Errorf("the hub did not get to the admin step: %w", err)
	}
	if err := c.do(ctx, "POST", "/api/setup/admin", map[string]any{
		"token": token, "name": adminName, "email": adminEmail, "password": adminPassword, "url": "http://" + cfg.Listen,
	}, nil); err != nil {
		return nil, fmt.Errorf("setup, admin: %w", err)
	}
	return c, nil
}

// client talks to the hub's API, with the admin's session or a token.
type client struct {
	base  string
	http  *http.Client
	token string
}

func newClient(base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{base: base, http: &http.Client{Jar: jar, Timeout: 2 * time.Minute}}
}

// do sends body as JSON and reads the answer into out, if given.
func (c *client) do(ctx context.Context, method, path string, body, out any) error {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		r = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.base+path, r)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(res.Body)
	if res.StatusCode/100 != 2 {
		return fmt.Errorf("%s %s: %s %s", method, path, res.Status, strings.TrimSpace(string(b)))
	}
	if out != nil && len(b) > 0 {
		return json.Unmarshal(b, out)
	}
	return nil
}

// waitFor asks ok every half second until it says yes.
func waitFor(ctx context.Context, timeout time.Duration, ok func() bool) error {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	for {
		if ok() {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}
