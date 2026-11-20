// Command stackmedic diagnoses broken application stacks from local evidence.
package main

import (
	"fmt"
	"io"
	"os"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `StackMedic: evidence-first diagnostics for broken stacks.

Usage:
  stackmedic <command>

Commands:
  version   Print the StackMedic version
  help      Show this help
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run returns the process exit code: 0 ok, 2 usage error (1 is reserved for "findings reported").
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "version", "--version":
		fmt.Fprintf(stdout, "stackmedic %s\n", version)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "stackmedic: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}
