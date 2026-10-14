package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"lotse.sh/internal/apply"
)

// The command line for configuration as code (design 9.2): lotse apply
// sends a lotse.yaml to the hub, lotse export writes a scope as one. Both
// take the hub's address and an API token from flags or from LOTSE_HUB and
// LOTSE_TOKEN.

type remote struct {
	hub, token string
	insecure   bool
}

func (r *remote) flags(fs *flag.FlagSet) {
	fs.StringVar(&r.hub, "hub", os.Getenv("LOTSE_HUB"), "the hub's address (LOTSE_HUB)")
	fs.StringVar(&r.token, "token", os.Getenv("LOTSE_TOKEN"), "an API token, lotse_tok_… (LOTSE_TOKEN)")
	fs.BoolVar(&r.insecure, "insecure", os.Getenv("LOTSE_INSECURE") == "true", "send the token to a hub on another machine without TLS")
}

// check refuses to send a token in the clear, unless to this machine or
// when told to (decision 0006).
func (r *remote) check() error {
	if r.hub == "" || r.token == "" {
		return errors.New("--hub and --token are needed, or LOTSE_HUB and LOTSE_TOKEN")
	}
	u, err := url.Parse(r.hub)
	if err != nil || u.Host == "" {
		return fmt.Errorf("%q is not a hub's address", r.hub)
	}
	local := u.Hostname() == "localhost" || u.Hostname() == "127.0.0.1" || u.Hostname() == "::1"
	if u.Scheme == "http" && !local && !r.insecure {
		return fmt.Errorf("%s is not https; the token would go in the clear (--insecure to send it anyway)", r.hub)
	}
	return nil
}

func (r *remote) do(ctx context.Context, method, path string, body []byte) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(r.hub, "/")+path, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.token)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	res, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(res.Body, 32<<20))
	if res.StatusCode/100 != 2 {
		var e struct{ Error string }
		if json.Unmarshal(b, &e) == nil && e.Error != "" {
			return nil, errors.New(e.Error)
		}
		return nil, fmt.Errorf("the hub answered %s", res.Status)
	}
	return b, nil
}

func applyCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("apply", flag.ExitOnError)
	var r remote
	r.flags(fs)
	file := fs.String("f", "lotse.yaml", "the file to apply; - for standard input")
	dry := fs.Bool("dry-run", false, "show what would change, and change nothing")
	prune := fs.Bool("prune", false, "also delete, within --scope, what the file does not name")
	scope := fs.String("scope", "", "the scope to prune, such as acme/shop; the token's if not given")
	fs.Parse(args)
	if err := r.check(); err != nil {
		return err
	}
	var b []byte
	var err error
	if *file == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(*file)
	}
	if err != nil {
		return err
	}
	doc, err := apply.Parse(b)
	if err != nil {
		return fmt.Errorf("%s: %w", *file, err)
	}
	if err := doc.ResolveFromEnv(os.Getenv); err != nil {
		return fmt.Errorf("from_env: %w", err)
	}
	body, _ := json.Marshal(doc)
	q := url.Values{}
	if *dry {
		q.Set("dry_run", "true")
	}
	if *prune {
		q.Set("prune", "true")
		if *scope != "" {
			q.Set("scope", *scope)
		}
	}
	out, err := r.do(ctx, http.MethodPost, "/api/v1/apply?"+q.Encode(), body)
	if err != nil {
		return err
	}
	var res struct {
		Changes []struct {
			Action, Kind, Path, Field string
			Old, New                  json.RawMessage
			Last                      *struct {
				At      int64
				By, Via string
			}
		}
		Warnings []string
		Applied  bool
	}
	if err := json.Unmarshal(out, &res); err != nil {
		return err
	}
	signs := map[string]string{"create": "+", "update": "~", "delete": "-"}
	for _, c := range res.Changes {
		line := fmt.Sprintf("%s %-7s %-11s %s", signs[c.Action], c.Action, c.Kind, c.Path)
		if c.Field != "" {
			line += "  " + c.Field
		}
		switch {
		case c.Action == "update" && len(c.Old) > 0:
			line += fmt.Sprintf(": %s → %s", c.Old, c.New)
		case c.Action == "create" && len(c.New) > 0 && c.Kind == "setting":
			line += ": " + string(c.New)
		}
		if c.Last != nil {
			line += fmt.Sprintf("  (changed %s by %s via %s)", time.Unix(c.Last.At, 0).Format("2006-01-02"), c.Last.By, c.Last.Via)
		}
		fmt.Println(line)
	}
	for _, w := range res.Warnings {
		fmt.Println("! " + w)
	}
	switch {
	case len(res.Changes) == 0:
		fmt.Println("Nothing to change.")
	case res.Applied:
		fmt.Printf("Applied %d changes.\n", len(res.Changes))
	default:
		fmt.Printf("%d changes; run without --dry-run to make them.\n", len(res.Changes))
	}
	return nil
}

func exportCommand(ctx context.Context, args []string) error {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	var r remote
	r.flags(fs)
	fs.Parse(args)
	if err := r.check(); err != nil {
		return err
	}
	q := url.Values{}
	if fs.NArg() > 0 {
		q.Set("scope", fs.Arg(0))
	}
	out, err := r.do(ctx, http.MethodGet, "/api/v1/export?"+q.Encode(), nil)
	if err != nil {
		return err
	}
	_, err = os.Stdout.Write(out)
	return err
}
