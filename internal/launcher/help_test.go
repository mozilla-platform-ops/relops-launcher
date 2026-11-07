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
		{[]string{"c", "--help"}, "CATALOG SELECTION", "--owner OWNER"},
		{[]string{"catalog", "launch", "--help"}, "RELEASE UPDATES", "--owner OWNER"},
		{[]string{"repo", "--help"}, "Temporary checkouts are removed on exit.", "ENVIRONMENTS"},
		{[]string{"repo"}, "QUICK START", "\nEXECUTION\n"},
		{[]string{"catalog", "--help"}, "relops catalog info NAME", "SOURCE EXECUTION"},
		{[]string{"catalog", "list", "--help"}, "beta-only tools require -b", "SOURCE EXECUTION"},
		{[]string{"catalog", "info", "--help"}, "Example: relops catalog info fr", "SOURCE EXECUTION"},
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
	for _, args := range [][]string{{"c", "--offline"}, {"c", "-b"}, {"catalog", "launch", "--ref", "main"}, {"repo", "--offline"}, {"repo", "--ref", "main"}, {"--offline"}, {"--ref", "main"}} {
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
		if strings.Contains(text, `=""`) || strings.Contains(text, "\x1b[") || !strings.Contains(text, "Launch: relops c ") {
			t.Fatal(text)
		}
		if name == "replay-tc-pool" && !strings.Contains(text, "Launch: relops c -b replay-tc-pool") {
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
	if !strings.Contains(out.String(), "  relops catalog list\n    Find a tool.") {
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
