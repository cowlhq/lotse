package store_test

import (
	"encoding/json"
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestChannels(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		env, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		sealed := "sealed"
		c, err := s.CreateChannel(ctx, ada, store.Channel{ScopeID: store.GlobalScope, Slug: "ops", Name: "Ops", Type: "googlechat",
			Spec: json.RawMessage(`{}`), Secrets: &sealed, CreatedAt: 1})
		if err != nil || c.ID == 0 || c.ScopePath != "" || string(c.SecretRefs) != `{}` {
			t.Fatalf("create: %+v, %v", c, err)
		}
		if _, err := s.CreateChannel(ctx, ada, store.Channel{ScopeID: store.GlobalScope, Slug: "ops", Name: "x", Type: "webhook", Spec: json.RawMessage(`{}`)}); err != store.ErrExists {
			t.Errorf("the same key again: %v", err)
		}
		// The same key on another scope is another channel.
		mail, err := s.CreateChannel(ctx, ada, store.Channel{ScopeID: *env.ParentID, Slug: "ops", Name: "Shop ops", Type: "email",
			Spec: json.RawMessage(`{"to":["ops@example.com"]}`), CreatedAt: 1})
		if err != nil || mail.ScopePath != "acme/shop" {
			t.Fatalf("on a project: %+v, %v", mail, err)
		}
		mail.Name, mail.SecretRefs = "Shop", json.RawMessage(`{"password":{"env":"SHOP_SMTP"}}`)
		if _, err := s.UpdateChannel(ctx, ada, mail); err != nil {
			t.Fatal(err)
		}
		list, _ := s.Channels(ctx)
		if len(list) != 2 || list[0].Slug != "ops" || list[0].ScopePath != "" || list[1].Name != "Shop" || string(list[1].SecretRefs) != `{"password":{"env":"SHOP_SMTP"}}` {
			t.Errorf("list: %+v", list)
		}
		changes, _ := s.Changes(ctx, "channel", mail.ID, 10)
		if len(changes) != 3 || changes[0].Field != "secrets" || changes[1].Field != "name" || changes[2].SubjectPath != "acme/shop/ops" {
			t.Errorf("history: %+v", changes)
		}
		if err := s.DeleteChannel(ctx, ada, c.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Channel(ctx, c.ID); err != store.ErrNotFound {
			t.Errorf("deleted: %v", err)
		}

		if m, err := s.MailServer(ctx); m != nil || err != nil {
			t.Errorf("no mail server: %+v, %v", m, err)
		}
		if err := s.SetMailServer(ctx, ada, &store.MailServer{Spec: json.RawMessage(`{"host":"smtp.example.com"}`), Secrets: &sealed, UpdatedAt: 2}); err != nil {
			t.Fatal(err)
		}
		if m, err := s.MailServer(ctx); err != nil || string(m.Spec) != `{"host":"smtp.example.com"}` || *m.Secrets != "sealed" {
			t.Errorf("mail server: %+v, %v", m, err)
		}
		s.SetMailServer(ctx, ada, nil)
		if m, _ := s.MailServer(ctx); m != nil {
			t.Error("removed, the mail server is still there")
		}
		if changes, _ := s.Changes(ctx, "global", store.GlobalScope, 10); len(changes) != 2 || changes[0].New != nil {
			t.Errorf("mail server history: %+v", changes)
		}
	})
}

