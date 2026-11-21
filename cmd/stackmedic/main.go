// Command stackmedic diagnoses broken application stacks from local evidence.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
	"github.com/ibrahimelothmani/StackMedic/internal/redact"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `StackMedic: evidence-first diagnostics for broken stacks.

Usage:
  stackmedic <command>

Commands:
  snapshot  Print the collected Compose state as JSON (read-only)
            Flags: -file <compose.yaml> (default: Compose lookup), -tail <lines> (default 200)
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
	case "snapshot":
		return snapshot(args[1:], stdout, stderr)
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

func snapshot(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("snapshot", flag.ContinueOnError)
	fs.SetOutput(stderr)
	file := fs.String("file", "", "Compose file (default: Compose's own lookup)")
	tail := fs.Int("tail", 200, "log lines to keep per service")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	snap, err := collect.Collect(ctx, collect.Docker, *file, *tail)
	if err != nil {
		fmt.Fprintf(stderr, "stackmedic: %v\n", err)
		return 2
	}
	redact.Snapshot(snap)
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		fmt.Fprintf(stderr, "stackmedic: %v\n", err)
		return 2
	}
	return 0
}
