// Package selfupdate verifies publisher-signed releases before replacing relops.
package selfupdate

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// PublicKey is set at build time from the publisher's public key file.
// A build without a key can check versions but cannot install updates.
var PublicKey string

const Repository = "mozilla-platform-ops/relops-launcher"
const TeamID = "W5BHZ9R8A3"

type Artifact struct {
	Name   string `json:"name"`
	SHA256 string `json:"sha256"`
}
type Manifest struct {
	Version   string     `json:"version"`
	Commit    string     `json:"commit"`
	Artifacts []Artifact `json:"artifacts"`
}
type Config struct {
	Current, Key, Executable string
	Check                    bool
	Out                      io.Writer
	Client                   *http.Client
	// Tests inject platform signature verification; production uses codesign.
	Verify func(context.Context, string) error
}

func Version(v string) ([3]uint64, error) {
	var result [3]uint64
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return result, errors.New("expected stable MAJOR.MINOR.PATCH version")
	}
	for i, p := range parts {
		if p == "" || (len(p) > 1 && p[0] == '0') {
			return result, errors.New("invalid stable version")
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return result, errors.New("invalid stable version")
			}
		}
		n, e := strconv.ParseUint(p, 10, 64)
		if e != nil {
			return result, e
		}
		result[i] = n
	}
	return result, nil
}
func Newer(candidate, current string) (bool, error) {
	a, e := Version(candidate)
	if e != nil {
		return false, e
	}
	b, e := Version(current)
	if e != nil {
		return false, fmt.Errorf("current build has no stable version; install a release manually: %w", e)
	}
	for i := range a {
		if a[i] != b[i] {
			return a[i] > b[i], nil
		}
	}
	return false, nil
}
func VerifyManifest(data, signature []byte, key string) (Manifest, error) {
	var m Manifest
	pub, e := base64.StdEncoding.DecodeString(strings.TrimSpace(key))
	if e != nil || len(pub) != ed25519.PublicKeySize {
		return m, errors.New("update verification key is missing or invalid; install an official release")
	}
	sig, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(signature)))
	if e != nil || !ed25519.Verify(pub, data, sig) {
		return m, errors.New("release manifest signature is invalid")
	}
	e = json.Unmarshal(data, &m)
	if e != nil {
		return m, e
	}
	if _, e = Version(m.Version); e != nil {
		return m, e
	}
	return m, nil
}
func Run(ctx context.Context, c Config) error {
	if c.Out == nil {
		c.Out = io.Discard
	}
	if c.Client == nil {
		c.Client = &http.Client{Timeout: 2 * time.Minute, CheckRedirect: func(r *http.Request, via []*http.Request) error {
			if r.URL.Scheme != "https" {
				return errors.New("update redirect must use HTTPS")
			}
			if len(via) >= 10 {
				return errors.New("too many redirects")
			}
			return nil
		}}
	}
	get := func(url string, limit int64) ([]byte, error) {
		req, e := http.NewRequestWithContext(ctx, "GET", url, nil)
		if e != nil {
			return nil, e
		}
		req.Header.Set("User-Agent", "relops-launcher")
		resp, e := c.Client.Do(req)
		if e != nil {
			return nil, e
		}
		defer resp.Body.Close()
		if resp.StatusCode != 200 {
			return nil, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
		}
		b, e := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if int64(len(b)) > limit {
			return nil, errors.New("update exceeds size limit")
		}
		return b, e
	}
	b, e := get("https://api.github.com/repos/"+Repository+"/releases/latest", 1<<20)
	if e != nil {
		return e
	}
	var latest struct {
		Tag        string `json:"tag_name"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if e = json.Unmarshal(b, &latest); e != nil {
		return e
	}
	if latest.Draft || latest.Prerelease || !strings.HasPrefix(latest.Tag, "v") {
		return errors.New("latest release is not a stable release")
	}
	newer, e := Newer(latest.Tag, c.Current)
	if e != nil {
		return e
	}
	if !newer {
		fmt.Fprintf(c.Out, "relops %s is up to date\n", c.Current)
		return nil
	}
	if c.Check {
		fmt.Fprintf(c.Out, "relops %s available (installed: %s); run relops update\n", latest.Tag, c.Current)
		return nil
	}
	platform := runtime.GOOS + "-" + runtime.GOARCH
	if platform != "darwin-arm64" && platform != "linux-amd64" {
		return fmt.Errorf("updates are unsupported on %s", platform)
	}
	base := "https://github.com/" + Repository + "/releases/download/" + latest.Tag + "/"
	data, e := get(base+"update.json", 1<<20)
	if e != nil {
		return e
	}
	sig, e := get(base+"update.json.sig", 1024)
	if e != nil {
		return e
	}
	m, e := VerifyManifest(data, sig, c.Key)
	if e != nil {
		return e
	}
	if "v"+m.Version != latest.Tag {
		return errors.New("signed manifest version does not match release")
	}
	name := "relops-" + platform
	digest := ""
	for _, a := range m.Artifacts {
		if a.Name == name {
			if digest != "" {
				return errors.New("duplicate update artifact")
			}
			digest = a.SHA256
		}
	}
	d, e := hex.DecodeString(digest)
	if e != nil || len(d) != sha256.Size {
		return errors.New("missing or invalid update checksum")
	}
	destination := c.Executable
	if destination == "" {
		destination, e = os.Executable()
		if e != nil {
			return e
		}
	}
	destination, e = filepath.EvalSymlinks(destination)
	if e != nil {
		return e
	}
	original, e := os.Stat(destination)
	if e != nil {
		return e
	}
	if !original.Mode().IsRegular() {
		return errors.New("installed executable must be a regular file")
	}
	f, e := os.CreateTemp(filepath.Dir(destination), ".relops-update-*")
	if e != nil {
		return fmt.Errorf("cannot write installation directory; install in a user-writable directory: %w", e)
	}
	defer os.Remove(f.Name())
	defer f.Close()
	binary, e := get(base+name, 128<<20)
	if e != nil {
		return e
	}
	sum := sha256.Sum256(binary)
	if hex.EncodeToString(sum[:]) != digest {
		return errors.New("update checksum mismatch")
	}
	if _, e = f.Write(binary); e != nil {
		return e
	}
	if e = f.Chmod(original.Mode().Perm()); e != nil {
		return e
	}
	if e = f.Sync(); e != nil {
		return e
	}
	if e = f.Close(); e != nil {
		return e
	}
	verify := c.Verify
	if verify == nil {
		verify = verifyPlatform
	}
	if e = verify(ctx, f.Name()); e != nil {
		return e
	}
	current, e := os.Stat(destination)
	if e != nil {
		return e
	}
	if !os.SameFile(original, current) {
		return errors.New("installation changed during update; retry")
	}
	if e = os.Rename(f.Name(), destination); e != nil {
		return fmt.Errorf("replace executable: %w", e)
	}
	fmt.Fprintf(c.Out, "Updated relops to %s\n", m.Version)
	return nil
}
func verifyPlatform(ctx context.Context, file string) error {
	if runtime.GOOS != "darwin" {
		return nil
	}
	requirement := `anchor apple generic and certificate leaf[subject.OU] = "` + TeamID + `" and certificate leaf[field.1.2.840.113635.100.6.1.13] exists and notarized`
	b, e := exec.CommandContext(ctx, "codesign", "--verify", "--strict", "--check-notarization", "-R="+requirement, file).CombinedOutput()
	if e != nil {
		return fmt.Errorf("macOS signature/notarization verification: %w: %s", e, b)
	}
	return nil
}
