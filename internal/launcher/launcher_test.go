package launcher

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

type fixture struct {
	t                     *testing.T
	source, cache, caller string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	// Keep developer signing settings, filters, URL rewrites, and templates out of
	// local integration tests. Never connect to GitHub from the tests.
	t.Setenv("GIT_CONFIG_GLOBAL", os.DevNull)
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	base := t.TempDir()
	f := &fixture{t: t, source: filepath.Join(base, "remote with spaces"), cache: filepath.Join(base, "cache with spaces"), caller: filepath.Join(base, "caller with spaces")}
	for _, dir := range []string{f.source, f.caller} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	f.git("init", "-b", "trunk")
	f.git("config", "user.name", "Relops Test")
	f.git("config", "user.email", "relops@example.invalid")
	f.git("config", "commit.gpgsign", "false")
	f.write("tool.sh", "#!/bin/sh\nprintf 'original\\n'\n", 0755)
	f.commit()
	return f
}

func (f *fixture) git(args ...string) string {
	f.t.Helper()
	cmd := exec.Command("git", append([]string{"-c", "core.hooksPath=/dev/null"}, args...)...)
	cmd.Dir = f.source
	output, err := cmd.CombinedOutput()
	if err != nil {
		f.t.Fatalf("git %v: %v\n%s", args, err, output)
	}
	return strings.TrimSpace(string(output))
}

func (f *fixture) write(name, content string, mode os.FileMode) {
	f.t.Helper()
	file := filepath.Join(f.source, name)
	if err := os.MkdirAll(filepath.Dir(file), 0700); err != nil {
		f.t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte(content), mode); err != nil {
		f.t.Fatal(err)
	}
	if err := os.Chmod(file, mode); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) commit() string {
	f.t.Helper()
	f.git("add", ".")
	f.git("commit", "-m", "fixture")
	return f.git("rev-parse", "HEAD")
}

func (f *fixture) app(out, errOut *bytes.Buffer) *app {
	return &app{cache: f.cache, caller: f.caller, in: strings.NewReader(""), out: out, errOut: errOut,
		remote: func(string, string) string { return f.source }, lockTimeout: 5 * time.Second}
}

func (f *fixture) run(args ...string) (int, string, string) {
	f.t.Helper()
	var out, errOut bytes.Buffer
	o, done, err := parse(args, &out, "test")
	if err != nil || done {
		f.t.Fatalf("parse: done=%v err=%v", done, err)
	}
	code, err := f.app(&out, &errOut).run(context.Background(), o)
	if err != nil {
		fmt.Fprintln(&errOut, err)
	}
	return code, out.String(), errOut.String()
}

func (f *fixture) assertClean() {
	f.t.Helper()
	entries, err := os.ReadDir(filepath.Join(f.cache, "runs"))
	if err != nil || len(entries) != 0 {
		f.t.Fatalf("run directories not cleaned: %v %v", entries, err)
	}
	repo := filepath.Join(f.cache, "repos", defaultOwner, "example.git")
	var out, errOut bytes.Buffer
	listing, err := f.app(&out, &errOut).git(context.Background(), repo, "worktree", "list", "--porcelain")
	if err != nil || strings.Contains(listing, "detached") {
		f.t.Fatalf("worktree metadata not cleaned: %s %v", listing, err)
	}
}

func TestCLI(t *testing.T) {
	for _, args := range [][]string{
		{"https://github.com/org/repo/file"}, {"-evil/tool"}, {"example/../file"}, {"example//etc/passwd"},
		{"example/.git/config"}, {"--runner", "guess", "example/tool"}, {"--cwd", "elsewhere", "example/tool"},
		{"--ref=-oops", "example/tool"}, {"example/tool\\other"}, {"../file"},
		{"example/"}, {"--command", "../outside", "example"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}, "test"); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	oProject, _, projectErr := parse([]string{"--cwd", "caller", "--command", "fr", "fleetroll_mvp", "-h"}, &bytes.Buffer{}, "test")
	if projectErr != nil || oProject.repo != "fleetroll_mvp" || oProject.path != "" || !oProject.cwdSet || oProject.commandName != "fr" || strings.Join(oProject.args, "|") != "-h" {
		t.Fatalf("project argument boundary: %+v %v", oProject, projectErr)
	}
	o, _, err := parse([]string{"--ref", "feature/one", "example/tool", "--ref", "tool-ref", "--", "a b"}, &bytes.Buffer{}, "test")
	if err != nil || o.ref != "feature/one" || strings.Join(o.args, "|") != "--ref|tool-ref|--|a b" {
		t.Fatalf("argument boundary: %+v %v", o, err)
	}
	for _, arg := range []string{"--help", "--version"} {
		var out bytes.Buffer
		if _, done, err := parse([]string{arg}, &out, "test-version"); err != nil || !done || out.Len() == 0 {
			t.Fatalf("%s: done=%v output=%q err=%v", arg, done, out.String(), err)
		}
	}
}

