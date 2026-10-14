package hub

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"time"

	"lotse.sh/internal/alert"
	"lotse.sh/internal/config"
	"lotse.sh/internal/hub/api"
	"lotse.sh/internal/store"
)

// The alerts API: what fires now, what happened, and why.

// covers says whether a chain is, or is below, the subject kind:id; an
// empty kind covers everything.
func covers(chain []config.Level, kind string, id int64) bool {
	if kind == "" {
		return true
	}
	for _, l := range chain {
		if l.Kind == kind && l.ID == id {
			return true
		}
	}
	return false
}

// subjectFilter reads an optional kind:id.
func subjectFilter(subject *string) (string, int64, bool) {
	if subject == nil {
		return "", 0, true
	}
	return parseSubject(*subject)
}

func stateRank(st store.AlertState) int {
	switch st.State {
	case alert.Crit:
		return 0
	case alert.Warn:
		return 1
	case alert.Unknown:
		return 2
	}
	return 3
}

func (s apiServer) ListAlerts(ctx context.Context, req api.ListAlertsRequestObject) (api.ListAlertsResponseObject, error) {
	kind, id, ok := subjectFilter(req.Params.Subject)
	if !ok {
		return api.ListAlerts400JSONResponse{Error: "subject is kind:id, such as host:3"}, nil
	}
	t, err := s.loadTree(ctx)
	if err != nil {
		return nil, err
	}
	windows, err := s.db.Maintenances(ctx)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	s.alerts.mu.Lock()
	var list []store.AlertState
	for _, st := range s.alerts.states {
		if st.State != alert.OK || st.Pending != nil {
			list = append(list, st)
		}
	}
	values, reasons := map[store.AlertKey]*float64{}, map[store.AlertKey]string{}
	for _, st := range list {
		values[st.AlertKey], reasons[st.AlertKey] = s.alerts.values[st.AlertKey], s.alerts.reasons[st.AlertKey]
	}
	s.alerts.mu.Unlock()
	slices.SortFunc(list, func(a, b store.AlertState) int {
		return cmp.Or(cmp.Compare(stateRank(a), stateRank(b)), cmp.Compare(a.Since, b.Since))
	})
	out := api.ListAlerts200JSONResponse{}
	for _, st := range list {
		chain := t.chain(st.SubjectKind, st.SubjectID)
		if chain == nil || !covers(chain, kind, id) {
			continue
		}
		r, _ := config.LookupRule(st.Rule)
		p, _ := s.params(t, r, chain)
		x := api.AlertState{
			Rule: st.Rule, Description: r.Description, SubjectKind: api.AlertStateSubjectKind(st.SubjectKind),
			SubjectId: st.SubjectID, SubjectPath: t.path(st.SubjectKind, st.SubjectID), Instance: st.Instance,
			State: api.AlertStateState(st.State), Since: st.Since, Unit: api.AlertStateUnit(r.Unit),
			AlertId: ptrValue(st.AlertID), InMaintenance: inMaintenance(windows, chain, now),
			NotifiedState: st.NotifiedState, AckedBy: st.AckedBy, AckedAt: st.AckedAt,
		}
		if reason := reasons[st.AlertKey]; reason != "" {
			x.Reason = &reason
		}
		if st.Pending != nil {
			x.Pending = ptr(api.AlertStatePending(*st.Pending))
		}
		if !r.Condition() {
			x.Value = cmp.Or(values[st.AlertKey], st.Value)
			to := st.State
			if st.Pending != nil {
				to = *st.Pending
			}
			x.Threshold = alert.Threshold(p, alert.OK, to)
		}
		out = append(out, x)
	}
	return out, nil
}

func toAlert(a store.Alert) api.Alert {
	r, _ := config.LookupRule(a.Rule)
	return api.Alert{
		Id: a.ID, Rule: a.Rule, SubjectKind: api.AlertSubjectKind(a.SubjectKind), SubjectId: a.SubjectID,
		SubjectPath: a.SubjectPath, Instance: a.Instance, From: api.AlertFrom(a.From), To: api.AlertTo(a.To),
		At: a.At, Value: a.Value, Threshold: a.Threshold, Unit: api.AlertUnit(r.Unit), Reason: a.Reason,
		InMaintenance: a.InMaintenance,
	}
}

