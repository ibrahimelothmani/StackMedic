package rules

import (
	"fmt"
	"maps"
	"net"
	"regexp"
	"slices"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

var (
	hostKey  = regexp.MustCompile(`(?i)(host|hostname|server|addr|address|url|uri|dsn|endpoint)$`)
	dnsLabel = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9_-]*[a-zA-Z0-9])?$`) // one label: Compose names have no dots
	dnsError = regexp.MustCompile(`(?i)no such host|could not resolve|unable to resolve|name or service not known|name resolution|getaddrinfo|nodename nor servname|unknown host|eai_again|enotfound`)
)

// SM-COMPOSE-001: an env var points at a hostname that no Compose service, alias,
// container_name or hostname provides.
func composeDNSMismatch(s *collect.Snapshot) []Finding {
	known := map[string]bool{}
	var services []string
	for _, svc := range s.Services {
		services = append(services, svc.Name)
		names := []string{svc.Name, svc.ContainerName, svc.Hostname}
		for _, n := range svc.Networks {
			if n != nil {
				names = append(names, n.Aliases...)
			}
		}
		for _, l := range svc.Links {
			_, alias, _ := strings.Cut(l, ":")
			names = append(names, alias)
		}
		for _, c := range svc.Containers {
			names = append(names, c.Name)
		}
		for _, n := range names {
			if n != "" {
				known[strings.ToLower(n)] = true
			}
		}
	}

	var findings []Finding
	for _, svc := range s.Services {
		for _, key := range slices.Sorted(maps.Keys(svc.Environment)) {
			v := svc.Environment[key]
			if v == nil || !hostKey.MatchString(key) {
				continue
			}
			host := hostOf(*v)
			if !dnsLabel.MatchString(host) || strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil || known[strings.ToLower(host)] {
				continue
			}
			findings = append(findings, dnsFinding(svc, key, *v, host, services))
		}
	}
	return findings
}

func dnsFinding(svc collect.Service, key, value, host string, services []string) Finding {
	f := Finding{
		RuleID:   "SM-COMPOSE-001",
		Resource: "compose://" + svc.Name,
		Explanation: fmt.Sprintf("%s points to %q, but no Compose service, network alias, container_name or hostname has that name.",
			key, host),
	}

	var logLine string
	for _, l := range svc.Logs {
		if dnsError.MatchString(l) && strings.Contains(strings.ToLower(l), strings.ToLower(host)) {
			logLine = strings.TrimSpace(l)
			break
		}
	}
	if logLine != "" {
		f.Severity = "high"
		f.Summary = fmt.Sprintf("%s cannot resolve host %q", svc.Name, host)
		f.Evidence = append(f.Evidence, Evidence{"container_log", svc.Name + " log: " + logLine})
	} else {
		f.Severity = "medium"
		f.Summary = fmt.Sprintf("%s references unknown host %q", svc.Name, host)
		f.Explanation += " No resolution error was found in recent logs, so this may be an intentional external host."
	}
	f.Evidence = append(f.Evidence,
		Evidence{"compose_config", fmt.Sprintf("%s environment: %s=%s", svc.Name, key, value)},
		Evidence{"compose_config", "Compose services: " + strings.Join(services, ", ")},
	)

	// The single depends_on target is the best-supported replacement; otherwise list the options.
	target := ""
	if len(svc.DependsOn) == 1 {
		for name := range svc.DependsOn {
			target = name
		}
		f.Evidence = append(f.Evidence, Evidence{"compose_config", fmt.Sprintf("%s depends_on: %s", svc.Name, target)})
	}
	switch {
	case target != "" && value == host:
		f.Recommendation = fmt.Sprintf("Change %s to %q.", key, target)
	case target != "":
		f.Recommendation = fmt.Sprintf("Replace host %q with %q in %s.", host, target, key)
	default:
		others := slices.DeleteFunc(slices.Clone(services), func(n string) bool { return n == svc.Name })
		f.Recommendation = fmt.Sprintf("Point %s at one of the Compose services (%s), or add %q as a network alias.",
			key, strings.Join(others, ", "), host)
	}
	f.Verification = fmt.Sprintf("Recreate the service (docker compose up -d --force-recreate %s), then check its logs for a successful connection.", svc.Name)
	return f
}

// hostOf extracts the host from "host", "host:port" or a URL. Expects a redacted value,
// so userinfo is already "[REDACTED]" and contains no '/'.
func hostOf(v string) string {
	if i := strings.Index(v, "://"); i >= 0 {
		v = v[i+3:]
	}
	v = v[:strings.IndexAny(v+"/", "/?#")]
	if i := strings.LastIndex(v, "@"); i >= 0 {
		v = v[i+1:]
	}
	if h, _, err := net.SplitHostPort(v); err == nil {
		v = h
	}
	return v
}
