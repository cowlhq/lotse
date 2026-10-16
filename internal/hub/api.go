package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
	"lotse.sh/internal/version"
)

// apiServer implements the API as api/openapi.yaml describes it.
type apiServer struct{ *hub }

var _ api.StrictServerInterface = apiServer{}

func (h *hub) apiHandler(mux *http.ServeMux) {
	strict := api.NewStrictHandlerWithOptions(apiServer{h}, nil, api.StrictHTTPServerOptions{
		RequestErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
		ResponseErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			h.log.Error("API request failed", "path", r.URL.Path, "err", err)
			writeError(w, http.StatusInternalServerError, "the hub could not answer")
		},
	})
	api.HandlerWithOptions(strict, api.StdHTTPServerOptions{
		BaseRouter: mux,
		ErrorHandlerFunc: func(w http.ResponseWriter, r *http.Request, err error) {
			writeError(w, http.StatusBadRequest, err.Error())
		},
	})
}

func (s apiServer) GetHealth(ctx context.Context, _ api.GetHealthRequestObject) (api.GetHealthResponseObject, error) {
	out := api.GetHealth200JSONResponse{
		Status:  "ok",
		Version: version.Version,
		Setup:   s.setupStep(),
		Agents:  s.agents.count(),
	}
	if s.db != nil {
		d := api.HealthDatabase(s.db.Dialect().String())
		out.Database = &d
	}
	return out, nil
}

func (s apiServer) ListHosts(ctx context.Context, _ api.ListHostsRequestObject) (api.ListHostsResponseObject, error) {
	hosts, err := s.db.Hosts(ctx)
	if err != nil {
		return nil, err
	}
	t, err := s.treeFor(ctx)
	if err != nil {
		return nil, err
	}
	out := make(api.ListHosts200JSONResponse, 0, len(hosts))
	for _, h := range hosts {
		if !visible(ctx, t, "host", h.ID) {
			continue
		}
		sum := api.HostSummary{Host: s.toHost(ctx, h)}
		latest, err := s.db.LatestHostSample(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		sum.Latest = toHostReading(latest)
		fss, err := s.db.Filesystems(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		for _, fs := range fss {
			if fs.Used != nil && fs.Total != nil && *fs.Total > 0 {
				p := 100 * float64(*fs.Used) / float64(*fs.Total)
				if sum.Disk == nil || p > *sum.Disk {
					sum.Disk = &p
				}
			}
		}
		cs, err := s.db.Containers(ctx, h.ID)
		if err != nil {
			return nil, err
		}
		for _, c := range cs {
			if c.GoneAt != nil || !visible(ctx, t, "container", c.ID) {
				continue
			}
			sum.Containers.Total++
			if c.State == "running" {
				sum.Containers.Running++
			}
			// Docker keeps the last health of a stopped container; only a
			// running one can be unhealthy.
			if c.State == "running" && c.Health == "unhealthy" {
				sum.Containers.Unhealthy++
			}
		}
		out = append(out, sum)
	}
	return out, nil
}

func (s apiServer) GetHost(ctx context.Context, req api.GetHostRequestObject) (api.GetHostResponseObject, error) {
	h, err := s.db.Host(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "host", req.Id)
	}
	if notFound(err) {
		return api.GetHost404JSONResponse{Error: "no such host"}, nil
	}
	if err != nil {
		return nil, err
	}
	out := api.HostDetail{Host: s.toHost(ctx, h), Filesystems: []api.Filesystem{}, Containers: []api.Container{}}
	if sys, ok := systemOf(h); ok {
		out.System = toSystem(sys)
	}
	latest, err := s.db.LatestHostSample(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	out.Latest = toHostReading(latest)
	fss, err := s.db.Filesystems(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	for _, fs := range fss {
		out.Filesystems = append(out.Filesystems, api.Filesystem{
			Id: fs.ID, Mount: fs.Mount, Device: fs.Device, Fstype: fs.FSType,
			LastSeen: fs.LastSeen, Used: fs.Used, Total: fs.Total,
		})
	}
	cs, err := s.db.Containers(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	readings, err := s.db.LatestContainerSamples(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		return nil, err
	}
	for _, c := range cs {
		// A container its label puts in an environment the request does
		// not see is not shown with its host.
		if restricted(ctx) && !t.sees("container", c.ID, scopeOf(ctx)) {
			continue
		}
		ac := toContainer(c, h.Name)
		ac.Ignored = ptr(ignored(t, c.ID))
		if x, ok := readings[c.ID]; ok {
			ac.Latest = toContainerReading(x)
		}
		out.Containers = append(out.Containers, ac)
	}
	return api.GetHost200JSONResponse(out), nil
}

func (s apiServer) GetHostSeries(ctx context.Context, req api.GetHostSeriesRequestObject) (api.GetHostSeriesResponseObject, error) {
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.GetHostSeries400JSONResponse{Error: msg}, nil
	}
	_, err := s.db.Host(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "host", req.Id)
	}
	if notFound(err) {
		return api.GetHostSeries404JSONResponse{Error: "no such host"}, nil
	}
	if err != nil {
		return nil, err
	}
	sr, err := s.series(ctx, "host", req.Id, req.Params.From, req.Params.To, req.Params.Points, s.hostInterval)
	return api.GetHostSeries200JSONResponse(sr), err
}

func (s apiServer) GetFilesystemSeries(ctx context.Context, req api.GetFilesystemSeriesRequestObject) (api.GetFilesystemSeriesResponseObject, error) {
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.GetFilesystemSeries400JSONResponse{Error: msg}, nil
	}
	host, err := s.db.FilesystemHost(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "host", host)
	}
	if notFound(err) {
		return api.GetFilesystemSeries404JSONResponse{Error: "no such filesystem"}, nil
	}
	if err != nil {
		return nil, err
	}
	sr, err := s.series(ctx, "filesystem", req.Id, req.Params.From, req.Params.To, req.Params.Points, s.hostInterval)
	return api.GetFilesystemSeries200JSONResponse(sr), err
}

