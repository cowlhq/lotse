package store_test

import (
	"encoding/json"
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestChecks(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		env, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		c, err := s.CreateCheck(ctx, ada, store.Check{
			EnvironmentID: env.ID, Slug: "homepage", Name: "Homepage", Type: "http",
			Spec: json.RawMessage(`{"url":"https://shop.example.com"}`), CreatedAt: 1,
		}, "")
		if err != nil || c.Path != "acme/shop/prod/homepage" || c.State != "unknown" {
			t.Fatalf("create: %+v, %v", c, err)
		}
		if _, err := s.CreateCheck(ctx, ada, store.Check{EnvironmentID: env.ID, Slug: "homepage", Name: "x", Type: "tcp", Spec: json.RawMessage(`{}`), CreatedAt: 1}, ""); err != store.ErrExists {
			t.Errorf("the same key again: %v", err)
		}
		sealed := "sealed"
		hb, err := s.CreateCheck(ctx, ada, store.Check{EnvironmentID: env.ID, Slug: "backup", Name: "Backup", Type: "heartbeat",
			Spec: json.RawMessage(`{"period":"24h","grace":"1h"}`), PingSealed: &sealed, CreatedAt: 1}, "ping-hash")
		if err != nil {
			t.Fatal(err)
		}
		if got, err := s.Ping(ctx, "ping-hash", 50); err != nil || got.ID != hb.ID || *got.LastPing != 50 {
			t.Errorf("ping: %+v, %v", got, err)
		}
		if _, err := s.Ping(ctx, "nope", 50); err != store.ErrNotFound {
			t.Errorf("a wrong ping: %v", err)
		}

		if _, err := s.UpdateCheck(ctx, ada, c.ID, "Shop", json.RawMessage(`{"url":"https://shop.example.com","keyword":"Welcome"}`)); err != nil {
			t.Fatal(err)
		}
		changes, _ := s.Changes(ctx, "check", c.ID, 10)
		if len(changes) != 3 || changes[0].Field != "spec" || changes[1].Field != "name" {
			t.Errorf("history: %+v", changes)
		}

		if changed, _ := s.SetCheckState(ctx, c.ID, "up", 10); !changed {
			t.Error("unknown to up did not change")
		}
		if changed, _ := s.SetCheckState(ctx, c.ID, "up", 20); changed {
			t.Error("up to up changed")
		}

		hub := int64(store.BuiltinProbe)
		lat := 120.0
		code := 200
		s.Write(ctx, store.Batch{CheckResults: []store.CheckResult{
			{CheckID: c.ID, ProbeID: hub, TS: 100, OK: true, LatencyMS: &lat, StatusCode: &code},
			{CheckID: c.ID, ProbeID: hub, TS: 160, OK: false, Error: "timeout"},
		}})
		latest, err := s.LatestResults(ctx, c.ID)
		if err != nil || len(latest) != 1 || latest[0].TS != 160 || latest[0].OK || latest[0].Error != "timeout" {
			t.Errorf("latest: %+v, %v", latest, err)
		}
		sr, _ := s.Series(ctx, "check", store.Raw, c.ID, 0, 200, 200)
		if len(sr.Time) != 1 || *sr.Columns["ok"][0] != 0.5 || *sr.Columns["latency_ms"][0] != 120 {
			t.Errorf("series: %+v", sr.Columns)
		}
		results, _ := s.Results(ctx, c.ID, 0, 200, 10)
		if len(results) != 2 || results[0].TS != 160 {
			t.Errorf("results: %+v", results)
		}

		if err := s.DeleteScope(ctx, ada, env.ID); err != store.ErrNotEmpty {
			t.Errorf("deleting an environment with checks: %v", err)
		}
		if err := s.DeleteCheck(ctx, ada, c.ID); err != nil {
			t.Fatal(err)
		}
		all, _ := s.Checks(ctx, env.ID)
		if len(all) != 1 || all[0].Slug != "backup" {
			t.Errorf("after deleting: %+v", all)
		}
	})
}

func TestCheckSecrets(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		env, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		sealed := "sealed-1"
		c, err := s.CreateCheck(ctx, ada, store.Check{EnvironmentID: env.ID, Slug: "api", Name: "API", Type: "http",
			Spec: json.RawMessage(`{"url":"https://api.example.com"}`), Secrets: &sealed, SecretRefs: json.RawMessage(`{"basic_auth.password":{"env":"P"}}`), CreatedAt: 1}, "")
		if err != nil || *c.Secrets != "sealed-1" || string(c.SecretRefs) != `{"basic_auth.password":{"env":"P"}}` {
			t.Fatalf("create: %+v, %v", c, err)
		}
		if c, err = s.SetCheckSecrets(ctx, ada, c.ID, nil, nil); err != nil || c.Secrets != nil || string(c.SecretRefs) != "{}" {
			t.Errorf("cleared: %+v, %v", c, err)
		}
		changes, _ := s.Changes(ctx, "check", c.ID, 10)
		if len(changes) == 0 || changes[0].Field != "secrets" {
			t.Errorf("history: %+v", changes)
		}
		if _, err := s.SetCheckSecrets(ctx, ada, 9999, nil, nil); err != store.ErrNotFound {
			t.Errorf("no such check: %v", err)
		}
	})
}

func TestProbes(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		probes, _ := s.Probes(ctx)
		if len(probes) != 1 || !probes[0].Builtin || probes[0].Slug != "hub" {
			t.Fatalf("probes: %+v", probes)
		}
		if _, err := s.EnrollProbe(ctx, "probe-token", "fra", "Frankfurt", "DE", "cred-1", 1); err != store.ErrNotFound {
			t.Errorf("enrolling without a token: %v", err)
		}
		s.NewEnrollmentToken(ctx, ada, store.GlobalScope, "probe-token", "sealed", 1)
		p, err := s.EnrollProbe(ctx, "probe-token", "fra", "Frankfurt", "DE", "cred-1", 1)
		if err != nil || p.ID == store.BuiltinProbe || p.Builtin {
			t.Fatalf("enroll: %+v, %v", p, err)
		}
		if got, err := s.ProbeByCredential(ctx, "cred-1", "0.2.0", 5); err != nil || got.ID != p.ID || got.Version != "0.2.0" {
			t.Errorf("credential: %+v, %v", got, err)
		}
		if _, err := s.EnrollProbe(ctx, "probe-token", "hub", "x", "", "cred-x", 1); err == nil {
			t.Error("enrolled as the hub's own probe")
		}
		if err := s.RevokeProbe(ctx, ada, p.ID, 9); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ProbeByCredential(ctx, "cred-1", "0.2.0", 10); err != store.ErrNotFound {
			t.Errorf("a revoked probe: %v", err)
		}
		if err := s.RevokeProbe(ctx, ada, store.BuiltinProbe, 9); err == nil {
			t.Error("revoked the hub's own probe")
		}
	})
}
