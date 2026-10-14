package hub

import (
	"context"
	"errors"
	"strings"
	"time"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// The read API (design 3.1, decision 0020): /api/v1, for other programs.
// It answers what the web interface's API answers, but keeps its shape, and
// shows only what is in the scope of the token it is asked with.

// readScope is the path a request may read below: a token's scope, or
// everything for someone logged in.
func readScope(ctx context.Context) string {
	if t, ok := tokenOf(ctx); ok {
		return t.ScopePath
	}
	return ""
}

func within(path, scope string) bool {
	return scope == "" || path == scope || strings.HasPrefix(path, scope+"/")
}

// envPaths are the paths of the environments, by ID.
func (s apiServer) envPaths(ctx context.Context) (map[int64]string, error) {
	list, err := s.db.Scopes(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64]string{}
	for _, x := range list {
		out[x.ID] = x.Path
	}
	return out, nil
}

// hostVisible says whether a host is in the scope: by its environment, and
// one without an environment only to those who see everything.
func hostVisible(h api.Host, paths map[int64]string, scope string) bool {
	if scope == "" {
		return true
	}
	return h.EnvironmentId != nil && within(paths[*h.EnvironmentId], scope)
}

var errNotVisible = errors.New("not in the token's scope")

// seeHost checks that a host is there and in the scope.
func (s apiServer) seeHost(ctx context.Context, id int64) error {
	h, err := s.db.Host(ctx, id)
	if err != nil {
		return err
	}
	scope := readScope(ctx)
	if scope == "" {
		return nil
	}
	paths, err := s.envPaths(ctx)
	if err != nil {
		return err
	}
	if h.EnvironmentID == nil || !within(paths[*h.EnvironmentID], scope) {
		return errNotVisible
	}
	return nil
}

func notFound(err error) bool {
	return errors.Is(err, store.ErrNotFound) || errors.Is(err, errNotVisible)
}

func (s apiServer) ReadStatus(ctx context.Context, _ api.ReadStatusRequestObject) (api.ReadStatusResponseObject, error) {
	hosts, err := s.ReadHosts(ctx, api.ReadHostsRequestObject{})
	if err != nil {
		return nil, err
	}
	checks, err := s.ReadChecks(ctx, api.ReadChecksRequestObject{})
	if err != nil {
		return nil, err
	}
	alerts, err := s.ReadAlerts(ctx, api.ReadAlertsRequestObject{})
	if err != nil {
		return nil, err
	}
	out := api.ReadStatus200JSONResponse{Level: api.StatusLevelOk}
	for _, h := range hosts.(api.ReadHosts200JSONResponse) {
		out.Hosts++
		if h.Host.Online {
			out.HostsOnline++
		}
	}
	for _, c := range checks.(api.ReadChecks200JSONResponse) {
		out.Checks++
		if c.State == api.CheckStateUp {
			out.ChecksUp++
		}
	}
	for _, a := range alerts.(api.ReadAlerts200JSONResponse) {
		switch a.State {
		case api.AlertStateStateCrit:
			out.Critical++
			out.Level = api.StatusLevelCrit
		case api.AlertStateStateWarn:
			out.Warnings++
			if out.Level == api.StatusLevelOk {
				out.Level = api.StatusLevelWarn
			}
		}
	}
	return out, nil
}

func (s apiServer) ReadScopes(ctx context.Context, _ api.ReadScopesRequestObject) (api.ReadScopesResponseObject, error) {
	list, err := s.db.Scopes(ctx)
	if err != nil {
		return nil, err
	}
	scope := readScope(ctx)
	out := api.ReadScopes200JSONResponse{}
	for _, x := range list {
		if within(x.Path, scope) {
			out = append(out, toScope(x))
		}
	}
	return out, nil
}

func (s apiServer) ReadHosts(ctx context.Context, _ api.ReadHostsRequestObject) (api.ReadHostsResponseObject, error) {
	res, err := s.ListHosts(ctx, api.ListHostsRequestObject{})
	if err != nil {
		return nil, err
	}
	paths, err := s.envPaths(ctx)
	if err != nil {
		return nil, err
	}
	scope := readScope(ctx)
	out := api.ReadHosts200JSONResponse{}
	for _, h := range res.(api.ListHosts200JSONResponse) {
		if hostVisible(h.Host, paths, scope) {
			out = append(out, h)
		}
	}
	return out, nil
}

func (s apiServer) ReadHost(ctx context.Context, req api.ReadHostRequestObject) (api.ReadHostResponseObject, error) {
	if err := s.seeHost(ctx, req.Id); notFound(err) {
		return api.ReadHost404JSONResponse{Error: "no such host"}, nil
	} else if err != nil {
		return nil, err
	}
	res, err := s.GetHost(ctx, api.GetHostRequestObject{Id: req.Id})
	if err != nil {
		return nil, err
	}
	if d, ok := res.(api.GetHost200JSONResponse); ok {
		return api.ReadHost200JSONResponse(d), nil
	}
	return api.ReadHost404JSONResponse{Error: "no such host"}, nil
}

func (s apiServer) ReadHostSeries(ctx context.Context, req api.ReadHostSeriesRequestObject) (api.ReadHostSeriesResponseObject, error) {
	if err := s.seeHost(ctx, req.Id); notFound(err) {
		return api.ReadHostSeries404JSONResponse{Error: "no such host"}, nil
	} else if err != nil {
		return nil, err
	}
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.ReadHostSeries400JSONResponse{Error: msg}, nil
	}
	sr, err := s.series(ctx, "host", req.Id, req.Params.From, req.Params.To, req.Params.Points, s.hostInterval)
	return api.ReadHostSeries200JSONResponse(sr), err
}

