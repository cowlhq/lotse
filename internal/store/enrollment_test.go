package store_test

import (
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestEnrollment(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		prod, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		staging, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "staging", Name: "Staging"}})
		if _, err := s.EnrollmentToken(ctx, prod.ID); err != store.ErrNotFound {
			t.Errorf("no token yet: %v", err)
		}
		s.NewEnrollmentToken(ctx, ada, prod.ID, "old-hash", "old-sealed", 1)
		s.NewEnrollmentToken(ctx, ada, prod.ID, "prod-hash", "prod-sealed", 2)
		s.NewEnrollmentToken(ctx, ada, staging.ID, "staging-hash", "staging-sealed", 2)
		if tok, err := s.EnrollmentToken(ctx, prod.ID); err != nil || tok.Sealed != "prod-sealed" {
			t.Errorf("current token: %+v, %v", tok, err)
		}

		if _, err := s.Enroll(ctx, "old-hash", "m1", "web1", "cred-0", 3); err != store.ErrNotFound {
			t.Errorf("a replaced token enrolled: %v", err)
		}
		e, err := s.Enroll(ctx, "prod-hash", "m1", "web1", "cred-1", 3)
		if err != nil || e.TookOver || e.Host.EnvironmentID == nil || *e.Host.EnvironmentID != prod.ID || e.Host.Name != "web1" {
			t.Fatalf("enroll: %+v, %v", e, err)
		}
		if h, err := s.AgentHost(ctx, "cred-1", 4); err != nil || h.ID != e.Host.ID {
			t.Errorf("credential: %+v, %v", h, err)
		}
		// Enrolled with staging's token, it stays where it is: taking over
		// moves no host.
		if _, err := s.Enroll(ctx, "staging-hash", "m1", "web1", "cred-x", 5); err != store.ErrElsewhere {
			t.Fatalf("enroll into another environment: %v", err)
		}
		// Reinstalled: the same host; the first credential is revoked.
		again, err := s.Enroll(ctx, "prod-hash", "m1", "web1", "cred-2", 5)
		if err != nil || !again.TookOver || again.Host.ID != e.Host.ID || *again.Host.EnvironmentID != prod.ID {
			t.Fatalf("enroll again: %+v, %v", again, err)
		}
		if _, err := s.AgentHost(ctx, "cred-1", 6); err != store.ErrNotFound {
			t.Errorf("the first credential still works: %v", err)
		}
		if err := s.RevokeAgents(ctx, ada, e.Host.ID, 7); err != nil {
			t.Fatal(err)
		}
		if _, err := s.AgentHost(ctx, "cred-2", 8); err != store.ErrNotFound {
			t.Errorf("a revoked credential still works: %v", err)
		}
		changes, _ := s.Changes(ctx, "host", e.Host.ID, 10)
		var fields []string
		for _, c := range changes {
			fields = append(fields, c.Field)
		}
		want := "agent revoked,enrolled again,enrolled"
		if got := join(fields); got != want {
			t.Errorf("history %q, want %q", got, want)
		}
	})
}

func join(s []string) string {
	out := ""
	for i, x := range s {
		if i > 0 {
			out += ","
		}
		out += x
	}
	return out
}

// A host named before its agent: the machine of that name becomes it.
func TestWaitingHost(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		env, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		w, err := s.CreateWaitingHost(ctx, ada, env.ID, "web1")
		if err != nil || !w.Waiting() || w.LastSeen != 0 {
			t.Fatalf("waiting: %+v, %v", w, err)
		}
		s.SetSetting(ctx, ada, store.Subject{Kind: "host", ID: w.ID}, "agent.auto_update", []byte("true"), 1)
		s.NewEnrollmentToken(ctx, ada, env.ID, "enr", "sealed", 1)
		e, err := s.Enroll(ctx, "enr", "m1", "web1", "cred", 10)
		if err != nil || e.Host.ID != w.ID || e.Host.Waiting() || e.Host.MachineID != "m1" || e.TookOver {
			t.Fatalf("enrolled: %+v, %v", e, err)
		}
		if set, _ := s.Settings(ctx, []store.Subject{{Kind: "host", ID: w.ID}}); string(set[0]["agent.auto_update"]) != "true" {
			t.Errorf("settings lost: %v", set)
		}
		// Another name is another host.
		if e, _ := s.Enroll(ctx, "enr", "m2", "web2", "cred2", 11); e.Host.ID == w.ID {
			t.Error("web2 took web1's place")
		}
		// Only a waiting host can be deleted.
		if err := s.DeleteWaitingHost(ctx, ada, w.ID); err != store.ErrNotEmpty {
			t.Errorf("an enrolled host deleted: %v", err)
		}
		w3, _ := s.CreateWaitingHost(ctx, ada, env.ID, "web3")
		if err := s.DeleteWaitingHost(ctx, ada, w3.ID); err != nil {
			t.Fatal(err)
		}
	})
}