func TestDefaultBranchAndFreshBranchTips(t *testing.T) {
	f := newFixture(t)
	old := f.git("rev-parse", "HEAD")
	code, out, log := f.run("--verbose", "example/tool.sh")
	if code != 0 || out != "original\n" || !strings.Contains(log, "ref=refs/heads/trunk commit="+old) {
		t.Fatalf("default branch: %d %q %s", code, out, log)
	}
	info, err := os.Stat(f.cache)
	if err != nil || info.Mode().Perm() != 0700 {
		t.Fatalf("private cache: %v %v", info, err)
	}
	f.git("checkout", "-b", "feature/one")
	f.write("tool.sh", "#!/bin/sh\nprintf 'feature\\n'\n", 0755)
	sha := f.commit()
	code, out, log = f.run("--verbose", "--ref", "feature/one", "example/tool.sh")
	if code != 0 || out != "feature\n" || !strings.Contains(log, "commit="+sha) {
		t.Fatalf("feature branch: %d %q %s", code, out, log)
	}
	f.write("tool.sh", "#!/bin/sh\nprintf 'updated\\n'\n", 0755)
	f.commit()
	code, out, log = f.run("--ref", "feature/one", "example/tool.sh")
	if code != 0 || out != "updated\n" {
		t.Fatalf("updated branch: %d %q %s", code, out, log)
	}
	code, out, log = f.run("--ref", old, "example/tool.sh")
	if code != 0 || out != "original\n" {
		t.Fatalf("pinned SHA: %d %q %s", code, out, log)
	}
	// The source repository's HEAD is also its advertised default branch.
	code, out, log = f.run("--verbose", "example/tool.sh")
	if code != 0 || out != "updated\n" || !strings.Contains(log, "ref=refs/heads/feature/one") {
		t.Fatalf("default branch changed: %d %q %s", code, out, log)
	}
	f.assertClean()
}

func TestTagsAmbiguityAndRefDeletion(t *testing.T) {
	f := newFixture(t)
	f.git("tag", "-a", "release", "-m", "release")
	code, out, log := f.run("--ref", "release", "example/tool.sh")
	if code != 0 || out != "original\n" {
		t.Fatalf("annotated tag: %d %q %s", code, out, log)
	}
	f.git("branch", "release")
	code, _, log = f.run("--ref", "release", "example/tool.sh")
	if code != 1 || !strings.Contains(log, "ambiguous") {
		t.Fatalf("ambiguous ref: %d %s", code, log)
	}
	for _, ref := range []string{"refs/heads/release", "refs/tags/release"} {
		code, _, log = f.run("--ref", ref, "example/tool.sh")
		if code != 0 {
			t.Fatalf("explicit %s: %d %s", ref, code, log)
		}
	}
	f.git("branch", "-D", "release")
	f.git("tag", "-d", "release")
	code, _, log = f.run("--ref", "release", "example/tool.sh")
	if code != 1 || !strings.Contains(log, "not found") {
		t.Fatalf("deleted refs must not be stale: %d %s", code, log)
	}
	for _, ref := range []string{"missing", "trunk~1", "refs/remotes/origin/trunk", "abc1234", strings.Repeat("a", 40)} {
		code, _, log = f.run("--ref", ref, "example/tool.sh")
		if code != 1 {
			t.Fatalf("invalid/unavailable %s: %d %s", ref, code, log)
		}
	}
	f.assertClean()
}