func (s apiServer) ReadContainerSeries(ctx context.Context, req api.ReadContainerSeriesRequestObject) (api.ReadContainerSeriesResponseObject, error) {
	c, err := s.db.Container(ctx, req.Id)
	if err == nil {
		err = s.seeHost(ctx, c.HostID)
	}
	if notFound(err) {
		return api.ReadContainerSeries404JSONResponse{Error: "no such container"}, nil
	}
	if err != nil {
		return nil, err
	}
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.ReadContainerSeries400JSONResponse{Error: msg}, nil
	}
	sr, err := s.series(ctx, "container", req.Id, req.Params.From, req.Params.To, req.Params.Points, s.containerInterval)
	return api.ReadContainerSeries200JSONResponse(sr), err
}

func (s apiServer) ReadChecks(ctx context.Context, _ api.ReadChecksRequestObject) (api.ReadChecksResponseObject, error) {
	list, err := s.db.Checks(ctx, 0)
	if err != nil {
		return nil, err
	}
	scope := readScope(ctx)
	out := api.ReadChecks200JSONResponse{}
	for _, c := range list {
		if within(c.Path, scope) {
			x := s.toCheck(ctx, c)
			// The ping address is a heartbeat's secret; the read API does
			// not hand it out.
			x.PingUrl = nil
			out = append(out, x)
		}
	}
	return out, nil
}

func (s apiServer) ReadCheckSeries(ctx context.Context, req api.ReadCheckSeriesRequestObject) (api.ReadCheckSeriesResponseObject, error) {
	c, err := s.db.Check(ctx, req.Id)
	if err == nil && !within(c.Path, readScope(ctx)) {
		err = errNotVisible
	}
	if notFound(err) {
		return api.ReadCheckSeries404JSONResponse{Error: "no such check"}, nil
	}
	if err != nil {
		return nil, err
	}
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.ReadCheckSeries400JSONResponse{Error: msg}, nil
	}
	sr, err := s.series(ctx, "check", req.Id, req.Params.From, req.Params.To, req.Params.Points, 10*time.Second)
	return api.ReadCheckSeries200JSONResponse(sr), err
}

func (s apiServer) ReadAlerts(ctx context.Context, _ api.ReadAlertsRequestObject) (api.ReadAlertsResponseObject, error) {
	res, err := s.ListAlerts(ctx, api.ListAlertsRequestObject{})
	if err != nil {
		return nil, err
	}
	scope := readScope(ctx)
	out := api.ReadAlerts200JSONResponse{}
	for _, a := range res.(api.ListAlerts200JSONResponse) {
		if within(a.SubjectPath, scope) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (s apiServer) ReadAlertHistory(ctx context.Context, req api.ReadAlertHistoryRequestObject) (api.ReadAlertHistoryResponseObject, error) {
	res, err := s.ListAlertHistory(ctx, api.ListAlertHistoryRequestObject{Params: api.ListAlertHistoryParams{From: req.Params.From, To: req.Params.To}})
	if err != nil {
		return nil, err
	}
	scope := readScope(ctx)
	out := api.ReadAlertHistory200JSONResponse{}
	for _, a := range res.(api.ListAlertHistory200JSONResponse) {
		if within(a.SubjectPath, scope) {
			out = append(out, a)
		}
	}
	return out, nil
}
