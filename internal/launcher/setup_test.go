package launcher

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestSetupDiagnostics(t *testing.T) {
	for _, tc := range []struct {
		name                   string
		verbose, capture, fail bool
	}{
		{name: "quiet success"},
		{name: "quiet failure", fail: true},
		{name: "verbose success", verbose: true},
		{name: "metadata success", capture: true},
		{name: "metadata failure", capture: true, fail: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var log bytes.Buffer
			a := &app{cache: t.TempDir(), errOut: &log}
			script := "printf 'setup stdout\\n'; printf 'setup stderr\\n' >&2"
			wantCode := 0
			if tc.fail {
				script += "; exit 7"
				wantCode = 7
			}
			output, code, err := a.runSetup(context.Background(), options{verbose: tc.verbose}, exec.Command("sh", "-c", script), tc.capture)
			if err != nil || code != wantCode {
				t.Fatalf("setup result: %d %v", code, err)
			}
			if tc.capture && string(output) != "setup stdout\n" {
				t.Fatalf("metadata capture: %q", output)
			}
			wantDiagnostics := tc.fail || tc.verbose
			if strings.Contains(log.String(), "setup stderr") != wantDiagnostics {
				t.Fatalf("stderr visibility: %q", log.String())
			}
			if strings.Contains(log.String(), "setup stdout") != (tc.fail || (tc.verbose && !tc.capture)) {
				t.Fatalf("stdout visibility: %q", log.String())
			}
			files, err := os.ReadDir(a.cache)
			if err != nil || len(files) != 0 {
				t.Fatalf("setup log cleanup: %v %v", files, err)
			}
		})
	}
}
