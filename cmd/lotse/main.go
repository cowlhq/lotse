// lotse is the hub, the agent, the probe and the command line, in one binary.
package main

import (
	"fmt"
	"os"

	"lotse.sh/internal/version"
)

const usage = `usage: lotse <command>

commands:
  version   print the version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "version":
		fmt.Println(version.Version)
	default:
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "lotse:", err)
		os.Exit(1)
	}
}
