package launcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

type releaseTransport func(*http.Request) (*http.Response, error)

func (f releaseTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

type releaseFixture struct {
	t          *testing.T
	mu         sync.Mutex
	cache      string
	caller     string
	tag        string
	binary     string
	badBinary  bool
	draft      bool
	prerelease bool
	missing    string
	sums       *string
	status     int
	etag       string
	requests   map[string]int
}

func newReleaseFixture(t *testing.T) *releaseFixture {
	t.Helper()
	t.Setenv("GH_TOKEN", "")
	t.Setenv("GITHUB_TOKEN", "")
	if _, err := fleetbenchAsset("", runtime.GOOS, runtime.GOARCH); err != nil {
		t.Skip(err)
	}
	base := t.TempDir()
	f := &releaseFixture{t: t, cache: filepath.Join(base, "cache"), caller: base, tag: "v0.4.6",
		binary: "#!/bin/sh\nprintf 'released\\n'\n", requests: make(map[string]int)}
	return f
}

func (f *releaseFixture) transport(r *http.Request) (*http.Response, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests[r.URL.String()]++
	status := f.status
	if status == 0 {
		status = 200
	}
	asset, _ := fleetbenchAsset(f.tag, runtime.GOOS, runtime.GOARCH)
	var body string
	headers := make(http.Header)
	switch r.URL.String() {
	case "https://api.github.com/repos/" + defaultOwner + "/fleetbench/releases/latest":
		if f.etag != "" {
			headers.Set("ETag", f.etag)
			if r.Header.Get("If-None-Match") == f.etag && status == 200 {
				status = http.StatusNotModified
			}
		}
		if r.Header.Get("Accept") != "application/vnd.github+json" || r.Header.Get("User-Agent") == "" {
			f.t.Error("missing GitHub request headers")
		}
		release := githubRelease{Tag: f.tag, Draft: f.draft, Prerelease: f.prerelease}
		for _, name := range []string{asset, "SHA256SUMS"} {
			if f.missing != name {
				release.Assets = append(release.Assets, struct {
					Name string `json:"name"`
				}{name})
			}
		}
		data, _ := json.Marshal(release)
		body = string(data)
	case releaseAssetURL(options{owner: defaultOwner, repo: "fleetbench"}, f.tag, "SHA256SUMS"):
		body = fmt.Sprintf("%x  %s\n", sha256.Sum256([]byte(f.binary)), asset)
		if f.sums != nil {
			body = *f.sums
		}
	case releaseAssetURL(options{owner: defaultOwner, repo: "fleetbench"}, f.tag, asset):
		body = f.binary
		if f.badBinary {
			body += "corruption"
		}
	default:
		f.t.Errorf("unexpected request: %s", r.URL)
		status = 404
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(strings.NewReader(body)), Header: headers}, nil
}

func (f *releaseFixture) app(out, log *bytes.Buffer) *app {
	return &app{cache: f.cache, caller: f.caller, in: strings.NewReader(""), out: out, errOut: log,
		lockTimeout: 5 * time.Second, httpClient: &http.Client{Transport: releaseTransport(f.transport)}}
}

func (f *releaseFixture) run(args ...string) (int, string, string) {
	f.t.Helper()
	var out, log bytes.Buffer
	o, _, err := parse(args, &out, "test")
	if err != nil {
		f.t.Fatal(err)
	}
	code, err := f.app(&out, &log).run(context.Background(), o)
	if err != nil {
		fmt.Fprintln(&log, err)
	}
	return code, out.String(), log.String()
}

func TestReleaseCLI(t *testing.T) {
	for _, flags := range [][]string{
		{"--ref", "feature"}, {"--ref="}, {"--runner", "auto"}, {"--command", "fleetbench"}, {"--cwd", "repo"},
	} {
		if _, _, err := parse(append(flags, "fleetbench"), &bytes.Buffer{}, "test"); err == nil {
			t.Errorf("accepted incompatible release flags: %v", flags)
		}
	}
	o, _, err := parse([]string{"--cwd", "caller", "fleetbench", "cpu", "--ref", "passed-through"}, &bytes.Buffer{}, "test")
	if err != nil || !isReleaseTarget(o) || strings.Join(o.args, "|") != "cpu|--ref|passed-through" {
		t.Fatalf("release argument boundary: %+v %v", o, err)
	}
	o, _, err = parse([]string{"--ref", "feature", "fleetbench/runner"}, &bytes.Buffer{}, "test")
	if err != nil || isReleaseTarget(o) {
		t.Fatalf("source target changed: %+v %v", o, err)
	}
}

func TestReleasePlatformMapping(t *testing.T) {
	for _, tc := range []struct{ os, arch, want string }{
		{"darwin", "arm64", "fleetbench-v1.2.3-macos-aarch64"},
		{"linux", "amd64", "fleetbench-v1.2.3-linux-x86_64"},
		{"darwin", "amd64", ""}, {"linux", "arm64", ""}, {"windows", "amd64", ""},
	} {
		got, err := fleetbenchAsset("v1.2.3", tc.os, tc.arch)
		if got != tc.want || (err != nil) != (tc.want == "") {
			t.Errorf("%s/%s: %s %v", tc.os, tc.arch, got, err)
		}
	}
}

func TestReleaseExecutionCacheAndOffline(t *testing.T) {
	f := newReleaseFixture(t)
	// Release execution must work without Git, Rust, or any environment manager.
	t.Setenv("PATH", "")
	f.binary = `#!/bin/sh
IFS= read -r input
printf 'input=%s\n' "$input"
printf '<%s>\n' "$@"
printf 'cwd=%s\ncaller=%s\n' "$PWD" "$RELOPS_CALLER_CWD"
printf 'binary stderr\n' >&2
exit 7
`
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	a.in = strings.NewReader("hello\n")
	o, _, _ := parse([]string{"fleetbench", "cpu", "a b", "--help", "--", ""}, &out, "test")
	code, err := a.run(context.Background(), o)
	want := "input=hello\n<cpu>\n<a b>\n<--help>\n<-->\n<>\ncwd=" + f.caller + "\ncaller=" + f.caller + "\n"
	if err != nil || code != 7 || out.String() != want || !strings.Contains(log.String(), "release=v0.4.6") || !strings.Contains(log.String(), "binary stderr") {
		t.Fatalf("execution: %d %v stdout=%q stderr=%s", code, err, out.String(), log.String())
	}
	asset, _ := fleetbenchAsset(f.tag, runtime.GOOS, runtime.GOARCH)
	binaryURL := releaseAssetURL(o, f.tag, asset)
	code, _, logText := f.run("fleetbench")
	if code != 7 || f.requests[binaryURL] != 1 || f.requests["https://api.github.com/repos/"+defaultOwner+"/fleetbench/releases/latest"] != 1 {
		t.Fatalf("online reuse: %d %s requests=%v", code, logText, f.requests)
	}
	count := len(f.requests)
	f.status = 503
	code, output, logText := f.run("--refresh", "fleetbench")
	if code != 1 || output != "" || !strings.Contains(logText, "use --offline") {
		t.Fatalf("online failure ran cached binary: %d %q %s", code, output, logText)
	}
	before := f.requests[binaryURL]
	code, _, logText = f.run("--offline", "fleetbench")
	if code != 7 || !strings.Contains(logText, "offline cached") || len(f.requests) != count || f.requests[binaryURL] != before {
		t.Fatalf("offline reuse: %d %s", code, logText)
	}
	// Missing cached releases must not trigger network requests in offline mode.
	code, _, logText = f.run("--offline", "--owner", "another-owner", "fleetbench")
	if code != 1 || !strings.Contains(logText, "run once online") {
		t.Fatalf("owner isolation: %d %s", code, logText)
	}
}

func TestReleaseUpdateFailureAndRepair(t *testing.T) {
	f := newReleaseFixture(t)
	if code, _, log := f.run("fleetbench"); code != 0 {
		t.Fatal(log)
	}
	f.tag = "v0.4.7"
	f.binary = "#!/bin/sh\nprintf 'updated\\n'\n"
	f.badBinary = true
	code, out, log := f.run("--refresh", "fleetbench")
	if code != 1 || out != "" || !strings.Contains(log, "SHA-256 mismatch") {
		t.Fatalf("bad download: %d %q %s", code, out, log)
	}
	code, out, log = f.run("--offline", "fleetbench")
	if code != 0 || out != "released\n" || !strings.Contains(log, "release=v0.4.6") {
		t.Fatalf("failed update lost last good release: %d %q %s", code, out, log)
	}
	f.badBinary = false
	code, out, log = f.run("--refresh", "fleetbench")
	if code != 0 || out != "updated\n" || !strings.Contains(log, "release=v0.4.7") {
		t.Fatalf("update: %d %q %s", code, out, log)
	}
	asset, _ := fleetbenchAsset(f.tag, runtime.GOOS, runtime.GOARCH)
	digest := fmt.Sprintf("%x", sha256.Sum256([]byte(f.binary)))
	binary := filepath.Join(f.cache, "releases", defaultOwner, "fleetbench", runtime.GOOS+"-"+runtime.GOARCH, f.tag, digest, asset)
	if err := os.WriteFile(binary, []byte("#!/bin/sh\nprintf 'tampered\\n'\n"), 0700); err != nil {
		t.Fatal(err)
	}
	code, out, log = f.run("--offline", "fleetbench")
	if code != 1 || out != "" || !strings.Contains(log, "missing or corrupt") {
		t.Fatalf("offline corruption: %d %q %s", code, out, log)
	}
	code, out, log = f.run("fleetbench")
	if code != 0 || out != "updated\n" {
		t.Fatalf("repair: %d %q %s", code, out, log)
	}
	entries, err := os.ReadDir(filepath.Dir(binary))
	if err != nil || len(entries) != 1 {
		t.Fatalf("partial downloads retained: %v %v", entries, err)
	}
}

func TestReleaseRejectsInvalidMetadata(t *testing.T) {
	for _, name := range []string{"draft", "prerelease", "beta tag", "unsafe tag", "missing binary", "missing checksums", "bad checksums", "HTTP error"} {
		t.Run(name, func(t *testing.T) {
			f := newReleaseFixture(t)
			switch name {
			case "draft":
				f.draft = true
			case "prerelease":
				f.prerelease = true
			case "beta tag":
				f.tag = "v0.5.0-beta.1"
			case "unsafe tag":
				f.tag = "../../escape"
			case "missing binary":
				f.missing, _ = fleetbenchAsset(f.tag, runtime.GOOS, runtime.GOARCH)
			case "missing checksums":
				f.missing = "SHA256SUMS"
			case "bad checksums":
				sums := "not a checksum\n"
				f.sums = &sums
			case "HTTP error":
				f.status = 403
			}
			code, out, log := f.run("fleetbench")
			if code != 1 || out != "" {
				t.Fatalf("invalid release ran: %d %q %s", code, out, log)
			}
			code, _, _ = f.run("--offline", "fleetbench")
			if code != 1 {
				t.Fatal("invalid release marked ready")
			}
		})
	}
}

func TestReleaseChecksumParsing(t *testing.T) {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte("binary")))
	for _, entry := range []string{hash + "  asset", strings.ToUpper(hash) + " *asset"} {
		got, err := releaseChecksum([]byte(entry+"\n"), "asset")
		if err != nil || got != hash {
			t.Fatalf("checksum: %q %v", got, err)
		}
	}
	for _, entry := range []string{"", "bad  asset", hash + "  other", hash + "  asset\n" + hash + "  asset"} {
		if _, err := releaseChecksum([]byte(entry), "asset"); err == nil {
			t.Errorf("accepted invalid checksums: %q", entry)
		}
	}
}

