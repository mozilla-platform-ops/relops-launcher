package sitebuild

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveCatalogRef(t *testing.T) {
	branch := strings.Repeat("a", 40)
	tag := strings.Repeat("b", 40)
	refs := map[string]string{
		"HEAD": branch, "refs/heads/main": branch,
		"refs/tags/v1": tag, "refs/tags/v1^{}": branch,
		"refs/heads/shared": branch, "refs/tags/shared": tag,
	}
	for _, tc := range []struct{ ref, want, failure string }{
		{"", branch, ""},
		{"main", branch, ""},
		{"refs/heads/main", branch, ""},
		{"v1", branch, ""},
		{"refs/tags/v1", branch, ""},
		{tag, tag, ""},
		{"shared", "", "ambiguous"},
		{"missing", "", "not found"},
		{"origin/main", "", "not found"},
		{"refs/pull/1/head", "", "only refs/heads/"},
	} {
		t.Run(tc.ref, func(t *testing.T) {
			got, err := resolveCatalogRef(tc.ref, refs)
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) {
					t.Fatalf("expected %q, got %v", tc.failure, err)
				}
			} else if err != nil || got != tc.want {
				t.Fatalf("got %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

func TestCheckToolsLocalRemote(t *testing.T) {
	remote := t.TempDir()
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", remote}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@example.com", "GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@example.com")
		data, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, data)
		}
		return strings.TrimSpace(string(data))
	}
	git("init", "-b", "main")
	for _, file := range []string{"tool.sh", "pyproject.toml", "uv.lock"} {
		if err := os.WriteFile(filepath.Join(remote, file), []byte("not executed\n"), 0644); err != nil {
			t.Fatal(err)
		}
	}
	git("add", ".")
	git("-c", "core.hooksPath=/dev/null", "-c", "commit.gpgsign=false", "commit", "-m", "fixture")
	sha := git("rev-parse", "HEAD")
	git("-c", "tag.gpgSign=false", "tag", "-a", "v1", "-m", "fixture")
	for _, tc := range []struct{ name, ref, path, failure string }{
		{"default", "", "tool.sh", ""},
		{"branch", "main", "tool.sh", ""},
		{"tag", "v1", "tool.sh", ""},
		{"commit", sha, "tool.sh", ""},
		{"project", "", "", ""},
		{"missing-ref", "absent", "tool.sh", "not found"},
		{"missing-script", "main", "absent.sh", "target does not exist"},
		{"missing-repo", "", "tool.sh", "git ls-remote"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			entry := catalog.Tool{ID: "example", Name: "Example", Description: "Example tool", Default: &catalog.Target{Kind: "source", Owner: "mozilla-platform-ops", Repo: "repo", Ref: tc.ref, Path: tc.path}}
			data, err := json.Marshal(catalog.Catalog{SchemaVersion: 1, Revision: "test", Tools: []catalog.Tool{entry}})
			if err != nil {
				t.Fatal(err)
			}
			catalogPath := filepath.Join(t.TempDir(), "tools.json")
			if err := os.WriteFile(catalogPath, data, 0644); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			err = checkTools(context.Background(), catalogPath, &out, func(entry tool) string {
				if entry.Owner != "mozilla-platform-ops" {
					t.Errorf("unexpected default owner: %s", entry.Owner)
				}
				if tc.name == "missing-repo" {
					return filepath.Join(remote, "missing.git")
				}
				return remote
			})
			if tc.failure != "" {
				if err == nil || !strings.Contains(err.Error(), tc.failure) || !strings.Contains(out.String(), "FAIL example-default: mozilla-platform-ops/repo") {
					t.Fatalf("expected %q: %v; output %s", tc.failure, err, out.String())
				}
			} else if err != nil || !strings.Contains(out.String(), "commit="+sha) {
				t.Fatalf("check: %v; output %s", err, out.String())
			}
		})
	}
}
