package launcher

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestFocusedHelp(t *testing.T) {
	for _, tc := range []struct {
		args         []string
		want, absent string
	}{
		{nil, "QUICK START", "ENVIRONMENTS"},
		{[]string{"--help"}, "QUICK START", "ENVIRONMENTS"},
		{[]string{"c", "--help"}, "Read tool descriptions (alias: c).", "SOURCE EXECUTION"},
		{[]string{"run", "--help"}, "relops r [options]", "--owner OWNER"},
		{[]string{"r", "--help"}, "relops run [options]", "--owner OWNER"},
		{[]string{"catalog", "launch", "--help"}, "RELEASE UPDATES", "--owner OWNER"},
		{[]string{"repo", "--help"}, "Temporary checkouts are removed on exit.", "ENVIRONMENTS"},
		{[]string{"repo"}, "QUICK START", "\nEXECUTION\n"},
		{[]string{"catalog", "--help"}, "relops info NAME", "SOURCE EXECUTION"},
		{[]string{"info", "--help"}, "Example: relops info fr", "SOURCE EXECUTION"},
		{[]string{"catalog", "list", "--help"}, "beta-only tools require -b", "SOURCE EXECUTION"},
		{[]string{"catalog", "info", "--help"}, "Example: relops info fr", "SOURCE EXECUTION"},
	} {
		var out bytes.Buffer
		_, done, err := parse(tc.args, &out, "test")
		text := out.String()
		if err != nil || !done || !strings.Contains(text, tc.want) || strings.Contains(text, tc.absent) || strings.Contains(text, "\x1b[") || strings.Contains(text, wordmark) {
			t.Fatalf("help %v: done=%v err=%v output=%q", tc.args, done, err, text)
		}
	}
}

func TestMissingExecutionTarget(t *testing.T) {
	for _, args := range [][]string{{"run", "--offline"}, {"r", "-b"}, {"c", "--offline"}, {"c", "-b"}, {"catalog", "launch", "--ref", "main"}, {"repo", "--offline"}, {"repo", "--ref", "main"}, {"--offline"}, {"--ref", "main"}} {
		var out bytes.Buffer
		_, done, err := parse(args, &out, "test")
		if err == nil || done || out.Len() != 0 {
			t.Fatalf("missing target %v: done=%v err=%v output=%q", args, done, err, out.String())
		}
	}
}

func TestCatalogReadableDetails(t *testing.T) {
	for _, name := range []string{"fr", "fleetbench", "replay-tc-pool"} {
		var out bytes.Buffer
		if err := catalogDisplay([]string{"info", name}, &out); err != nil {
			t.Fatal(err)
		}
		text := out.String()
		if strings.Contains(text, `=""`) || strings.Contains(text, "\x1b[") || !strings.Contains(text, "Launch: relops run ") {
			t.Fatal(text)
		}
		if name == "replay-tc-pool" && !strings.Contains(text, "Launch: relops run -b replay-tc-pool") {
			t.Fatal(text)
		}
		if name == "fleetbench" && (strings.Contains(text, "Runner:") || !strings.Contains(text, "latest stable release")) {
			t.Fatal(text)
		}
	}
}

func TestNarrowHelp(t *testing.T) {
	t.Setenv("COLUMNS", "50")
	var out bytes.Buffer
	if err := writeHelp(&out, "root", "test"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "  relops run\n    Browse tools and launch commands.") {
		t.Fatal(out.String())
	}
}

func TestPaletteAndPlainErrors(t *testing.T) {
	if got := (palette{true}).text(styleBeta, "beta"); got != "\x1b[33mbeta\x1b[0m" {
		t.Fatal(got)
	}
	var out bytes.Buffer
	writeError(&out, errors.New("unknown target; use relops catalog list"))
	if out.String() != "relops: error: unknown target; use relops catalog list\n" {
		t.Fatal(out.String())
	}
	for _, env := range []string{"NO_COLOR", "TERM"} {
		value := "1"
		if env == "TERM" {
			value = "dumb"
		}
		t.Setenv(env, value)
		if terminalPalette(&out).enabled {
			t.Fatal("unexpected styling")
		}
	}
}

func TestBetaOnlyLaunchGuidance(t *testing.T) {
	var out bytes.Buffer
	_, done, err := parse([]string{"r", "tcwl"}, &out, "test")
	if err == nil || done {
		t.Fatalf("parse: done=%v err=%v", done, err)
	}
	writeError(&out, err)
	want := "relops: taskcluster-web-launcher is only available in beta.\n\n  Try: relops r -b tcwl\n"
	if out.String() != want {
		t.Fatalf("output = %q; want %q", out.String(), want)
	}
}

func TestRedirectedFilesArePlain(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) {
		t.Fatal("pipe detected as terminal")
	}
	file, err := os.CreateTemp(t.TempDir(), "help")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if isTerminal(file) {
		t.Fatal("file detected as terminal")
	}
	if err := writeHelp(file, "root", "test"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(file.Name())
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(wordmark)) || bytes.Contains(data, []byte("\x1b[")) {
		t.Fatal(string(data))
	}
}