func TestConcurrentReleasePreparation(t *testing.T) {
	f := newReleaseFixture(t)
	var wg sync.WaitGroup
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			code, out, log := f.run("fleetbench")
			if code != 0 || out != "released\n" {
				t.Errorf("concurrent release: %d %q %s", code, out, log)
			}
		}()
	}
	wg.Wait()
	if n := f.requests["https://api.github.com/repos/"+defaultOwner+"/fleetbench/releases/latest"]; n != 1 {
		t.Fatalf("resolved latest %d times", n)
	}
	asset, _ := fleetbenchAsset(f.tag, runtime.GOOS, runtime.GOARCH)
	if n := f.requests[releaseAssetURL(options{owner: defaultOwner, repo: "fleetbench"}, f.tag, asset)]; n != 1 {
		t.Fatalf("downloaded binary %d times", n)
	}
}

func TestReleaseRequestCancellation(t *testing.T) {
	f := newReleaseFixture(t)
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	started := make(chan struct{})
	a.httpClient.Transport = releaseTransport(func(r *http.Request) (*http.Response, error) {
		close(started)
		<-r.Context().Done()
		return nil, r.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := a.run(ctx, options{owner: defaultOwner, repo: "fleetbench"})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil || !strings.Contains(err.Error(), "context canceled") {
			t.Fatalf("cancellation: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cancelled release request did not stop")
	}
}

// Opt-in live smoke check: downloads and runs --version, never a benchmark.
func TestRealFleetbenchRelease(t *testing.T) {
	if os.Getenv("RELOPS_TEST_REAL_FLEETBENCH") != "1" {
		t.Skip("set RELOPS_TEST_REAL_FLEETBENCH=1 to check GitHub downloads")
	}
	f := newReleaseFixture(t)
	var out, log bytes.Buffer
	a := f.app(&out, &log)
	a.httpClient = nil
	o, _, err := parse([]string{"fleetbench", "--version"}, &out, "test")
	if err != nil {
		t.Fatal(err)
	}
	code, err := a.run(context.Background(), o)
	if err != nil || code != 0 || !strings.HasPrefix(out.String(), "fleetbench ") {
		t.Fatalf("live release: %d %v %q %s", code, err, out.String(), log.String())
	}
	t.Logf("%s%s", log.String(), out.String())
	out.Reset()
	log.Reset()
	o.offline = true
	a.httpClient = &http.Client{Transport: releaseTransport(func(r *http.Request) (*http.Response, error) {
		t.Errorf("offline requested %s", r.URL)
		return nil, fmt.Errorf("offline network access")
	})}
	code, err = a.run(context.Background(), o)
	if err != nil || code != 0 || !strings.HasPrefix(out.String(), "fleetbench ") || !strings.Contains(log.String(), "offline cached") {
		t.Fatalf("live offline reuse: %d %v %q %s", code, err, out.String(), log.String())
	}
}
