package launcher

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestAutoShebangAndExplicitOverride(t *testing.T) {
	f := newFixture(t)
	f.write("nonexec.sh", "#!/bin/sh -e\nfalse\nprintf 'must not run'\n", 0644)
	f.write("override.sh", "#!/missing/interpreter\nprintf 'override\\n'\n", 0755)
	f.write("env-script", "#!/usr/bin/env sh\nprintf 'env\\n'\n", 0644)
	f.write("cwd.sh", "#!/bin/sh\ncat ./input.txt\n", 0644)
	f.commit()
	if err := os.WriteFile(filepath.Join(f.caller, "input.txt"), []byte("caller default"), 0600); err != nil {
		t.Fatal(err)
	}
	if code, out, log := f.run("repo", "example/nonexec.sh"); code != 1 || out != "" {
		t.Fatalf("non-executable shebang flags: %d %q %s", code, out, log)
	}
	if code, out, log := f.run("repo", "--runner", "sh", "example/override.sh"); code != 0 || out != "override\n" {
		t.Fatalf("explicit override: %d %q %s", code, out, log)
	}
	if code, out, log := f.run("repo", "example/env-script"); code != 0 || out != "env\n" {
		t.Fatalf("env shebang: %d %q %s", code, out, log)
	}
	if code, out, log := f.run("repo", "example/cwd.sh"); code != 0 || out != "caller default" {
		t.Fatalf("default caller cwd: %d %q %s", code, out, log)
	}
	f.assertClean()
}

func TestPythonShebangDetection(t *testing.T) {
	for _, tc := range []struct {
		script, interpreter, optional string
		want                          bool
	}{
		{"tool.py", "", "", true},
		{"tool", "/usr/bin/python3", "-u", true},
		{"tool", "/usr/bin/env", "-S python3 -u", true},
		{"tool.py", "/bin/bash", "", false},
		{"tool", "/usr/bin/env", "-S bash python3", false},
	} {
		if got := pythonScript(tc.script, tc.interpreter, tc.optional); got != tc.want {
			t.Errorf("Python detection %+v: got %v", tc, got)
		}
	}
}

