package launcher

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func TestSelectUVCommand(t *testing.T) {
	for _, tc := range []struct {
		name                       string
		meta                       uvMetadata
		requested, want, errorText string
	}{
		{"project name", uvMetadata{"fleetroll", map[string]string{"fr": "fleetroll:main", "fleetroll": "fleetroll:main", "other": "tools:other"}}, "", "fleetroll", ""},
		{"normalized name", uvMetadata{"my-tool", map[string]string{"my_tool": "tool:main", "other": "tools:other"}}, "", "my_tool", ""},
		{"only command", uvMetadata{"package", map[string]string{"tool": "tool:main"}}, "", "tool", ""},
		{"aliases", uvMetadata{"package", map[string]string{"b": "tool:main", "a": "tool:main"}}, "", "a", ""},
		{"explicit alias", uvMetadata{"fleetroll", map[string]string{"fr": "fleetroll:main", "fleetroll": "fleetroll:main"}}, "fr", "fr", ""},
		{"ambiguous", uvMetadata{"package", map[string]string{"b": "tool:b", "a": "tool:a"}}, "", "", "a, b; select one with --command"},
		{"unknown", uvMetadata{"package", map[string]string{"tool": "tool:main"}}, "missing", "", "unknown console command"},
		{"none", uvMetadata{"package", nil}, "", "", "no installed console entry points"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := selectUVCommand(tc.meta, tc.requested)
			if tc.errorText != "" {
				if err == nil || !strings.Contains(err.Error(), tc.errorText) {
					t.Fatalf("got %q %v", got, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q %v; want %q", got, err, tc.want)
			}
		})
	}
}

func uvFiles(f *fixture) {
	f.write("pyproject.toml", "[project]\nname = 'example'\nversion = '1.0'\n", 0644)
	f.write("uv.lock", "version = 1\n", 0644)
	f.write("tool.py", "import sys\nprint(sys.argv[1])\n", 0644)
	f.commit()
}

// Fake only uv and its installed artifacts. Setup assertions protect the locked,
// non-editable runtime contract without contacting an index during normal tests.
func fakeUV(t *testing.T, f *fixture) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 is not installed")
	}
	bin := filepath.Join(f.caller, "uv-bin")
	if err := os.Mkdir(bin, 0700); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(f.caller, "uv.log")
	t.Setenv("RELOPS_TEST_UV_LOG", log)
	t.Setenv("RELOPS_TEST_PYTHON", python)
	script := `#!/bin/sh
set -eu
printf '%s\n' "$*" >> "$RELOPS_TEST_UV_LOG"
[ "$UV_NO_ENV_FILE" = 1 ]
[ -z "${VIRTUAL_ENV:-}" ]
case "$1" in
sync)
  case "$*" in *'--locked --no-default-groups --no-dev --no-editable --no-python-downloads --project '*) ;; *) exit 11;; esac
  [ "${RELOPS_TEST_UV_FAIL:-}" != sync ]
  mkdir -p "$UV_PROJECT_ENVIRONMENT/bin"
  cat > "$UV_PROJECT_ENVIRONMENT/bin/python" <<'PYTHON'
#!/bin/sh
if [ "${1:-}" = -I ]; then
  printf '%s\n' '{"name":"example","scripts":{"example":"example:main","alias":"example:main"}}'
else
  exec "$RELOPS_TEST_PYTHON" "$@"
fi
PYTHON
  cat > "$UV_PROJECT_ENVIRONMENT/bin/example" <<'COMMAND'
#!/bin/sh
printf '%s\n' "$*"
printf 'cwd=%s\ncaller=%s\nvenv=%s\n' "$PWD" "$RELOPS_CALLER_CWD" "$VIRTUAL_ENV"
COMMAND
  cp "$UV_PROJECT_ENVIRONMENT/bin/example" "$UV_PROJECT_ENVIRONMENT/bin/alias"
  chmod +x "$UV_PROJECT_ENVIRONMENT/bin/"*
  printf 'setup diagnostics\n'
  ;;
version) printf '%s\n' '{"package_name":"example","version":"1.0"}' ;;
*) exit 12 ;;
esac
`
	if err := os.WriteFile(filepath.Join(bin, "uv"), []byte(script), 0755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))
	return log
}

