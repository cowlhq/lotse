// lotse is the hub, the agent, the probe and the command line, in one binary.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"lotse.sh/internal/hub"
	"lotse.sh/internal/update"
	"lotse.sh/internal/version"
)

const usage = `usage: lotse <command>

commands:
  hub       run the hub (configured by LOTSE_* variables)
  agent     run the agent: lotse agent --hub URL --token TOKEN
            or only enroll it: lotse agent enroll --hub URL --token TOKEN
  start     run hub or agent in their newest installed version: lotse start hub|agent
  version   print the version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	var err error
	switch os.Args[1] {
	case "hub":
		err = hub.Run(ctx, hub.ConfigFromEnv(), log)
	case "agent":
		err = agentCommand(ctx, os.Args[2:], log)
	case "start":
		code := startCommand(ctx, os.Args[2:], log)
		stop()
		os.Exit(code)
	case "version":
		fmt.Println(version.Version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if errors.Is(err, update.ErrRestart) {
		// The starter runs the version just installed.
		stop()
		os.Exit(update.ExitRestart)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lotse:", err)
		os.Exit(1)
	}
}
