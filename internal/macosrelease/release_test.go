package macosrelease

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type fakeApple struct {
	t                                   *testing.T
	signs, submits, waits, onlineChecks int
	waitError, ticketError              bool
	status                              string
}

func (f *fakeApple) command(_ context.Context, name string, args ...string) ([]byte, error) {
	f.t.Helper()
	switch name {
	case "codesign":
		if args[0] == "--force" {
			f.signs++
			file := args[len(args)-1]
			data, err := os.ReadFile(file)
			if err != nil {
				return nil, err
			}
			return nil, os.WriteFile(file, append(data, []byte(";Developer ID signed")...), 0755)
		}
		joined := strings.Join(args, " ")
		if !strings.Contains(joined, "certificate leaf[subject.OU]") || !strings.Contains(joined, "1.2.840.113635.100.6.1.13") {
			f.t.Fatal("signature verification did not require the Developer ID certificate and Team ID")
		}
		if strings.Contains(joined, "--check-notarization") {
			f.onlineChecks++
			if !strings.Contains(joined, "and notarized") {
				f.t.Fatal("ticket verification did not require notarization")
			}
			if f.ticketError {
				return nil, errors.New("ticket has not propagated")
			}
		}
		return nil, nil
	case "ditto":
		return nil, os.WriteFile(args[len(args)-1], []byte("submission archive"), 0600)
	case "xcrun":
		switch args[1] {
		case "submit":
			f.submits++
			return []byte(`{"id":"12345678-1234-1234-1234-123456789abc"}`), nil
		case "wait":
			f.waits++
			if f.waitError {
				return nil, errors.New("wait timed out")
			}
			data, _ := json.Marshal(map[string]string{"status": f.status})
			return data, nil
		case "log":
			data, _ := json.Marshal(map[string]any{"status": f.status, "issues": []string{}})
			return nil, os.WriteFile(args[3], data, 0600)
		}
	}
	f.t.Fatalf("unexpected command %s %v", name, args)
	return nil, nil
}

func fixture(t *testing.T) Config {
	t.Helper()
	cfg := Config{Dist: t.TempDir(), Version: "0.1.1", Identity: "Developer ID Application: Test", TeamID: "W5BHZ9R8A3", Profile: "test-profile", Timeout: time.Minute}
	resetRaw(t, cfg)
	return cfg
}

func resetRaw(t *testing.T, cfg Config) {
	t.Helper()
	for _, name := range allFiles {
		if err := os.WriteFile(filepath.Join(cfg.Dist, name), []byte("unsigned "+name), 0755); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSignedArtifactsAndCacheReuse(t *testing.T) {
	cfg := fixture(t)
	apple := &fakeApple{t: t, status: "Accepted"}
	for i := 0; i < 2; i++ {
		// just signed-site starts with a fresh ordinary release on every run.
		resetRaw(t, cfg)
		if err := run(context.Background(), cfg, io.Discard, apple.command); err != nil {
			t.Fatal(err)
		}
		receipt, err := ValidateReceipt(filepath.Join(cfg.Dist, "macos-notarization.json"), cfg.Version, cfg.Dist)
		if err != nil || receipt.Status != "Accepted" {
			t.Fatalf("receipt: %+v %v", receipt, err)
		}
		sums, err := os.ReadFile(filepath.Join(cfg.Dist, "SHA256SUMS"))
		if err != nil {
			t.Fatal(err)
		}
		for _, name := range allFiles {
			data, err := os.ReadFile(filepath.Join(cfg.Dist, name))
			if err != nil {
				t.Fatal(err)
			}
			if strings.HasPrefix(name, "relops-linux-") && strings.Contains(string(data), "Developer ID") {
				t.Fatal("Linux artifact was changed by signing")
			}
			digest, _ := hashFile(filepath.Join(cfg.Dist, name))
			if !strings.Contains(string(sums), digest+"  "+name+"\n") {
				t.Fatal("checksums were not regenerated after signing")
			}
		}
	}
	if apple.signs != 1 || apple.submits != 1 || apple.waits != 1 || apple.onlineChecks != 2 {
		t.Fatalf("did not reuse accepted signed bytes and recheck tickets: %+v", apple)
	}
}

func TestResumeSubmissionAfterTimeout(t *testing.T) {
	cfg := fixture(t)
	apple := &fakeApple{t: t, status: "Accepted", waitError: true}
	err := run(context.Background(), cfg, io.Discard, apple.command)
	if err == nil || !strings.Contains(err.Error(), "resume without resubmitting") {
		t.Fatalf("timeout: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Dist, "macos-notarization.json")); !os.IsNotExist(err) {
		t.Fatal("a timed-out release was exported")
	}
	apple.waitError = false
	resetRaw(t, cfg)
	if err := run(context.Background(), cfg, io.Discard, apple.command); err != nil {
		t.Fatal(err)
	}
	if apple.submits != 1 || apple.signs != 1 || apple.waits != 2 {
		t.Fatalf("resubmitted after timeout: %+v", apple)
	}
}

func TestRejectedSubmissionDoesNotExport(t *testing.T) {
	cfg := fixture(t)
	apple := &fakeApple{t: t, status: "Invalid"}
	err := run(context.Background(), cfg, io.Discard, apple.command)
	if err == nil || !strings.Contains(err.Error(), "notarization status is Invalid") {
		t.Fatalf("rejection: %v", err)
	}
	if _, err := os.Stat(filepath.Join(cfg.Dist, "macos-notarization.json")); !os.IsNotExist(err) {
		t.Fatal("rejected release was exported")
	}
	if err := run(context.Background(), cfg, io.Discard, apple.command); err == nil || apple.submits != 1 {
		t.Fatal("rejected bytes were submitted again")
	}
}

func TestTicketDelayDoesNotResubmit(t *testing.T) {
	cfg := fixture(t)
	apple := &fakeApple{t: t, status: "Accepted", ticketError: true}
	if err := run(context.Background(), cfg, io.Discard, apple.command); err == nil {
		t.Fatal("missing ticket was accepted")
	}
	apple.ticketError = false
	resetRaw(t, cfg)
	if err := run(context.Background(), cfg, io.Discard, apple.command); err != nil {
		t.Fatal(err)
	}
	if apple.submits != 1 || apple.signs != 1 {
		t.Fatalf("ticket delay caused a new submission: %+v", apple)
	}
}

func TestSignOnlyAndChangedArtifact(t *testing.T) {
	cfg := fixture(t)
	cfg.SignOnly = true
	apple := &fakeApple{t: t, status: "Accepted"}
	if err := run(context.Background(), cfg, io.Discard, apple.command); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(cfg.Dist, "macos-notarization.json")
	if _, err := ValidateReceipt(path, cfg.Version, cfg.Dist); err == nil {
		t.Fatal("signing alone was treated as notarization")
	}
	if apple.submits != 0 || apple.waits != 0 {
		t.Fatal("sign-only invoked the notary service")
	}
	cfg.SignOnly = false
	resetRaw(t, cfg)
	if err := run(context.Background(), cfg, io.Discard, apple.command); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateReceipt(path, "0.1.2", cfg.Dist); err == nil {
		t.Fatal("accepted another version's approval")
	}
	if err := os.WriteFile(filepath.Join(cfg.Dist, macFiles[0]), []byte("different bytes"), 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateReceipt(path, cfg.Version, cfg.Dist); err == nil {
		t.Fatal("accepted approval for different bytes")
	}
}