func TestUVProjectCacheOfflineCWDAndScripts(t *testing.T) {
	f := newFixture(t)
	uvFiles(f)
	logFile := fakeUV(t, f)
	code, out, log := f.run("example", "host-monitor", "-h")
	if code != 0 || !strings.HasPrefix(out, "host-monitor -h\ncwd=") || strings.Contains(out, "setup diagnostics") || !strings.Contains(out, "caller="+f.caller) || !strings.Contains(log, "setup diagnostics") {
		t.Fatalf("project command: %d %q %s", code, out, log)
	}
	lines := strings.Split(out, "\n")
	projectCWD := strings.TrimPrefix(lines[1], "cwd=")
	if _, err := os.Stat(projectCWD); !os.IsNotExist(err) {
		t.Fatalf("checkout retained: %s %v", projectCWD, err)
	}
	for _, args := range [][]string{
		{"--offline", "--command", "alias", "--cwd", "caller", "example", "-h"},
		{"--verbose", "--cwd", "caller", "example", "-h"},
	} {
		code, out, log := f.run(args...)
		if code != 0 || !strings.Contains(out, "cwd="+f.caller) {
			t.Fatalf("reuse/override: %d %q %s", code, out, log)
		}
	}
	// Python script targets share the project environment and retain caller cwd.
	if code, out, log := f.run("example/tool.py", "script"); code != 0 || out != "script\n" {
		t.Fatalf("uv script: %d %q %s", code, out, log)
	}
	data, _ := os.ReadFile(logFile)
	if strings.Count(string(data), "sync ") != 1 || strings.Count(string(data), "version ") != 1 {
		t.Fatalf("environment was rebuilt: %s", data)
	}
	if code, _, log := f.run("--command", "missing", "example"); code != 1 || !strings.Contains(log, "unknown console command") {
		t.Fatalf("unknown command: %d %s", code, log)
	}
	f.write("changed", "new commit", 0644)
	f.commit()
	if code, _, log := f.run("example/tool.sh"); code != 0 {
		t.Fatal(log)
	}
	if code, _, log := f.run("--offline", "example"); code != 1 || !strings.Contains(log, "uv environment is not cached") {
		t.Fatalf("new commit reused environment: %d %s", code, log)
	}
	f.assertClean()
}

func TestUVFailureRetryAndExplicitBypass(t *testing.T) {
	f := newFixture(t)
	uvFiles(f)
	logFile := fakeUV(t, f)
	t.Setenv("RELOPS_TEST_UV_FAIL", "sync")
	if code, out, log := f.run("example", "-h"); code != 1 || out != "" || !strings.Contains(log, "uv sync failed") {
		t.Fatalf("setup failure: %d %q %s", code, out, log)
	}
	t.Setenv("RELOPS_TEST_UV_FAIL", "")
	if code, _, log := f.run("example", "-h"); code != 0 {
		t.Fatal(log)
	}
	before, _ := os.ReadFile(logFile)
	if code, out, log := f.run("--runner", "python3", "example/tool.py", "explicit"); code != 0 || out != "explicit\n" {
		t.Fatalf("override: %d %q %s", code, out, log)
	}
	after, _ := os.ReadFile(logFile)
	if string(before) != string(after) {
		t.Fatal("explicit runner invoked uv")
	}
	f.assertClean()
}

func TestUVRequiredFilesAndMissingExecutable(t *testing.T) {
	f := newFixture(t)
	if code, _, log := f.run("example"); code != 1 || !strings.Contains(log, "requires committed pyproject.toml and uv.lock") {
		t.Fatalf("missing project: %d %s", code, log)
	}
	uvFiles(f)
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
	t.Setenv("PATH", bin)
	if code, _, log := f.run("example"); code != 1 || !strings.Contains(log, "uv is not on PATH; install uv") {
		t.Fatalf("missing uv: %d %s", code, log)
	}
	f.assertClean()
}

func TestUVProjectBoundariesAndContainment(t *testing.T) {
	f := newFixture(t)
	uvFiles(f)
	fakeUV(t, f)
	f.write("nested/pyproject.toml", "[project]\nname='example'\nversion='1.0'\n", 0644)
	f.write("nested/uv.lock", "version=1\n", 0644)
	f.write("nested/tool.py", "print('nested')\n", 0644)
	f.commit()
	if code, out, log := f.run("example/nested", "-h"); code != 0 || !strings.Contains(out, "/checkout/nested\n") {
		t.Fatalf("nested project: %d %q %s", code, out, log)
	}
	if err := os.Remove(filepath.Join(f.source, "nested/uv.lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join(f.caller, "outside.lock"), filepath.Join(f.source, "nested/uv.lock")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(f.caller, "outside.lock"), []byte("version=1"), 0600); err != nil {
		t.Fatal(err)
	}
	f.commit()
	if code, _, log := f.run("example/nested/tool.py"); code != 1 || !strings.Contains(log, "symlink escapes") {
		t.Fatalf("external lock: %d %s", code, log)
	}
	f.assertClean()
}

func TestUVConcurrentSetup(t *testing.T) {
	f := newFixture(t)
	uvFiles(f)
	logFile := fakeUV(t, f)
	var workers sync.WaitGroup
	for range 2 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if code, out, log := f.run("example", "-h"); code != 0 || !strings.HasPrefix(out, "-h\n") {
				t.Errorf("concurrent command: %d %q %s", code, out, log)
			}
		}()
	}
	workers.Wait()
	data, _ := os.ReadFile(logFile)
	if strings.Count(string(data), "sync ") != 1 {
		t.Fatalf("concurrent setup was not serialized: %s", data)
	}
	f.assertClean()
}