func TestOfflineAndFailedOnlineFetch(t *testing.T) {
	f := newFixture(t)
	code, _, log := f.run("--offline", "example/tool.sh")
	if code != 1 || !strings.Contains(log, "no cached repository") {
		t.Fatalf("offline cache miss: %d %s", code, log)
	}
	code, _, log = f.run("example/tool.sh")
	if code != 0 {
		t.Fatal(log)
	}
	if err := os.Rename(f.source, f.source+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	code, out, log := f.run("example/tool.sh")
	if code != 1 || out != "" || !strings.Contains(log, "use --offline") {
		t.Fatalf("failed fetch silently ran code: %d %q %s", code, out, log)
	}
	code, out, log = f.run("--offline", "example/tool.sh")
	if code != 0 || out != "original\n" || !strings.Contains(log, "(cached)") {
		t.Fatalf("offline reuse: %d %q %s", code, out, log)
	}
	code, _, log = f.run("--offline", "--ref", strings.Repeat("a", 40), "example/tool.sh")
	if code != 1 || !strings.Contains(log, "not cached") {
		t.Fatalf("offline commit miss: %d %s", code, log)
	}
	f.assertClean()
}

func TestPartialInitializationIsNotReused(t *testing.T) {
	f := newFixture(t)
	if err := os.Rename(f.source, f.source+"-unavailable"); err != nil {
		t.Fatal(err)
	}
	code, _, log := f.run("example/tool.sh")
	if code != 1 || !strings.Contains(log, "verify access") {
		t.Fatalf("initial fetch failure: %d %s", code, log)
	}
	entries, err := os.ReadDir(filepath.Join(f.cache, "repos", defaultOwner))
	if err != nil || len(entries) != 0 {
		t.Fatalf("partial initialization retained: %v %v", entries, err)
	}
	if err := os.Rename(f.source+"-unavailable", f.source); err != nil {
		t.Fatal(err)
	}
	code, _, log = f.run("example/tool.sh")
	if code != 0 {
		t.Fatalf("recovery: %d %s", code, log)
	}
	f.assertClean()
}

func TestToolIOArgumentsAndExitStatus(t *testing.T) {
	f := newFixture(t)
	f.write("tool.sh", "#!/bin/sh\nIFS= read -r input\nprintf 'input=%s\\n' \"$input\"\nprintf '<%s>\\n' \"$@\"\nprintf 'tool stderr\\n' >&2\nexit 7\n", 0755)
	f.commit()
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	a.in = strings.NewReader("hello input\n")
	o, _, err := parse([]string{"example/tool.sh", "a b", "--ref", "--", "$(echo nope)", ""}, &out, "test")
	if err != nil {
		t.Fatal(err)
	}
	code, err := a.run(context.Background(), o)
	if err != nil || code != 7 || out.String() != "input=hello input\n<a b>\n<--ref>\n<-->\n<$(echo nope)>\n<>\n" || !strings.Contains(log.String(), "tool stderr") {
		t.Fatalf("IO/exit: %d %v stdout=%q stderr=%q", code, err, out.String(), log.String())
	}
	f.assertClean()
}

func TestWorkingDirectoriesAndRunners(t *testing.T) {
	f := newFixture(t)
	f.write("input.txt", "repository", 0644)
	f.write("read.sh", "printf '%s\\n' \"$RELOPS_CALLER_CWD\"\ncat \"$1\"\n", 0644)
	f.commit()
	if err := os.WriteFile(filepath.Join(f.caller, "input.txt"), []byte("caller"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ cwd, want string }{{"repo", "repository"}, {"caller", "caller"}} {
		code, out, log := f.run("--runner", "sh", "--cwd", tc.cwd, "example/read.sh", "./input.txt")
		if code != 0 || out != f.caller+"\n"+tc.want {
			t.Fatalf("cwd %s: %d %q %s", tc.cwd, code, out, log)
		}
	}
	code, _, log := f.run("example/read.sh")
	if code != 1 || !strings.Contains(log, "cannot detect a runner") {
		t.Fatalf("no extension guessing: %d %s", code, log)
	}
	f.write("bad.sh", "#!/nonexistent/relops-test-interpreter\n", 0755)
	f.commit()
	code, _, log = f.run("example/bad.sh")
	if code != 1 || !strings.Contains(log, "shebang/interpreter") {
		t.Fatalf("missing shebang interpreter: %d %s", code, log)
	}
	f.assertClean()
}

func TestSymlinksAndNonFiles(t *testing.T) {
	f := newFixture(t)
	if err := os.Symlink("tool.sh", filepath.Join(f.source, "inside")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/bin/sh", filepath.Join(f.source, "outside")); err != nil {
		t.Fatal(err)
	}
	f.write("dir/file", "data", 0644)
	f.commit()
	code, out, log := f.run("example/inside")
	if code != 0 || out != "original\n" {
		t.Fatalf("internal symlink: %d %q %s", code, out, log)
	}
	for _, name := range []string{"outside", "dir", "missing"} {
		code, _, log = f.run("example/" + name)
		if code != 1 {
			t.Fatalf("invalid target %s: %d %s", name, code, log)
		}
	}
	f.assertClean()
}

func TestConcurrentRunsHaveSeparateWritableCheckouts(t *testing.T) {
	f := newFixture(t)
	// Each tool announces readiness outside the checkout, then waits until both
	// tools run. A shared checkout would overwrite the per-invocation marker.
	f.write("tool.sh", `#!/bin/sh
set -eu
printf '%s' "$1" > local-marker
touch "$RELOPS_CALLER_CWD/$1.ready"
count=0
while [ ! -f "$RELOPS_CALLER_CWD/one.ready" ] || [ ! -f "$RELOPS_CALLER_CWD/two.ready" ]; do
  count=$((count + 1))
  [ "$count" -lt 100 ] || exit 9
  sleep 0.05
done
cat local-marker
`, 0755)
	f.commit()
	var wg sync.WaitGroup
	for _, name := range []string{"one", "two"} {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			var out, log bytes.Buffer
			o, _, err := parse([]string{"--cwd", "repo", "example/tool.sh", name}, &out, "test")
			if err != nil {
				t.Error(err)
				return
			}
			code, err := f.app(&out, &log).run(context.Background(), o)
			if err != nil || code != 0 || out.String() != name {
				t.Errorf("concurrent %s: %d %v %q %s", name, code, err, out.String(), log.String())
			}
		}(name)
	}
	wg.Wait()
	f.assertClean()
}

func TestRepositoryLockTimeoutAndCancellation(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	unlock, err := lock(context.Background(), path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	start := time.Now()
	if _, err := lock(context.Background(), path, 100*time.Millisecond); err == nil || !strings.Contains(err.Error(), "retry") {
		t.Fatalf("lock timeout: %v", err)
	}
	if time.Since(start) > time.Second {
		t.Fatal("lock timeout not bounded")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := lock(ctx, path, time.Second); err == nil {
		t.Fatal("cancelled lock wait succeeded")
	}
}

func waitForFile(t *testing.T, name string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(name); err == nil {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", name)
}

func TestInterruptKillsDescendantsAndCleansCheckout(t *testing.T) {
	f := newFixture(t)
	f.write("tool.sh", `#!/bin/sh
set -eu
sleep 60 &
child=$!
trap 'kill "$child" 2>/dev/null; wait "$child" 2>/dev/null; exit 143' TERM
printf '%s' "$child" > "$RELOPS_CALLER_CWD/child.pid"
wait "$child"
`, 0755)
	f.commit()
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	o, _, err := parse([]string{"example/tool.sh"}, &out, "test")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	type result struct {
		code int
		err  error
	}
	done := make(chan result, 1)
	go func() { code, err := a.run(ctx, o); done <- result{code, err} }()
	pidFile := filepath.Join(f.caller, "child.pid")
	waitForFile(t, pidFile)
	data, err := os.ReadFile(pidFile)
	if err != nil {
		t.Fatal(err)
	}
	var child int
	if _, err := fmt.Sscanf(string(data), "%d", &child); err != nil {
		t.Fatal(err)
	}
	cancel(interrupted{syscall.SIGTERM})
	select {
	case result := <-done:
		if result.err != nil || result.code != 143 {
			t.Fatalf("interruption: %+v %s", result, log.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("interrupted tool did not exit")
	}
	if err := syscall.Kill(child, 0); err == nil {
		t.Fatalf("descendant %d survived interruption", child)
	}
	f.assertClean()
}
