package sitebuild

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/mozilla-platform-ops/relops-launcher/internal/macosrelease"
)

func fixture(t *testing.T) (Config, func(string) (metadata, error)) {
	t.Helper()
	base := t.TempDir()
	cfg := Config{Source: filepath.Join(base, "source"), Binaries: filepath.Join(base, "dist"), Version: "0.1.0"}
	if err := os.MkdirAll(cfg.Binaries, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cfg.Source, 0755); err != nil {
		t.Fatal(err)
	}
	for _, item := range []string{"index.html", "css", "js", "tools.json"} {
		if err := copySource(filepath.Join("..", "..", "quick_site", item), filepath.Join(cfg.Source, item)); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(cfg.Source, "future.html"), []byte("concept only"), 0644); err != nil {
		t.Fatal(err)
	}
	for _, platform := range platforms {
		name := "relops-" + platform.os + "-" + platform.arch
		if err := os.WriteFile(filepath.Join(cfg.Binaries, name), []byte("artifact "+name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	inspect := func(path string) (metadata, error) {
		parts := strings.Split(filepath.Base(path), "-")
		return metadata{os: parts[1], arch: parts[2], commit: strings.Repeat("a", 40), goVersion: "go1.27.3"}, nil
	}
	return cfg, inspect
}

func TestBundleDownloadsInventoryAndLinks(t *testing.T) {
	cfg, inspect := fixture(t)
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(output, "releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var release Release
	if err := json.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	if release.Version != cfg.Version || release.SourceCommit != strings.Repeat("a", 40) || release.GoVersion != "go1.27.3" || release.BuildCommand != "just version=0.1.0 site" || len(release.Artifacts) != 2 {
		t.Fatalf("release metadata: %+v", release)
	}
	sums, err := os.ReadFile(filepath.Join(output, "downloads", "v0.1.0", "SHA256SUMS"))
	if err != nil {
		t.Fatal(err)
	}
	for _, artifact := range release.Artifacts {
		binary, err := os.ReadFile(filepath.Join(output, artifact.Path))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(binary)
		if artifact.SHA256 != hex.EncodeToString(digest[:]) || artifact.Size != len(binary) || !strings.Contains(string(sums), artifact.SHA256+"  "+filepath.Base(artifact.Path)+"\n") {
			t.Fatalf("incorrect downloadable artifact/checksum: %+v", artifact)
		}
	}
	page, err := os.ReadFile(filepath.Join(output, "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(page), "{{RELOPS_") || strings.Contains(string(page), "releases/latest/download") || !strings.Contains(string(page), "v0.1.0") {
		t.Fatal("unrendered or stale release information")
	}
	for _, command := range []string{
		"relops --owner aerickson --ref 022526-tool_tweaks android-tools/worker_health/replay_tc_pool.py -h",
		"relops fleetroll_mvp --help",
	} {
		if !strings.Contains(string(page), command) {
			t.Errorf("missing catalog command: %s", command)
		}
	}
	// Download/asset links must all resolve within the assembled site. Hash links
	// and external URLs/the inline SVG favicon do not refer to deployed files.
	for _, match := range regexp.MustCompile(`(?:href|src)="([^"]+)"`).FindAllStringSubmatch(string(page), -1) {
		path := match[1]
		if strings.HasPrefix(path, "#") || strings.HasPrefix(path, "data:") || strings.HasPrefix(path, "https://") || strings.HasPrefix(path, "http://") {
			continue
		}
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Errorf("broken packaged link %s: %v", path, err)
		}
	}
	var inventory struct{ Files []string }
	data, err = os.ReadFile(filepath.Join(output, "index.json"))
	if err != nil || json.Unmarshal(data, &inventory) != nil {
		t.Fatalf("inventory: %s %v", data, err)
	}
	for _, path := range inventory.Files {
		if _, err := os.Stat(filepath.Join(output, path)); err != nil {
			t.Errorf("inventory file %s: %v", path, err)
		}
	}
	if _, err := os.Stat(filepath.Join(output, "future.html")); !os.IsNotExist(err) {
		t.Fatal("concept page was included in deployment")
	}
	// Rebuilding unchanged inputs produces the same metadata and inventory, and
	// removes files left in the generated output from an earlier build.
	if err := os.WriteFile(filepath.Join(output, "stale-file"), nil, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := build(cfg, inspect); err != nil {
		t.Fatal(err)
	}
	after, err := os.ReadFile(filepath.Join(output, "index.json"))
	if err != nil || string(after) != string(data) {
		t.Fatalf("non-deterministic inventory: %v", err)
	}
	if _, err := os.Stat(filepath.Join(output, "stale-file")); !os.IsNotExist(err) {
		t.Fatal("stale generated file retained")
	}
	source, err := os.ReadFile(filepath.Join(cfg.Source, "index.html"))
	if err != nil || !strings.Contains(string(source), "{{RELOPS_VERSION}}") {
		t.Fatal("source template was modified")
	}
}

func TestBadArtifactsPreservePreviousSite(t *testing.T) {
	cfg, inspect := fixture(t)
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	previous, _ := os.ReadFile(filepath.Join(output, "releases.json"))
	for _, tc := range []struct {
		name   string
		mutate func(string, metadata) metadata
	}{
		{"wrong target", func(_ string, m metadata) metadata { m.os = "wrong"; return m }},
		{"mixed source", func(path string, m metadata) metadata {
			if strings.HasSuffix(path, "linux-amd64") {
				m.commit = "different"
			}
			return m
		}},
		{"mixed toolchain", func(path string, m metadata) metadata {
			if strings.HasSuffix(path, "linux-amd64") {
				m.goVersion = "other"
			}
			return m
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := build(cfg, func(path string) (metadata, error) { m, err := inspect(path); return tc.mutate(path, m), err })
			if err == nil {
				t.Fatal("accepted inconsistent artifacts")
			}
			after, _ := os.ReadFile(filepath.Join(output, "releases.json"))
			if string(after) != string(previous) {
				t.Fatal("failed build replaced the previous site")
			}
		})
	}
}

func TestRejectUnsafeInputs(t *testing.T) {
	cfg, inspect := fixture(t)
	cfg.Version = "../../escape"
	if _, err := build(cfg, inspect); err == nil {
		t.Fatal("accepted path components in release version")
	}
	cfg.Version = "0.1.0"
	if err := os.Symlink("/etc/passwd", filepath.Join(cfg.Source, "css", "outside.css")); err != nil {
		t.Fatal(err)
	}
	if _, err := build(cfg, inspect); err == nil {
		t.Fatal("accepted a symlink in deployable assets")
	}
	if _, err := inspectBinary(filepath.Join(cfg.Binaries, "relops-darwin-arm64")); err == nil {
		t.Fatal("accepted a non-Go binary as a launcher artifact")
	}
}

func TestSignedSiteRequiresApprovalForExactArtifacts(t *testing.T) {
	cfg, inspect := fixture(t)
	cfg.RequireNotarized = true
	if _, err := build(cfg, inspect); err == nil {
		t.Fatal("signed site accepted missing notarization receipt")
	}
	receipt := macosrelease.Receipt{SchemaVersion: 1, Version: cfg.Version, TeamID: "W5BHZ9R8A3", Status: "Accepted", SubmissionID: "12345678-1234-1234-1234-123456789abc", SHA256: make(map[string]string)}
	for _, name := range []string{"relops-darwin-arm64"} {
		data, err := os.ReadFile(filepath.Join(cfg.Binaries, name))
		if err != nil {
			t.Fatal(err)
		}
		digest := sha256.Sum256(data)
		receipt.SHA256[name] = hex.EncodeToString(digest[:])
	}
	data, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(cfg.Binaries, "macos-notarization.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	output, err := build(cfg, inspect)
	if err != nil {
		t.Fatal(err)
	}
	data, err = os.ReadFile(filepath.Join(output, "releases.json"))
	if err != nil {
		t.Fatal(err)
	}
	var release Release
	if err := json.Unmarshal(data, &release); err != nil {
		t.Fatal(err)
	}
	if release.MacOSNotarization == nil || release.MacOSNotarization.Status != "Accepted" || release.BuildCommand != "just version=0.1.0 signed-site" {
		t.Fatalf("missing signed release metadata: %+v", release)
	}
	if err := os.WriteFile(filepath.Join(cfg.Binaries, "relops-darwin-arm64"), []byte("rebuilt unsigned bytes"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := build(cfg, inspect); err == nil {
		t.Fatal("stale notarization receipt survived rebuilding the binary")
	}
}
