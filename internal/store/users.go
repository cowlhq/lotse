package store

import "context"

// CountUsers says how many users there are. A hub without any is not set up
// yet.
func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.queryRow(ctx, `SELECT count(*) FROM users`).Scan(&n)
	return n, err
}
