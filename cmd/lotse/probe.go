package main

import (
	"cmp"
	"context"
	"flag"
	"log/slog"
	"os"

	"lotse.sh/internal/probe"
)

func probeCommand(ctx context.Context, args []string, log *slog.Logger) error {
	enroll := len(args) > 0 && args[0] == "enroll"
	if enroll {
		args = args[1:]
	}
	flags := flag.NewFlagSet("lotse probe", flag.ExitOnError)
	var cfg probe.Config
	flags.StringVar(&cfg.Hub, "hub", os.Getenv("LOTSE_HUB"), "the hub's address (or LOTSE_HUB)")
	flags.StringVar(&cfg.Token, "token", os.Getenv("LOTSE_TOKEN"), "the probe token, needed once (or LOTSE_TOKEN)")
	flags.StringVar(&cfg.Slug, "name", os.Getenv("LOTSE_PROBE_NAME"), "the probe's short name, such as fra (or LOTSE_PROBE_NAME)")
	flags.StringVar(&cfg.Location, "location", os.Getenv("LOTSE_PROBE_LOCATION"), "where it is, such as Frankfurt (or LOTSE_PROBE_LOCATION)")
	flags.StringVar(&cfg.DataDir, "data-dir", cmp.Or(os.Getenv("LOTSE_DATA_DIR"), "/var/lib/lotse"), "where the probe keeps its credential (or LOTSE_DATA_DIR)")
	flags.BoolVar(&cfg.Insecure, "insecure", os.Getenv("LOTSE_INSECURE") == "true", "allow plain HTTP to a hub on another machine")
	flags.Parse(args)
	cfg.Name = cfg.Slug
	if enroll {
		return probe.Enroll(ctx, cfg, log)
	}
	return probe.Run(ctx, cfg, log)
}
