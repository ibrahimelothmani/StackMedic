package rules

import (
	"fmt"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

var refused = regexp.MustCompile(`(?i)connection refused|econnrefused|errno 111|could not connect to server`)

// defaultPorts maps well-known server images to the port they listen on by default.
var defaultPorts = map[string]string{
	"postgres": "5432", "postgresql": "5432", "mysql": "3306", "mariadb": "3306",
	"redis": "6379", "mongo": "27017", "mongodb": "27017", "rabbitmq": "5672",
	"memcached": "11211", "elasticsearch": "9200",
}

// SM-COMPOSE-007: a service logs "connection refused" for a host that is another Compose service.
func composeConnRefused(s *collect.Snapshot) []Finding {
	byName := map[string]*collect.Service{}
	for i := range s.Services {
		byName[strings.ToLower(s.Services[i].Name)] = &s.Services[i]
		if cn := s.Services[i].ContainerName; cn != "" {
			byName[strings.ToLower(cn)] = &s.Services[i]
		}
	}

	var findings []Finding
	for _, svc := range s.Services {
		line := ""
		for _, l := range svc.Logs {
			if refused.MatchString(l) {
				line = strings.TrimSpace(l)
				break
			}
		}
		if line == "" {
			continue
		}

		// Candidate targets from host-like env vars; prefer one named in the log line.
		type target struct {
			key, value, host string
			svc              *collect.Service
		}
		var targets []target
		for _, key := range slices.Sorted(maps.Keys(svc.Environment)) {
			v := svc.Environment[key]
			if v == nil || !hostKey.MatchString(key) {
				continue
			}
			host := hostOf(*v)
			if t := byName[strings.ToLower(host)]; t != nil && t.Name != svc.Name {
				targets = append(targets, target{key, *v, host, t})
			}
		}
		var pick *target
		for i := range targets {
			if strings.Contains(strings.ToLower(line), strings.ToLower(targets[i].host)) {
				pick = &targets[i]
				break
			}
		}
		if pick == nil && len(targets) == 1 {
			pick = &targets[0]
		}
		if pick == nil {
			continue // no target, or ambiguous between several
		}
		findings = append(findings, connFinding(svc, pick.key, pick.value, pick.svc, line))
	}
	return findings
}

func connFinding(svc collect.Service, key, value string, t *collect.Service, line string) Finding {
	f := Finding{
		RuleID:   "SM-COMPOSE-007",
		Severity: "high",
		Resource: "compose://" + svc.Name,
		Evidence: []Evidence{
			{"container_log", svc.Name + " log: " + line},
			{"compose_config", fmt.Sprintf("%s environment: %s=%s", svc.Name, key, value)},
		},
		Verification: fmt.Sprintf("Recreate the service (docker compose up -d --force-recreate %s), then check its logs for a successful connection.", svc.Name),
	}

	// Port: from host:port in the value, else the sibling *_PORT variable (DATABASE_HOST -> DATABASE_PORT).
	_, port := hostPort(value)
	portKey := key
	if port == "" {
		if loc := hostKey.FindStringIndex(key); loc != nil {
			pk := key[:loc[0]] + "PORT"
			if v := svc.Environment[pk]; v != nil && *v != "" {
				port, portKey = *v, pk
				f.Evidence = append(f.Evidence, Evidence{"compose_config", fmt.Sprintf("%s environment: %s=%s", svc.Name, pk, port)})
			}
		}
	}

	running, state := false, "no containers"
	for _, c := range t.Containers {
		state = fmt.Sprintf("%s: status=%s", c.Name, c.Status)
		if c.Health != "" {
			state += " health=" + c.Health
		}
		if c.Status == "running" {
			running = true
			break
		}
	}
	f.Evidence = append(f.Evidence, Evidence{"container_state", t.Name + " " + state})

	def := defaultPorts[imageName(t.Image)]
	exposed := false
	for _, p := range t.Ports {
		exposed = exposed || strconv.Itoa(int(p.Target)) == port
	}

	switch {
	case !running:
		f.Summary = fmt.Sprintf("%s cannot connect: %s is not running", svc.Name, t.Name)
		f.Explanation = fmt.Sprintf("%s's connection to %s was refused because %s has no running container.", svc.Name, t.Name, t.Name)
		f.Recommendation = fmt.Sprintf("Start %s and find out why it stopped (docker compose logs %s).", t.Name, t.Name)
	case port != "" && def != "" && port != def && !exposed:
		f.Summary = fmt.Sprintf("%s connects to %s on port %s, but %s listens on %s", svc.Name, t.Name, port, imageName(t.Image), def)
		f.Explanation = fmt.Sprintf("%s is running, so the refusal points at the port: the %s image listens on %s by default.", t.Name, imageName(t.Image), def)
		f.Evidence = append(f.Evidence, Evidence{"image_default", fmt.Sprintf("%s image default port: %s", imageName(t.Image), def)})
		if portKey == key {
			f.Recommendation = fmt.Sprintf("Change the port in %s to %s.", key, def)
		} else {
			f.Recommendation = fmt.Sprintf("Change %s to %q.", portKey, def)
		}
	case svc.DependsOn[t.Name].Condition != "service_healthy":
		f.Severity = "medium"
		f.Summary = fmt.Sprintf("%s connected to %s before it was ready", svc.Name, t.Name)
		f.Explanation = fmt.Sprintf("%s is running now, and %s does not wait for it to be healthy, so it likely tried to connect during %s's startup.", t.Name, svc.Name, t.Name)
		f.Recommendation = fmt.Sprintf("Give %s a healthcheck and add depends_on: {%s: {condition: service_healthy}} to %s, or retry connections on startup.", t.Name, t.Name, svc.Name)
	default:
		f.Severity = "medium"
		f.Summary = fmt.Sprintf("%s's connection to %s was refused", svc.Name, t.Name)
		f.Explanation = fmt.Sprintf("%s is running and healthy, so it may listen on a different port or only on 127.0.0.1 inside its container.", t.Name)
		f.Recommendation = fmt.Sprintf("Check which address and port %s listens on (docker compose exec %s netstat -ltn) and align %s.", t.Name, t.Name, key)
	}
	return f
}

// imageName reduces "registry/org/postgres:17-alpine" to "postgres".
func imageName(image string) string {
	image = image[strings.LastIndex(image, "/")+1:]
	if i := strings.IndexAny(image, ":@"); i >= 0 {
		image = image[:i]
	}
	return image
}
