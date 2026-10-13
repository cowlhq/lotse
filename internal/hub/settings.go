package hub

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/coder/websocket/wsjson"

	"lotse.sh/internal/config"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/protocol"
	"lotse.sh/internal/store"
)

// levels returns the object a subject names and the scopes above it, in
// the order settings are inherited: host:3, environment:4, project:5,
// customer:6 or global:1.
func (h *hub) levels(ctx context.Context, subject string) ([]config.Level, error) {
	kind, idText, _ := strings.Cut(subject, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	if err != nil {
		return nil, errBadSubject
	}
	var levels []config.Level
	var scopeID int64
	switch kind {
	case "check":
		c, err := h.db.Check(ctx, id)
		if err != nil {
			return nil, err
		}
		levels = append(levels, config.Level{Kind: "check", ID: c.ID, Path: c.Path, Name: c.Name})
		scopeID = c.EnvironmentID
	case "host":
		host, err := h.db.Host(ctx, id)
		if err != nil {
			return nil, err
		}
		levels = append(levels, config.Level{Kind: "host", ID: host.ID, Path: host.Name, Name: host.Name})
		scopeID = store.GlobalScope
		if host.EnvironmentID != nil {
			scopeID = *host.EnvironmentID
		}
	case "environment", "project", "customer", "global":
		scopeID = id
	default:
		return nil, errBadSubject
	}
	for {
		x, err := h.db.Scope(ctx, scopeID)
		if err != nil {
			return nil, err
		}
		if len(levels) == 0 && x.Kind != kind {
			return nil, store.ErrNotFound
		}
		levels = append(levels, config.Level{Kind: x.Kind, ID: x.ID, Path: x.Path, Name: x.Name})
		if x.ParentID == nil {
			return levels, nil
		}
		scopeID = *x.ParentID
	}
}

var errBadSubject = errors.New("subject must be check:<id>, host:<id>, environment:<id>, project:<id>, customer:<id> or global:1")

// effective resolves the settings of a subject.
func (h *hub) effective(ctx context.Context, subject string) ([]config.Value, error) {
	levels, err := h.levels(ctx, subject)
	if err != nil {
		return nil, err
	}
	subs := make([]store.Subject, len(levels))
	for i, l := range levels {
		subs[i] = store.Subject{Kind: l.Kind, ID: l.ID}
	}
	set, err := h.db.Settings(ctx, subs)
	if err != nil {
		return nil, err
	}
	return config.Resolve(levels[0].Kind, levels, set, h.defaults()), nil
}

// defaults are the hub's own defaults where they differ from the built-in
// ones, as in tests that read every second.
func (h *hub) defaults() map[string]json.RawMessage {
	d := map[string]json.RawMessage{}
	for key, v := range map[string]time.Duration{
		"agent.interval.host": h.hostInterval, "agent.interval.containers": h.containerInterval,
	} {
		if v > 0 {
			d[key], _ = json.Marshal(v.String())
		}
	}
	return d
}

// agentConfig is what a host's agent is told: its effective settings.
func (h *hub) agentConfig(ctx context.Context, hostID int64) protocol.Config {
	c := protocol.Config{
		HostInterval:      int(h.hostInterval / time.Second),
		ContainerInterval: int(h.containerInterval / time.Second),
	}
	values, err := h.effective(ctx, "host:"+strconv.FormatInt(hostID, 10))
	if err != nil {
		h.log.Error("cannot work out an agent's settings", "host_id", hostID, "err", err)
		return c
	}
	for _, v := range values {
		d, err := config.Duration(v.Value)
		if err != nil || d < time.Second {
			continue
		}
		switch v.Key {
		case "agent.interval.host":
			c.HostInterval = int(d / time.Second)
		case "agent.interval.containers":
			c.ContainerInterval = int(d / time.Second)
		}
	}
	return c
}

// pushConfig tells every connected agent its settings again, after they
// may have changed.
func (h *hub) pushConfig(ctx context.Context) {
	for _, a := range h.agents.all() {
		c := h.agentConfig(ctx, a.hostID)
		wctx, cancel := context.WithTimeout(ctx, writeTimeout)
		wsjson.Write(wctx, a.conn, protocol.Message{Type: protocol.TypeConfig, Config: &c})
		cancel()
	}
}

func toLevel(l config.Level) api.ConfigLevel {
	return api.ConfigLevel{Kind: api.ConfigLevelKind(l.Kind), Id: l.ID, Path: l.Path, Name: l.Name}
}

func (s apiServer) GetConfig(ctx context.Context, req api.GetConfigRequestObject) (api.GetConfigResponseObject, error) {
	values, err := s.effective(ctx, req.Params.Subject)
	switch {
	case errors.Is(err, errBadSubject):
		return api.GetConfig400JSONResponse{Error: err.Error()}, nil
	case errors.Is(err, store.ErrNotFound):
		return api.GetConfig404JSONResponse{Error: "there is no such object"}, nil
	case err != nil:
		return nil, err
	}
	out := make(api.GetConfig200JSONResponse, len(values))
	for i, v := range values {
		k, _ := config.Lookup(v.Key)
		out[i] = api.ConfigValue{
			Key: v.Key, Type: api.ConfigValueType(k.Type), Description: k.Description,
			Value: v.Value, Overrides: []api.ConfigOverride{},
		}
		if v.From != nil {
			out[i].From = ptr(toLevel(*v.From))
		}
		for _, o := range v.Overrides {
			out[i].Overrides = append(out[i].Overrides, api.ConfigOverride{Level: toLevel(o.Level), Value: o.Value})
		}
	}
	return out, nil
}

func (s apiServer) SetSetting(ctx context.Context, req api.SetSettingRequestObject) (api.SetSettingResponseObject, error) {
	if !isAdmin(ctx) {
		return api.SetSetting403JSONResponse{Error: forbidden}, nil
	}
	key, ok := config.Lookup(req.Body.Key)
	if !ok {
		return api.SetSetting400JSONResponse{Error: "there is no setting " + strconv.Quote(req.Body.Key)}, nil
	}
	levels, err := s.levels(ctx, req.Body.Subject)
	switch {
	case errors.Is(err, errBadSubject):
		return api.SetSetting400JSONResponse{Error: err.Error()}, nil
	case errors.Is(err, store.ErrNotFound):
		return api.SetSetting404JSONResponse{Error: "there is no such object"}, nil
	case err != nil:
		return nil, err
	}
	here := levels[0]
	if !key.SettableOn(here.Kind) {
		return api.SetSetting400JSONResponse{Error: key.Name + " cannot be set on a " + here.Kind}, nil
	}
	var value json.RawMessage
	if req.Body.Value != nil {
		value, err = json.Marshal(req.Body.Value)
		if err != nil {
			return nil, err
		}
		if err := key.Check(value); err != nil {
			return api.SetSetting400JSONResponse{Error: key.Name + ": " + err.Error()}, nil
		}
	}
	sub := store.Subject{Kind: here.Kind, ID: here.ID, Path: here.Path}
	if err := s.db.SetSetting(ctx, actor(ctx), sub, key.Name, value, time.Now().Unix()); err != nil {
		return nil, err
	}
	s.pushConfig(context.WithoutCancel(ctx))
	s.reassign(context.WithoutCancel(ctx))
	return api.SetSetting204Response{}, nil
}
