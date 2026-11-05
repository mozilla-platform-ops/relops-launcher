// Package macosrelease signs and notarizes standalone release binaries. Private
// keys and notarization credentials remain in the publisher's macOS Keychain.
package macosrelease

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"syscall"
	"time"
)

type Config struct {
	Dist, Version, Identity, TeamID, Profile string
	Timeout                                  time.Duration
	SignOnly                                 bool
}

// Receipt records the exact signed bytes that Apple accepted. It contains no
// credentials and is checked against the actual artifacts before site assembly.
type Receipt struct {
	SchemaVersion int               `json:"schema_version"`
	Version       string            `json:"version"`
	Identity      string            `json:"identity"`
	TeamID        string            `json:"team_id"`
	Status        string            `json:"status"`
	SubmissionID  string            `json:"submission_id,omitempty"`
	ArchiveSHA256 string            `json:"archive_sha256"`
	InputSHA256   map[string]string `json:"input_sha256"`
	SHA256        map[string]string `json:"sha256"`
}

var macFiles = []string{"relops-darwin-arm64"}
var allFiles = []string{"relops-darwin-arm64", "relops-linux-amd64"}
var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)
var teamPattern = regexp.MustCompile(`^[A-Z0-9]{10}$`)

type runner func(context.Context, string, ...string) ([]byte, error)

func Run(ctx context.Context, cfg Config, out io.Writer) error {
	return run(ctx, cfg, out, func(ctx context.Context, name string, args ...string) ([]byte, error) {
		fmt.Fprintf(out, "+ %s %s\n", name, strings.Join(args, " "))
		cmd := exec.CommandContext(ctx, name, args...)
		cmd.WaitDelay = 2 * time.Second
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return stdout.Bytes(), fmt.Errorf("%s: %w\n%s", name, err, stderr.String())
		}
		_, _ = out.Write(stderr.Bytes())
		return stdout.Bytes(), nil
	})
}

