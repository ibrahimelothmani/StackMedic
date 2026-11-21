// Package redact masks secrets in a collected snapshot before anything is printed or evaluated.
package redact

import (
	"regexp"
	"slices"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

// Mask replaces every secret value.
const Mask = "[REDACTED]"

const secretWords = `pass|secret|token|key|credential|private|auth`

var (
	secretKey = regexp.MustCompile(`(?i)` + secretWords)
	// scheme://user:password@host
	urlCreds = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:/@\s]+):[^@\s]+@`)
	// password=x, "api_key": "x", TOKEN: x
	keyValue = regexp.MustCompile(`(?i)\b([\w.-]*(?:` + secretWords + `)[\w.-]*)(["']?\s*[=:]\s*)("[^"]*"|'[^']*'|[^\s,;&]+)`)
	// Authorization: Bearer x / Basic x
	authHeader = regexp.MustCompile(`(?i)\b(bearer|basic)\s+[A-Za-z0-9._~+/-]+=*`)
)

// minLiteral avoids masking unrelated text with short secret values like "1" or "dev".
const minLiteral = 4

// Snapshot masks secrets in place: values of secret-named env vars, plus URL credentials,
// key=value secrets, auth headers and any echoed secret value in all free text.
// Empty and unset values are kept, since "missing secret" is itself evidence.
func Snapshot(s *collect.Snapshot) {
	var literals []string
	for i := range s.Services {
		env := s.Services[i].Environment
		for k, v := range env {
			if v == nil || *v == "" || !secretKey.MatchString(k) {
				continue
			}
			if len(*v) >= minLiteral {
				literals = append(literals, *v)
			}
			masked := Mask
			env[k] = &masked
		}
	}
	// Longest first so a secret containing another is masked whole.
	slices.SortFunc(literals, func(a, b string) int { return len(b) - len(a) })

	for i := range s.Services {
		svc := &s.Services[i]
		for k, v := range svc.Environment {
			if v != nil {
				r := scrub(*v, literals)
				svc.Environment[k] = &r
			}
		}
		if svc.Healthcheck != nil {
			for j := range svc.Healthcheck.Test {
				svc.Healthcheck.Test[j] = scrub(svc.Healthcheck.Test[j], literals)
			}
		}
		for j := range svc.Logs {
			svc.Logs[j] = scrub(svc.Logs[j], literals)
		}
		for j := range svc.Containers {
			c := &svc.Containers[j]
			c.Error = scrub(c.Error, literals)
			c.HealthOutput = scrub(c.HealthOutput, literals)
		}
	}
}

func scrub(t string, literals []string) string {
	for _, l := range literals {
		t = strings.ReplaceAll(t, l, Mask)
	}
	t = urlCreds.ReplaceAllString(t, "${1}:"+Mask+"@")
	t = keyValue.ReplaceAllStringFunc(t, func(m string) string {
		p := keyValue.FindStringSubmatch(m)
		if v := strings.ToLower(p[3]); v == "bearer" || v == "basic" {
			return m // the token after the scheme is masked by authHeader
		}
		return p[1] + p[2] + Mask
	})
	return authHeader.ReplaceAllString(t, "${1} "+Mask)
}
