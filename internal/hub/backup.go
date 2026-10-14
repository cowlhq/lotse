package hub

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"

	"lotse.sh/internal/store"
)

// Backup writes a consistent copy of a SQLite hub's database to path, while
// the hub runs (lotse backup). It finds the database as the hub does, and
// does not overwrite a file that is there.
func Backup(ctx context.Context, cfg Config, path string) error {
	saved, err := readHubFile(cfg.DataDir)
	if err != nil {
		return err
	}
	url := cmp.Or(cfg.DatabaseURL, saved.DatabaseURL)
	if url == "" {
		return errors.New("this hub has no database yet")
	}
	if _, err := os.Stat(path); err == nil {
		return fmt.Errorf("%s is there already", path)
	}
	db, err := store.OpenAsIs(url)
	if err != nil {
		return err
	}
	defer db.Close()
	return db.Backup(ctx, path)
}
