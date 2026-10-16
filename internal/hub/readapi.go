package hub

import (
	"context"

	"lotse.sh/internal/hub/api"
)

// The read API (design 3.1, decision 0020): /api/v1, for other programs.
// It answers what the web interface's API answers, but keeps its shape, and
// shows only what is in the scope of the token it is asked with.

// The read API answers what the interface's API answers, which shows a
// token only what is in its scope (access.go).

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
	res, err := s.ListScopes(ctx, api.ListScopesRequestObject{})
	if err != nil {
		return nil, err
	}
	return api.ReadScopes200JSONResponse(res.(api.ListScopes200JSONResponse)), nil
}

func (s apiServer) ReadHosts(ctx context.Context, _ api.ReadHostsRequestObject) (api.ReadHostsResponseObject, error) {
	res, err := s.ListHosts(ctx, api.ListHostsRequestObject{})
	if err != nil {
		return nil, err
	}
	return api.ReadHosts200JSONResponse(res.(api.ListHosts200JSONResponse)), nil
}

func (s apiServer) ReadHost(ctx context.Context, req api.ReadHostRequestObject) (api.ReadHostResponseObject, error) {
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
	res, err := s.GetHostSeries(ctx, api.GetHostSeriesRequestObject{Id: req.Id, Params: api.GetHostSeriesParams(req.Params)})
	if err != nil {
		return nil, err
	}
	switch r := res.(type) {
	case api.GetHostSeries200JSONResponse:
		return api.ReadHostSeries200JSONResponse(r), nil
	case api.GetHostSeries400JSONResponse:
		return api.ReadHostSeries400JSONResponse{Error: r.Error}, nil
	}
	return api.ReadHostSeries404JSONResponse{Error: "no such host"}, nil
}

func (s apiServer) ReadContainerSeries(ctx context.Context, req api.ReadContainerSeriesRequestObject) (api.ReadContainerSeriesResponseObject, error) {
	res, err := s.GetContainerSeries(ctx, api.GetContainerSeriesRequestObject{Id: req.Id, Params: api.GetContainerSeriesParams(req.Params)})
	if err != nil {
		return nil, err
	}
	switch r := res.(type) {
	case api.GetContainerSeries200JSONResponse:
		return api.ReadContainerSeries200JSONResponse(r), nil
	case api.GetContainerSeries400JSONResponse:
		return api.ReadContainerSeries400JSONResponse{Error: r.Error}, nil
	}
	return api.ReadContainerSeries404JSONResponse{Error: "no such container"}, nil
}

func (s apiServer) ReadChecks(ctx context.Context, _ api.ReadChecksRequestObject) (api.ReadChecksResponseObject, error) {
	res, err := s.ListChecks(ctx, api.ListChecksRequestObject{})
	if err != nil {
		return nil, err
	}
	out := api.ReadChecks200JSONResponse{}
	for _, x := range res.(api.ListChecks200JSONResponse) {
		// The ping address is a heartbeat's secret; the read API does not
		// hand it out.
		x.PingUrl = nil
		out = append(out, x)
	}
	return out, nil
}

func (s apiServer) ReadCheckSeries(ctx context.Context, req api.ReadCheckSeriesRequestObject) (api.ReadCheckSeriesResponseObject, error) {
	res, err := s.GetCheckSeries(ctx, api.GetCheckSeriesRequestObject{Id: req.Id, Params: api.GetCheckSeriesParams(req.Params)})
	if err != nil {
		return nil, err
	}
	switch r := res.(type) {
	case api.GetCheckSeries200JSONResponse:
		return api.ReadCheckSeries200JSONResponse(r), nil
	case api.GetCheckSeries400JSONResponse:
		return api.ReadCheckSeries400JSONResponse{Error: r.Error}, nil
	}
	return api.ReadCheckSeries404JSONResponse{Error: "no such check"}, nil
}

func (s apiServer) ReadAlerts(ctx context.Context, _ api.ReadAlertsRequestObject) (api.ReadAlertsResponseObject, error) {
	res, err := s.ListAlerts(ctx, api.ListAlertsRequestObject{})
	if err != nil {
		return nil, err
	}
	return api.ReadAlerts200JSONResponse(res.(api.ListAlerts200JSONResponse)), nil
}

func (s apiServer) ReadAlertHistory(ctx context.Context, req api.ReadAlertHistoryRequestObject) (api.ReadAlertHistoryResponseObject, error) {
	res, err := s.ListAlertHistory(ctx, api.ListAlertHistoryRequestObject{Params: api.ListAlertHistoryParams{From: req.Params.From, To: req.Params.To}})
	if err != nil {
		return nil, err
	}
	return api.ReadAlertHistory200JSONResponse(res.(api.ListAlertHistory200JSONResponse)), nil
}
