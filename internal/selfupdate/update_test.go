package selfupdate

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type transport func(*http.Request) (*http.Response, error)

func (f transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestVersions(t *testing.T) {
	for _, tc := range []struct {
		candidate, current string
		want               bool
		bad                bool
	}{{"v1.10.0", "1.9.9", true, false}, {"v1.0.0", "1.0.0", false, false}, {"v0.9.0", "1.0.0", false, false}, {"v1.0.0-rc1", "0.1.0", false, true}, {"v1.0.0", "dev", false, true}, {"1.02.0", "1.0.0", false, true}} {
		got, e := Newer(tc.candidate, tc.current)
		if got != tc.want || (e != nil) != tc.bad {
			t.Fatalf("%+v: %v %v", tc, got, e)
		}
	}
}
func TestUpdate(t *testing.T) {
	for _, mode := range []string{"install", "check", "corrupt", "signature", "wrong-version", "notarization", "download", "cancel", "symlink"} {
		t.Run(mode, func(t *testing.T) {
			pub, priv, e := ed25519.GenerateKey(rand.Reader)
			if e != nil {
				t.Fatal(e)
			}
			binary := []byte("new executable")
			sum := sha256.Sum256(binary)
			m := Manifest{Version: "1.2.0", Artifacts: []Artifact{{Name: "relops-" + runtime.GOOS + "-" + runtime.GOARCH, SHA256: hex.EncodeToString(sum[:])}}}
			if mode == "wrong-version" {
				m.Version = "1.3.0"
			}
			data, _ := json.Marshal(m)
			sig := base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data))
			if mode == "signature" {
				data = append(data, ' ')
			}
			dir := t.TempDir()
			file := filepath.Join(dir, "relops")
			if e = os.WriteFile(file, []byte("original"), 0755); e != nil {
				t.Fatal(e)
			}
			target := file
			if mode == "symlink" {
				target = filepath.Join(dir, "link")
				if e = os.Symlink(file, target); e != nil {
					t.Fatal(e)
				}
			}
			calls := 0
			verified := false
			var out bytes.Buffer
			client := &http.Client{Transport: transport(func(r *http.Request) (*http.Response, error) {
				calls++
				if e := r.Context().Err(); e != nil {
					return nil, e
				}
				var body []byte
				status := 200
				switch {
				case strings.HasSuffix(r.URL.Path, "/releases/latest"):
					body = []byte(`{"tag_name":"v1.2.0"}`)
				case strings.HasSuffix(r.URL.Path, "/update.json"):
					body = data
				case strings.HasSuffix(r.URL.Path, "/update.json.sig"):
					body = []byte(sig)
				default:
					body = binary
					if mode == "corrupt" {
						body = []byte("tampered")
					}
					if mode == "download" {
						status = 503
					}
				}
				return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(body)), Header: make(http.Header)}, nil
			})}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancel" {
				cancel()
			}
			e = Run(ctx, Config{Current: "1.0.0", Key: base64.StdEncoding.EncodeToString(pub), Executable: target, Check: mode == "check", Client: client, Out: &out, Verify: func(context.Context, string) error {
				verified = true
				if mode == "notarization" {
					return errors.New("rejected")
				}
				return nil
			}})
			success := mode == "install" || mode == "symlink"
			if (e == nil) != (success || mode == "check") {
				t.Fatalf("unexpected result: %v", e)
			}
			got, _ := os.ReadFile(file)
			want := "original"
			if success {
				want = string(binary)
				if !verified {
					t.Fatal("signature verification skipped")
				}
			}
			if string(got) != want {
				t.Fatalf("installation changed: %q", got)
			}
			files, _ := filepath.Glob(filepath.Join(dir, ".relops-update-*"))
			if len(files) != 0 {
				t.Fatal("staged downloads remain")
			}
			if mode == "check" && calls != 1 {
				t.Fatal("check downloaded assets")
			}
		})
	}
}
func TestManifestRejectsUnknownKey(t *testing.T) {
	pub, priv, _ := ed25519.GenerateKey(rand.Reader)
	data := []byte(`{"version":"1.0.0"}`)
	sig := []byte(base64.StdEncoding.EncodeToString(ed25519.Sign(priv, data)))
	if _, e := VerifyManifest(data, sig, base64.StdEncoding.EncodeToString(pub)); e != nil {
		t.Fatal(e)
	}
	other, _, _ := ed25519.GenerateKey(rand.Reader)
	if _, e := VerifyManifest(data, sig, base64.StdEncoding.EncodeToString(other)); e == nil {
		t.Fatal("accepted unknown publisher")
	}
	if _, e := VerifyManifest(data, sig, ""); e == nil {
		t.Fatal("accepted missing key")
	}
}
