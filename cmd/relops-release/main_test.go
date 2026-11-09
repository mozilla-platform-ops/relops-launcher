package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestKeygen(t *testing.T) {
	dir := t.TempDir()
	priv := filepath.Join(dir, "private")
	pub := filepath.Join(dir, "public")
	args := []string{"--keygen", "--private-key", priv, "--public-key", pub}
	if e := run(args); e != nil {
		t.Fatal(e)
	}
	raw, e := os.ReadFile(priv)
	if e != nil {
		t.Fatal(e)
	}
	key, e := base64.StdEncoding.DecodeString(strings.TrimSpace(string(raw)))
	if e != nil || len(key) != ed25519.PrivateKeySize {
		t.Fatal("invalid private key")
	}
	public, _ := os.ReadFile(pub)
	if strings.TrimSpace(string(public)) != base64.StdEncoding.EncodeToString(ed25519.PrivateKey(key).Public().(ed25519.PublicKey)) {
		t.Fatal("key mismatch")
	}
	info, _ := os.Stat(priv)
	if info.Mode().Perm() != 0600 {
		t.Fatal("private key permissions")
	}
	if e = run(args); e == nil {
		t.Fatal("overwrote existing keys")
	}
	after, _ := os.ReadFile(priv)
	if string(raw) != string(after) {
		t.Fatal("private key changed")
	}
	other := filepath.Join(dir, "other-private")
	if e = run([]string{"--keygen", "--private-key", other, "--public-key", pub}); e == nil {
		t.Fatal("overwrote public key")
	}
	if _, e = os.Stat(other); !os.IsNotExist(e) {
		t.Fatal("partial private key remains")
	}
}
func TestReleaseRequiresInputs(t *testing.T) {
	for _, args := range [][]string{{}, {"--private-key", "missing", "--version", "dev"}, {"--private-key", "missing", "--version", "1.0.0"}} {
		if e := run(args); e == nil {
			t.Fatalf("accepted %v", args)
		}
	}
}
