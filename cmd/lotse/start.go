package main

import (
	"cmp"
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"lotse.sh/internal/update"
	"lotse.sh/internal/version"
)

// startCommand runs the hub or the agent through the starter, which picks
// the newest installed version (decision 0009). It returns the exit code.
func startCommand(ctx context.Context, args []string, log *slog.Logger) int {
	if len(args) == 0 || (args[0] != "hub" && args[0] != "agent") {
		fmt.Fprintln(os.Stderr, "usage: lotse start hub|agent [flags]")
		return 2
	}
	own, err := os.Executable()
	if err != nil {
		fmt.Fprintln(os.Stderr, "lotse:", err)
		return 1
	}
	dataDir := cmp.Or(os.Getenv("LOTSE_DATA_DIR"), "/var/lib/lotse")
	s := &update.Starter{
		Dir:        cmp.Or(os.Getenv("LOTSE_BIN_DIR"), filepath.Join(dataDir, "bin")),
		Own:        own,
		OwnVersion: version.Version,
		Log:        log.With("starter", true),
	}
	return s.Run(ctx, args)
}
