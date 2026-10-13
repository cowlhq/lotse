package store_test

import (
	"encoding/json"
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

var ada = store.Actor{Kind: "user", ID: ptr[int64](1), Name: "ada@example.com", Via: "ui"}

func TestScopes(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		levels := [3]store.ScopeName{{"acme", "ACME GmbH"}, {"shop", "Shop"}, {"prod", "Production"}}
		env, err := s.EnsureEnvironment(ctx, ada, levels)
		if err != nil {
			t.Fatal(err)
		}
		if env.Path != "acme/shop/prod" || env.Kind != "environment" || env.ID <= store.GlobalScope {
			t.Errorf("environment: %+v", env)
		}
		// A second environment in the same project creates only itself.
		staging, err := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{"acme", "ignored"}, {"shop", "ignored"}, {"staging", "Staging"}})
		if err != nil || staging.Path != "acme/shop/staging" || *staging.ParentID != *env.ParentID {
			t.Fatalf("staging: %+v, %v", staging, err)
		}
		all, _ := s.Scopes(ctx)
		if len(all) != 4 || all[0].Name != "ACME GmbH" {
			t.Errorf("scopes: %+v", all)
		}
		if again, _ := s.EnsureEnvironment(ctx, ada, levels); again.ID != env.ID {
			t.Errorf("the same path again made %d, want %d", again.ID, env.ID)
		}

		if err := s.RenameScope(ctx, ada, all[0].ID, "ACME AG"); err != nil {
			t.Fatal(err)
		}
		changes, err := s.Changes(ctx, "customer", all[0].ID, 10)
		if err != nil || len(changes) != 2 || changes[0].Field != "name" || changes[1].Field != "created" {
			t.Fatalf("customer history: %+v, %v", changes, err)
		}
		var oldName, newName string
		json.Unmarshal(changes[0].Old, &oldName)
		json.Unmarshal(changes[0].New, &newName)
		if oldName != "ACME GmbH" || newName != "ACME AG" || changes[0].ActorName != "ada@example.com" || changes[0].Via != "ui" {
			t.Errorf("rename: %+v", changes[0])
		}

		h, _, _ := s.SaveHost(ctx, store.Host{MachineID: "m1", Hostname: "web1", LastSeen: 1})
		if err := s.SetHostEnvironment(ctx, ada, h.ID, &env.ID); err != nil {
			t.Fatal(err)
		}
		if got, _ := s.Host(ctx, h.ID); got.EnvironmentID == nil || *got.EnvironmentID != env.ID {
			t.Errorf("host environment: %v", got.EnvironmentID)
		}
		if err := s.SetHostEnvironment(ctx, ada, h.ID, all[0].ParentID); err == nil {
			t.Error("a host was put into the global scope")
		}
		if err := s.DeleteScope(ctx, ada, env.ID); err != store.ErrNotEmpty {
			t.Errorf("deleting an environment with a host: %v", err)
		}
		if err := s.DeleteScope(ctx, ada, staging.ID); err != nil {
			t.Errorf("deleting an empty environment: %v", err)
		}
		hostChanges, _ := s.Changes(ctx, "host", h.ID, 10)
		if len(hostChanges) != 1 || string(hostChanges[0].New) != `"acme/shop/prod"` || hostChanges[0].Old != nil {
			t.Errorf("host history: %+v", hostChanges)
		}
	})
}