func (s apiServer) ListAlertHistory(ctx context.Context, req api.ListAlertHistoryRequestObject) (api.ListAlertHistoryResponseObject, error) {
	kind, id, ok := subjectFilter(req.Params.Subject)
	if !ok {
		return api.ListAlertHistory400JSONResponse{Error: "subject is kind:id, such as host:3"}, nil
	}
	from, to := timeRange(req.Params.From, req.Params.To)
	list, err := s.db.Alerts(ctx, store.AlertFilter{From: from, To: to, Limit: 5000})
	if err != nil {
		return nil, err
	}
	var t *tree
	if kind != "" {
		if t, err = s.loadTree(ctx); err != nil {
			return nil, err
		}
	}
	out := api.ListAlertHistory200JSONResponse{}
	for _, a := range list {
		if len(out) == 500 {
			break
		}
		if kind == "" || covers(t.chain(a.SubjectKind, a.SubjectID), kind, id) {
			out = append(out, toAlert(a))
		}
	}
	return out, nil
}

func (s apiServer) GetAlert(ctx context.Context, req api.GetAlertRequestObject) (api.GetAlertResponseObject, error) {
	a, err := s.db.Alert(ctx, req.Id)
	if errors.Is(err, store.ErrNotFound) {
		return api.GetAlert404JSONResponse{Error: "no such alert"}, nil
	}
	if err != nil {
		return nil, err
	}
	list, err := s.db.Notifications(ctx, a.ID)
	if err != nil {
		return nil, err
	}
	r, _ := config.LookupRule(a.Rule)
	var settings []api.RuleSetting
	json.Unmarshal(a.Effective, &settings)
	out := api.AlertDetail{Description: r.Description, Settings: settings, Notifications: []api.Notification{}}
	b, _ := json.Marshal(toAlert(a))
	json.Unmarshal(b, &out)
	if out.Settings == nil {
		out.Settings = []api.RuleSetting{}
	}
	for _, n := range list {
		x := api.Notification{
			Id: n.ID, Channel: n.Channel, ChannelId: n.ChannelID, Kind: api.NotificationKind(n.Kind),
			Status: api.NotificationStatus(n.Status), Attempts: n.Attempts, CreatedAt: n.CreatedAt, SentAt: n.SentAt,
		}
		if n.Error != "" {
			x.Error = ptr(n.Error)
		}
		out.Notifications = append(out.Notifications, x)
	}
	return api.GetAlert200JSONResponse(out), nil
}

func (s apiServer) AcknowledgeAlert(ctx context.Context, req api.AcknowledgeAlertRequestObject) (api.AcknowledgeAlertResponseObject, error) {
	if !isAdmin(ctx) {
		return api.AcknowledgeAlert403JSONResponse{Error: forbidden}, nil
	}
	var found *store.AlertState
	s.alerts.mu.Lock()
	for _, st := range s.alerts.states {
		if st.AlertID != nil && *st.AlertID == req.Id {
			found = &st
		}
	}
	s.alerts.mu.Unlock()
	notFiring := api.AcknowledgeAlert404JSONResponse{Error: "the alert does not fire, or is acknowledged already"}
	if found == nil || !alert.Firing(found.State) {
		return notFiring, nil
	}
	a, err := s.db.Alert(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	now := time.Now().Unix()
	who := actor(ctx)
	err = s.db.Acknowledge(ctx, who, found.AlertKey, a.SubjectPath, now)
	if errors.Is(err, store.ErrNotFound) {
		return notFiring, nil
	}
	if err != nil {
		return nil, err
	}
	s.alerts.mu.Lock()
	if st, ok := s.alerts.states[found.AlertKey]; ok && ptrEqual(st.AlertID, found.AlertID) {
		st.AckedBy, st.AckedAt = &who.Name, &now
		s.alerts.states[found.AlertKey] = st
	}
	s.alerts.mu.Unlock()
	s.live.publish(LiveMessage{Type: "alerts"})
	return api.AcknowledgeAlert204Response{}, nil
}
