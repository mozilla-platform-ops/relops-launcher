package launcher

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"
)

// Release support is deliberately explicit: never guess asset names for an
// arbitrary repository. Paths within fleetbench still use source execution.
func isReleaseTarget(o options) bool {
	return !o.sourceTarget && o.repo == "fleetbench" && o.path == ""
}

func fleetbenchAsset(tag, goos, arch string) (string, error) {
	platforms := map[string]string{
		"darwin/arm64": "macos-aarch64",
		"linux/amd64":  "linux-x86_64",
	}
	platform, ok := platforms[goos+"/"+arch]
	if !ok {
		return "", fmt.Errorf("fleetbench releases are not supported on %s/%s; supported platforms: darwin/arm64, linux/amd64", goos, arch)
	}
	return "fleetbench-" + tag + "-" + platform, nil
}

var stableReleaseTag = regexp.MustCompile(`^v[0-9]+\.[0-9]+\.[0-9]+$`)

type releaseMetadata struct {
	Tag       string    `json:"tag"`
	Asset     string    `json:"asset"`
	SHA256    string    `json:"sha256"`
	CheckedAt time.Time `json:"checked_at,omitempty"`
	ETag      string    `json:"etag,omitempty"`
}

const releaseCheckInterval = 15 * time.Minute

type githubRelease struct {
	Tag        string `json:"tag_name"`
	Draft      bool   `json:"draft"`
	Prerelease bool   `json:"prerelease"`
	Assets     []struct {
		Name string `json:"name"`
	} `json:"assets"`
}

func (a *app) runRelease(ctx context.Context, o options) (int, error) {
	binary, meta, err := a.prepareRelease(ctx, o)
	if err != nil {
		return 1, err
	}
	mode := "stable release; checked " + meta.CheckedAt.UTC().Format(time.RFC3339)
	if o.offline {
		mode = "offline cached"
	}
	fmt.Fprintf(a.errOut, "relops: %s/%s release=%s (%s)\n", o.owner, o.repo, meta.Tag, mode)
	cmd := exec.Command(binary, o.args...)
	cmd.Dir = a.caller
	cmd.Env = replaceEnv(os.Environ(), "RELOPS_CALLER_CWD", a.caller)
	cmd.Env = replaceEnv(cmd.Env, "PWD", a.caller)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = a.in, a.out, a.errOut
	if o.verbose {
		fmt.Fprintf(a.errOut, "relops: binary=%s cwd=%s\n", binary, a.caller)
	}
	return runTool(ctx, cmd)
}

func (a *app) prepareRelease(ctx context.Context, o options) (string, releaseMetadata, error) {
	meta := releaseMetadata{}
	if _, err := fleetbenchAsset("", runtime.GOOS, runtime.GOARCH); err != nil {
		return "", meta, err
	}
	base := filepath.Join(a.cache, "releases", o.owner, o.repo, runtime.GOOS+"-"+runtime.GOARCH)
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", meta, err
	}
	unlock, err := lock(ctx, filepath.Join(base, "release.lock"), a.lockTimeout)
	if err != nil {
		return "", meta, err
	}
	defer unlock()
	latest := filepath.Join(base, "latest.json")
	data, readErr := os.ReadFile(latest)
	cacheErr := readErr
	if cacheErr == nil {
		cacheErr = json.Unmarshal(data, &meta)
	}
	if cacheErr == nil {
		cacheErr = validateReleaseMetadata(meta)
	}
	now := time.Now()
	fresh := cacheErr == nil && !meta.CheckedAt.IsZero() && !meta.CheckedAt.After(now) && now.Sub(meta.CheckedAt) < releaseCheckInterval
	refreshed := false
	if o.offline {
		if cacheErr != nil {
			return "", meta, fmt.Errorf("no valid cached release for %s/%s; run once online first: %w", o.owner, o.repo, cacheErr)
		}
	} else if o.refresh || !fresh {
		if cacheErr != nil {
			meta = releaseMetadata{}
		}
		resolved, err := a.latestRelease(ctx, o, meta)
		if err != nil {
			guidance := "no verified cached release is available"
			if cacheErr == nil && verifyReleaseBinary(filepath.Join(base, meta.Tag, meta.SHA256, meta.Asset), meta.SHA256) == nil {
				guidance = "use --offline explicitly to run the verified cached release " + meta.Tag
			}
			return "", meta, fmt.Errorf("resolve latest stable release: %w; %s", err, guidance)
		}
		meta = resolved
		refreshed = true
	}
	if err := validateReleaseMetadata(meta); err != nil {
		return "", meta, err
	}
	// Keep different bytes at different paths even if a publisher replaces an
	// asset under the same tag. Concurrent runs retain the version they verified.
	binary := filepath.Join(base, meta.Tag, meta.SHA256, meta.Asset)
	if err := verifyReleaseBinary(binary, meta.SHA256); err != nil {
		if o.offline {
			return "", meta, fmt.Errorf("cached release is missing or corrupt; run online to repair: %w", err)
		}
		fmt.Fprintf(a.errOut, "relops: downloading %s/%s %s\n", o.owner, o.repo, meta.Asset)
		if err := os.MkdirAll(filepath.Dir(binary), 0700); err != nil {
			return "", meta, err
		}
		url := releaseAssetURL(o, meta.Tag, meta.Asset)
		if err := a.installReleaseBinary(ctx, url, binary, meta.SHA256); err != nil {
			return "", meta, err
		}
	}
	if refreshed {
		data, err := json.Marshal(meta)
		if err != nil {
			return "", meta, err
		}
		if err := atomicReleaseFile(latest, bytes.NewReader(data), 0600); err != nil {
			return "", meta, err
		}
	}
	return binary, meta, nil
}

