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
