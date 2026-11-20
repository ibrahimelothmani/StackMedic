// Package collect gathers a read-only snapshot of a Docker Compose project:
// its resolved configuration, container runtime state and recent logs.
package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"slices"
	"strconv"
	"strings"
)

// Runner runs a docker CLI command and returns its stdout.
type Runner func(ctx context.Context, args ...string) ([]byte, error)

// Docker is the real Runner. Only read-only subcommands are ever passed to it.
func Docker(ctx context.Context, args ...string) ([]byte, error) {
	var stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "docker", args...)
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return out, nil
}

// Snapshot is everything StackMedic knows about a Compose project at one moment.
type Snapshot struct {
	Project  string    `json:"project"`
	Services []Service `json:"services"`
}

// Service mirrors the `docker compose config --format json` service shape, plus runtime data.
type Service struct {
	Name        string                     `json:"name"`
	Image       string                     `json:"image,omitempty"`
	Environment map[string]*string         `json:"environment,omitempty"` // nil value = declared without a value
	Ports       []Port                     `json:"ports,omitempty"`
	Volumes     []Volume                   `json:"volumes,omitempty"`
	DependsOn   map[string]Dependency      `json:"depends_on,omitempty"`
	Networks    map[string]*ServiceNetwork `json:"networks,omitempty"`
	Healthcheck *Healthcheck               `json:"healthcheck,omitempty"`
	Restart     string                     `json:"restart,omitempty"`

	Containers []Container `json:"containers,omitempty"`
	Logs       []string    `json:"logs,omitempty"`
}

type Port struct {
	Target    uint32 `json:"target"`
	Published string `json:"published,omitempty"`
	HostIP    string `json:"host_ip,omitempty"`
	Protocol  string `json:"protocol,omitempty"`
}

type Volume struct {
	Type     string `json:"type"`
	Source   string `json:"source,omitempty"`
	Target   string `json:"target"`
	ReadOnly bool   `json:"read_only,omitempty"`
}

type Dependency struct {
	Condition string `json:"condition"`
	Required  bool   `json:"required"`
}

type ServiceNetwork struct {
	Aliases []string `json:"aliases,omitempty"`
}

type Healthcheck struct {
	Test    []string `json:"test,omitempty"`
	Disable bool     `json:"disable,omitempty"`
}

// Container is the runtime state of one container of a service.
type Container struct {
	Name         string `json:"name"`
	Status       string `json:"status"`
	ExitCode     int    `json:"exit_code"`
	RestartCount int    `json:"restart_count"`
	OOMKilled    bool   `json:"oom_killed"`
	Error        string `json:"error,omitempty"`
	Health       string `json:"health,omitempty"`
	HealthOutput string `json:"health_output,omitempty"` // output of the latest health probe
	StartedAt    string `json:"started_at,omitempty"`
	FinishedAt   string `json:"finished_at,omitempty"`
}

type inspectEntry struct {
	Name         string
	RestartCount int
	Config       struct{ Labels map[string]string }
	State        struct {
		Status, Error, StartedAt, FinishedAt string
		ExitCode                             int
		OOMKilled                            bool
		Health                               *struct {
			Status string
			Log    []struct{ Output string }
		}
	}
}

// Collect builds a Snapshot for the Compose file (empty file = Compose's own default lookup),
// keeping the last tail log lines per service.
func Collect(ctx context.Context, run Runner, file string, tail int) (*Snapshot, error) {
	base := []string{"compose"}
	if file != "" {
		base = append(base, "-f", file)
	}
	compose := func(args ...string) ([]byte, error) {
		return run(ctx, append(slices.Clone(base), args...)...)
	}

	out, err := compose("config", "--format", "json")
	if err != nil {
		return nil, err
	}
	var cfg struct {
		Name     string             `json:"name"`
		Services map[string]Service `json:"services"`
	}
	if err := json.Unmarshal(out, &cfg); err != nil {
		return nil, fmt.Errorf("parse compose config: %w", err)
	}

	snap := &Snapshot{Project: cfg.Name}
	for name, svc := range cfg.Services {
		svc.Name = name
		snap.Services = append(snap.Services, svc)
	}
	slices.SortFunc(snap.Services, func(a, b Service) int { return strings.Compare(a.Name, b.Name) })
	index := make(map[string]int, len(snap.Services))
	for i, s := range snap.Services {
		index[s.Name] = i
	}

	out, err = compose("ps", "-a", "-q")
	if err != nil {
		return nil, err
	}
	if ids := strings.Fields(string(out)); len(ids) > 0 {
		out, err = run(ctx, append([]string{"inspect"}, ids...)...)
		if err != nil {
			return nil, err
		}
		var entries []inspectEntry
		if err := json.Unmarshal(out, &entries); err != nil {
			return nil, fmt.Errorf("parse docker inspect: %w", err)
		}
		for _, e := range entries {
			i, ok := index[e.Config.Labels["com.docker.compose.service"]]
			if !ok {
				continue // orphan container from a service no longer in the file
			}
			c := Container{
				Name:         strings.TrimPrefix(e.Name, "/"),
				Status:       e.State.Status,
				ExitCode:     e.State.ExitCode,
				RestartCount: e.RestartCount,
				OOMKilled:    e.State.OOMKilled,
				Error:        e.State.Error,
				StartedAt:    e.State.StartedAt,
				FinishedAt:   e.State.FinishedAt,
			}
			if h := e.State.Health; h != nil {
				c.Health = h.Status
				if n := len(h.Log); n > 0 {
					c.HealthOutput = strings.TrimSpace(h.Log[n-1].Output)
				}
			}
			snap.Services[i].Containers = append(snap.Services[i].Containers, c)
		}
	}

	for i := range snap.Services {
		s := &snap.Services[i]
		if len(s.Containers) == 0 {
			continue
		}
		slices.SortFunc(s.Containers, func(a, b Container) int { return strings.Compare(a.Name, b.Name) })
		// compose logs writes both container streams to stdout.
		out, err := compose("logs", "--no-color", "--no-log-prefix", "--tail", strconv.Itoa(tail), s.Name)
		if err != nil {
			return nil, err
		}
		if text := strings.TrimRight(string(out), "\n"); text != "" {
			s.Logs = strings.Split(text, "\n")
		}
	}
	return snap, nil
}