func (s apiServer) GetContainer(ctx context.Context, req api.GetContainerRequestObject) (api.GetContainerResponseObject, error) {
	c, err := s.db.Container(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "container", req.Id)
	}
	if notFound(err) {
		return api.GetContainer404JSONResponse{Error: "no such container"}, nil
	}
	if err != nil {
		return nil, err
	}
	h, err := s.db.Host(ctx, c.HostID)
	if err != nil {
		return nil, err
	}
	out := toContainer(c, h.Name)
	t, err := s.loadTree(ctx)
	if err != nil {
		return nil, err
	}
	out.Ignored = ptr(ignored(t, c.ID))
	readings, err := s.db.LatestContainerSamples(ctx, h.ID)
	if err != nil {
		return nil, err
	}
	if x, ok := readings[c.ID]; ok {
		out.Latest = toContainerReading(x)
	}
	return api.GetContainer200JSONResponse(out), nil
}

func (s apiServer) GetContainerSeries(ctx context.Context, req api.GetContainerSeriesRequestObject) (api.GetContainerSeriesResponseObject, error) {
	if msg := checkPoints(req.Params.Points); msg != "" {
		return api.GetContainerSeries400JSONResponse{Error: msg}, nil
	}
	_, err := s.db.Container(ctx, req.Id)
	if err == nil {
		err = s.see(ctx, "container", req.Id)
	}
	if notFound(err) {
		return api.GetContainerSeries404JSONResponse{Error: "no such container"}, nil
	}
	if err != nil {
		return nil, err
	}
	sr, err := s.series(ctx, "container", req.Id, req.Params.From, req.Params.To, req.Params.Points, s.containerInterval)
	return api.GetContainerSeries200JSONResponse(sr), err
}

