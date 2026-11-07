package launcher

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOwnerValidationAndDefaults(t *testing.T) {
	for _, owner := range []string{"", "../fork", "org/repo", "git@github.com:fork", "-fork", "fork-", "fork_name", strings.Repeat("a", 40)} {
		if _, _, err := parse([]string{"repo", "--owner", owner, "example/tool.py"}, &bytes.Buffer{}, "test"); err == nil {
			t.Errorf("accepted invalid owner %q", owner)
		}
	}
	o, _, err := parse([]string{"repo", "example/tool.py", "--owner", "tool-argument"}, &bytes.Buffer{}, "test")
	if err != nil || o.owner != defaultOwner || o.cwd != "caller" || strings.Join(o.args, "|") != "--owner|tool-argument" {
		t.Fatalf("defaults/boundary: %+v %v", o, err)
	}
	o, _, err = parse([]string{"repo", "--owner", "Aerickson", "example/tool.py"}, &bytes.Buffer{}, "test")
	if err != nil || o.owner != "aerickson" || githubRemote(o.owner, o.repo) != "git@github.com:aerickson/example.git" {
		t.Fatalf("owner resolution: %+v %v", o, err)
	}
}

func TestOwnersHaveSeparateReposRefsAndLocks(t *testing.T) {
	f := newFixture(t)
	forkPath := filepath.Join(f.caller, "fork")
	f.git("clone", f.source, forkPath)
	fork := &fixture{t: t, source: forkPath}
	fork.git("config", "user.name", "Relops Test")
	fork.git("config", "user.email", "relops@example.invalid")
	fork.git("config", "commit.gpgsign", "false")
	fork.write("tool.sh", "#!/bin/sh\nprintf 'fork\\n'\n", 0755)
	fork.commit()
	remote := func(owner, repo string) string {
		if owner == "aerickson" {
			return forkPath
		}
		return f.source
	}
	run := func(args ...string) (int, string, string) {
		t.Helper()
		var out, log bytes.Buffer
		a := f.app(&out, &log)
		a.remote, a.lockTimeout = remote, 200*time.Millisecond
		o, _, err := parse(append([]string{"repo"}, args...), &out, "test")
		if err != nil {
			t.Fatal(err)
		}
		code, err := a.run(context.Background(), o)
		if err != nil {
			log.WriteString(err.Error())
		}
		return code, out.String(), log.String()
	}
	if code, out, log := run("example/tool.sh"); code != 0 || out != "original\n" || !strings.Contains(log, defaultOwner+"/example") {
		t.Fatalf("organization: %d %q %s", code, out, log)
	}
	if code, _, log := run("--owner", "aerickson", "--offline", "example/tool.sh"); code != 1 || !strings.Contains(log, "no cached repository for aerickson/example") {
		t.Fatalf("fork must not reuse organization cache: %d %s", code, log)
	}
	unlock, err := lock(context.Background(), filepath.Join(f.cache, "locks", defaultOwner, "example.lock"), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// A held organization lock must not prevent creating or cleaning a fork run.
	code, out, log := run("--owner", "Aerickson", "example/tool.sh")
	unlock()
	if code != 0 || out != "fork\n" || !strings.Contains(log, "aerickson/example") {
		t.Fatalf("fork while organization is locked: %d %q %s", code, out, log)
	}
	if code, out, log := run("--owner", "aerickson", "--offline", "example/tool.sh"); code != 0 || out != "fork\n" {
		t.Fatalf("offline fork: %d %q %s", code, out, log)
	}
	if code, out, log := run("--offline", "example/tool.sh"); code != 0 || out != "original\n" {
		t.Fatalf("offline organization: %d %q %s", code, out, log)
	}
	f.assertClean()
}
