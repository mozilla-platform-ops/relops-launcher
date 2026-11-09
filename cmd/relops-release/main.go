// relops-release prepares authenticated assets locally; it never publishes them.
package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/macosrelease"
	"github.com/mozilla-platform-ops/relops-launcher/internal/selfupdate"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
func run(args []string) error {
	flags := flag.NewFlagSet("relops-release", flag.ContinueOnError)
	keygen := flags.Bool("keygen", false, "generate an update signing key pair (never overwrite)")
	private := flags.String("private-key", "", "private key path outside the repository")
	public := flags.String("public-key", "release-public-key.txt", "public key path")
	dir := flags.String("dir", "", "retained signed release directory")
	version := flags.String("version", "", "stable release version")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("unexpected arguments")
	}
	if *private == "" {
		return fmt.Errorf("--private-key is required")
	}
	if *keygen {
		pub, priv, e := ed25519.GenerateKey(rand.Reader)
		if e != nil {
			return e
		}
		// Open both destinations exclusively before writing either file.
		f, e := os.OpenFile(*private, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if e != nil {
			return e
		}
		defer f.Close()
		p, e := os.OpenFile(*public, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if e != nil {
			os.Remove(*private)
			return e
		}
		defer p.Close()
		if _, e = fmt.Fprintln(f, base64.StdEncoding.EncodeToString(priv)); e != nil {
			return e
		}
		_, e = fmt.Fprintln(p, base64.StdEncoding.EncodeToString(pub))
		return e
	}
	if _, e := selfupdate.Version(*version); e != nil {
		return e
	}
	if strings.HasPrefix(*version, "v") {
		return fmt.Errorf("--version must omit v")
	}
	if *dir == "" {
		return fmt.Errorf("--dir is required")
	}
	key, e := os.ReadFile(*private)
	if e != nil {
		return e
	}
	priv, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(key)))
	if e != nil || len(priv) != ed25519.PrivateKeySize {
		return fmt.Errorf("invalid private key")
	}
	pub, e := os.ReadFile(*public)
	if e != nil {
		return e
	}
	if strings.TrimSpace(string(pub)) != base64.StdEncoding.EncodeToString(ed25519.PrivateKey(priv).Public().(ed25519.PublicKey)) {
		return fmt.Errorf("private key does not match public key")
	}
	if _, e = macosrelease.ValidateReceipt(filepath.Join(*dir, "macos-notarization.json"), *version, *dir); e != nil {
		return e
	}
	m := selfupdate.Manifest{Version: *version}
	out, e := exec.Command("git", "rev-parse", "--verify", "refs/tags/v"+*version+"^{commit}").Output()
	if e != nil {
		return fmt.Errorf("create the version tag first: %w", e)
	}
	m.Commit = strings.TrimSpace(string(out))
	for _, name := range []string{"relops-darwin-arm64", "relops-linux-amd64"} {
		file := filepath.Join(*dir, name)
		info, err := buildinfo.ReadFile(file)
		if err != nil {
			return err
		}
		settings := map[string]string{}
		for _, s := range info.Settings {
			settings[s.Key] = s.Value
		}
		b, e := os.ReadFile(file)
		if e != nil {
			return e
		}
		if info.Path != "github.com/mozilla-platform-ops/relops-launcher/cmd/relops" || settings["vcs.revision"] != m.Commit || settings["vcs.modified"] != "false" || settings["-trimpath"] != "true" || settings["CGO_ENABLED"] != "0" || "relops-"+settings["GOOS"]+"-"+settings["GOARCH"] != name || !embeddedReleaseIdentity(b, *version, strings.TrimSpace(string(pub))) {
			return fmt.Errorf("%s must be a clean tagged build with the expected platform, version and update public key", name)
		}
		sum := sha256.Sum256(b)
		m.Artifacts = append(m.Artifacts, selfupdate.Artifact{Name: name, SHA256: hex.EncodeToString(sum[:])})
	}
	// Release must correspond to the checked-out version tag; never create a tag implicitly.
	data, e := json.MarshalIndent(m, "", "  ")
	if e != nil {
		return e
	}
	data = append(data, '\n')
	sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)) + "\n"
	if e = os.WriteFile(filepath.Join(*dir, "update.json"), data, 0644); e != nil {
		return e
	}
	return os.WriteFile(filepath.Join(*dir, "update.json.sig"), []byte(sig), 0644)
}

// Trimpath builds omit linker flags from buildinfo. The linker-injected version
// marker and public key remain in the executable, including after codesigning.
func embeddedReleaseIdentity(binary []byte, version, key string) bool {
	pub, err := base64.StdEncoding.DecodeString(key)
	return err == nil && len(pub) == ed25519.PublicKeySize && bytes.Contains(binary, []byte("relops-launcher-version:"+version+":end")) && bytes.Contains(binary, []byte(key))
}
