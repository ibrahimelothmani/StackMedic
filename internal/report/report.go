// Package report renders findings for humans (text) and machines (JSON).
package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"unicode"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
	"github.com/ibrahimelothmani/StackMedic/internal/rules"
)

// JSON writes {"project", "findings", "changes_applied"}; findings is [] (never null) when clean.
func JSON(w io.Writer, s *collect.Snapshot, findings []rules.Finding) error {
	if findings == nil {
		findings = []rules.Finding{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(struct {
		Project        string          `json:"project"`
		Findings       []rules.Finding `json:"findings"`
		ChangesApplied bool            `json:"changes_applied"`
	}{s.Project, findings, false})
}

// Text writes the terminal report. target names what was diagnosed (file or project).
func Text(w io.Writer, target string, s *collect.Snapshot, findings []rules.Finding) {
	ok, failing, notStarted := 0, 0, 0
	for _, svc := range s.Services {
		switch {
		case len(svc.Containers) == 0:
			notStarted++
		case healthy(svc):
			ok++
		default:
			failing++
		}
	}

	fmt.Fprintln(w, "  🩺 StackMedic · Local diagnosis")
	fmt.Fprintln(w, "  "+strings.Repeat("─", 51))
	field(w, "Target", target)
	field(w, "Services", fmt.Sprintf("%d discovered", len(s.Services)))
	field(w, "Health", fmt.Sprintf("%d ok · %d failing · %d not started", ok, failing, notStarted))

	for _, f := range findings {
		fmt.Fprintf(w, "\n  %-5s %s · %s\n\n", strings.ToUpper(f.Severity), f.RuleID, printable(f.Summary))
		field(w, "Affected", strings.TrimPrefix(f.Resource, "compose://"))
		field(w, "Suspected", f.Explanation)
		var ev []string
		for _, e := range f.Evidence {
			ev = append(ev, "• "+e.Message)
		}
		field(w, "Evidence", ev...)
		field(w, "Try next", f.Recommendation)
		field(w, "Verify", f.Verification)
	}

	noun := "findings"
	if len(findings) == 1 {
		noun = "finding"
	}
	fmt.Fprintln(w)
	field(w, "Summary", fmt.Sprintf("%d %s · 0 changes applied", len(findings), noun))
}

// healthy: every container is running and none reports an unhealthy probe.
func healthy(svc collect.Service) bool {
	for _, c := range svc.Containers {
		if c.Status != "running" || c.Health == "unhealthy" {
			return false
		}
	}
	return true
}

// field prints a labelled value; extra lines align under the first.
func field(w io.Writer, label string, lines ...string) {
	for i, l := range lines {
		if i > 0 {
			label = ""
		}
		fmt.Fprintf(w, "  %-12s %s\n", label, printable(l))
	}
}

// printable neutralizes control characters (ANSI/OSC escapes, CR, BEL) from untrusted
// log and config text so they cannot rewrite or hijack the user's terminal.
func printable(s string) string {
	return strings.Map(func(r rune) rune {
		if r == '\t' {
			return ' '
		}
		if !unicode.IsPrint(r) {
			return '�'
		}
		return r
	}, s)
}
