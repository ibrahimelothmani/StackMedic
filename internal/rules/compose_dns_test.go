package rules

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

// loadFixture reads a redacted snapshot recorded from examples/compose-dns-mismatch.
func loadFixture(t *testing.T) *collect.Snapshot {
	t.Helper()
	b, err := os.ReadFile("testdata/dns-mismatch.json")
	if err != nil {
		t.Fatal(err)
	}
	var s collect.Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

func ptr(s string) *string { return &s }

func TestDNSMismatchFixture(t *testing.T) {
	got := Run(loadFixture(t))
	if len(got) != 1 {
		t.Fatalf("got %d findings, want 1: %+v", len(got), got)
	}
	f := got[0]
	if f.ID != "finding-001" || f.RuleID != "SM-COMPOSE-001" || f.Severity != "high" || f.Resource != "compose://api" {
		t.Errorf("finding header = %+v", f)
	}
	if f.Recommendation != `Change DATABASE_HOST to "postgres".` {
		t.Errorf("recommendation = %q", f.Recommendation)
	}
	ev := ""
	for _, e := range f.Evidence {
		ev += e.Source + ": " + e.Message + "\n"
	}
	for _, want := range []string{
		"container_log: api log: [api] ERROR: unable to resolve database hostname 'database'",
		"compose_config: api environment: DATABASE_HOST=database",
		"compose_config: Compose services: api, postgres",
		"compose_config: api depends_on: postgres",
	} {
		if !strings.Contains(ev, want) {
			t.Errorf("evidence missing %q in:\n%s", want, ev)
		}
	}
	if f.ChangesApplied {
		t.Error("diagnosis must never apply changes")
	}
}

func TestDNSMismatchFixedIsSilent(t *testing.T) {
	s := loadFixture(t)
	// What a real fixed run looks like: new config, and api resolved the host and exited 0.
	s.Services[0].Environment["DATABASE_HOST"] = ptr("postgres")
	s.Services[0].Containers[0].ExitCode, s.Services[0].Containers[0].RestartCount = 0, 0
	s.Services[0].Logs = []string{"[api] Database hostname 'postgres' resolves successfully"}
	if got := Run(s); len(got) != 0 {
		t.Fatalf("fixed stack still reports: %+v", got)
	}
}

func TestDNSMismatchWithoutLogsIsMedium(t *testing.T) {
	s := loadFixture(t)
	s.Services[0].Logs = nil
	got := Run(s)
	if len(got) != 1 || got[0].Severity != "medium" || !strings.Contains(got[0].Explanation, "external host") {
		t.Fatalf("got %+v", got)
	}
}

func TestDNSMismatchIgnoresKnownAndExternalHosts(t *testing.T) {
	s := &collect.Snapshot{Services: []collect.Service{
		{Name: "db", Networks: map[string]*collect.ServiceNetwork{"default": {Aliases: []string{"primary"}}}},
		{Name: "cache", ContainerName: "redis-main"},
		{Name: "app", Environment: map[string]*string{
			"DB_HOST":      ptr("primary"),                               // network alias
			"CACHE_URL":    ptr("redis://:[REDACTED]@redis-main:6379/0"), // container_name inside URL
			"API_URL":      ptr("https://api.example.com/v1"),            // external FQDN
			"BIND_ADDRESS": ptr("0.0.0.0"),                               // IP
			"SELF_HOST":    ptr("localhost"),                             // loopback
			"STORE_DSN":    ptr("sqlite:///data/app.db"),                 // no host
			"DB_PORT":      ptr("5432"),                                  // not a host key
		}},
	}}
	if got := Run(s); len(got) != 0 {
		t.Fatalf("unexpected findings: %+v", got)
	}
}

func TestHostOf(t *testing.T) {
	for in, want := range map[string]string{
		"database":      "database",
		"database:5432": "database",
		"postgres://[REDACTED]@database:5432/app": "database",
		"http://svc/health?x=1":                   "svc",
		"[::1]:8080":                              "::1",
	} {
		if got := hostOf(in); got != want {
			t.Errorf("hostOf(%q) = %q, want %q", in, got, want)
		}
	}
}
