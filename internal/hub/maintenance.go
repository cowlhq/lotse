package hub

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"lotse.sh/internal/config"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// Maintenance (design 8.4): while a window covers an object, through the
// object itself or a scope above it, its alerts are recorded but not sent.
// What still fires when the window ends is sent then.

// activeAt says whether a window covers a time. Schedules are in the hub's
// time zone.
func activeAt(m store.Maintenance, now time.Time) bool {
	if m.Schedule != nil {
		sc, err := config.ParseSchedule(*m.Schedule)
		return err == nil && sc.Contains(now.In(time.Local))
	}
	return m.StartsAt != nil && m.EndsAt != nil && now.Unix() >= *m.StartsAt && now.Unix() < *m.EndsAt
}

// inMaintenance says whether a window covers anything in a chain now.
func inMaintenance(windows []store.Maintenance, chain []config.Level, now time.Time) bool {
	for _, m := range windows {
		if !activeAt(m, now) {
			continue
		}
		for _, l := range chain {
			if l.Kind == m.SubjectKind && l.ID == m.SubjectID {
				return true
			}
		}
	}
	return false
}

// parseSubject reads kind:id.
func parseSubject(subject string) (string, int64, bool) {
	kind, idText, _ := strings.Cut(subject, ":")
	id, err := strconv.ParseInt(idText, 10, 64)
	return kind, id, err == nil
}

func (s apiServer) toMaintenance(t *tree, m store.Maintenance, now time.Time) api.Maintenance {
	return api.Maintenance{
		Id: m.ID, SubjectKind: api.MaintenanceSubjectKind(m.SubjectKind), SubjectId: m.SubjectID,
		SubjectPath: t.path(m.SubjectKind, m.SubjectID), StartsAt: m.StartsAt, EndsAt: m.EndsAt,
		Schedule: m.Schedule, Reason: m.Reason, CreatedBy: m.CreatedBy, CreatedAt: m.CreatedAt,
		Active: activeAt(m, now),
	}
}

func (s apiServer) ListMaintenance(ctx context.Context, req api.ListMaintenanceRequestObject) (api.ListMaintenanceResponseObject, error) {
	t, err := s.loadTree(ctx)
	if err != nil {
		return nil, err
	}
	var chain []config.Level
	if req.Params.Subject != nil {
		kind, id, ok := parseSubject(*req.Params.Subject)
		if !ok {
			return api.ListMaintenance400JSONResponse{Error: "subject is kind:id, such as host:3"}, nil
		}
		if chain = t.chain(kind, id); chain == nil {
			return api.ListMaintenance200JSONResponse{}, nil
		}
	}
	windows, err := s.db.Maintenances(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	out := api.ListMaintenance200JSONResponse{}
	for _, m := range windows {
		covers := chain == nil
		for _, l := range chain {
			covers = covers || l.Kind == m.SubjectKind && l.ID == m.SubjectID
		}
		if covers {
			out = append(out, s.toMaintenance(t, m, now))
		}
	}
	return out, nil
}

func (s apiServer) CreateMaintenance(ctx context.Context, req api.CreateMaintenanceRequestObject) (api.CreateMaintenanceResponseObject, error) {
	if !isAdmin(ctx) {
		return api.CreateMaintenance403JSONResponse{Error: forbidden}, nil
	}
	b := req.Body
	kind, id, ok := parseSubject(b.Subject)
	switch kind {
	case "host", "container", "check", "environment", "project", "customer", "global":
	default:
		ok = false
	}
	if !ok {
		return api.CreateMaintenance400JSONResponse{Error: "subject is a host, container, check or scope, as kind:id"}, nil
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		return nil, err
	}
	if t.chain(kind, id) == nil {
		return api.CreateMaintenance404JSONResponse{Error: "there is no such object"}, nil
	}
	reason := strings.TrimSpace(ptrValue(b.Reason))
	if len(reason) > 200 {
		return api.CreateMaintenance400JSONResponse{Error: "the reason is up to 200 characters"}, nil
	}
	now := time.Now()
	m := store.Maintenance{SubjectKind: kind, SubjectID: id, Reason: reason, CreatedBy: actor(ctx).Name, CreatedAt: now.Unix()}
	switch schedule := strings.TrimSpace(ptrValue(b.Schedule)); {
	case schedule != "" && (b.StartsAt != nil || b.EndsAt != nil):
		return api.CreateMaintenance400JSONResponse{Error: "a window is either from starts_at to ends_at, or by schedule"}, nil
	case schedule != "":
		if _, err := config.ParseSchedule(schedule); err != nil {
			return api.CreateMaintenance400JSONResponse{Error: err.Error()}, nil
		}
		m.Schedule = &schedule
	case b.StartsAt == nil || b.EndsAt == nil:
		return api.CreateMaintenance400JSONResponse{Error: "a window needs starts_at and ends_at, or a schedule"}, nil
	case *b.EndsAt <= *b.StartsAt || *b.EndsAt <= now.Unix():
		return api.CreateMaintenance400JSONResponse{Error: "a window ends after it starts, and after now"}, nil
	case *b.EndsAt-*b.StartsAt > 366*24*3600:
		return api.CreateMaintenance400JSONResponse{Error: "a window is up to a year long"}, nil
	default:
		m.StartsAt, m.EndsAt = b.StartsAt, b.EndsAt
	}
	m, err = s.db.AddMaintenance(ctx, actor(ctx), m, t.path(kind, id))
	if err != nil {
		return nil, err
	}
	s.alerts.poke()
	return api.CreateMaintenance201JSONResponse(s.toMaintenance(t, m, now)), nil
}

func (s apiServer) DeleteMaintenance(ctx context.Context, req api.DeleteMaintenanceRequestObject) (api.DeleteMaintenanceResponseObject, error) {
	if !isAdmin(ctx) {
		return api.DeleteMaintenance403JSONResponse{Error: forbidden}, nil
	}
	m, err := s.db.Maintenance(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.DeleteMaintenance404JSONResponse{Error: "no such window"}, nil
	}
	if err != nil {
		return nil, err
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		return nil, err
	}
	if err := s.db.DeleteMaintenance(ctx, actor(ctx), m.ID, t.path(m.SubjectKind, m.SubjectID)); err != nil {
		return nil, err
	}
	s.alerts.poke()
	return api.DeleteMaintenance204Response{}, nil
}
