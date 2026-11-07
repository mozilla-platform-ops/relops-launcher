package launcher

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestCatalogCLI(t *testing.T) {
	for _, args := range [][]string{
		{"c", "fr", "host-monitor", "--beta", "a b", ""},
		{"catalog", "launch", "fleetroll", "host-monitor", "--beta", "a b", ""},
	} {
		o, done, err := parse(args, &bytes.Buffer{}, "test")
		if err != nil || done || o.repo != "fleetroll_mvp" || o.catalogName != "fleetroll" || o.catalogChannel != "default" || !reflect.DeepEqual(o.args, []string{"host-monitor", "--beta", "a b", ""}) {
			t.Fatalf("parse %v: %+v %v %v", args, o, done, err)
		}
	}
	for _, flag := range []string{"-b", "--beta"} {
		o, _, err := parse([]string{"c", flag, "replay-tc-pool"}, &bytes.Buffer{}, "test")
		if err != nil || o.owner != "aerickson" || o.catalogChannel != "beta" || len(o.args) != 0 {
			t.Fatalf("beta: %+v %v", o, err)
		}
	}
	for _, args := range [][]string{
		{"c", "unknown"}, {"c", "list"}, {"c", "beta"},
		{"c", "-b", "fr"}, {"c", "replay-tc-pool"},
		{"c", "-b", "--ref", "another", "replay-tc-pool"},
		{"c", "--owner", "someone", "fr"}, {"c", "--ref", "main", "fleetbench"},
		{"catalog", "unknown"}, {"catalog", "list", "extra"}, {"catalog", "info"},
		{"catalog", "info", "unknown"}, {"repo", "-b", "example/tool.sh"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}, "test"); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	for _, args := range [][]string{{"--offline", "c", "fr"}, {"c", "--offline", "fr"}, {"catalog", "launch", "--offline", "fr"}} {
		o, _, err := parse(args, &bytes.Buffer{}, "test")
		if err != nil || !o.offline {
			t.Fatalf("global option %v: %+v %v", args, o, err)
		}
	}
	o, _, err := parse([]string{"c", "--ref", "test-branch", "fr"}, &bytes.Buffer{}, "test")
	if err != nil || o.ref != "test-branch" {
		t.Fatalf("ref override: %+v %v", o, err)
	}
	o, _, err = parse([]string{"c", "fleetbench", "--version"}, &bytes.Buffer{}, "test")
	if err != nil || !isReleaseTarget(o) || !reflect.DeepEqual(o.args, []string{"--version"}) {
		t.Fatalf("release: %+v %v", o, err)
	}
}

func TestBareCatalogLaunch(t *testing.T) {
	for _, args := range [][]string{{"c"}, {"catalog", "launch"}} {
		var out bytes.Buffer
		_, done, err := parse(args, &out, "test")
		text := out.String()
		if err != nil || !done || !strings.Contains(text, "relops c fb") || !strings.Contains(text, "TOOLS (BETA)") || !strings.Contains(text, "relops c -b replay-tc-pool") || strings.Contains(text, "Run host benchmarks") || strings.Contains(text, "relops c --help") {
			t.Fatalf("bare launch %v: done=%v err=%v output=%q", args, done, err, text)
		}
	}
}

func TestCatalogDiscovery(t *testing.T) {
	for _, args := range [][]string{{"catalog", "list"}, {"catalog", "info", "fr"}} {
		var out bytes.Buffer
		_, done, err := parse(args, &out, "test")
		if err != nil || !done || !strings.Contains(out.String(), "fleetroll") || !strings.Contains(out.String(), "fr") {
			t.Fatalf("discovery %v: %q %v", args, out.String(), err)
		}
	}
	var out bytes.Buffer
	_, _, err := parse([]string{"catalog", "info", "replay-tc-pool"}, &out, "test")
	if err != nil || !strings.Contains(out.String(), "BETA\n  Source: aerickson/android-tools") || strings.Contains(out.String(), "DEFAULT\n") {
		t.Fatalf("beta info: %s %v", out.String(), err)
	}
}

func TestCatalogListAlias(t *testing.T) {
	for _, suffix := range [][]string{nil, {"--help"}, {"-h"}, {"extra"}} {
		var list, ls bytes.Buffer
		_, listDone, listErr := parse(append([]string{"catalog", "list"}, suffix...), &list, "test")
		_, lsDone, lsErr := parse(append([]string{"catalog", "ls"}, suffix...), &ls, "test")
		if list.String() != ls.String() || listDone != lsDone || (listErr == nil) != (lsErr == nil) {
			t.Fatalf("list/ls differ for %v: list=%q/%v/%v ls=%q/%v/%v", suffix, list.String(), listDone, listErr, ls.String(), lsDone, lsErr)
		}
	}
}

func TestExplicitRepositoryEscape(t *testing.T) {
	for _, target := range []string{"c", "catalog", "repo", "fleetbench"} {
		o, _, err := parse([]string{"repo", "--ref", "branch", target, "--beta"}, &bytes.Buffer{}, "test")
		if err != nil || o.repo != target || o.ref != "branch" || o.catalogName != "" || isReleaseTarget(o) || !reflect.DeepEqual(o.args, []string{"--beta"}) {
			t.Fatalf("escape %s: %+v %v", target, o, err)
		}
	}
}

func TestCatalogExecutionAndOffline(t *testing.T) {
	f := newFixture(t)
	script := "#!/bin/sh\nprintf '%s\\n' \"$@\"\nexit 19\n"
	f.write("worker_health/create_tc_task.py", script, 0755)
	f.write("worker_health/replay_tc_pool.py", script, 0755)
	f.commit()
	f.git("branch", "20261105-more_tool_development")
	for _, args := range [][]string{
		{"c", "create-tc-task", "a b", "--beta"},
		{"c", "--offline", "create-tc-task", "a b", "--beta"},
		{"c", "-b", "replay-tc-pool", "a b", "--beta"},
	} {
		code, out, errOut := f.run(args...)
		if code != 19 || out != "a b\n--beta\n" || !strings.Contains(errOut, " · ") || strings.Count(errOut, "\n") != 1 || strings.Contains(errOut, "catalog=") {
			t.Fatalf("launch %v: code=%d out=%q stderr=%s", args, code, out, errOut)
		}
	}
	code, out, errOut := f.run("repo", "c/tool.sh")
	if code != 0 || out != "original\n" {
		t.Fatalf("repository escape: %d %q %s", code, out, errOut)
	}
}
