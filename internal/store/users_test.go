package store_test

import (
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestUsersAndSessions(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		u, err := s.CreateUser(ctx, store.User{Email: " Admin@Example.com ", Name: "Ada", PasswordHash: "h", Role: "admin", CreatedAt: 1})
		if err != nil || u.ID == 0 || u.Email != "admin@example.com" {
			t.Fatalf("create: %+v, %v", u, err)
		}
		if _, err := s.CreateUser(ctx, store.User{Email: "ADMIN@example.com", PasswordHash: "h", Role: "viewer", CreatedAt: 1}); err != store.ErrExists {
			t.Errorf("the same address again: %v", err)
		}
		if got, err := s.UserByEmail(ctx, "admin@EXAMPLE.com"); err != nil || got.ID != u.ID {
			t.Errorf("by email: %+v, %v", got, err)
		}
		if n, _ := s.CountUsers(ctx); n != 1 {
			t.Errorf("%d users", n)
		}

		if err := s.CreateSession(ctx, store.Session{IDHash: "x", UserID: u.ID, CreatedAt: 10, ExpiresAt: 100, LastSeenAt: 10}); err != nil {
			t.Fatal(err)
		}
		if _, got, err := s.SessionUser(ctx, "x", 50); err != nil || got.ID != u.ID {
			t.Errorf("a valid session: %+v, %v", got, err)
		}
		if _, _, err := s.SessionUser(ctx, "x", 100); err != store.ErrNotFound {
			t.Errorf("an expired session: %v", err)
		}
		s.ExtendSession(ctx, "x", 90, 200)
		if _, _, err := s.SessionUser(ctx, "x", 150); err != nil {
			t.Errorf("an extended session: %v", err)
		}
		s.DeleteSession(ctx, "x")
		if _, _, err := s.SessionUser(ctx, "x", 150); err != store.ErrNotFound {
			t.Errorf("a deleted session: %v", err)
		}
	})
}

// Invitations: a link works once and until it expires, a new one replaces
// it, and the hub keeps an admin who can log in.
func TestUserLinks(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		sys := store.Actor{Kind: "system", Name: "test", Via: "system"}
		admin, _ := s.CreateUser(ctx, store.User{Email: "admin@example.com", PasswordHash: "h", Role: "admin", CreatedAt: 1})
		u, err := s.InviteUser(ctx, sys, store.User{Email: "Bea@example.com", Role: "admin", CreatedAt: 2}, "link1", 100)
		if err != nil || !u.Invited() || u.LinkExpiresAt == nil || *u.LinkExpiresAt != 100 {
			t.Fatalf("invite: %+v, %v", u, err)
		}
		if _, err := s.InviteUser(ctx, sys, store.User{Email: "bea@example.com", Role: "viewer", CreatedAt: 2}, "link9", 100); err != store.ErrExists {
			t.Errorf("invite again: %v", err)
		}
		// An invited admin cannot log in, so the hub may not rely on them.
		yes := true
		if _, err := s.UpdateUser(ctx, sys, admin.ID, store.UserChange{Disabled: &yes}, 5); err != store.ErrLastAdmin {
			t.Errorf("disable the only admin who can log in: %v", err)
		}
		if got, _ := s.UserByID(ctx, admin.ID); got.DisabledAt != nil {
			t.Error("the refused change stuck")
		}
		if _, err := s.UserByLink(ctx, "link1", 100); err != store.ErrNotFound {
			t.Errorf("an expired link: %v", err)
		}
		if _, err := s.SetUserLink(ctx, sys, u.ID, "link2", 200); err != nil {
			t.Fatal(err)
		}
		if _, err := s.UserByLink(ctx, "link1", 50); err != store.ErrNotFound {
			t.Errorf("a replaced link: %v", err)
		}
		if err := s.CreateSession(ctx, store.Session{IDHash: "old", UserID: u.ID, CreatedAt: 1, ExpiresAt: 1000, LastSeenAt: 1}); err != nil {
			t.Fatal(err)
		}
		got, err := s.UseLink(ctx, "link2", "Bea", "hash", 150)
		if err != nil || got.Invited() || got.Name != "Bea" || got.LinkExpiresAt != nil {
			t.Fatalf("use: %+v, %v", got, err)
		}
		if _, _, err := s.SessionUser(ctx, "old", 160); err != store.ErrNotFound {
			t.Errorf("a session from before the new password: %v", err)
		}
		if _, err := s.UseLink(ctx, "link2", "", "hash2", 150); err != store.ErrNotFound {
			t.Errorf("a spent link: %v", err)
		}
		if err := s.WithdrawInvitation(ctx, sys, u.ID); err != store.ErrNotInvited {
			t.Errorf("withdraw an accepted invitation: %v", err)
		}
		// Now Bea can log in, so the first admin may go.
		if _, err := s.UpdateUser(ctx, sys, admin.ID, store.UserChange{Disabled: &yes}, 5); err != nil {
			t.Errorf("disable an admin while another remains: %v", err)
		}
		changes, _ := s.Changes(ctx, "global", 1, 50)
		if len(changes) < 4 {
			t.Errorf("history: %d changes", len(changes))
		}
	})
}
