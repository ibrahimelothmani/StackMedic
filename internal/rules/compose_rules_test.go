package rules

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

func load(t *testing.T, name string) *collect.Snapshot {
	t.Helper()
	b, err := os.ReadFile("testdata/" + name + ".json")
	if err != nil {
		t.Fatal(err)
	}
	var s collect.Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatal(err)
	}
	return &s
}

func svc(s *collect.Snapshot, name string) *collect.Service {
	for i := range s.Services {
		if s.Services[i].Name == name {
			return &s.Services[i]
		}
	}
	panic("no service " + name)
}

func evidenceText(f Finding) string {
	var b strings.Builder
	for _, e := range f.Evidence {
		b.WriteString(e.Source + ": " + e.Message + "\n")
	}
	return b.String()
}

// Each recorded fixture must yield exactly its rule, and the fixed state must be silent.
func TestFixtures(t *testing.T) {
	tests := []struct {
		fixture, rule, resource, recommendation string
		evidence                                []string
		fix                                     func(*collect.Snapshot)
	}{
		{
			fixture: "crash-loop", rule: "SM-COMPOSE-002", resource: "compose://worker",
			recommendation: "Fix the error reported in the last log lines",
			evidence:       []string{"exit_code=3 restart_count=3", "FATAL: required setting QUEUE_NAME is empty"},
			fix: func(s *collect.Snapshot) {
				c := &svc(s, "worker").Containers[0]
				c.Status, c.ExitCode, c.RestartCount = "running", 0, 0
			},
		},
		{
			fixture: "unhealthy", rule: "SM-COMPOSE-003", resource: "compose://postgres",
			recommendation: `docker compose exec postgres sh -c "pg_isready -U demo -d demo -p 5433"`,
			evidence:       []string{"latest probe output: /run/postgresql:5433 - no response", "waiting for it with condition service_healthy: api"},
			fix:            func(s *collect.Snapshot) { svc(s, "postgres").Containers[0].Health = "healthy" },
		},
		{
			fixture: "oom", rule: "SM-COMPOSE-004", resource: "compose://importer",
			recommendation: "Raise the memory limit",
			evidence:       []string{"oom_killed=true exit_code=137", "importer mem_limit: 64 MiB"},
			fix: func(s *collect.Snapshot) {
				c := &svc(s, "importer").Containers[0]
				c.OOMKilled, c.ExitCode = false, 0
			},
		},
		{
			fixture: "port-collision", rule: "SM-COMPOSE-005", resource: "compose://admin",
			recommendation: `Publish a different host port for admin, e.g. "18081:8000".`,
			evidence:       []string{"web publishes 18080/tcp", "admin publishes 18080/tcp", "port is already allocated"},
			fix: func(s *collect.Snapshot) {
				svc(s, "admin").Ports[0].Published = "18081"
				svc(s, "admin").Containers[0] = collect.Container{Name: "admin", Status: "running"}
			},
		},
		{
			fixture: "missing-mount", rule: "SM-COMPOSE-006", resource: "compose://proxy",
			recommendation: "/work/examples/compose-missing-mount/settings.json on the host",
			evidence:       []string{"settings.json: missing"},
			fix:            func(s *collect.Snapshot) { svc(s, "proxy").Volumes[0].SourceState = "file" },
		},
		{
			fixture: "db-refused", rule: "SM-COMPOSE-007", resource: "compose://api",
			recommendation: `Change DATABASE_PORT to "5432".`,
			evidence:       []string{"[Errno 111] Connection refused", "DATABASE_PORT=5433", "postgres image default port: 5432"},
			fix: func(s *collect.Snapshot) {
				api := svc(s, "api")
				api.Environment["DATABASE_PORT"] = ptr("5432")
				api.Containers[0].ExitCode, api.Containers[0].RestartCount = 0, 0
				api.Logs = []string{"[api] Connected to postgres:5432"}
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.fixture, func(t *testing.T) {
			got := Run(load(t, tt.fixture))
			if len(got) != 1 {
				t.Fatalf("got %d findings, want 1: %+v", len(got), got)
			}
			f := got[0]
			if f.RuleID != tt.rule || f.Severity != "high" || f.Resource != tt.resource {
				t.Errorf("finding = %s %s %s", f.RuleID, f.Severity, f.Resource)
			}
			if !strings.Contains(f.Recommendation, tt.recommendation) {
				t.Errorf("recommendation = %q, want it to contain %q", f.Recommendation, tt.recommendation)
			}
			ev := evidenceText(f)
			for _, want := range tt.evidence {
				if !strings.Contains(ev, want) {
					t.Errorf("evidence missing %q in:\n%s", want, ev)
				}
			}

			fixed := load(t, tt.fixture)
			tt.fix(fixed)
			if got := Run(fixed); len(got) != 0 {
				t.Errorf("fixed state still reports: %+v", got)
			}
		})
	}
}

func TestCrashSymptomDroppedWhenExplained(t *testing.T) {
	// db-refused's api also crash-loops; only the specific cause must be reported.
	for _, f := range Run(load(t, "db-refused")) {
		if f.RuleID == "SM-COMPOSE-002" {
			t.Fatalf("crash symptom not suppressed: %+v", f)
		}
	}
}

func TestCrashEdgeCases(t *testing.T) {
	run := func(c collect.Container) []Finding {
		return Run(&collect.Snapshot{Services: []collect.Service{{Name: "app", Containers: []collect.Container{c}}}})
	}
	if got := run(collect.Container{Status: "exited", ExitCode: 143}); len(got) != 0 {
		t.Errorf("SIGTERM stop reported as crash: %+v", got)
	}
	got := run(collect.Container{Status: "exited", ExitCode: 127})
	if len(got) != 1 || got[0].Severity != "medium" || !strings.Contains(got[0].Recommendation, "command was not found") {
		t.Errorf("exit 127 = %+v", got)
	}
}

func TestPortHeldByHostProcess(t *testing.T) {
	s := load(t, "port-collision")
	s.Services = s.Services[:1] // drop web: nothing else in the file claims 18080
	got := Run(s)
	if len(got) != 1 || got[0].Summary != "admin cannot bind host port 18080" || !strings.Contains(got[0].Recommendation, "ss -ltnp | grep ':18080'") {
		t.Fatalf("got %+v", got)
	}
}

func TestBindSourceIsDirectory(t *testing.T) {
	s := load(t, "missing-mount")
	svc(s, "proxy").Volumes[0].SourceState = "dir"
	got := Run(s)
	if len(got) != 1 || got[0].Severity != "medium" || !strings.Contains(got[0].Summary, "is a directory, not a file") {
		t.Fatalf("got %+v", got)
	}
}

func TestConnRefusedVariants(t *testing.T) {
	t.Run("target down", func(t *testing.T) {
		s := load(t, "db-refused")
		svc(s, "postgres").Containers[0].Status = "exited"
		got := Run(s)
		if len(got) != 1 || got[0].Summary != "api cannot connect: postgres is not running" {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("startup race", func(t *testing.T) {
		s := load(t, "db-refused")
		api := svc(s, "api")
		api.Environment["DATABASE_PORT"] = ptr("5432")
		api.DependsOn = nil
		got := Run(s)
		if len(got) != 1 || got[0].Severity != "medium" || !strings.Contains(got[0].Recommendation, "condition: service_healthy") {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("port in URL", func(t *testing.T) {
		s := load(t, "db-refused")
		api := svc(s, "api")
		delete(api.Environment, "DATABASE_HOST")
		delete(api.Environment, "DATABASE_PORT")
		api.Environment["DATABASE_URL"] = ptr("postgres://demo:[REDACTED]@postgres:5433/demo")
		got := Run(s)
		if len(got) != 1 || got[0].Recommendation != "Change the port in DATABASE_URL to 5432." {
			t.Fatalf("got %+v", got)
		}
	})
}

func TestImageName(t *testing.T) {
	for in, want := range map[string]string{
		"postgres:17-alpine":           "postgres",
		"docker.io/library/redis":      "redis",
		"ghcr.io/org/mysql@sha256:abc": "mysql",
		"registry:5000/team/mongo:7":   "mongo",
	} {
		if got := imageName(in); got != want {
			t.Errorf("imageName(%q) = %q, want %q", in, got, want)
		}
	}
}
