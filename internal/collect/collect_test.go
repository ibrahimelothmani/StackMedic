package collect

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// fakeDocker answers docker commands from testdata recorded off examples/compose-dns-mismatch.
func fakeDocker(t *testing.T, responses map[string]string) Runner {
	return func(_ context.Context, args ...string) ([]byte, error) {
		key := strings.Join(args, " ")
		file, ok := responses[key]
		if !ok {
			t.Fatalf("unexpected docker call: %q", key)
		}
		if file == "" {
			return nil, nil
		}
		return os.ReadFile("testdata/" + file)
	}
}

const f = "compose -f compose.yaml "

func TestCollectFixture(t *testing.T) {
	ps, _ := os.ReadFile("testdata/ps.txt")
	run := fakeDocker(t, map[string]string{
		f + "config --format json": "config.json",
		f + "ps -a -q":             "ps.txt",
		"inspect " + strings.Join(strings.Fields(string(ps)), " "): "inspect.json",
		f + "logs --no-color --no-log-prefix --tail 50 api":        "logs-api.txt",
		f + "logs --no-color --no-log-prefix --tail 50 postgres":   "logs-postgres.txt",
	})

	snap, err := Collect(context.Background(), run, "compose.yaml", 50)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Project != "compose-dns-mismatch" || len(snap.Services) != 2 {
		t.Fatalf("got project %q with %d services", snap.Project, len(snap.Services))
	}

	api, pg := snap.Services[0], snap.Services[1]
	if api.Name != "api" || pg.Name != "postgres" {
		t.Fatalf("services not sorted: %s, %s", api.Name, pg.Name)
	}
	if h := api.Environment["DATABASE_HOST"]; h == nil || *h != "database" {
		t.Errorf("api DATABASE_HOST = %v, want database", h)
	}
	if api.DependsOn["postgres"].Condition != "service_healthy" {
		t.Errorf("api depends_on = %+v", api.DependsOn)
	}
	if len(api.Containers) != 1 {
		t.Fatalf("api containers = %d, want 1", len(api.Containers))
	}
	c := api.Containers[0]
	if c.Name != "compose-dns-mismatch-api-1" || c.Status != "exited" || c.ExitCode != 1 || c.RestartCount != 3 {
		t.Errorf("api container = %+v", c)
	}
	if len(api.Logs) == 0 || !strings.Contains(strings.Join(api.Logs, "\n"), "unable to resolve database hostname 'database'") {
		t.Errorf("api logs missing DNS error: %q", api.Logs)
	}
	if pg.Containers[0].Health != "healthy" || pg.Healthcheck == nil || len(pg.Healthcheck.Test) == 0 {
		t.Errorf("postgres health = %+v, healthcheck = %+v", pg.Containers[0], pg.Healthcheck)
	}
}

func TestCollectStackNotRunning(t *testing.T) {
	run := fakeDocker(t, map[string]string{
		"compose config --format json": "config.json",
		"compose ps -a -q":             "", // no containers: no inspect or logs calls expected
	})
	snap, err := Collect(context.Background(), run, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range snap.Services {
		if len(s.Containers) != 0 || len(s.Logs) != 0 {
			t.Errorf("%s: expected no runtime data, got %+v", s.Name, s)
		}
	}
}

func TestCollectConfigError(t *testing.T) {
	want := errors.New("docker compose config: exit status 1: no configuration file provided")
	run := func(context.Context, ...string) ([]byte, error) { return nil, want }
	if _, err := Collect(context.Background(), run, "", 50); !errors.Is(err, want) {
		t.Fatalf("err = %v, want %v", err, want)
	}
}
