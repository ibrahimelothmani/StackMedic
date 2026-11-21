package redact

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/ibrahimelothmani/StackMedic/internal/collect"
)

func TestScrub(t *testing.T) {
	tests := []struct{ in, want string }{
		{"postgres://demo:s3cr3t@db:5432/app", "postgres://demo:[REDACTED]@db:5432/app"},
		{"redis://:s3cr3t@cache:6379", "redis://:[REDACTED]@cache:6379"},
		{"postgres://demo:p@ss@db/app", "postgres://demo:[REDACTED]@db/app"},
		{"postgres://demo:ab/cd@db/app", "postgres://demo:[REDACTED]@db/app"},
		{"https://ghp_abc123@github.com/org/repo", "https://[REDACTED]@github.com/org/repo"},
		{"connecting with password=hunter22 ok", "connecting with password=[REDACTED] ok"},
		{`{"api_key": "abc123"}`, `{"api_key": [REDACTED]}`},
		{"Authorization: Bearer eyJhbGciOi.x.y", "Authorization: Bearer [REDACTED]"},
		{"echo literal-secret-value here", "echo [REDACTED] here"},
		// Evidence must survive.
		{"[api] ERROR: unable to resolve database hostname 'database'", "[api] ERROR: unable to resolve database hostname 'database'"},
		{"FATAL: password authentication failed for user \"demo\"", "FATAL: password authentication failed for user \"demo\""},
	}
	for _, tt := range tests {
		if got := scrub(tt.in, []string{"literal-secret-value"}); got != tt.want {
			t.Errorf("scrub(%q)\n got %q\nwant %q", tt.in, got, tt.want)
		}
	}
}

func ptr(s string) *string { return &s }

func TestSnapshotNeverLeaksSecrets(t *testing.T) {
	const pw = "local-demo-only-password"
	snap := &collect.Snapshot{Services: []collect.Service{
		{
			Name: "api",
			Environment: map[string]*string{
				"DATABASE_HOST": ptr("database"),
				"DATABASE_URL":  ptr("postgres://demo:" + pw + "@database/demo"),
				"API_TOKEN":     ptr(""), // empty: kept as evidence of missing config
				"JWT_SECRET":    nil,     // declared without value: kept
			},
			Logs: []string{
				"[api] ERROR: unable to resolve database hostname 'database'",
				"debug: using " + pw,
			},
			Containers: []collect.Container{{Error: "auth failed: " + pw}},
		},
		{
			Name:        "postgres",
			Environment: map[string]*string{"POSTGRES_PASSWORD": ptr(pw), "POSTGRES_USER": ptr("demo")},
			Healthcheck: &collect.Healthcheck{Test: []string{"CMD-SHELL", "psql -c 'select 1' postgres://demo:" + pw + "@localhost"}},
			Containers:  []collect.Container{{HealthOutput: "connected as demo using " + pw}},
		},
	}}

	Snapshot(snap)
	out, _ := json.Marshal(snap)
	if strings.Contains(string(out), pw) {
		t.Fatalf("secret leaked: %s", out)
	}

	api, pg := snap.Services[0], snap.Services[1]
	if *api.Environment["DATABASE_HOST"] != "database" || *pg.Environment["POSTGRES_USER"] != "demo" {
		t.Errorf("non-secret env altered: %s", out)
	}
	if *api.Environment["API_TOKEN"] != "" || api.Environment["JWT_SECRET"] != nil {
		t.Errorf("empty/unset secrets must be kept as-is: %s", out)
	}
	if *pg.Environment["POSTGRES_PASSWORD"] != Mask {
		t.Errorf("POSTGRES_PASSWORD = %q", *pg.Environment["POSTGRES_PASSWORD"])
	}
	if api.Logs[0] != "[api] ERROR: unable to resolve database hostname 'database'" {
		t.Errorf("evidence log altered: %q", api.Logs[0])
	}
}

func TestSnapshotURLPasswordEchoedAndEncoded(t *testing.T) {
	const pw = "p@ss w0rd!" // lives only inside a URL under an innocent key name
	snap := &collect.Snapshot{Services: []collect.Service{{
		Name: "api",
		Environment: map[string]*string{
			"DATABASE_URL": ptr("postgres://demo:p%40ss%20w0rd%21@db/app"),
			"CACHE_URL":    ptr("redis://:sl/ash99@cache"),                      // unencoded '/': url.Parse would fail
			"MQ_URL":       ptr("amqp://u:mqpass1@mq/v?admin=ops@corp.example"), // later '@' fools the greedy regex
			"NODES":        ptr("redis://:node1pw@h1,redis://:node2pw@h2"),
			"COMMA_URL":    ptr("postgres://u:com,ma99@db"),
		},
		Logs: []string{
			"retrying with " + pw, "query=p%40ss+w0rd%21", "cache auth sl/ash99 rejected",
			"mq login mqpass1 failed", "nodes node1pw node2pw", "pw com,ma99",
		},
	}}}
	Snapshot(snap)
	out, _ := json.Marshal(snap)
	for _, leak := range []string{pw, "p%40ss", "w0rd", "sl/ash99", "mqpass1", "node1pw", "node2pw", "com,ma99"} {
		if strings.Contains(string(out), leak) {
			t.Errorf("leaked %q: %s", leak, out)
		}
	}
}