func run(ctx context.Context, cfg Config, out io.Writer, command runner) error {
	if !versionPattern.MatchString(cfg.Version) || !teamPattern.MatchString(cfg.TeamID) || cfg.Identity == "" || (!cfg.SignOnly && cfg.Profile == "") || cfg.Timeout <= 0 {
		return errors.New("provide a release version, signing identity, 10-character Team ID, Keychain profile, and positive timeout")
	}
	dist, err := filepath.Abs(cfg.Dist)
	if err != nil {
		return err
	}
	lock, err := os.OpenFile(filepath.Join(dist, ".macos-release.lock"), os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open release lock: %w; run just release first", err)
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return errors.New("another macOS release operation is running in this dist directory; retry after it finishes")
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	inputs := make(map[string]string)
	for _, name := range allFiles {
		digest, err := hashFile(filepath.Join(dist, name))
		if err != nil {
			return fmt.Errorf("read release artifact %s: %w; run just release first", name, err)
		}
		if strings.HasPrefix(name, "relops-darwin-") {
			inputs[name] = digest
		}
	}
	key, _ := json.Marshal(struct {
		Version, Identity, Team string
		Inputs                  map[string]string
	}{cfg.Version, cfg.Identity, cfg.TeamID, inputs})
	digest := sha256.Sum256(key)
	cache := filepath.Join(dist, "macos", hex.EncodeToString(digest[:]))
	payload := filepath.Join(cache, "payload")
	if err := os.MkdirAll(payload, 0700); err != nil {
		return err
	}
	statePath := filepath.Join(cache, "state.json")
	archive := filepath.Join(cache, "relops-macos.zip")
	receipt := Receipt{SchemaVersion: 1, Version: cfg.Version, Identity: cfg.Identity, TeamID: cfg.TeamID, Status: "Signed", InputSHA256: inputs, SHA256: make(map[string]string)}
	data, err := os.ReadFile(statePath)
	if err == nil {
		if err := json.Unmarshal(data, &receipt); err != nil {
			return fmt.Errorf("read signing state %s: %w", statePath, err)
		}
		if receipt.SchemaVersion != 1 || receipt.Version != cfg.Version || receipt.Identity != cfg.Identity || receipt.TeamID != cfg.TeamID || !maps.Equal(receipt.InputSHA256, inputs) {
			return fmt.Errorf("signing state does not match release inputs; move aside %s and retry", cache)
		}
		for _, name := range macFiles {
			actual, err := hashFile(filepath.Join(payload, name))
			if err != nil || actual != receipt.SHA256[name] {
				return fmt.Errorf("cached signed artifact %s changed; move aside %s and retry", name, cache)
			}
		}
		actual, err := hashFile(archive)
		if err != nil || actual != receipt.ArchiveSHA256 {
			return fmt.Errorf("cached submission archive changed; move aside %s and retry", cache)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	} else {
		for _, name := range macFiles {
			file := filepath.Join(payload, name)
			if err := copyBinary(filepath.Join(dist, name), file); err != nil {
				return err
			}
			if _, err := command(ctx, "codesign", "--force", "--sign", cfg.Identity, "--identifier", "org.mozilla.relops", "--options", "runtime", "--timestamp", file); err != nil {
				return err
			}
			if err := verify(ctx, command, file, cfg.TeamID, false); err != nil {
				return err
			}
			receipt.SHA256[name], err = hashFile(file)
			if err != nil {
				return err
			}
		}
		if _, err := command(ctx, "ditto", "-c", "-k", "--keepParent", payload, archive); err != nil {
			return err
		}
		receipt.ArchiveSHA256, err = hashFile(archive)
		if err != nil {
			return err
		}
		if err := save(statePath, receipt); err != nil {
			return err
		}
	}
	if !cfg.SignOnly {
		if receipt.Status == "Invalid" || receipt.Status == "Rejected" {
			return fmt.Errorf("Apple rejected submission %s; inspect %s/notary-log.json and fix the release before retrying", receipt.SubmissionID, cache)
		}
		if receipt.SubmissionID == "" {
			response, err := command(ctx, "xcrun", "notarytool", "submit", archive, "--keychain-profile", cfg.Profile, "--no-wait", "--output-format", "json")
			if err != nil {
				return fmt.Errorf("submit for notarization: %w; check the Keychain profile and Apple Developer account agreements; configure credentials with just notary-setup", err)
			}
			var result struct {
				ID string `json:"id"`
			}
			if err := json.Unmarshal(response, &result); err != nil || result.ID == "" {
				return fmt.Errorf("notarytool returned no submission ID: %s", response)
			}
			receipt.SubmissionID, receipt.Status = result.ID, "Submitted"
			if err := save(statePath, receipt); err != nil {
				return err
			}
		}
		if receipt.Status != "Accepted" {
			fmt.Fprintf(out, "Waiting for Apple submission %s (rerun just signed-site to resume).\n", receipt.SubmissionID)
			response, err := command(ctx, "xcrun", "notarytool", "wait", receipt.SubmissionID, "--keychain-profile", cfg.Profile, "--timeout", fmt.Sprintf("%ds", int(cfg.Timeout.Seconds())), "--output-format", "json")
			var result struct {
				Status string `json:"status"`
			}
			if json.Unmarshal(response, &result) == nil && result.Status != "" {
				receipt.Status = result.Status
				if saveErr := save(statePath, receipt); saveErr != nil {
					return saveErr
				}
			}
			if err != nil {
				return fmt.Errorf("waiting for submission %s: %w; rerun just signed-site to resume without resubmitting", receipt.SubmissionID, err)
			}
			if receipt.Status != "Accepted" {
				_, _ = command(ctx, "xcrun", "notarytool", "log", receipt.SubmissionID, filepath.Join(cache, "notary-log.json"), "--keychain-profile", cfg.Profile)
				return fmt.Errorf("notarization status is %s; inspect %s/notary-log.json; signed site was not generated", receipt.Status, cache)
			}
		}
		logPath := filepath.Join(cache, "notary-log.json")
		if _, err := os.Stat(logPath); errors.Is(err, os.ErrNotExist) {
			if _, err := command(ctx, "xcrun", "notarytool", "log", receipt.SubmissionID, logPath, "--keychain-profile", cfg.Profile); err != nil {
				return fmt.Errorf("retrieve accepted submission log: %w; rerun just signed-site to retry", err)
			}
		}
		log, err := os.ReadFile(logPath)
		if err != nil {
			return err
		}
		var report struct {
			Status string `json:"status"`
			Issues []struct {
				Message string `json:"message"`
			} `json:"issues"`
		}
		if err := json.Unmarshal(log, &report); err != nil || report.Status != "Accepted" {
			return fmt.Errorf("notarization log is not accepted: %s", logPath)
		}
		for _, issue := range report.Issues {
			fmt.Fprintln(out, "Apple notarization log:", issue.Message)
		}
	}
	for _, name := range macFiles {
		if err := verify(ctx, command, filepath.Join(payload, name), cfg.TeamID, !cfg.SignOnly); err != nil {
			return fmt.Errorf("verify signed artifact %s: %w; notarization tickets may take time to propagate; rerun just signed-site to retry", name, err)
		}
	}
	for _, name := range macFiles {
		if err := copyBinary(filepath.Join(payload, name), filepath.Join(dist, name)); err != nil {
			return err
		}
	}
	if err := save(filepath.Join(dist, "macos-notarization.json"), receipt); err != nil {
		return err
	}
	var sums strings.Builder
	for _, name := range allFiles {
		digest, err := hashFile(filepath.Join(dist, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&sums, "%s  %s\n", digest, name)
	}
	if err := os.WriteFile(filepath.Join(dist, "SHA256SUMS"), []byte(sums.String()), 0644); err != nil {
		return err
	}
	fmt.Fprintf(out, "macOS release %s: %s; receipt: %s\n", cfg.Version, receipt.Status, filepath.Join(dist, "macos-notarization.json"))
	return nil
}

func verify(ctx context.Context, command runner, file, team string, notarized bool) error {
	requirement := `anchor apple generic and certificate leaf[subject.OU] = "` + team + `" and certificate leaf[field.1.2.840.113635.100.6.1.13] exists`
	args := []string{"--verify", "--strict", "--verbose=2"}
	if notarized {
		requirement += " and notarized"
		args = append(args, "--check-notarization")
	}
	_, err := command(ctx, "codesign", append(args, "-R="+requirement, file)...)
	return err
}

// ValidateReceipt rejects stale approvals, including approvals for pre-signing
// bytes or another version. Apple remains the authority for Gatekeeper checks.
func ValidateReceipt(path, version, dist string) (*Receipt, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var receipt Receipt
	if err := json.Unmarshal(data, &receipt); err != nil {
		return nil, err
	}
	if receipt.SchemaVersion != 1 || receipt.Version != version || receipt.Status != "Accepted" || receipt.SubmissionID == "" || !teamPattern.MatchString(receipt.TeamID) {
		return nil, errors.New("macOS notarization receipt is not accepted for this version")
	}
	for _, name := range macFiles {
		digest, err := hashFile(filepath.Join(dist, name))
		if err != nil || receipt.SHA256[name] != digest {
			return nil, fmt.Errorf("macOS notarization receipt does not match %s; run just signed-site", name)
		}
	}
	return &receipt, nil
}

func hashFile(path string) (string, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", path)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(data)
	return hex.EncodeToString(digest[:]), nil
}

func copyBinary(source, target string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return err
	}
	return atomicWrite(target, data, 0755)
}

func save(path string, value any) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return atomicWrite(path, append(data, '\n'), 0600)
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".release-write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Chmod(mode); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