// Fake only the external Pipenv process, not the launcher's environment logic.
// No package index is contacted by these tests. The real Pipenv smoke test is
// opt-in below and uses an empty lockfile, with no dependencies to download.
func fakePipenv(t *testing.T, f *fixture) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	python, err = filepath.Abs(python)
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(f.caller, "fake-bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(f.caller, "pipenv.log")
	t.Setenv("RELOPS_TEST_PIPENV_LOG", log)
	t.Setenv("RELOPS_TEST_PYTHON", python)
	script := `#!/bin/sh
set -eu
[ "$PIPENV_VENV_IN_PROJECT" = 0 ]
[ "$PIPENV_IGNORE_VIRTUALENVS" = 1 ]
[ "$PIPENV_DONT_LOAD_ENV" = 1 ]
printf '%s|%s\n' "$1" "$PIPENV_PIPFILE" >> "$RELOPS_TEST_PIPENV_LOG"
case "$1" in
  verify) [ "${RELOPS_TEST_PIPENV_FAIL:-}" != verify ] ;;
  sync)
    [ "${RELOPS_TEST_PIPENV_FAIL:-}" != sync ]
    mkdir -p "$WORKON_HOME/$PIPENV_CUSTOM_VENV_NAME/bin"
    ln -sf "$RELOPS_TEST_PYTHON" "$WORKON_HOME/$PIPENV_CUSTOM_VENV_NAME/bin/python"
    ln -sf "$RELOPS_TEST_PYTHON" "$WORKON_HOME/$PIPENV_CUSTOM_VENV_NAME/bin/python3"
    printf 'dependency setup output\n'
    ;;
  *) exit 8 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "pipenv"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func pipenvFiles(f *fixture) {
	f.write("Pipfile", "[packages]\n", 0644)
	f.write("Pipfile.lock", `{"_meta": {}, "default": {}, "develop": {}}`, 0644)
	f.write("nested/Pipfile", "[packages]\n", 0644)
	f.write("nested/Pipfile.lock", `{"_meta": {}, "default": {}, "develop": {}}`, 0644)
	f.write("nested/tool.py", "import os, sys\nprint(sys.argv[1])\nprint(os.environ['VIRTUAL_ENV'])\n", 0644)
	f.commit()
}

func TestAutoPipenvSetupCacheOfflineAndOverride(t *testing.T) {
	f := newFixture(t)
	pipenvFiles(f)
	logFile := fakePipenv(t, f)
	if code, _, log := f.run("repo", "--offline", "example/nested/tool.py", "hello"); code != 1 {
		t.Fatalf("uncached offline run: %d %s", code, log)
	}
	for _, args := range [][]string{
		{"example/nested/tool.py", "hello"},
		{"--offline", "example/nested/tool.py", "hello"},
		{"--verbose", "example/nested/tool.py", "hello"},
	} {
		code, out, log := f.run(append([]string{"repo"}, args...)...)
		if code != 0 || !strings.HasPrefix(out, "hello\n"+filepath.Join(f.cache, "envs", defaultOwner, "example")) || strings.Contains(out, "dependency setup") {
			t.Fatalf("automatic Pipenv: %d %q %s", code, out, log)
		}
	}
	data, err := os.ReadFile(logFile)
	if err != nil || strings.Count(string(data), "verify|") != 1 || strings.Count(string(data), "sync|") != 1 || strings.Contains(string(data), "|"+filepath.Join(f.source, "Pipfile")) || !strings.Contains(string(data), "/nested/Pipfile") {
		t.Fatalf("nearest project setup must run once: %q %v", data, err)
	}
	// An explicit runner skips Pipenv entirely, even when a Pipfile is present.
	f.write("nested/plain.py", "print('explicit')\n", 0644)
	f.commit()
	if code, out, log := f.run("repo", "--runner", "python3", "example/nested/plain.py"); code != 0 || out != "explicit\n" {
		t.Fatalf("explicit bypass: %d %q %s", code, out, log)
	}
	after, _ := os.ReadFile(logFile)
	if !bytes.Equal(after, data) {
		t.Fatal("explicit runner invoked Pipenv")
	}
	// A new commit has a separate environment, including for offline lookup.
	if code, _, log := f.run("repo", "example/tool.sh"); code != 0 {
		t.Fatal(log)
	}
	if code, _, log := f.run("repo", "--offline", "example/nested/tool.py", "hello"); code != 1 || !strings.Contains(log, "environment is not cached") {
		t.Fatalf("new commit reused an old environment: %d %s", code, log)
	}
	f.assertClean()
}

func TestAutoPythonWithoutPipfile(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	f := newFixture(t)
	f.write("tool.py", "print('detected')\n", 0644)
	f.commit()
	if code, out, log := f.run("repo", "example/tool.py"); code != 0 || out != "detected\n" {
		t.Fatalf("Python extension detection: %d %q %s", code, out, log)
	}
	f.assertClean()
}

func TestPipenvSetupFailureDoesNotExecuteTool(t *testing.T) {
	f := newFixture(t)
	pipenvFiles(f)
	fakePipenv(t, f)
	t.Setenv("RELOPS_TEST_PIPENV_FAIL", "sync")
	code, out, log := f.run("repo", "example/nested/tool.py", "must not run")
	if code != 1 || out != "" || !strings.Contains(log, "pipenv sync failed") {
		t.Fatalf("failed setup: %d %q %s", code, out, log)
	}
	t.Setenv("RELOPS_TEST_PIPENV_FAIL", "")
	code, out, log = f.run("repo", "example/nested/tool.py", "recovered")
	if code != 0 || !strings.HasPrefix(out, "recovered\n") {
		t.Fatalf("retry after setup failure: %d %q %s", code, out, log)
	}
	f.assertClean()
}

func TestMissingPipenvAndInvalidLock(t *testing.T) {
	f := newFixture(t)
	pipenvFiles(f)
	// A PATH containing Git but no Pipenv ensures a deterministic missing-tool
	// check without depending on whether the developer has Pipenv installed.
	git, err := exec.LookPath("git")
	if err != nil {
		t.Fatal(err)
	}
	bin := filepath.Join(f.caller, "git-only")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(git, filepath.Join(bin, "git")); err != nil {
		t.Fatal(err)
	}
	oldPath := os.Getenv("PATH")
	t.Setenv("PATH", bin)
	code, out, log := f.run("repo", "example/nested/tool.py", "unused")
	if code != 1 || out != "" || !strings.Contains(log, "pipenv is not on PATH") {
		t.Fatalf("missing Pipenv: %d %q %s", code, out, log)
	}
	t.Setenv("PATH", oldPath)
	fakePipenv(t, f)
	f.write("nested/Pipfile.lock", `{"default":{"local":{"editable":true}}}`, 0644)
	f.commit()
	code, _, log = f.run("repo", "example/nested/tool.py", "unused")
	if code != 1 || !strings.Contains(log, "editable runtime dependency") {
		t.Fatalf("editable dependency: %d %s", code, log)
	}
	f.assertClean()
}

// Exercise installed Pipenv with an empty, valid project. Opt-in because creating
// the virtualenv can write to Pipenv/virtualenv caches outside the test directory.
func TestRealPipenv(t *testing.T) {
	if os.Getenv("RELOPS_TEST_REAL_PIPENV") != "1" {
		t.Skip("opt-in installed Pipenv smoke test")
	}
	if _, err := exec.LookPath("pipenv"); err != nil {
		t.Skip("pipenv is not installed")
	}
	f := newFixture(t)
	f.write("Pipfile", "[packages]\n[dev-packages]\n", 0644)
	cmd := exec.Command("pipenv", "lock")
	cmd.Dir = f.source
	cmd.Env = replaceEnv(os.Environ(), "PIPENV_PIPFILE", filepath.Join(f.source, "Pipfile"))
	cmd.Env = replaceEnv(cmd.Env, "WORKON_HOME", filepath.Join(f.caller, "bootstrap"))
	cmd.Env = replaceEnv(cmd.Env, "PIPENV_IGNORE_VIRTUALENVS", "1")
	if err := os.MkdirAll(filepath.Join(f.caller, "bootstrap"), 0700); err != nil {
		t.Fatal(err)
	}
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("prepare empty lockfile: %v\n%s", err, output)
	}
	program := "import os, sys\nprint('real pipenv')\nassert os.path.realpath(sys.prefix) == os.path.realpath(os.environ['VIRTUAL_ENV'])\n"
	f.write("tool.py", program, 0644)
	f.write("tool-env.py", "#!/usr/bin/env python3\n"+program, 0755)
	f.write("tool-env-nonexec.py", "#!/usr/bin/env python3\n"+program, 0644)
	f.commit()
	for _, args := range [][]string{{"example/tool.py"}, {"--offline", "example/tool-env.py"}, {"--offline", "example/tool-env-nonexec.py"}} {
		code, out, log := f.run(append([]string{"repo"}, args...)...)
		if code != 0 || out != "real pipenv\n" {
			t.Fatalf("real Pipenv: %d %q %s", code, out, log)
		}
	}
	f.assertClean()
}

func Example_githubRemote() {
	fmt.Println(githubRemote("aerickson", "android-tools"))
	// Output: git@github.com:aerickson/android-tools.git
}
