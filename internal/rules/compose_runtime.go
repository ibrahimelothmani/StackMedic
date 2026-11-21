package rules

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

// SM-COMPOSE-002: a container exited non-zero or keeps restarting.
// A symptom: Run drops it when a more specific rule explains the same service.
func composeCrash(s *collect.Snapshot) []Finding {
	var findings []Finding
	for _, svc := range s.Services {
		for _, c := range svc.Containers {
			if c.OOMKilled {
				continue // SM-COMPOSE-004
			}
			// 130/143 = SIGINT/SIGTERM: stopped on purpose (docker stop, Ctrl+C), not a crash.
			crashed := c.Status == "exited" && c.ExitCode != 0 && c.ExitCode != 130 && c.ExitCode != 143
			looping := c.Status == "restarting" || c.RestartCount >= 3
			if !crashed && !looping {
				continue
			}
			f := Finding{
				RuleID:   "SM-COMPOSE-002",
				Severity: "medium",
				Resource: "compose://" + svc.Name,
				Summary:  fmt.Sprintf("%s exited with code %d", svc.Name, c.ExitCode),
				Explanation: "The container's main process exits with a non-zero code. " +
					"The last log lines usually name the cause.",
				Evidence: append([]Evidence{{"container_state",
					fmt.Sprintf("%s: status=%s exit_code=%d restart_count=%d", c.Name, c.Status, c.ExitCode, c.RestartCount)}},
					lastLogs(svc, 3)...),
				Recommendation: "Fix the error reported in the last log lines, then restart the service.",
				Verification:   fmt.Sprintf("Run docker compose up -d %s, then confirm docker compose ps shows it running and its restart count stays the same.", svc.Name),
			}
			if looping {
				f.Severity = "high"
				f.Summary = fmt.Sprintf("%s is crash-looping (exit %d, %d restarts)", svc.Name, c.ExitCode, c.RestartCount)
			}
			switch c.ExitCode {
			case 126:
				f.Recommendation = "The command exists but is not executable: check file permissions or the shebang of the entrypoint/command."
			case 127:
				f.Recommendation = "The command was not found in the image: check the service's command/entrypoint and that the binary is installed."
			}
			findings = append(findings, f)
			break // one finding per service
		}
	}
	return findings
}

// SM-COMPOSE-003: a container fails its healthcheck.
func composeUnhealthy(s *collect.Snapshot) []Finding {
	var findings []Finding
	for _, svc := range s.Services {
		for _, c := range svc.Containers {
			if c.Health != "unhealthy" {
				continue
			}
			f := Finding{
				RuleID:      "SM-COMPOSE-003",
				Severity:    "high",
				Resource:    "compose://" + svc.Name,
				Summary:     svc.Name + " fails its healthcheck",
				Explanation: "Docker marked the container unhealthy after its health probe failed repeatedly.",
				Evidence:    []Evidence{{"container_state", c.Name + ": health=unhealthy"}},
				Verification: fmt.Sprintf("Recreate the service (docker compose up -d --force-recreate %s) and wait for docker compose ps to show it as healthy.",
					svc.Name),
			}
			if c.HealthOutput != "" {
				f.Evidence = append(f.Evidence, Evidence{"container_state", "latest probe output: " + c.HealthOutput})
			}
			probe := ""
			if hc := svc.Healthcheck; hc != nil && len(hc.Test) > 1 {
				probe = strings.Join(hc.Test[1:], " ")
				f.Evidence = append(f.Evidence, Evidence{"compose_config", svc.Name + " healthcheck: " + probe})
				if hc.Test[0] == "CMD-SHELL" {
					probe = "sh -c " + strconv.Quote(probe)
				}
			}
			var waiting []string
			for _, other := range s.Services {
				if other.DependsOn[svc.Name].Condition == "service_healthy" {
					waiting = append(waiting, other.Name)
				}
			}
			if len(waiting) > 0 {
				f.Evidence = append(f.Evidence, Evidence{"compose_config",
					"waiting for it with condition service_healthy: " + strings.Join(waiting, ", ")})
				f.Explanation += fmt.Sprintf(" Services that wait for it with condition: service_healthy (%s) will not start.", strings.Join(waiting, ", "))
			}
			if probe != "" {
				f.Recommendation = fmt.Sprintf("Run the probe by hand to see why it fails: docker compose exec %s %s. Then fix the probe or the service it checks.", svc.Name, probe)
			} else {
				f.Recommendation = fmt.Sprintf("Inspect the probe history with docker inspect --format '{{json .State.Health}}' %s and fix the probe or the service it checks.", c.Name)
			}
			findings = append(findings, f)
			break
		}
	}
	return findings
}

// SM-COMPOSE-004: the kernel killed a container for exceeding its memory limit.
func composeOOM(s *collect.Snapshot) []Finding {
	var findings []Finding
	for _, svc := range s.Services {
		for _, c := range svc.Containers {
			if !c.OOMKilled {
				continue
			}
			f := Finding{
				RuleID:   "SM-COMPOSE-004",
				Severity: "high",
				Resource: "compose://" + svc.Name,
				Summary:  svc.Name + " was killed for running out of memory",
				Explanation: "Docker reports the container as OOM-killed: it exceeded its memory limit, " +
					"or the host ran out of memory.",
				Evidence: append([]Evidence{{"container_state",
					fmt.Sprintf("%s: oom_killed=true exit_code=%d", c.Name, c.ExitCode)}}, lastLogs(svc, 2)...),
				Recommendation: "Raise the memory limit (mem_limit or deploy.resources.limits.memory) " +
					"or reduce the service's memory use, for example by streaming data instead of loading it all at once.",
				Verification: fmt.Sprintf("Recreate the service (docker compose up -d --force-recreate %s) and confirm it is no longer OOM-killed (docker inspect --format '{{.State.OOMKilled}}' %s).",
					svc.Name, c.Name),
			}
			if svc.MemLimit != "" {
				limit := svc.MemLimit
				if b, err := strconv.ParseInt(svc.MemLimit, 10, 64); err == nil {
					limit = fmt.Sprintf("%d MiB", b/(1<<20))
				}
				f.Evidence = append(f.Evidence, Evidence{"compose_config", svc.Name + " mem_limit: " + limit})
			}
			findings = append(findings, f)
			break
		}
	}
	return findings
}
