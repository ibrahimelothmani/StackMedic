// Package redact masks secrets in a collected snapshot before anything is printed or evaluated.
package redact

import (
	"encoding/json"
	"net/url"
	"regexp"
	"slices"
	"strings"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

// Mask replaces every secret value.
const Mask = "[REDACTED]"

const secretWords = `pass|pwd|secret|token|key|credential|private|auth|dsn|salt|cookie|session|cert|signature`

var (
	secretKey = regexp.MustCompile(`(?i)` + secretWords)
	// scheme://user:password@host, empty user allowed; greedy to the last '@' before whitespace so
	// passwords may contain '@' or '/' (over-masks a later '@' in the same token, which is the safe side).
	urlCreds = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://[^:/@\s]*):(\S*)@`)
	// scheme://token@host: userinfo without a colon is often a bare token.
	urlToken = regexp.MustCompile(`([a-zA-Z][a-zA-Z0-9+.-]*://)[^\s/:@]+@`)
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
	addLiteral := func(l string) {
		if len(l) < minLiteral {
			return
		}
		jsonForm, _ := json.Marshal(l)
		// Also catch the secret when it is echoed percent-encoded or JSON-escaped.
		for _, form := range []string{l, url.QueryEscape(l), strings.Trim(string(jsonForm), `"`)} {
			if !slices.Contains(literals, form) {
				literals = append(literals, form)
			}
		}
	}
	for i := range s.Services {
		env := s.Services[i].Environment
		for k, v := range env {
			if v == nil || *v == "" {
				continue
			}
			// Regex, not url.Parse: it rejects unencoded '/' or '@' in passwords, which then leak.
			for _, m := range urlCreds.FindAllStringSubmatch(*v, -1) {
				addLiteral(m[2])
				if dec, err := url.PathUnescape(m[2]); err == nil {
					addLiteral(dec)
				}
			}
			if !secretKey.MatchString(k) {
				continue
			}
			addLiteral(*v)
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
	t = urlToken.ReplaceAllString(t, "${1}"+Mask+"@")
	t = keyValue.ReplaceAllStringFunc(t, func(m string) string {
		p := keyValue.FindStringSubmatch(m)
		if v := strings.ToLower(p[3]); v == "bearer" || v == "basic" {
			return m // the token after the scheme is masked by authHeader
		}
		return p[1] + p[2] + Mask
	})
	return authHeader.ReplaceAllString(t, "${1} "+Mask)
}
