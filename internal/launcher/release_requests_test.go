package launcher

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func (f *releaseFixture) metadataFile() string {
	return filepath.Join(f.cache, "releases", defaultOwner, "fleetbench", runtime.GOOS+"-"+runtime.GOARCH, "latest.json")
}

func (f *releaseFixture) expireMetadata(t *testing.T) {
	t.Helper()
	data, err := os.ReadFile(f.metadataFile())
	if err != nil {
		t.Fatal(err)
	}
	var meta releaseMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	meta.CheckedAt = time.Now().Add(-releaseCheckInterval - time.Minute)
	data, _ = json.Marshal(meta)
	if err := os.WriteFile(f.metadataFile(), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func TestReleaseFreshnessAndETag(t *testing.T) {
	f := newReleaseFixture(t)
	f.etag = `"release-1"`
	if code, _, log := f.run("run", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	first, err := os.ReadFile(f.metadataFile())
	if err != nil {
		t.Fatal(err)
	}
	// A fresh invocation must not request even checksums, or extend freshness.
	f.status = 503
	if code, _, log := f.run("run", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	second, _ := os.ReadFile(f.metadataFile())
	if !bytes.Equal(first, second) {
		t.Fatal("cache hit extended the freshness interval")
	}
	for url, count := range f.requests {
		if count != 1 {
			t.Fatalf("extra request %s: %d", url, count)
		}
	}
	f.status = 0
	f.expireMetadata(t)
	// The fixture returns 304 only for the matching ETag. Altering the JSON
	// would otherwise be rejected, demonstrating reuse of validated metadata.
	f.draft = true
	if code, _, log := f.run("run", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	if code, _, log := f.run("run", "--refresh", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	api := "https://api.github.com/repos/" + defaultOwner + "/fleetbench/releases/latest"
	if f.requests[api] != 3 {
		t.Fatalf("refresh requests: %v", f.requests)
	}
	asset, _ := fleetbenchAsset(f.tag, runtime.GOOS, runtime.GOARCH)
	if f.requests[releaseAssetURL(options{owner: defaultOwner, repo: "fleetbench"}, f.tag, asset)] != 1 {
		t.Fatal("304 redownloaded binary")
	}
}

func TestReleaseCooldownAcrossLaunches(t *testing.T) {
	f := newReleaseFixture(t)
	if code, _, log := f.run("run", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	f.expireMetadata(t)
	before, _ := os.ReadFile(f.metadataFile())
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	requests := 0
	a.httpClient.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		return &http.Response{StatusCode: 403, Header: http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(time.Now().Add(time.Hour).Unix())}}, Body: io.NopCloser(strings.NewReader(`{"message":"API rate limit exceeded"}`))}, nil
	})
	_, _, err := a.prepareRelease(context.Background(), options{owner: defaultOwner, repo: "fleetbench"})
	if err == nil || !strings.Contains(err.Error(), "retry after") || !strings.Contains(err.Error(), "--offline") {
		t.Fatalf("rate error: %v", err)
	}
	after, _ := os.ReadFile(f.metadataFile())
	if !bytes.Equal(before, after) {
		t.Fatal("failed refresh replaced last successful resolution")
	}
	if requests != 1 {
		t.Fatalf("initial request count: %d", requests)
	}
	// f.run constructs a new app each time. Cooldown also covers --refresh and
	// another owner; any network access would fail through the fixture.
	for _, args := range [][]string{{"fleetbench"}, {"--refresh", "fleetbench"}} {
		if code, output, log := f.run(append([]string{"run"}, args...)...); code != 1 || output != "" || !strings.Contains(log, "requests paused until") {
			t.Fatalf("cooldown %v: %d %q %s", args, code, output, log)
		}
	}
	var ownerOut, ownerLog bytes.Buffer
	_, _, ownerErr := f.app(&ownerOut, &ownerLog).prepareRelease(context.Background(), options{owner: "another-owner", repo: "fleetbench"})
	if ownerErr == nil || !strings.Contains(ownerErr.Error(), "requests paused until") {
		t.Fatalf("cross-owner cooldown: %v", ownerErr)
	}
	if code, _, log := f.run("run", "--offline", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	// Expired cooldown allows another request.
	data, _ := json.Marshal(releaseCooldown{Until: time.Now().Add(-time.Minute)})
	if err := os.WriteFile(filepath.Join(f.cache, "releases", "cooldown.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	if code, _, log := f.run("run", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
}

func TestReleaseRateLimitResponses(t *testing.T) {
	now := time.Unix(1800000000, 0).UTC()
	for _, tc := range []struct {
		name    string
		status  int
		headers http.Header
		message string
		wait    time.Duration
	}{
		{"primary", 403, http.Header{"X-Ratelimit-Remaining": {"0"}, "X-Ratelimit-Reset": {fmt.Sprint(now.Add(time.Hour).Unix())}}, "quota exhausted", time.Hour},
		{"retry seconds", 429, http.Header{"Retry-After": {"120"}}, "slow down", 2 * time.Minute},
		{"retry date", 403, http.Header{"Retry-After": {now.Add(3 * time.Minute).Format(http.TimeFormat)}}, "secondary rate limit", 3 * time.Minute},
		{"secondary fallback", 403, nil, "secondary rate limit", time.Minute},
		{"429 fallback", 429, nil, "", time.Minute},
		{"permission", 403, nil, "access forbidden", 0},
		{"server", 503, nil, "temporarily unavailable", 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body, _ := json.Marshal(map[string]string{"message": tc.message})
			err, until := releaseResponseError("https://api.github.com/test", &http.Response{StatusCode: tc.status, Header: tc.headers, Body: io.NopCloser(bytes.NewReader(body))}, now)
			if err == nil || (tc.message != "" && !strings.Contains(err.Error(), tc.message)) {
				t.Fatalf("diagnostic: %v", err)
			}
			if tc.wait == 0 {
				if !until.IsZero() {
					t.Fatalf("unexpected cooldown: %v", until)
				}
			} else if !until.Equal(now.Add(tc.wait)) {
				t.Fatalf("cooldown: %v want %v", until, now.Add(tc.wait))
			}
		})
	}
}

func TestReleaseAuthentication(t *testing.T) {
	for _, tc := range []struct{ name, gh, github, want string }{{"none", "", "", ""}, {"github", "", "github-token", "Bearer github-token"}, {"precedence", "gh-token", "github-token", "Bearer gh-token"}} {
		t.Run(tc.name, func(t *testing.T) {
			f := newReleaseFixture(t)
			t.Setenv("GH_TOKEN", tc.gh)
			t.Setenv("GITHUB_TOKEN", tc.github)
			var out, log bytes.Buffer
			a := f.app(&out, &log)
			a.httpClient.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
				want := ""
				if r.URL.Host == "api.github.com" {
					want = tc.want
				}
				if r.Header.Get("Authorization") != want {
					t.Error("unexpected authorization header")
				}
				return f.transport(r)
			})
			if _, _, err := a.prepareRelease(context.Background(), options{owner: defaultOwner, repo: "fleetbench"}); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReleaseRefreshCLI(t *testing.T) {
	for _, args := range [][]string{{"run", "--refresh", "--offline", "fleetbench"}, {"repo", "--refresh", "example/tool.sh"}, {"repo", "--refresh", "fleetbench"}} {
		if _, _, err := parse(args, &bytes.Buffer{}, "test"); err == nil {
			t.Fatalf("accepted incompatible refresh: %v", args)
		}
	}
	o, _, err := parse([]string{"run", "fleetbench", "--refresh"}, &bytes.Buffer{}, "test")
	if err != nil || o.refresh || len(o.args) != 1 || o.args[0] != "--refresh" {
		t.Fatalf("tool flag boundary: %+v %v", o, err)
	}
}

func TestReleaseReplacedAssetOn304(t *testing.T) {
	f := newReleaseFixture(t)
	f.etag = `"unchanged"`
	if code, _, log := f.run("run", "fleetbench"); code != 0 {
		t.Fatal(log)
	}
	f.binary = "#!/bin/sh\nprintf 'replacement\\n'\n"
	if code, out, log := f.run("run", "--refresh", "fleetbench"); code != 0 || out != "replacement\n" {
		t.Fatalf("replaced asset: %d %q %s", code, out, log)
	}
}

func TestReleaseLegacyAndFutureCacheRefresh(t *testing.T) {
	for _, name := range []string{"legacy", "future"} {
		t.Run(name, func(t *testing.T) {
			f := newReleaseFixture(t)
			if code, _, log := f.run("run", "fleetbench"); code != 0 {
				t.Fatal(log)
			}
			data, err := os.ReadFile(f.metadataFile())
			if err != nil {
				t.Fatal(err)
			}
			var meta releaseMetadata
			if err := json.Unmarshal(data, &meta); err != nil {
				t.Fatal(err)
			}
			meta.CheckedAt = time.Time{}
			if name == "future" {
				meta.CheckedAt = time.Now().Add(time.Hour)
			}
			data, _ = json.Marshal(meta)
			if err := os.WriteFile(f.metadataFile(), data, 0600); err != nil {
				t.Fatal(err)
			}
			f.tag = "v0.4.7"
			if code, _, log := f.run("run", "fleetbench"); code != 0 || !strings.Contains(log, "· v0.4.7") {
				t.Fatalf("refresh: %d %s", code, log)
			}
		})
	}
}

func TestReleaseTokenRemovedOnRedirect(t *testing.T) {
	f := newReleaseFixture(t)
	t.Setenv("GH_TOKEN", "test-token")
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	a.httpClient = releaseHTTPClient()
	requests := 0
	a.httpClient.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		requests++
		if requests == 1 {
			if r.Header.Get("Authorization") != "Bearer test-token" {
				t.Error("missing API authentication")
			}
			// Go normally forwards credentials to subdomains. Our redirect
			// policy must strip them even in this case.
			return &http.Response{StatusCode: 302, Header: http.Header{"Location": {"https://downloads.api.github.com/asset"}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		}
		if r.Header.Get("Authorization") != "" {
			t.Error("token leaked to redirect host")
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("ok"))}, nil
	})
	data, err := a.releaseBytes(context.Background(), "https://api.github.com/test", 1024)
	if err != nil || string(data) != "ok" || requests != 2 {
		t.Fatalf("redirect: %q %v requests=%d", data, err, requests)
	}
}
