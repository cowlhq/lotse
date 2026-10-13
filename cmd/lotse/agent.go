package main

import (
	"cmp"
	"context"
	"flag"
	"log/slog"
	"os"
	"path/filepath"

	"lotse.sh/internal/agent"
)

func agentCommand(ctx context.Context, args []string, log *slog.Logger) error {
	enroll := len(args) > 0 && args[0] == "enroll"
	if enroll {
		args = args[1:]
	}
	flags := flag.NewFlagSet("lotse agent", flag.ExitOnError)
	var cfg agent.Config
	flags.StringVar(&cfg.Hub, "hub", os.Getenv("LOTSE_HUB"), "the hub's address (or LOTSE_HUB)")
	flags.StringVar(&cfg.Token, "token", os.Getenv("LOTSE_TOKEN"), "the environment's enrollment token, needed once (or LOTSE_TOKEN)")
	flags.StringVar(&cfg.DataDir, "data-dir", cmp.Or(os.Getenv("LOTSE_DATA_DIR"), "/var/lib/lotse"), "where the agent keeps its credential (or LOTSE_DATA_DIR)")
	flags.StringVar(&cfg.Root, "root", cmp.Or(os.Getenv("LOTSE_HOST_ROOT"), "/"), "where the machine's / is mounted, when the agent runs in a container (or LOTSE_HOST_ROOT)")
	flags.BoolVar(&cfg.Insecure, "insecure", os.Getenv("LOTSE_INSECURE") == "true", "allow plain HTTP to a hub on another machine")
	flags.StringVar(&cfg.ReleaseURL, "release-url", os.Getenv("LOTSE_RELEASE_URL"), "where updates come from, for a mirror (or LOTSE_RELEASE_URL); default GitHub")
	flags.Parse(args)
	cfg.BinDir = cmp.Or(os.Getenv("LOTSE_BIN_DIR"), filepath.Join(cfg.DataDir, "bin"))
	if enroll {
		return agent.Enroll(ctx, cfg, log)
	}
	return agent.Run(ctx, cfg, log)
}
