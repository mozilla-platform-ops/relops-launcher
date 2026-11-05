package launcher

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
)

func TestExplicitPythonRunner(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 is not installed")
	}
	f := newFixture(t)
	f.write("tool.py", "import sys\nprint(sys.argv[1])\n", 0644)
	f.commit()
	code, out, log := f.run("--runner", "python3", "example/tool.py", "hello coworker")
	if code != 0 || out != "hello coworker\n" {
		t.Fatalf("Python runner: %d %q %s", code, out, log)
	}
	f.assertClean()
}

func TestMissingRunner(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	if _, err := toolCommand("unused.py", "python3", nil); err == nil || !strings.Contains(err.Error(), "not on PATH") {
		t.Fatalf("missing runner: %v", err)
	}
}

func TestGitContextAndCheckoutHooks(t *testing.T) {
	f := newFixture(t)
	t.Setenv("GIT_DIR", filepath.Join(f.source, ".git"))
	t.Setenv("GIT_WORK_TREE", f.source)
	code, _, log := f.run("example/tool.sh")
	if code != 0 {
		t.Fatalf("inherited Git environment: %d %s", code, log)
	}
	hooks := filepath.Join(f.caller, "hooks")
	if err := os.Mkdir(hooks, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(f.caller, "hook-ran")
	if err := os.WriteFile(filepath.Join(hooks, "post-checkout"), []byte("#!/bin/sh\ntouch '"+marker+"'\n"), 0755); err != nil {
		t.Fatal(err)
	}
	var out, errOut bytes.Buffer
	a := f.app(&out, &errOut)
	if _, err := a.git(context.Background(), filepath.Join(f.cache, "repos", "example.git"), "config", "core.hooksPath", hooks); err != nil {
		t.Fatal(err)
	}
	code, _, log = f.run("example/tool.sh")
	if code != 0 {
		t.Fatal(log)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("checkout hook ran: %v", err)
	}
	f.assertClean()
}

func TestGitTransportRewritePreservesCacheValidation(t *testing.T) {
	f := newFixture(t)
	alias := f.source + "-alias"
	t.Setenv("GIT_CONFIG_COUNT", "1")
	t.Setenv("GIT_CONFIG_KEY_0", "url."+f.source+".insteadOf")
	t.Setenv("GIT_CONFIG_VALUE_0", alias)
	for i := 0; i < 2; i++ {
		var out, log bytes.Buffer
		a := f.app(&out, &log)
		a.remote = func(string) string { return alias }
		o, _, err := parse([]string{"example/tool.sh"}, &out, "test")
		if err != nil {
			t.Fatal(err)
		}
		code, err := a.run(context.Background(), o)
		if err != nil || code != 0 || out.String() != "original\n" {
			t.Fatalf("rewritten transport run %d: %d %v %q %s", i, code, err, out.String(), log.String())
		}
	}
	f.assertClean()
}

func TestToolCanUseItsOwnGitWorktree(t *testing.T) {
	f := newFixture(t)
	f.write("tool.sh", "#!/bin/sh\ngit rev-parse --is-inside-work-tree\n", 0755)
	f.commit()
	code, out, log := f.run("example/tool.sh")
	if code != 0 || out != "true\n" {
		t.Fatalf("tool Git operations: %d %q %s", code, out, log)
	}
	f.assertClean()
}

// Build with go test -c, then run this test in a real terminal with
// RELOPS_TEST_TERMINAL=1. Type hello, then press Ctrl-C when prompted.
func TestInteractiveTerminal(t *testing.T) {
	if os.Getenv("RELOPS_TEST_TERMINAL") != "1" {
		t.Skip("manual terminal check; see README")
	}
	cmd := exec.Command("/bin/sh", "-c", `printf 'Type hello and Enter: '; read -r answer; [ "$answer" = hello ] || exit 9; printf 'Now press Ctrl-C\n'; sleep 60`)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	code, err := runTool(context.Background(), cmd)
	if err != nil || code != 130 {
		t.Fatalf("interactive interruption: %d %v", code, err)
	}
	var foreground int32
	if err := terminalGroup(os.Stdin, syscall.TIOCGPGRP, &foreground); err != nil || foreground != int32(syscall.Getpgrp()) {
		t.Fatalf("launcher did not regain terminal: group=%d err=%v", foreground, err)
	}
}
