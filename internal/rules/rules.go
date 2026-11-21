// Package rules evaluates a redacted snapshot and emits evidence-backed findings.
package rules

import (
	"fmt"
	"slices"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

// Evidence is one observed signal supporting a finding.
type Evidence struct {
	Source  string `json:"source"` // compose_config | container_log | container_state
	Message string `json:"message"`
}

// Finding is a supported hypothesis, not a guarantee.
type Finding struct {
	ID             string     `json:"id"`
	RuleID         string     `json:"rule_id"`
	Severity       string     `json:"severity"` // high | medium | low
	Resource       string     `json:"resource"` // compose://<service>
	Summary        string     `json:"summary"`
	Explanation    string     `json:"explanation"`
	Evidence       []Evidence `json:"evidence"`
	Recommendation string     `json:"recommendation"`
	Verification   string     `json:"verification"`
	ChangesApplied bool       `json:"changes_applied"`
}

// Rule inspects a snapshot. It must be deterministic and must not touch the system.
type Rule func(*collect.Snapshot) []Finding

var all = []Rule{
	composeDNSMismatch,  // SM-COMPOSE-001
	composeCrash,        // SM-COMPOSE-002 (symptom)
	composeUnhealthy,    // SM-COMPOSE-003
	composeOOM,          // SM-COMPOSE-004
	composePortConflict, // SM-COMPOSE-005
	composeBindMount,    // SM-COMPOSE-006
	composeConnRefused,  // SM-COMPOSE-007
}

// symptoms only describe what happened; they are dropped when another rule explains the same resource.
var symptoms = map[string]bool{"SM-COMPOSE-002": true}

// Run evaluates every rule and numbers the findings in a stable order.
// The snapshot must already be redacted.
func Run(s *collect.Snapshot) []Finding {
	findings := []Finding{}
	for _, r := range all {
		findings = append(findings, r(s)...)
	}
	explained := map[string]bool{}
	for _, f := range findings {
		if !symptoms[f.RuleID] {
			explained[f.Resource] = true
		}
	}
	findings = slices.DeleteFunc(findings, func(f Finding) bool { return symptoms[f.RuleID] && explained[f.Resource] })
	for i := range findings {
		findings[i].ID = fmt.Sprintf("finding-%03d", i+1)
	}
	return findings
}

// lastLogs returns up to n distinct trailing non-empty log lines, prefixed with the service name.
func lastLogs(svc collect.Service, n int) []Evidence {
	var ev []Evidence
	seen := map[string]bool{} // restarts repeat the same lines
	for i := len(svc.Logs) - 1; i >= 0 && len(ev) < n; i-- {
		if l := strings.TrimSpace(svc.Logs[i]); l != "" && !seen[l] {
			seen[l] = true
			ev = append(ev, Evidence{"container_log", svc.Name + " log: " + l})
		}
	}
	slices.Reverse(ev)
	return ev
}
