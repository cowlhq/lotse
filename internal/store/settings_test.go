package store_test

import (
	"encoding/json"
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestSettings(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		host := store.Subject{Kind: "host", ID: 7, Path: "web1"}
		global := store.Subject{Kind: "global", ID: store.GlobalScope}
		if err := s.SetSetting(ctx, ada, host, "agent.interval.host", json.RawMessage(`"20s"`), 10); err != nil {
			t.Fatal(err)
		}
		s.SetSetting(ctx, ada, host, "agent.interval.host", json.RawMessage(`"25s"`), 11)
		s.SetSetting(ctx, ada, global, "agent.interval.host", json.RawMessage(`"60s"`), 12)
		got, err := s.Settings(ctx, []store.Subject{host, global, {Kind: "customer", ID: 99}})
		if err != nil {
			t.Fatal(err)
		}
		if string(got[0]["agent.interval.host"]) != `"25s"` || string(got[1]["agent.interval.host"]) != `"60s"` || len(got[2]) != 0 {
			t.Errorf("settings: %v", got)
		}
		s.SetSetting(ctx, ada, host, "agent.interval.host", nil, 13)
		got, _ = s.Settings(ctx, []store.Subject{host})
		if len(got[0]) != 0 {
			t.Errorf("after unsetting: %v", got)
		}
		changes, _ := s.Changes(ctx, "host", 7, 10)
		if len(changes) != 3 || changes[0].New != nil || string(changes[0].Old) != `"25s"` || string(changes[1].Old) != `"20s"` {
			t.Errorf("history: %+v", changes)
		}
	})
}
