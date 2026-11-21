//go:build integration

// End-to-end tests against real Docker. Run with:
//
//	go test -tags integration -run Integration -count=1 ./cmd/stackmedic
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
	"github.com/ibrahimelothmani/StackMedic/internal/rules"
)

const fixtureDir = "../../examples/compose-dns-mismatch"

func TestIntegrationDNSMismatch(t *testing.T) {
	file, err := filepath.Abs(filepath.Join(fixtureDir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	up(t, "stackmedic-it-broken", file)
	waitFor(t, file, "api to log its DNS failure", func(api *collect.Service) bool {
		return strings.Contains(strings.Join(api.Logs, "\n"), "unable to resolve")
	})

	var out, errOut bytes.Buffer
	if code := run([]string{"diagnose", "-file", file, "-format", "json"}, &out, &errOut); code != 1 {
		t.Fatalf("diagnose exit = %d, want 1\nstdout: %s\nstderr: %s", code, out.String(), errOut.String())
	}
	var rep struct{ Findings []rules.Finding }
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatal(err)
	}
	if len(rep.Findings) != 1 {
		t.Fatalf("findings = %+v, want exactly 1", rep.Findings)
	}
	if f := rep.Findings[0]; f.RuleID != "SM-COMPOSE-001" || f.Severity != "high" || f.Recommendation != `Change DATABASE_HOST to "postgres".` {
		t.Errorf("finding = %+v", f)
	}

	out.Reset()
	if code := run([]string{"snapshot", "-file", file}, &out, &errOut); code != 0 {
		t.Fatalf("snapshot exit = %d: %s", code, errOut.String())
	}
	if strings.Contains(out.String(), "local-demo-only-password") {
		t.Error("snapshot leaked the fixture password")
	}
}

func TestIntegrationDNSFixedIsClean(t *testing.T) {
	dir := t.TempDir()
	src, err := os.ReadFile(filepath.Join(fixtureDir, "compose.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	fixed := strings.Replace(string(src), "DATABASE_HOST: database", "DATABASE_HOST: postgres", 1)
	if fixed == string(src) {
		t.Fatal("fixture changed: could not apply the fix")
	}
	app, err := os.ReadFile(filepath.Join(fixtureDir, "app.py"))
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(dir, "compose.yaml")
	if err := os.WriteFile(file, []byte(fixed), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "app.py"), app, 0o644); err != nil {
		t.Fatal(err)
	}

	up(t, "stackmedic-it-fixed", file)
	waitFor(t, file, "api to exit successfully", func(api *collect.Service) bool {
		return len(api.Containers) > 0 && api.Containers[0].Status == "exited" && api.Containers[0].ExitCode == 0
	})

	var out, errOut bytes.Buffer
	if code := run([]string{"diagnose", "-file", file}, &out, &errOut); code != 0 {
		t.Fatalf("diagnose exit = %d, want 0\nstdout: %s\nstderr: %s", code, out.String(), errOut.String())
	}
	if !strings.Contains(out.String(), "0 findings") || !strings.Contains(out.String(), "2 ok · 0 failing") {
		t.Errorf("want clean report, got:\n%s", out.String())
	}
}

// up starts the stack under an isolated project name and always tears it down.
func up(t *testing.T, project, file string) {
	t.Helper()
	t.Setenv("COMPOSE_PROJECT_NAME", project) // also picked up by the CLI's own docker calls
	t.Cleanup(func() { docker(t, "compose", "-p", project, "-f", file, "down", "-v", "--remove-orphans") })
	// Broken fixtures may make `up` itself fail (unhealthy dependency, port clash, bad mount); the assertions decide.
	if out, err := exec.Command("docker", "compose", "-p", project, "-f", file, "up", "-d").CombinedOutput(); err != nil {
		t.Logf("compose up (expected for some fixtures): %v\n%s", err, out)
	}
}

// Every broken fixture must be diagnosed as exactly its rule, end to end.
func TestIntegrationFixtures(t *testing.T) {
	for _, tt := range []struct{ fixture, rule string }{
		{"compose-crash-loop", "SM-COMPOSE-002"},
		{"compose-unhealthy", "SM-COMPOSE-003"},
		{"compose-oom", "SM-COMPOSE-004"},
		{"compose-port-collision", "SM-COMPOSE-005"},
		{"compose-missing-mount", "SM-COMPOSE-006"},
		{"compose-db-refused", "SM-COMPOSE-007"},
	} {
		t.Run(tt.fixture, func(t *testing.T) {
			file, err := filepath.Abs(filepath.Join("../../examples", tt.fixture, "compose.yaml"))
			if err != nil {
				t.Fatal(err)
			}
			up(t, "stackmedic-it-"+strings.TrimPrefix(tt.fixture, "compose-"), file)

			deadline := time.Now().Add(3 * time.Minute)
			for {
				var out, errOut bytes.Buffer
				code := run([]string{"diagnose", "-file", file, "-format", "json"}, &out, &errOut)
				var rep struct{ Findings []rules.Finding }
				_ = json.Unmarshal(out.Bytes(), &rep)
				if code == 1 && len(rep.Findings) == 1 && rep.Findings[0].RuleID == tt.rule && rep.Findings[0].Severity == "high" {
					return
				}
				if time.Now().After(deadline) {
					t.Fatalf("want exactly one high %s finding; exit %d\nstdout: %s\nstderr: %s", tt.rule, code, out.String(), errOut.String())
				}
				time.Sleep(2 * time.Second)
			}
		})
	}
}

func docker(t *testing.T, args ...string) {
	t.Helper()
	if out, err := exec.Command("docker", args...).CombinedOutput(); err != nil {
		t.Fatalf("docker %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

// waitFor polls the real snapshot until cond holds for the api service.
func waitFor(t *testing.T, file, what string, cond func(*collect.Service) bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Minute)
	for time.Now().Before(deadline) {
		if s, err := collect.Collect(context.Background(), collect.Docker, file, 50); err == nil {
			for i := range s.Services {
				if s.Services[i].Name == "api" && cond(&s.Services[i]) {
					return
				}
			}
		}
		time.Sleep(2 * time.Second)
	}
	t.Fatalf("timed out waiting for %s", what)
}
