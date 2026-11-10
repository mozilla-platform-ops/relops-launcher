package sitebuild

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"html"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

func TestToolPagesForEntireCatalog(t *testing.T) {
	cfg, inspect := fixture(t)
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := readTools(filepath.Join(cfg.Source, "tools.json"))
	if err != nil {
		t.Fatal(err)
	}
	home, _ := os.ReadFile(filepath.Join(output, "index.html"))
	if bytes.Contains(home, []byte("docs ↗")) {
		t.Fatal("homepage should link tool pages rather than repeat upstream docs links")
	}
	for _, entry := range entries {
		page, err := os.ReadFile(filepath.Join(output, "tools", entry.Shortcut, "index.html"))
		if err != nil {
			t.Fatal(err)
		}
		for _, value := range []string{entry.Name, entry.Description, entry.WhenToUse, entry.Command(), entry.Prerequisites, entry.Documentation, entry.SourceURL()} {
			if !strings.Contains(html.UnescapeString(string(page)), value) {
				t.Errorf("%s page missing catalog value %q", entry.Shortcut, value)
			}
		}
		if strings.Count(string(page), `id="when-to-use-heading"`) != 1 || strings.Contains(string(page), `<h2 id="when-to-use-it">`) {
			t.Errorf("%s needs exactly one catalog-generated usage section", entry.Shortcut)
		}
		if !strings.Contains(string(home), `href="tools/`+entry.Shortcut+`/"`) {
			t.Errorf("homepage does not link %s", entry.Shortcut)
		}
	}
	for _, id := range []string{"hangar", "pool-classifier", "create-tc-task", "replay-tc-pool"} {
		page, _ := os.ReadFile(filepath.Join(output, "tools", id, "index.html"))
		if !bytes.Contains(page, []byte(`class="tool-guide"`)) || !bytes.Contains(page, []byte("<pre><code")) || bytes.Contains(page, []byte("{{launch}}")) {
			t.Errorf("missing rendered walkthrough/example for %s", id)
		}
	}
	fallback, _ := os.ReadFile(filepath.Join(output, "tools", "replay-tc-task", "index.html"))
	if bytes.Contains(fallback, []byte(`id="tool-guide"`)) || !bytes.Contains(fallback, []byte(`id="tool-requirements"`)) {
		t.Fatal("metadata-only page should show reference without an empty guide")
	}
	if _, err := os.Stat(filepath.Join(output, "tool-guides")); !os.IsNotExist(err) {
		t.Fatal("Markdown source was packaged")
	}
}

func TestToolSourceURL(t *testing.T) {
	for _, tc := range []struct {
		name, kind, ref, path, want string
	}{
		{"default script", "source", "", "worker_health/create_tc_task.py", "https://github.com/org/repo/tree/HEAD/worker_health/create_tc_task.py"},
		{"project root", "source", "", "", "https://github.com/org/repo/tree/HEAD"},
		{"project directory", "source", "release", "python/project", "https://github.com/org/repo/tree/release/python/project"},
		{"branch", "source", "refs/heads/feature/new", "a b.py", "https://github.com/org/repo/tree/feature%2Fnew/a%20b.py"},
		{"tag", "source", "refs/tags/v1", "tool.py", "https://github.com/org/repo/tree/v1/tool.py"},
		{"web", "web", "", "", ""},
		{"release", "release", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := tool{Kind: tc.kind, Owner: "org", Repo: "repo", Ref: tc.ref, Path: tc.path}
			if got := entry.SourceURL(); got != tc.want {
				t.Fatalf("source link: got %s, want %s", got, tc.want)
			}
		})
	}
}

