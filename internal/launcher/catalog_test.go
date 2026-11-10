package launcher

import (
	"bytes"
	"reflect"
	"strings"
	"testing"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

func TestCatalogCLI(t *testing.T) {
	for _, args := range [][]string{
		{"run", "fr", "host-monitor", "--beta", "a b", ""},
		{"r", "fr", "host-monitor", "--beta", "a b", ""},
		{"catalog", "launch", "fleetroll", "host-monitor", "--beta", "a b", ""},
	} {
		o, done, err := parse(args, &bytes.Buffer{}, "test")
		if err != nil || done || o.repo != "fleetroll_mvp" || o.catalogName != "fleetroll" || o.catalogChannel != "default" || !reflect.DeepEqual(o.args, []string{"host-monitor", "--beta", "a b", ""}) {
			t.Fatalf("parse %v: %+v %v %v", args, o, done, err)
		}
	}
	for _, flag := range []string{"-b", "--beta"} {
		o, _, err := parse([]string{"run", flag, "replay-tc-pool"}, &bytes.Buffer{}, "test")
		if err != nil || o.owner != "aerickson" || o.catalogChannel != "beta" || len(o.args) != 0 {
			t.Fatalf("beta: %+v %v", o, err)
		}
	}
	for _, args := range [][]string{
		{"run", "unknown"}, {"run", "list"}, {"run", "beta"},
		{"run", "-b", "fr"}, {"run", "replay-tc-pool"},
		{"run", "-b", "--ref", "another", "replay-tc-pool"},
		{"run", "--owner", "someone", "fr"}, {"run", "--ref", "main", "fleetbench"},
		{"catalog", "unknown"}, {"catalog", "list", "extra"}, {"catalog", "info"},
		{"catalog", "info", "unknown"}, {"repo", "-b", "example/tool.sh"},
	} {
		if _, _, err := parse(args, &bytes.Buffer{}, "test"); err == nil {
			t.Errorf("accepted %q", args)
		}
	}
	for _, args := range [][]string{{"--offline", "run", "fr"}, {"run", "--offline", "fr"}, {"catalog", "launch", "--offline", "fr"}} {
		o, _, err := parse(args, &bytes.Buffer{}, "test")
		if err != nil || !o.offline {
			t.Fatalf("global option %v: %+v %v", args, o, err)
		}
	}
	o, _, err := parse([]string{"run", "--ref", "test-branch", "fr"}, &bytes.Buffer{}, "test")
	if err != nil || o.ref != "test-branch" {
		t.Fatalf("ref override: %+v %v", o, err)
	}
	o, _, err = parse([]string{"run", "fleetbench", "--version"}, &bytes.Buffer{}, "test")
	if err != nil || !isReleaseTarget(o) || !reflect.DeepEqual(o.args, []string{"--version"}) {
		t.Fatalf("release: %+v %v", o, err)
	}
}

func TestBareCatalogLaunch(t *testing.T) {
	for _, args := range [][]string{{"run"}, {"r"}, {"catalog", "launch"}} {
		var out bytes.Buffer
		_, done, err := parse(args, &out, "test")
		text := out.String()
		if err != nil || !done || !strings.Contains(text, "relops run fb") || !strings.Contains(text, "TOOLS (BETA)") || !strings.Contains(text, "relops run -b replay-tc-pool") || strings.Contains(text, "Run host benchmarks") || strings.Contains(text, "relops run --help") {
			t.Fatalf("bare launch %v: done=%v err=%v output=%q", args, done, err, text)
		}
	}
}

func TestRunAliases(t *testing.T) {
	for _, suffix := range [][]string{
		{"--offline", "ct", "--ref", "tool-argument", "a b", ""},
		{"-b", "replay-tc-pool", "--help"},
		{"--ref", "my-feature", "fr", "--help"},
		{"fb", "--version"},
	} {
		want, _, err := parse(append([]string{"run"}, suffix...), &bytes.Buffer{}, "test")
		if err != nil {
			t.Fatal(err)
		}
		for _, command := range []string{"r"} {
			got, done, err := parse(append([]string{command}, suffix...), &bytes.Buffer{}, "test")
			if err != nil || done || !reflect.DeepEqual(got, want) {
				t.Fatalf("alias %s %v: got=%+v want=%+v done=%v err=%v", command, suffix, got, want, done, err)
			}
		}
	}
	for _, command := range []string{"run", "r"} {
		got, done, err := parse([]string{"--offline", command, "ct"}, &bytes.Buffer{}, "test")
		if err != nil || done || !got.offline || got.catalogName != "create-tc-task" {
			t.Fatalf("prefixed alias %s: %+v %v %v", command, got, done, err)
		}
	}
}

func TestCatalogDiscovery(t *testing.T) {
	for _, args := range [][]string{{"catalog"}, {"info", "fr"}, {"catalog", "list"}, {"catalog", "info", "fr"}} {
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

func TestDirectDiscoveryAliases(t *testing.T) {
	for _, pair := range []struct{ direct, alias []string }{
		{[]string{"info", "ct"}, []string{"i", "ct"}},
		{[]string{"info", "--help"}, []string{"i", "--help"}},
		{[]string{"info", "-h"}, []string{"i", "-h"}},
		{[]string{"catalog"}, []string{"c"}},
		{[]string{"catalog", "--help"}, []string{"c", "--help"}},
		{[]string{"catalog", "list"}, []string{"c", "list"}},
		{[]string{"info", "fr"}, []string{"c", "info", "fr"}},
		{[]string{"catalog"}, []string{"catalog", "list"}},
		{[]string{"catalog"}, []string{"catalog", "ls"}},
		{[]string{"info", "fb"}, []string{"catalog", "info", "fb"}},
		{[]string{"info", "--help"}, []string{"catalog", "info", "--help"}},
	} {
		var direct, alias bytes.Buffer
		_, done, err := parse(pair.direct, &direct, "test")
		_, aliasDone, aliasErr := parse(pair.alias, &alias, "test")
		if err != nil || aliasErr != nil || !done || !aliasDone || direct.String() != alias.String() {
			t.Fatalf("discovery alias %v/%v: %v/%v", pair.direct, pair.alias, err, aliasErr)
		}
	}
	for _, args := range [][]string{{"i"}, {"i", "unknown"}, {"i", "fr", "extra"}, {"--offline", "i", "fr"}, {"info"}, {"info", "unknown"}, {"info", "fr", "extra"}, {"--offline", "info", "fr"}, {"--offline", "catalog"}, {"c", "ct"}, {"c", "fr", "--help"}, {"--offline", "c"}} {
		if _, _, err := parse(args, &bytes.Buffer{}, "test"); err == nil {
			t.Fatalf("accepted invalid discovery args %v", args)
		}
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
	for _, target := range []string{"run", "r", "c", "catalog", "info", "i", "repo", "fleetbench"} {
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
		{"run", "ct", "a b", "--beta"},
		{"r", "ct", "a b", "--beta"},
		{"run", "create-tc-task", "a b", "--beta"},
		{"run", "--offline", "create-tc-task", "a b", "--beta"},
		{"run", "-b", "replay-tc-pool", "a b", "--beta"},
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

func TestCatalogInfoShowsDataFiles(t *testing.T) {
	var out bytes.Buffer
	if err := catalogDisplay([]string{"info", "create-tc-task"}, &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "Data files: ~/.tc_token") {
		t.Fatalf("missing data file in catalog info: %s", out.String())
	}
}

func TestCatalogInfoShowsWhenToUse(t *testing.T) {
	c, err := catalog.Bundled()
	if err != nil {
		t.Fatal(err)
	}
	for _, tool := range c.Tools {
		var out bytes.Buffer
		if err := catalogDisplay([]string{"info", tool.ID}, &out); err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(out.String()), " ")
		if !strings.Contains(text, "WHEN TO USE IT "+tool.WhenToUse) {
			t.Errorf("missing catalog usage guidance for %s: %s", tool.ID, text)
		}
	}
}

func TestCatalogInfoShowsSharedSetupGuides(t *testing.T) {
	for _, name := range []string{"create-tc-task", "replay-tc-pool", "replay-tc-task", "quarantine-tool", "fleetroll"} {
		var out bytes.Buffer
		if err := catalogDisplay([]string{"info", name}, &out); err != nil {
			t.Fatal(err)
		}
		text := strings.Join(strings.Fields(out.String()), " ")
		if !strings.Contains(text, "Setup guide: Taskcluster credentials:") || !strings.Contains(text, "https://relops-launcher.quick.mozilla.cloud/requirements/taskcluster-credentials/") {
			t.Errorf("missing shared setup guide for %s: %s", name, text)
		}
	}
	var out bytes.Buffer
	if err := catalogDisplay([]string{"info", "pool-classifier"}, &out); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "Setup guide:") {
		t.Fatal("hosted dashboard should not acquire local Taskcluster setup requirements")
	}
}