func (s apiServer) ListEvents(ctx context.Context, req api.ListEventsRequestObject) (api.ListEventsResponseObject, error) {
	kind, idText, _ := strings.Cut(req.Params.Subject, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if (kind != "host" && kind != "container" && kind != "check") || err != nil {
		return api.ListEvents400JSONResponse{Error: "subject must be host:<id>, container:<id> or check:<id>"}, nil
	}
	if err := s.see(ctx, kind, id); notFound(err) {
		return api.ListEvents200JSONResponse{}, nil
	} else if err != nil {
		return nil, err
	}
	from, to := timeRange(req.Params.From, req.Params.To)
	events, err := s.db.Events(ctx, kind, id, from, to)
	if err != nil {
		return nil, err
	}
	out := make(api.ListEvents200JSONResponse, 0, len(events))
	for _, e := range events {
		data := map[string]any{}
		json.Unmarshal([]byte(e.Data), &data)
		out = append(out, api.Event{
			Id: e.ID, Ts: e.TS, SubjectKind: api.EventSubjectKind(e.SubjectKind),
			SubjectId: e.SubjectID, Kind: e.Kind, Data: data,
		})
	}
	return out, nil
}

// series reads samples in steps no shorter than the interval they were
// taken at, and long enough to stay below the points asked for.
func (s apiServer) series(ctx context.Context, kind string, id int64, from, to *int64, points *int, interval time.Duration) (api.Series, error) {
	f, t := timeRange(from, to)
	n := int64(cmp.Or(ptrValue(points), 600))
	// Older readings are kept as averages only: read the finest that
	// reaches back far enough.
	res, err := s.db.Resolution(ctx, kind, id, f)
	if err != nil {
		return api.Series{}, err
	}
	step := max(int64(interval/time.Second), (t-f+n-1)/n, store.ResolutionSeconds[res])
	sr, err := s.db.Series(ctx, kind, res, id, f, t, step)
	if err != nil {
		return api.Series{}, err
	}
	return api.Series{Time: sr.Time, Step: step, Columns: sr.Columns}, nil
}

// checkPoints checks what oapi-codegen does not: the bounds in the spec.
func checkPoints(points *int) string {
	if points != nil && (*points < 1 || *points > 5000) {
		return "points must be between 1 and 5000"
	}
	return ""
}

// timeRange defaults to the hour up to now.
func timeRange(from, to *int64) (int64, int64) {
	t := time.Now().Unix()
	if to != nil {
		t = *to
	}
	f := t - 3600
	if from != nil {
		f = *from
	}
	return f, max(t, f)
}

func ptrValue[T any](p *T) T {
	if p == nil {
		var zero T
		return zero
	}
	return *p
}

func (s apiServer) toHost(ctx context.Context, h store.Host) api.Host {
	revoked, err := s.db.AgentRevoked(ctx, h.ID)
	if err != nil {
		s.log.Error("cannot tell whether a host's agent is revoked", "host_id", h.ID, "err", err)
	}
	return api.Host{
		AgentRevoked: revoked,
		Waiting:      h.Waiting(),
		Id:           h.ID, Name: h.Name, Hostname: h.Hostname, Os: h.OS, Platform: h.Platform,
		Kernel: h.Kernel, Arch: h.Arch, CpuModel: h.CPUModel, Cores: h.Cores,
		MemTotal: h.MemTotal, BootTime: h.BootTime, AgentVersion: h.AgentVersion,
		DockerVersion: h.DockerVersion, FirstSeen: h.FirstSeen, LastSeen: h.LastSeen,
		Online: s.agents.connected(h.ID), EnvironmentId: h.EnvironmentID,
		ClockOffset:    nonZero(s.clocks.get(h.ID)),
		RebootRequired: rebootRequired(h),
	}
}

func rebootRequired(h store.Host) *bool {
	if s, ok := systemOf(h); ok && s.RebootRequired {
		return &s.RebootRequired
	}
	return nil
}

func nonZero(v int64) *int64 {
	if v == 0 {
		return nil
	}
	return &v
}

func toHostReading(x *store.HostSample) *api.HostReading {
	if x == nil {
		return nil
	}
	return &api.HostReading{
		Ts: x.TS, Cpu: x.CPU, Iowait: x.IOWait, Steal: x.Steal,
		Load1: x.Load1, Load5: x.Load5, Load15: x.Load15,
		MemUsed: x.MemUsed, MemAvailable: x.MemAvailable, SwapUsed: x.SwapUsed, SwapTotal: x.SwapTotal,
		NetRx: x.NetRx, NetTx: x.NetTx, DiskRead: x.DiskRead, DiskWrite: x.DiskWrite, Uptime: x.Uptime,
	}
}

func toContainer(c store.Container, hostName string) api.Container {
	labels := c.Labels
	if labels == nil {
		labels = map[string]string{}
	}
	return api.Container{
		Id: c.ID, HostId: c.HostID, HostName: hostName, Slug: c.Slug, DockerId: c.DockerID,
		Name: c.Name, Image: c.Image, ComposeProject: c.ComposeProject,
		ComposeService: c.ComposeService, State: c.State, Health: c.Health,
		ExitCode: c.ExitCode, Restarts: c.Restarts, StartedAt: c.StartedAt, Labels: labels,
		FirstSeen: c.FirstSeen, LastSeen: c.LastSeen, GoneAt: c.GoneAt,
	}
}

func toContainerReading(x store.ContainerSample) *api.ContainerReading {
	return &api.ContainerReading{
		Ts: x.TS, Cpu: x.CPU, Mem: x.Mem, MemLimit: x.MemLimit, NetRx: x.NetRx, NetTx: x.NetTx,
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, map[string]string{"error": message})
}

// ignored says whether lotse leaves a container alone.
func ignored(t *tree, id int64) bool {
	v, _ := t.value(t.chain("container", id), "ignore")
	return string(v) == "true"
}
