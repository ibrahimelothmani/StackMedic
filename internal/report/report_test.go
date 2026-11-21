package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
	"github.com/ibrahimelothmani/StackMedic/internal/rules"
)

var snap = &collect.Snapshot{Project: "demo", Services: []collect.Service{
	{Name: "api", Containers: []collect.Container{{Status: "exited", ExitCode: 1}}},
	{Name: "migrate", Containers: []collect.Container{{Status: "exited", ExitCode: 0}}}, // finished one-shot job
	{Name: "postgres", Containers: []collect.Container{{Status: "running", Health: "healthy"}}},
	{Name: "worker"},
}}

var finding = rules.Finding{
	ID: "finding-001", RuleID: "SM-COMPOSE-001", Severity: "high", Resource: "compose://api",
	Summary:     `api cannot resolve host "database"`,
	Explanation: "DATABASE_HOST points to \"database\"...",
	Evidence: []rules.Evidence{
		{Source: "container_log", Message: "api log: lookup database: no such host"},
		{Source: "compose_config", Message: "api environment: DATABASE_HOST=database"},
	},
	Recommendation: `Change DATABASE_HOST to "postgres".`,
	Verification:   "Recreate the service...",
}

func TestText(t *testing.T) {
	var b bytes.Buffer
	Text(&b, "compose.yaml", snap, []rules.Finding{finding})
	out := b.String()
	for _, want := range []string{
		"  Target       compose.yaml\n",
		"  Health       2 ok · 1 failing · 1 not started\n",
		"  HIGH  SM-COMPOSE-001 · api cannot resolve host \"database\"\n",
		"  Affected     api\n",
		"  Evidence     • api log: lookup database: no such host\n" +
			"               • api environment: DATABASE_HOST=database\n",
		"  Try next     Change DATABASE_HOST to \"postgres\".\n",
		"  Summary      1 finding · 0 changes applied\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
}

func TestTextCountsFlaggedRunningServiceAsFailing(t *testing.T) {
	s := &collect.Snapshot{Services: []collect.Service{
		{Name: "api", Containers: []collect.Container{{Status: "running"}}}, // between crash-loop restarts
		{Name: "postgres", Containers: []collect.Container{{Status: "running"}}},
	}}
	var b bytes.Buffer
	Text(&b, "compose.yaml", s, []rules.Finding{finding})
	if !strings.Contains(b.String(), "Health       1 ok · 1 failing · 0 not started") {
		t.Errorf("got:\n%s", b.String())
	}
}

func TestTextNeutralizesTerminalEscapes(t *testing.T) {
	evil := finding
	evil.Summary = "api \x1b]0;pwned\x07 fails"
	evil.Evidence = []rules.Evidence{{Source: "container_log", Message: "log \x1b[2J\x1b[31mfake OK\r overwritten"}}
	var b bytes.Buffer
	Text(&b, "compose.yaml", snap, []rules.Finding{evil})
	if strings.ContainsAny(b.String(), "\x1b\x07\r") {
		t.Fatalf("control characters reached the terminal: %q", b.String())
	}
	if !strings.Contains(b.String(), "fake OK") {
		t.Errorf("printable text should survive: %q", b.String())
	}
}

func TestJSONCleanIsEmptyArray(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, snap, nil); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(b.String(), `"findings": []`) {
		t.Errorf("want empty findings array, got %s", b.String())
	}
}

func TestJSONRoundTrip(t *testing.T) {
	var b bytes.Buffer
	if err := JSON(&b, snap, []rules.Finding{finding}); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Project        string          `json:"project"`
		Findings       []rules.Finding `json:"findings"`
		ChangesApplied bool            `json:"changes_applied"`
	}
	if err := json.Unmarshal(b.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Project != "demo" || len(got.Findings) != 1 || got.Findings[0].RuleID != "SM-COMPOSE-001" || got.ChangesApplied {
		t.Errorf("round trip = %+v", got)
	}
}
