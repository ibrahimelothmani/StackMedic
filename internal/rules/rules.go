// Package rules evaluates a redacted snapshot and emits evidence-backed findings.
package rules

import (
	"fmt"

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

var all = []Rule{composeDNSMismatch}

// Run evaluates every rule and numbers the findings in a stable order.
// The snapshot must already be redacted.
func Run(s *collect.Snapshot) []Finding {
	findings := []Finding{}
	for _, r := range all {
		findings = append(findings, r(s)...)
	}
	for i := range findings {
		findings[i].ID = fmt.Sprintf("finding-%03d", i+1)
	}
	return findings
}