func TestMaintenance(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		env, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		start, end, weekly := int64(100), int64(200), "sun 02:00-04:00"
		once, err := s.AddMaintenance(ctx, ada, store.Maintenance{SubjectKind: "environment", SubjectID: env.ID, StartsAt: &start, EndsAt: &end,
			Reason: "moving racks", CreatedBy: ada.Name, CreatedAt: 1}, env.Path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddMaintenance(ctx, ada, store.Maintenance{SubjectKind: "environment", SubjectID: env.ID, Schedule: &weekly, CreatedBy: ada.Name, CreatedAt: 2}, env.Path); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AddMaintenance(ctx, ada, store.Maintenance{SubjectKind: "environment", SubjectID: env.ID, CreatedBy: ada.Name}, env.Path); err == nil {
			t.Error("a window without times")
		}
		list, _ := s.Maintenances(ctx)
		if len(list) != 2 || *list[0].Schedule != weekly || list[1].Reason != "moving racks" {
			t.Errorf("list: %+v", list)
		}
		if err := s.DeleteMaintenance(ctx, ada, once.ID, env.Path); err != nil {
			t.Fatal(err)
		}
		changes, _ := s.Changes(ctx, "environment", env.ID, 10)
		if len(changes) != 4 || changes[0].Field != "maintenance" || changes[0].New != nil || changes[3].Field != "created" {
			t.Errorf("history: %+v", changes)
		}
		// Deleting the environment takes its windows and settings along.
		s.SetSetting(ctx, ada, store.Subject{Kind: "environment", ID: env.ID}, "notify.channels", json.RawMessage(`["ops"]`), 3)
		if err := s.DeleteScope(ctx, ada, env.ID); err != nil {
			t.Fatal(err)
		}
		if list, _ := s.Maintenances(ctx); len(list) != 0 {
			t.Errorf("windows of a deleted environment: %+v", list)
		}
		if all, _ := s.AllSettings(ctx); len(all) != 0 {
			t.Errorf("settings of a deleted environment: %+v", all)
		}
	})
}

func TestAlerts(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		key := store.AlertKey{Rule: "host.disk", SubjectKind: "host", SubjectID: 7, Instance: "/data"}
		v, th := 96.0, 95.0
		crit, err := s.SaveAlertState(ctx, store.AlertState{AlertKey: key, State: "crit", Since: 10, Value: &v, NotifiedState: "ok"},
			&store.Alert{SubjectPath: "web1", From: "ok", To: "crit", At: 10, Value: &v, Threshold: &th, Effective: json.RawMessage(`{"crit":95}`)})
		if err != nil || crit.AlertID == nil {
			t.Fatalf("save: %+v, %v", crit, err)
		}
		states, _ := s.AlertStates(ctx)
		if len(states) != 1 || states[0].State != "crit" || *states[0].AlertID != *crit.AlertID || states[0].Instance != "/data" {
			t.Errorf("states: %+v", states)
		}

		// Two channels, one sent, the other held back.
		ops, _ := s.CreateChannel(ctx, ada, store.Channel{ScopeID: store.GlobalScope, Slug: "ops", Name: "Ops", Type: "webhook", Spec: json.RawMessage(`{}`)})
		one, now := ops.ID, int64(20)
		crit.NotifiedState, crit.NotifiedAt = "crit", &now
		if err := s.Notify(ctx, crit, []store.Notification{
			{AlertID: *crit.AlertID, ChannelID: &one, Channel: "ops", Kind: "change", Status: "pending", CreatedAt: 20, NextAt: &now},
			{AlertID: *crit.AlertID, Channel: "gone", Kind: "change", Status: "suppressed", CreatedAt: 20},
		}); err != nil {
			t.Fatal(err)
		}
		pending, alerts, err := s.PendingNotifications(ctx, 19)
		if err != nil || len(pending) != 0 {
			t.Errorf("pending too early: %+v, %v", pending, err)
		}
		pending, alerts, err = s.PendingNotifications(ctx, 20)
		if err != nil {
			t.Fatal(err)
		}
		if len(pending) != 1 || alerts[pending[0].AlertID].To != "crit" || string(alerts[pending[0].AlertID].Effective) != `{"crit":95}` {
			t.Fatalf("pending: %+v %+v", pending, alerts)
		}
		pending[0].Status, pending[0].Attempts, pending[0].SentAt, pending[0].NextAt = "sent", 1, &now, nil
		if err := s.FinishNotifications(ctx, pending); err != nil {
			t.Fatal(err)
		}
		if pending, _, _ := s.PendingNotifications(ctx, 100); len(pending) != 0 {
			t.Errorf("still pending: %+v", pending)
		}
		sent, _ := s.Notifications(ctx, *crit.AlertID)
		if len(sent) != 2 || sent[0].Status != "sent" || sent[1].Status != "suppressed" {
			t.Errorf("notifications: %+v", sent)
		}

		if err := s.Acknowledge(ctx, ada, key, "web1", 30); err != nil {
			t.Fatal(err)
		}
		if err := s.Acknowledge(ctx, ada, key, "web1", 31); err != store.ErrNotFound {
			t.Errorf("acknowledged twice: %v", err)
		}
		states, _ = s.AlertStates(ctx)
		if *states[0].AckedBy != ada.Name || states[0].NotifiedState != "crit" {
			t.Errorf("acknowledged: %+v", states[0])
		}
		if changes, _ := s.Changes(ctx, "host", 7, 10); len(changes) != 1 || changes[0].Field != "alert host.disk /data" {
			t.Errorf("history: %+v", changes)
		}

		// Back to OK, and sent as such: forgotten.
		ok, err := s.SaveAlertState(ctx, store.AlertState{AlertKey: key, State: "ok", Since: 40, NotifiedState: "crit"},
			&store.Alert{SubjectPath: "web1", From: "crit", To: "ok", At: 40, Effective: json.RawMessage(`{}`)})
		if err != nil {
			t.Fatal(err)
		}
		ok.NotifiedState = "ok"
		s.Notify(ctx, ok, nil)
		if states, _ := s.AlertStates(ctx); len(states) != 0 {
			t.Errorf("states after recovery: %+v", states)
		}
		list, _ := s.Alerts(ctx, store.AlertFilter{SubjectKind: "host", SubjectID: 7, From: 0, To: 100, Limit: 10})
		if len(list) != 2 || list[0].To != "ok" || list[1].To != "crit" || *list[1].Threshold != 95 {
			t.Errorf("history: %+v", list)
		}
		if list, _ := s.Alerts(ctx, store.AlertFilter{From: 0, To: 20, Limit: 10}); len(list) != 1 {
			t.Errorf("by time: %+v", list)
		}
	})
}

