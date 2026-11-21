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
	"runtime/debug"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
	"github.com/ibrahimelothmani/StackMedic/internal/redact"
	"github.com/ibrahimelothmani/StackMedic/internal/report"
	"github.com/ibrahimelothmani/StackMedic/internal/rules"
)

// version is overridden at release time via -ldflags "-X main.version=...".
var version = "dev"

const usage = `StackMedic: evidence-first diagnostics for broken stacks.

Usage:
  stackmedic <command> [flags]

Commands:
  diagnose  Diagnose a Docker Compose project (read-only)
  snapshot  Print the collected, redacted Compose state as JSON (read-only)
  version   Print the StackMedic version
  help      Show this help

Flags (diagnose, snapshot):
  -file <path>   Compose file (default: Compose's own lookup)
  -tail <n>      Log lines to read per service (default 200)
  -format <f>    diagnose only: text or json (default text)

Exit codes: 0 no findings, 1 findings reported, 2 error.
`

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

// run returns the process exit code: 0 ok, 1 findings reported, 2 error.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprint(stderr, usage)
		return 2
	}
	switch args[0] {
	case "diagnose":
		return diagnose(args[1:], stdout, stderr)
	case "snapshot":
		return snapshot(args[1:], stdout, stderr)
	case "version", "--version":
		v := version
		// `go install ...@v0.1.0` builds without ldflags; the module version is in the build info.
		if bi, ok := debug.ReadBuildInfo(); ok && v == "dev" && bi.Main.Version != "" && bi.Main.Version != "(devel)" {
			v = bi.Main.Version
		}
		fmt.Fprintf(stdout, "stackmedic %s\n", v)
		return 0
	case "help", "-h", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "stackmedic: unknown command %q\n\n%s", args[0], usage)
		return 2
	}
}

func diagnose(args []string, stdout, stderr io.Writer) int {
	fs, file, tail := newFlagSet("diagnose", stderr)
	format := fs.String("format", "text", "output format: text or json")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *format != "text" && *format != "json" {
		fmt.Fprintf(stderr, "stackmedic: unknown format %q (want text or json)\n", *format)
		return 2
	}
	snap := collectRedacted(*file, *tail, stderr)
	if snap == nil {
		return 2
	}

	findings := rules.Run(snap)
	if *format == "json" {
		if err := report.JSON(stdout, snap, findings); err != nil {
			fmt.Fprintf(stderr, "stackmedic: %v\n", err)
			return 2
		}
	} else {
		target := *file
		if target == "" {
			target = "project " + snap.Project
		}
		report.Text(stdout, target, snap, findings)
	}
	if len(findings) > 0 {
		return 1
	}
	return 0
}

func snapshot(args []string, stdout, stderr io.Writer) int {
	fs, file, tail := newFlagSet("snapshot", stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	snap := collectRedacted(*file, *tail, stderr)
	if snap == nil {
		return 2
	}
	enc := json.NewEncoder(stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(snap); err != nil {
		fmt.Fprintf(stderr, "stackmedic: %v\n", err)
		return 2
	}
	return 0
}

func newFlagSet(name string, stderr io.Writer) (fs *flag.FlagSet, file *string, tail *int) {
	fs = flag.NewFlagSet(name, flag.ContinueOnError)
	fs.SetOutput(stderr)
	file = fs.String("file", "", "Compose file (default: Compose's own lookup)")
	tail = fs.Int("tail", 200, "log lines to read per service")
	return fs, file, tail
}

// collectRedacted gathers the snapshot and masks secrets before anything else sees it.
// It returns nil after printing the error.
func collectRedacted(file string, tail int, stderr io.Writer) *collect.Snapshot {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	snap, err := collect.Collect(ctx, collect.Docker, file, tail)
	if err != nil {
		fmt.Fprintf(stderr, "stackmedic: %v\n", err)
		return nil
	}
	redact.Snapshot(snap)
	return snap
}
