package sitebuild

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

func TestSharedSetupGuideLinksAndBacklinks(t *testing.T) {
	cfg, inspect := fixture(t)
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	page, err := os.ReadFile(filepath.Join(output, "requirements", "taskcluster-credentials", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"The token needs Taskcluster scopes", "clientId", "accessToken", "chmod 600 ~/.tc_token"} {
		if !bytes.Contains(page, []byte(expected)) {
			t.Errorf("setup guide missing %q", expected)
		}
	}
	for _, id := range []string{"create-tc-task", "replay-tc-pool", "replay-tc-task", "quarantine-tool", "fleetroll"} {
		if !bytes.Contains(page, []byte(`href="../../tools/`+id+`/"`)) {
			t.Errorf("missing guide backlink to %s", id)
		}
		toolPage, _ := os.ReadFile(filepath.Join(output, "tools", id, "index.html"))
		if !bytes.Contains(toolPage, []byte(`href="../../requirements/taskcluster-credentials/"`)) {
			t.Errorf("missing setup reference on %s", id)
		}
		if !bytes.Contains(toolPage, []byte(`href="../../requirements/taskcluster-credentials/"><code>~/.tc_token</code></a>`)) {
			t.Errorf("credential filename should link to setup on %s", id)
		}
		if bytes.Contains(toolPage, []byte("Taskcluster credentials setup guide →")) || bytes.Contains(toolPage, []byte("See the setup guide for when each file is used.")) {
			t.Errorf("redundant setup text on %s", id)
		}
	}
	for _, id := range []string{"pool-classifier", "hangar", "fleetbench", "taskcluster-web-launcher"} {
		if bytes.Contains(page, []byte(`href="../../tools/`+id+`/"`)) {
			t.Errorf("guide incorrectly includes %s", id)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "requirements", "taskcluster-credentials.md")); !os.IsNotExist(err) {
		t.Fatal("Markdown setup source must not be packaged")
	}
}

func TestSharedSetupGuideDeduplicatesChannels(t *testing.T) {
	cfg, _ := fixture(t)
	data, _ := os.ReadFile(filepath.Join(cfg.Source, "tools.json"))
	c, err := catalog.Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	for i := range c.Tools {
		if c.Tools[i].ID == "fleetroll" {
			beta := *c.Tools[i].Default
			c.Tools[i].Beta = &beta
		}
	}
	data, _ = json.Marshal(c)
	if err := os.WriteFile(filepath.Join(cfg.Source, "tools.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	stage := t.TempDir()
	if err := buildSetupPages(cfg.Source, stage, "1.0.0", "abcdef0"); err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(stage, "requirements", "taskcluster-credentials", "index.html"))
	if strings.Count(string(page), `href="../../tools/fleetroll/"`) != 1 {
		t.Fatal("tool with default and beta must have one guide backlink")
	}
}

func TestMissingSharedSetupGuidePreservesPreviousSite(t *testing.T) {
	cfg, inspect := fixture(t)
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := os.ReadFile(filepath.Join(output, "requirements", "taskcluster-credentials", "index.html"))
	if err := os.Remove(filepath.Join(cfg.Source, "requirements", "taskcluster-credentials.md")); err != nil {
		t.Fatal(err)
	}
	if _, err := build(cfg, inspect); err == nil || !strings.Contains(err.Error(), "setup guide taskcluster-credentials") {
		t.Fatalf("missing guide should fail the build: %v", err)
	}
	after, err := os.ReadFile(filepath.Join(output, "requirements", "taskcluster-credentials", "index.html"))
	if err != nil || !bytes.Equal(previous, after) {
		t.Fatal("missing guide replaced previous output")
	}
}