func validateReleaseMetadata(meta releaseMetadata) error {
	asset, err := fleetbenchAsset(meta.Tag, runtime.GOOS, runtime.GOARCH)
	digest, digestErr := hex.DecodeString(meta.SHA256)
	if err != nil || !stableReleaseTag.MatchString(meta.Tag) || meta.Asset != asset || digestErr != nil || len(digest) != sha256.Size {
		return errors.New("invalid release metadata; run online to refresh")
	}
	return nil
}

func releaseAssetURL(o options, tag, asset string) string {
	return "https://github.com/" + o.owner + "/" + o.repo + "/releases/download/" + tag + "/" + asset
}

func (a *app) latestRelease(ctx context.Context, o options, previous releaseMetadata) (releaseMetadata, error) {
	meta := releaseMetadata{}
	url := "https://api.github.com/repos/" + o.owner + "/" + o.repo + "/releases/latest"
	var release githubRelease
	resp, err := a.releaseRequest(ctx, url, previous.ETag)
	if err != nil {
		return meta, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotModified {
		meta = previous
	} else {
		data, err := io.ReadAll(io.LimitReader(resp.Body, (4<<20)+1))
		if err != nil {
			return meta, err
		}
		if len(data) > 4<<20 {
			return meta, errors.New("release response exceeds size limit")
		}
		if err := json.Unmarshal(data, &release); err != nil {
			return meta, fmt.Errorf("decode GitHub release: %w", err)
		}
		meta.ETag = resp.Header.Get("ETag")
	}
	if resp.StatusCode != http.StatusNotModified {
		if release.Draft || release.Prerelease || !stableReleaseTag.MatchString(release.Tag) {
			return meta, errors.New("GitHub latest release must be a published stable vMAJOR.MINOR.PATCH release")
		}
		meta.Tag = release.Tag
		meta.Asset, err = fleetbenchAsset(meta.Tag, runtime.GOOS, runtime.GOARCH)
		if err != nil {
			return meta, err
		}
		for _, name := range []string{meta.Asset, "SHA256SUMS"} {
			count := 0
			for _, asset := range release.Assets {
				if asset.Name == name {
					count++
				}
			}
			if count != 1 {
				return meta, fmt.Errorf("release %s must contain exactly one %s asset", meta.Tag, name)
			}
		}
	}
	// Recheck checksums even on a 304: release assets can be replaced under a tag.
	data, err := a.releaseBytes(ctx, releaseAssetURL(o, meta.Tag, "SHA256SUMS"), 1<<20)
	if err != nil {
		return meta, err
	}
	meta.SHA256, err = releaseChecksum(data, meta.Asset)
	meta.CheckedAt = time.Now().UTC()
	return meta, err
}

func releaseChecksum(data []byte, asset string) (string, error) {
	var checksum string
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 || strings.TrimPrefix(fields[1], "*") != asset {
			continue
		}
		digest, err := hex.DecodeString(fields[0])
		if err != nil || len(digest) != sha256.Size || checksum != "" {
			return "", fmt.Errorf("invalid or duplicate SHA256SUMS entry for %s", asset)
		}
		checksum = strings.ToLower(fields[0])
	}
	if checksum == "" {
		return "", fmt.Errorf("SHA256SUMS has no checksum for %s", asset)
	}
	return checksum, nil
}

