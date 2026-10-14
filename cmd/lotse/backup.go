package main

import (
	"context"
	"errors"
	"fmt"

	"lotse.sh/internal/hub"
)

// backupCommand copies a SQLite hub's database, with the hub's own
// configuration (LOTSE_DATA_DIR, LOTSE_DATABASE_URL).
func backupCommand(ctx context.Context, args []string) error {
	if len(args) != 1 {
		return errors.New("usage: lotse backup FILE")
	}
	cfg := hub.ConfigFromEnv()
	if err := hub.Backup(ctx, cfg, args[0]); err != nil {
		return err
	}
	fmt.Printf("wrote %s. Keep the secret key as well (%s/secret.key, or LOTSE_SECRET_KEY): it unseals the secrets in it.\n", args[0], cfg.DataDir)
	return nil
}
