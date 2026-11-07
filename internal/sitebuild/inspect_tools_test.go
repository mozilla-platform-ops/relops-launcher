package sitebuild

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

func TestInspectToolsLocalSource(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	repo := t.TempDir()
	git := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		if data, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git: %v: %s", err, data)
		}
	}
	git("init", "-b", "main")
	source := `from pathlib import Path
open(Path.home() / ".secret")
open(Path.home() / ".documented")
Path(__file__).with_name("result").write_text("x")
open("results.txt", "w")
raise RuntimeError("must never execute")
`
	for name, content := range map[string]string{"tool.py": source, "tool.sh": "#!/bin/sh\necho data > output.txt\n", "runner": "#!/usr/bin/env bash\necho data > output.txt\n", "pyproject.toml": "", "uv.lock": ""} {
		if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0600); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	c := catalog.Catalog{SchemaVersion: 1, Revision: "test", Tools: []catalog.Tool{
		{ID: "example", Name: "Example", Description: "Example", Default: &catalog.Target{Kind: "source", Owner: "owner", Repo: "repo", DataFiles: []string{"~/.documented"}}},
		{ID: "shell", Name: "Shell", Description: "Shell", Default: &catalog.Target{Kind: "source", Owner: "owner", Repo: "repo", Path: "runner", Cwd: "repo"}},
		{ID: "web", Name: "Web", Description: "Web", Default: &catalog.Target{Kind: "web", URL: "https://example.com"}},
	}}
	data, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	catalogPath := filepath.Join(t.TempDir(), "tools.json")
	if err := os.WriteFile(catalogPath, data, 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := inspectTools(context.Background(), catalogPath, &out, func(tool) string { return repo }); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{"INSPECT example-default", "commit=", "tool.py:2 [undocumented-data-file]", "tool.py:4 [checkout-write]", "tool.py:5 [relative-write]", "tool.sh:2 [relative-write]", "INSPECT shell-default", "runner:2 [relative-write]", "SKIP web-default"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	if strings.Contains(got, "~/.documented") {
		t.Errorf("documented file reported: %s", got)
	}
}

func TestPythonInspectorWorkingDirectoryAndParseErrors(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	inputs := []pythonInput{
		{Path: "caller.py", Source: `open("output.txt", "w")`, TemporaryCwd: false},
		{Path: "temporary.py", Source: `open("output.txt", "w")`, TemporaryCwd: true},
		{Path: "broken.py", Source: "def broken(:\n"},
	}
	payload, err := json.Marshal(inputs)
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("python3", "-I", "-c", pythonInspector)
	cmd.Stdin = bytes.NewReader(payload)
	data, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	var results [][]finding
	if err := json.Unmarshal(data, &results); err != nil {
		t.Fatal(err)
	}
	if len(results) != 3 || len(results[0]) != 0 || len(results[1]) != 1 || results[1][0].Rule != "relative-write" || len(results[2]) != 1 || results[2][0].Rule != "python-parse" {
		t.Fatalf("unexpected findings: %+v", results)
	}
}

func TestShellInspectorCheckoutWrites(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not available")
	}
	source := `#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(git rev-parse --show-toplevel)"
echo data > "$SCRIPT_DIR/output.txt"
cp input "$ROOT/copied.txt"
echo data > relative.txt
printf '%s\n' 'not > a redirect'
# echo data > ignored.txt
echo data > /tmp/safe.txt
`
	inputs := []pythonInput{{Path: "tool.sh", Source: source, TemporaryCwd: true}, {Path: "caller.sh", Source: source}}
	results, err := runInspector(context.Background(), "Shell", shellInspector, inputs)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.Join(results, "\n")
	for _, want := range []string{"tool.sh:4 [checkout-write]", "tool.sh:5 [checkout-write]", "tool.sh:6 [relative-write]", "caller.sh:4 [checkout-write]", "caller.sh:5 [checkout-write]"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
	for _, unwanted := range []string{"caller.sh:6", ":7 [", ":8 [", ":9 ["} {
		if strings.Contains(got, unwanted) {
			t.Errorf("unexpected %q in %s", unwanted, got)
		}
	}
}
