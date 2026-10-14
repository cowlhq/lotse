package client

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Hub reads from one hub with one API token. (Client and
// ClientWithResponses, made from the API's description, are the plain
// calls it is built on.)
type Hub struct {
	api *ClientWithResponses
}

// HubOption changes how a Hub works.
type HubOption func(*options)

type options struct {
	http *http.Client
}

// UseHTTPClient sends requests through c instead of a client with a
// timeout of 30 seconds.
func UseHTTPClient(c *http.Client) HubOption { return func(o *options) { o.http = c } }

// New makes a Hub for the hub at hub (https://lotse.example.com), with an
// API token (lotse_tok_…) of either kind.
func New(hub, token string, opts ...HubOption) (*Hub, error) {
	o := options{http: &http.Client{Timeout: 30 * time.Second}}
	for _, f := range opts {
		f(&o)
	}
	if token == "" {
		return nil, errors.New("lotse: a client needs an API token")
	}
	auth := func(_ context.Context, req *http.Request) error {
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("User-Agent", "lotse.sh/client")
		return nil
	}
	c, err := NewClientWithResponses(strings.TrimSuffix(hub, "/"), WithHTTPClient(o.http), WithRequestEditorFn(auth))
	if err != nil {
		return nil, err
	}
	return &Hub{api: c}, nil
}

// APIError is what the hub answered when it did not answer what was asked.
type APIError struct {
	// Status is the HTTP status, such as 401 for a token that does not
	// work, or 404 for something not there or not in the token's scope.
	Status  int
	Message string
}

func (e *APIError) Error() string { return fmt.Sprintf("lotse: %d: %s", e.Status, e.Message) }

// result takes a value out of a response, or makes its error.
func result[T any](res *http.Response, body []byte, value *T, err error) (T, error) {
	var zero T
	if err != nil {
		return zero, err
	}
	if value != nil && res.StatusCode == http.StatusOK {
		return *value, nil
	}
	e := &APIError{Status: res.StatusCode, Message: res.Status}
	var msg struct{ Error string }
	if json.Unmarshal(body, &msg) == nil && msg.Error != "" {
		e.Message = msg.Error
	}
	return zero, e
}

// Status is how everything the token sees stands, in figures.
func (c *Hub) Status(ctx context.Context) (Status, error) {
	r, err := c.api.ReadStatusWithResponse(ctx)
	if err != nil {
		return Status{}, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Scopes are the customers, projects and environments the token sees.
func (c *Hub) Scopes(ctx context.Context) ([]Scope, error) {
	r, err := c.api.ReadScopesWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Hosts are the hosts the token sees, with their newest reading.
func (c *Hub) Hosts(ctx context.Context) ([]HostSummary, error) {
	r, err := c.api.ReadHostsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Host is one host, with its filesystems and containers.
func (c *Hub) Host(ctx context.Context, id int64) (HostDetail, error) {
	r, err := c.api.ReadHostWithResponse(ctx, id)
	if err != nil {
		return HostDetail{}, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Range is a time range for series; points is about how many points to
// return (600 if 0).
type Range struct {
	From, To time.Time
	Points   int
}

func (r Range) params() (from, to *int64, points *int) {
	if !r.From.IsZero() {
		f := r.From.Unix()
		from = &f
	}
	if !r.To.IsZero() {
		t := r.To.Unix()
		to = &t
	}
	if r.Points > 0 {
		points = &r.Points
	}
	return
}

// HostSeries are a host's readings over a range: columns such as cpu,
// mem_used and net_rx, one value per step.
func (c *Hub) HostSeries(ctx context.Context, id int64, rg Range) (Series, error) {
	from, to, points := rg.params()
	r, err := c.api.ReadHostSeriesWithResponse(ctx, id, &ReadHostSeriesParams{From: from, To: to, Points: points})
	if err != nil {
		return Series{}, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// ContainerSeries are a container's readings over a range.
func (c *Hub) ContainerSeries(ctx context.Context, id int64, rg Range) (Series, error) {
	from, to, points := rg.params()
	r, err := c.api.ReadContainerSeriesWithResponse(ctx, id, &ReadContainerSeriesParams{From: from, To: to, Points: points})
	if err != nil {
		return Series{}, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Checks are the checks the token sees, with their state.
func (c *Hub) Checks(ctx context.Context) ([]Check, error) {
	r, err := c.api.ReadChecksWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// CheckSeries are a check's response time and share of passed runs over a
// range.
func (c *Hub) CheckSeries(ctx context.Context, id int64, rg Range) (Series, error) {
	from, to, points := rg.params()
	r, err := c.api.ReadCheckSeriesWithResponse(ctx, id, &ReadCheckSeriesParams{From: from, To: to, Points: points})
	if err != nil {
		return Series{}, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Alerts are the alerts that are not OK now, the worst first.
func (c *Hub) Alerts(ctx context.Context) ([]AlertState, error) {
	r, err := c.api.ReadAlertsWithResponse(ctx)
	if err != nil {
		return nil, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// AlertHistory are the transitions of alerts in a range, newest first, at
// most 500.
func (c *Hub) AlertHistory(ctx context.Context, rg Range) ([]Alert, error) {
	from, to, _ := rg.params()
	r, err := c.api.ReadAlertHistoryWithResponse(ctx, &ReadAlertHistoryParams{From: from, To: to})
	if err != nil {
		return nil, err
	}
	return result(r.HTTPResponse, r.Body, r.JSON200, nil)
}

// Export is a scope as lotse.yaml, such as acme/shop; "" for everything
// the token sees.
func (c *Hub) Export(ctx context.Context, scope string) ([]byte, error) {
	params := &ExportScopeParams{}
	if scope != "" {
		params.Scope = &scope
	}
	// The answer is YAML, which is kept as it is.
	res, err := c.api.ClientInterface.ExportScope(ctx, params)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == http.StatusOK {
		return body, nil
	}
	return result[[]byte](res, body, nil, nil)
}