func TestToolPageKeepsIndependentChannelsTogether(t *testing.T) {
	source, output := t.TempDir(), t.TempDir()
	if err := copySource(filepath.Join("..", "..", "quick_site", "tool.html"), filepath.Join(source, "tool.html")); err != nil {
		t.Fatal(err)
	}
	c := catalog.Catalog{SchemaVersion: 1, Revision: "channels", Tools: []catalog.Tool{{
		ID: "duet", Name: "Duet <two>", Description: "Two & separate targets", Aliases: []string{"du"},
		WhenToUse: "Compare <targets> & inspect behavior.",
		Default:   &catalog.Target{Kind: "source", Owner: "org", Repo: "stable", Ref: "v1", Command: "start", Cwd: "repo", Prerequisites: "Stable setup", Args: []string{"a b"}},
		Beta:      &catalog.Target{Kind: "source", Owner: "fork", Repo: "experiment", Ref: "next", Path: "tool.py", Runner: "python3", Prerequisites: "Beta setup", Args: []string{"--help"}},
	}}}
	data, _ := json.Marshal(c)
	if err := os.WriteFile(filepath.Join(source, "tools.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(source, "tool-guides"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "tool-guides", "duet.md"), []byte("## Try it\n\n```sh\n{{launch}} --help\n```\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := buildToolPages(source, output, "1.0.0", "abcdef0"); err != nil {
		t.Fatal(err)
	}
	data, _ = os.ReadFile(filepath.Join(output, "tools", "duet", "index.html"))
	page := html.UnescapeString(string(data))
	for _, expected := range []string{"relops run duet 'a b'", "relops run -b duet --help", "org/stable", "fork/experiment", "Stable setup", "Beta setup", "start", "python3", "Catalog channels", "relops run duet --help"} {
		if !strings.Contains(page, expected) {
			t.Errorf("missing channel detail %q", expected)
		}
	}
	if bytes.Contains(data, []byte("Duet <two>")) || !bytes.Contains(data, []byte("Duet &lt;two&gt;")) {
		t.Fatal("catalog prose was not escaped")
	}
	if !bytes.Contains(data, []byte("Compare &lt;targets&gt; &amp; inspect behavior.")) || bytes.Contains(data, []byte("Compare <targets>")) {
		t.Fatal("usage guidance was missing or not escaped")
	}
	pages, _ := os.ReadDir(filepath.Join(output, "tools"))
	if len(pages) != 1 {
		t.Fatal("channels must share a page")
	}
}

func TestGuideImagesPackagedAndLinked(t *testing.T) {
	cfg, inspect := fixture(t)
	image, _ := base64.StdEncoding.DecodeString("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAwMCAO+jC1sAAAAASUVORK5CYII=")
	if err := os.MkdirAll(filepath.Join(cfg.Source, "tool-guides", "images", "fleetroll"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Source, "tool-guides", "images", "fleetroll", "monitor.png"), image, 0644); err != nil {
		t.Fatal(err)
	}
	guide := "## Monitor\n\n![Worker monitor](images/fleetroll/monitor.png)\n\n*Monitor example.*\n\n[Run it](#tool-run)\n"
	if err := os.WriteFile(filepath.Join(cfg.Source, "tool-guides", "fleetroll.md"), []byte(guide), 0644); err != nil {
		t.Fatal(err)
	}
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	page, _ := os.ReadFile(filepath.Join(output, "tools", "fleetroll", "index.html"))
	if !bytes.Contains(page, []byte(`src="../../tool-guide-images/fleetroll/monitor.png" alt="Worker monitor"`)) {
		t.Fatal("image URL or alt text not rendered")
	}
	if !bytes.Contains(page, []byte(`<a href="../../tool-guide-images/fleetroll/monitor.png"><img`)) {
		t.Fatal("screenshot should link to its full-size file automatically")
	}
	packaged, _ := os.ReadFile(filepath.Join(output, "tool-guide-images", "fleetroll", "monitor.png"))
	if !bytes.Equal(packaged, image) {
		t.Fatal("image bytes changed or missing")
	}
}

func TestGuideScreenshotKeepsExplicitLink(t *testing.T) {
	guide := []byte(`[![Dashboard](images/fleetroll/monitor.png)](https://example.com/dashboard)`)
	page, err := renderGuide(guide, tool{Shortcut: "fleetroll"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(page), `<a href="https://example.com/dashboard"><img src="../../tool-guide-images/fleetroll/monitor.png"`) || strings.Count(string(page), "<a ") != 1 {
		t.Fatalf("explicit image link changed or nested: %s", page)
	}
}

func TestInvalidGuidesPreservePreviousBundle(t *testing.T) {
	for _, tc := range []struct {
		name, file, content, diagnostic string
	}{
		{"orphan", "unknown.md", "## No such tool\n", "no catalog tool"},
		{"missing-image", "fleetroll.md", "![Missing](images/fleetroll/no.png)\n", "broken link"},
		{"missing-page", "fleetroll.md", "[Missing](../not-a-tool/)\n", "broken link"},
		{"missing-anchor", "fleetroll.md", "[Missing](#absent)\n", "missing anchor"},
		{"cross-page-anchor", "fleetroll.md", "[Missing](../pool-classifier/#absent)\n", "missing anchor"},
		{"escape", "fleetroll.md", "[Outside](../../../../outside.txt)\n", "leaves site"},
		{"raw-html", "fleetroll.md", "<script>alert(1)</script>\n", "rather than raw HTML"},
		{"title", "fleetroll.md", "# Second title\n", "level-2"},
		{"duplicate-anchor", "fleetroll.md", "## tool-run\n", "duplicate anchor"},
		{"wrong-image-directory", "fleetroll.md", "![Wrong](images/other-tool/no.png)\n", "images/fleetroll/"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cfg, inspect := fixture(t)
			output, err := build(cfg, inspect)
			if err != nil {
				t.Fatal(err)
			}
			previous, _ := os.ReadFile(filepath.Join(output, "index.json"))
			if err := os.WriteFile(filepath.Join(output, "retained.txt"), []byte("keep previous site"), 0644); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(cfg.Source, "tool-guides", tc.file), []byte(tc.content), 0644); err != nil {
				t.Fatal(err)
			}
			if _, err := build(cfg, inspect); err == nil || !strings.Contains(err.Error(), tc.diagnostic) {
				t.Fatalf("expected %s; got %v", tc.diagnostic, err)
			}
			after, _ := os.ReadFile(filepath.Join(output, "index.json"))
			if !bytes.Equal(previous, after) {
				t.Fatal("failed guide build replaced previous bundle")
			}
			if _, err := os.Stat(filepath.Join(output, "retained.txt")); err != nil {
				t.Fatal("failed guide build removed previous site files")
			}
		})
	}
}
