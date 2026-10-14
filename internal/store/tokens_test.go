package store_test

import (
	"errors"
	"testing"

	"lotse.sh/internal/store"
	"lotse.sh/internal/store/storetest"
)

func TestTokens(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		env, _ := s.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
		tok, err := s.CreateToken(ctx, ada, store.Token{Name: "ci", Kind: "apply", ScopeID: *env.ParentID, CreatedBy: ada.Name, CreatedAt: 1}, "hash")
		if err != nil || tok.ScopePath != "acme/shop" {
			t.Fatalf("create: %+v, %v", tok, err)
		}
		got, err := s.TokenByHash(ctx, "hash", 100)
		if err != nil || got.ID != tok.ID || *got.LastUsedAt != 100 {
			t.Errorf("by hash: %+v, %v", got, err)
		}
		if err := s.RevokeToken(ctx, ada, tok.ID, 200); err != nil {
			t.Fatal(err)
		}
		if _, err := s.TokenByHash(ctx, "hash", 300); err != store.ErrNotFound {
			t.Errorf("a revoked token: %v", err)
		}
		list, _ := s.Tokens(ctx)
		if len(list) != 1 || *list[0].RevokedAt != 200 {
			t.Errorf("list: %+v", list)
		}
	})
}

// WithTx: all of it, or none.
func TestWithTx(t *testing.T) {
	storetest.Each(t, func(t *testing.T, s *store.Store) {
		ctx := t.Context()
		boom := errors.New("boom")
		err := s.WithTx(ctx, func(tx *store.Store) error {
			if _, err := tx.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}}); err != nil {
				return err
			}
			if list, _ := tx.Scopes(ctx); len(list) != 3 {
				t.Errorf("inside, %d scopes", len(list))
			}
			return boom
		})
		if err != boom {
			t.Fatal(err)
		}
		if list, _ := s.Scopes(ctx); len(list) != 0 {
			t.Errorf("rolled back, still %d scopes", len(list))
		}
		s.WithTx(ctx, func(tx *store.Store) error {
			_, err := tx.EnsureEnvironment(ctx, ada, [3]store.ScopeName{{Slug: "acme", Name: "ACME"}, {Slug: "shop", Name: "Shop"}, {Slug: "prod", Name: "Prod"}})
			return err
		})
		if list, _ := s.Scopes(ctx); len(list) != 3 {
			t.Errorf("committed, %d scopes", len(list))
		}
	})
}
