package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
	"lotse.sh/internal/update"
	"lotse.sh/internal/version"
)

func (s apiServer) updates() api.Updates {
	out := api.Updates{HubVersion: version.Version, CanUpdateHub: update.Started()}
	latest, checkedAt, checkErr := s.releases.get()
	if latest != nil {
		out.Latest = &api.Release{Version: latest.Version, Notes: latest.Notes, Url: latest.URL, PublishedAt: latest.PublishedAt}
	}
	if checkedAt > 0 {
		out.CheckedAt = &checkedAt
	}
	if checkErr != "" {
		out.CheckError = &checkErr
	}
	return out
}

func (s apiServer) GetUpdates(ctx context.Context, _ api.GetUpdatesRequestObject) (api.GetUpdatesResponseObject, error) {
	return api.GetUpdates200JSONResponse(s.updates()), nil
}

func (s apiServer) CheckUpdates(ctx context.Context, _ api.CheckUpdatesRequestObject) (api.CheckUpdatesResponseObject, error) {
	if !isAdmin(ctx) {
		return api.CheckUpdates403JSONResponse{Error: forbidden}, nil
	}
	s.releases.check(ctx)
	return api.CheckUpdates200JSONResponse(s.updates()), nil
}

// askUpdate asks a host's connected agents to update to version. It says
// whether there was an agent to ask.
func (h *hub) askUpdate(ctx context.Context, hostID int64, v string) bool {
	conns := h.agents.of(hostID)
	for _, c := range conns {
		wctx, cancel := context.WithTimeout(ctx, writeTimeout)
		wsjson.Write(wctx, c, protocol.Message{Type: protocol.TypeUpdate, Update: &protocol.Update{Version: v}})
		cancel()
	}
	return len(conns) > 0
}

func (s apiServer) UpdateHost(ctx context.Context, req api.UpdateHostRequestObject) (api.UpdateHostResponseObject, error) {
	host, err := s.db.Host(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, "host", req.Id)
	}
	if notFound(err) {
		return api.UpdateHost404JSONResponse{Error: "no such host"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.UpdateHost403JSONResponse{Error: forbidden}, nil
	}
	latest, _, _ := s.releases.get()
	switch {
	case latest == nil:
		return api.UpdateHost409JSONResponse{Error: "no release is known yet"}, nil
	case !newer(latest.Version, host.AgentVersion):
		return api.UpdateHost409JSONResponse{Error: "the agent has the newest release, or is a development build"}, nil
	case !s.askUpdate(context.WithoutCancel(ctx), host.ID, latest.Version):
		return api.UpdateHost409JSONResponse{Error: "the agent is not connected"}, nil
	}
	return api.UpdateHost202Response{}, nil
}

func (s apiServer) UpdateScope(ctx context.Context, req api.UpdateScopeRequestObject) (api.UpdateScopeResponseObject, error) {
	scope, err := s.db.Scope(ctx, req.Id)
	ok := false
	if err == nil {
		ok, err = s.mayChange(ctx, scope.Kind, scope.ID)
	}
	if notFound(err) {
		return api.UpdateScope404JSONResponse{Error: "no such scope"}, nil
	}
	if err != nil {
		return nil, err
	}
	if !ok {
		return api.UpdateScope403JSONResponse{Error: forbidden}, nil
	}
	latest, _, _ := s.releases.get()
	if latest == nil {
		return api.UpdateScope409JSONResponse{Error: "no release is known yet"}, nil
	}
	hosts, err := s.hostsBelow(ctx, scope)
	if err != nil {
		return nil, err
	}
	asked := 0
	for _, host := range hosts {
		if newer(latest.Version, host.AgentVersion) && s.askUpdate(context.WithoutCancel(ctx), host.ID, latest.Version) {
			asked++
		}
	}
	return api.UpdateScope202JSONResponse{Asked: asked}, nil
}

// hostsBelow returns the hosts in a scope's environments, or all hosts for
// the global scope.
func (h *hub) hostsBelow(ctx context.Context, scope store.Scope) ([]store.Host, error) {
	all, err := h.db.Hosts(ctx)
	if err != nil || scope.Kind == "global" {
		return all, err
	}
	scopes, err := h.db.Scopes(ctx)
	if err != nil {
		return nil, err
	}
	paths := map[int64]string{}
	for _, x := range scopes {
		paths[x.ID] = x.Path
	}
	var below []store.Host
	for _, host := range all {
		if host.EnvironmentID == nil {
			continue
		}
		p := paths[*host.EnvironmentID]
		if p == scope.Path || len(p) > len(scope.Path) && p[:len(scope.Path)+1] == scope.Path+"/" {
			below = append(below, host)
		}
	}
	return below, nil
}

func (s apiServer) UpdateHub(ctx context.Context, _ api.UpdateHubRequestObject) (api.UpdateHubResponseObject, error) {
	if !isAdmin(ctx) {
		return api.UpdateHub403JSONResponse{Error: forbidden}, nil
	}
	if !update.Started() {
		return api.UpdateHub409JSONResponse{Error: "the hub does not run under the starter (lotse start hub), which would run the new version"}, nil
	}
	latest, _, _ := s.releases.get()
	if latest == nil || !newer(latest.Version, version.Version) {
		return api.UpdateHub409JSONResponse{Error: "there is nothing newer to update to"}, nil
	}
	// A copy of the database as it was, in case the new version must be
	// left again: its migrations keep the previous version working
	// (design 5.4), but a copy costs little.
	if s.db.Dialect() == store.SQLite {
		dir := filepath.Join(s.cfg.DataDir, "backups")
		if err := os.MkdirAll(dir, 0o700); err != nil {
			return nil, err
		}
		if err := s.db.Backup(ctx, filepath.Join(dir, "before-"+latest.Version+".db")); err != nil {
			return nil, err
		}
	}
	in := &update.Installer{Dir: s.binDir(), ReleaseURL: s.cfg.ReleaseURL, Own: version.Version}
	if err := in.Install(ctx, latest.Version); err != nil {
		return api.UpdateHub409JSONResponse{Error: err.Error()}, nil
	}
	s.log.Info("hub update installed; restarting into it", "version", latest.Version)
	u, _ := userOf(ctx)
	s.event(0, "hub.update", mustJSON(map[string]string{"version": latest.Version, "by": u.Email}))
	// Answer first, then go.
	go func() {
		time.Sleep(500 * time.Millisecond)
		s.restartOnce.Do(func() { close(s.restart) })
	}()
	return api.UpdateHub202Response{}, nil
}

func (h *hub) binDir() string {
	return cmp.Or(os.Getenv("LOTSE_BIN_DIR"), filepath.Join(h.cfg.DataDir, "bin"))
}

// updateStatus records how an agent's update went.
func (h *hub) updateStatus(hostID int64, st protocol.UpdateStatus) {
	data := map[string]string{"version": st.Version}
	if st.Error != "" {
		data["error"] = st.Error
	}
	h.event(hostID, "agent.update."+st.State, mustJSON(data))
	h.live.publish(LiveMessage{Type: "update", HostID: hostID, Update: &st})
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}
