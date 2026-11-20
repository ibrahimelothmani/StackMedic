package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestRun(t *testing.T) {
	tests := []struct {
		args     []string
		wantCode int
		wantOut  string
		wantErr  string
	}{
		{nil, 2, "", "Usage:"},
		{[]string{"version"}, 0, "stackmedic dev", ""},
		{[]string{"help"}, 0, "Usage:", ""},
		{[]string{"bogus"}, 2, "", `unknown command "bogus"`},
	}
	for _, tt := range tests {
		var out, errOut bytes.Buffer
		code := run(tt.args, &out, &errOut)
		if code != tt.wantCode {
			t.Errorf("run(%q) code = %d, want %d", tt.args, code, tt.wantCode)
		}
		if !strings.Contains(out.String(), tt.wantOut) {
			t.Errorf("run(%q) stdout = %q, want substring %q", tt.args, out.String(), tt.wantOut)
		}
		if !strings.Contains(errOut.String(), tt.wantErr) {
			t.Errorf("run(%q) stderr = %q, want substring %q", tt.args, errOut.String(), tt.wantErr)
		}
	}
}