func TestAlertInputs(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		cpu, used, total := 0.5, int64(90), int64(100)
		s.Write(ctx, store.Batch{
			HostSamples: []store.HostSample{{HostID: h.ID, TS: 10, CPU: &cpu}, {HostID: h.ID, TS: 20, CPU: &cpu}},
			FSSamples:   []store.FSSample{{HostID: h.ID, Mount: "/", Device: "sda1", FSType: "ext4", TS: 20, Used: used, Total: total}},
			Events: []store.Event{
				{TS: 5, SubjectKind: "container", SubjectID: 3, Kind: "container.die"},
				{TS: 15, SubjectKind: "container", SubjectID: 3, Kind: "container.die"},
				{TS: 16, SubjectKind: "container", SubjectID: 4, Kind: "container.die"},
				{TS: 17, SubjectKind: "container", SubjectID: 4, Kind: "container.start"},
			},
		})
		latest, _ := s.LatestHostSamples(ctx)
		if latest[h.ID].TS != 20 {
			t.Errorf("latest: %+v", latest)
		}
		fs, _ := s.AllFilesystems(ctx)
		if len(fs) != 1 || fs[0].HostID != h.ID || *fs[0].Used != 90 || *fs[0].TS != 20 {
			t.Errorf("filesystems: %+v", fs)
		}
		counts, _ := s.CountEvents(ctx, "container.die", 10)
		if len(counts) != 2 || counts[3] != 1 || counts[4] != 1 {
			t.Errorf("counts: %+v", counts)
		}
	})
}
