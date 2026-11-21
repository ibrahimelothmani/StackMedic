package rules

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

var (
	portInUse  = regexp.MustCompile(`(?i)port is already allocated|address already in use`)
	failedBind = regexp.MustCompile(`:(\d+) failed`)
)

// SM-COMPOSE-005: a published host port is claimed twice in the file, or already taken on the host.
func composePortConflict(s *collect.Snapshot) []Finding {
	type hostPort struct{ port, proto string }
	// shortcut: ignores host_ip, so "127.0.0.1:80" and "10.0.0.1:80" count as a clash; split by IP if that bites.
	owners := map[hostPort][]string{}
	var order []hostPort
	for _, svc := range s.Services {
		for _, p := range svc.Ports {
			if p.Published == "" {
				continue
			}
			k := hostPort{p.Published, p.Protocol}
			if k.proto == "" {
				k.proto = "tcp"
			}
			if len(owners[k]) == 0 {
				order = append(order, k)
			}
			owners[k] = append(owners[k], svc.Name)
		}
	}
	bindErr := map[string]string{} // service -> container start error
	for _, svc := range s.Services {
		for _, c := range svc.Containers {
			if portInUse.MatchString(c.Error) {
				bindErr[svc.Name] = c.Error
			}
		}
	}

	var findings []Finding
	reported := map[string]bool{}
	for _, k := range order {
		names := owners[k]
		if len(names) < 2 {
			continue
		}
		loser := names[len(names)-1]
		for _, n := range names {
			if bindErr[n] != "" {
				loser = n
			}
		}
		f := Finding{
			RuleID:   "SM-COMPOSE-005",
			Severity: "high",
			Resource: "compose://" + loser,
			Summary:  fmt.Sprintf("host port %s/%s is published by %s", k.port, k.proto, strings.Join(names, " and ")),
			Explanation: "Only one container can bind a host port. Whichever service starts second fails " +
				"with \"port is already allocated\".",
			Recommendation: fmt.Sprintf("Publish a different host port for %s, e.g. %q.", loser, nextPort(k.port)+":"+targetPort(s, loser, k.port)),
			Verification:   "Run docker compose up -d, then confirm docker compose ps shows every service running.",
		}
		for _, n := range names {
			f.Evidence = append(f.Evidence, Evidence{"compose_config", fmt.Sprintf("%s publishes %s/%s", n, k.port, k.proto)})
		}
		if e := bindErr[loser]; e != "" {
			f.Evidence = append(f.Evidence, Evidence{"container_state", loser + " start error: " + e})
		}
		findings = append(findings, f)
		reported[loser] = true
	}

	// Start failed on a port nothing else in the file claims: another process on the host holds it.
	for _, svc := range s.Services {
		e := bindErr[svc.Name]
		if e == "" || reported[svc.Name] {
			continue
		}
		port := "the port"
		if m := failedBind.FindStringSubmatch(e); m != nil {
			port = m[1]
		}
		findings = append(findings, Finding{
			RuleID:      "SM-COMPOSE-005",
			Severity:    "high",
			Resource:    "compose://" + svc.Name,
			Summary:     fmt.Sprintf("%s cannot bind host port %s", svc.Name, port),
			Explanation: "No other service in this project publishes that port, so another process or container on the host is holding it.",
			Evidence:    []Evidence{{"container_state", svc.Name + " start error: " + e}},
			Recommendation: fmt.Sprintf("Find what holds the port (ss -ltnp | grep ':%s' or lsof -i :%s) and stop it, or publish a different host port for %s.",
				port, port, svc.Name),
			Verification: "Run docker compose up -d, then confirm docker compose ps shows the service running.",
		})
	}
	return findings
}

func nextPort(p string) string {
	if n, err := strconv.Atoi(p); err == nil && n < 65535 {
		return strconv.Itoa(n + 1)
	}
	return "<free port>"
}

func targetPort(s *collect.Snapshot, service, published string) string {
	for _, svc := range s.Services {
		if svc.Name != service {
			continue
		}
		for _, p := range svc.Ports {
			if p.Published == published {
				return strconv.Itoa(int(p.Target))
			}
		}
	}
	return "<container port>"
}

// SM-COMPOSE-006: a bind mount source is missing, or is a directory where a file was expected.
func composeBindMount(s *collect.Snapshot) []Finding {
	var findings []Finding
	for _, svc := range s.Services {
		for _, v := range svc.Volumes {
			mount := fmt.Sprintf("%s mounts %s -> %s", svc.Name, v.Source, v.Target)
			f := Finding{
				RuleID:       "SM-COMPOSE-006",
				Severity:     "high",
				Resource:     "compose://" + svc.Name,
				Evidence:     []Evidence{{"compose_config", mount}, {"host_filesystem", v.Source + ": " + v.SourceState}},
				Verification: fmt.Sprintf("Recreate the service (docker compose up -d --force-recreate %s) and check its logs.", svc.Name),
			}
			ext := filepath.Ext(v.Target)
			switch {
			case v.SourceState == "missing":
				f.Summary = fmt.Sprintf("%s bind mount source %s does not exist", svc.Name, filepath.Base(v.Source))
				f.Explanation = "Docker cannot mount a host path that does not exist. Depending on the mount syntax it " +
					"refuses to start the container, or silently creates an empty directory in its place."
				f.Recommendation = fmt.Sprintf("Create %s on the host or fix the source path in the Compose file.", v.Source)
			case v.SourceState == "dir" && ext != "" && filepath.Ext(v.Source) == ext:
				f.Severity = "medium"
				f.Summary = fmt.Sprintf("%s bind mount source %s is a directory, not a file", svc.Name, filepath.Base(v.Source))
				f.Explanation = "The source and target look like files, but the source is a directory. Docker creates " +
					"an empty directory when a bind source is missing, so the intended file was probably absent at first start."
				f.Recommendation = fmt.Sprintf("Replace the directory %s with the intended file, then recreate the service.", v.Source)
			default:
				continue
			}
			for _, c := range svc.Containers {
				if c.Error != "" {
					f.Evidence = append(f.Evidence, Evidence{"container_state", svc.Name + " start error: " + c.Error})
				}
			}
			findings = append(findings, f)
		}
	}
	return findings
}