func (a *app) releaseGet(ctx context.Context, url string) (*http.Response, error) {
	return a.releaseRequest(ctx, url, "")
}

func (a *app) releaseRequest(ctx context.Context, url, etag string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "relops-launcher")
	if strings.HasPrefix(url, "https://api.github.com/") {
		req.Header.Set("Accept", "application/vnd.github+json")
		req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		if etag != "" {
			req.Header.Set("If-None-Match", etag)
		}
		token := os.Getenv("GH_TOKEN")
		if token == "" {
			token = os.Getenv("GITHUB_TOKEN")
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
	}
	// All release requests share a persisted cooldown, including different
	// owners/platforms and fresh processes. Serialize checks with response headers.
	stateDir := filepath.Join(a.cache, "releases")
	if err := os.MkdirAll(stateDir, 0700); err != nil {
		return nil, err
	}
	unlock, err := lock(ctx, filepath.Join(stateDir, "requests.lock"), a.lockTimeout)
	if err != nil {
		return nil, err
	}
	defer unlock()
	cooldownFile := filepath.Join(stateDir, "cooldown.json")
	var cooldown releaseCooldown
	if data, err := os.ReadFile(cooldownFile); err == nil {
		if err := json.Unmarshal(data, &cooldown); err != nil {
			return nil, fmt.Errorf("invalid release request cooldown: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if time.Now().Before(cooldown.Until) {
		return nil, fmt.Errorf("GitHub requests paused until %s: %s", cooldown.Until.UTC().Format(time.RFC3339), cooldown.Reason)
	}
	client := a.httpClient
	if client == nil {
		client = releaseHTTPClient()
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", url, err)
	}
	if resp.StatusCode != http.StatusOK && !(resp.StatusCode == http.StatusNotModified && etag != "") {
		defer resp.Body.Close()
		requestErr, until := releaseResponseError(url, resp, time.Now())
		if !until.IsZero() {
			data, _ := json.Marshal(releaseCooldown{Until: until, Reason: requestErr.Error()})
			if err := atomicReleaseFile(cooldownFile, bytes.NewReader(data), 0600); err != nil {
				return nil, fmt.Errorf("%w; save cooldown: %v", requestErr, err)
			}
		}
		return nil, requestErr
	}
	return resp, nil
}

func releaseHTTPClient() *http.Client {
	return &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if req.URL.Host != "api.github.com" {
			req.Header.Del("Authorization")
		}
		if req.URL.Scheme != "https" {
			return errors.New("release download redirected away from HTTPS")
		}
		if len(via) >= 10 {
			return errors.New("too many release download redirects")
		}
		return nil
	}}
}

func (a *app) releaseBytes(ctx context.Context, url string, limit int64) ([]byte, error) {
	resp, err := a.releaseGet(ctx, url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err == nil && int64(len(data)) > limit {
		err = errors.New("release response exceeds size limit")
	}
	return data, err
}

func verifyReleaseBinary(file, want string) error {
	info, err := os.Lstat(file)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&0111 == 0 {
		return errors.New("cached release must be an executable regular file")
	}
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, f); err != nil {
		return err
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return errors.New("release binary SHA-256 mismatch")
	}
	return nil
}

func (a *app) installReleaseBinary(ctx context.Context, url, file, want string) error {
	resp, err := a.releaseGet(ctx, url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	// Verify before making the download executable or replacing a cached binary.
	tmp, err := os.CreateTemp(filepath.Dir(file), ".download-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	hash := sha256.New()
	const limit = 128 << 20
	n, err := io.Copy(io.MultiWriter(tmp, hash), io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return err
	}
	if n > limit {
		return errors.New("release binary exceeds size limit")
	}
	if hex.EncodeToString(hash.Sum(nil)) != want {
		return errors.New("downloaded release binary SHA-256 mismatch")
	}
	if err := tmp.Chmod(0700); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}

func atomicReleaseFile(file string, data io.Reader, mode os.FileMode) error {
	tmp, err := os.CreateTemp(filepath.Dir(file), ".metadata-")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	defer tmp.Close()
	if _, err := io.Copy(tmp, data); err != nil {
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), file)
}
