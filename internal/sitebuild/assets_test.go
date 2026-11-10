package sitebuild

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAssetURLsFollowContentAcrossAllPages(t *testing.T) {
	cfg, inspect := fixture(t)
	assets := []string{"css/tokens.css", "js/main.js"}
	previous := make(map[string]string)
	for cycle := 0; cycle < 3; cycle++ {
		// First two builds are identical; the third changes both asset types
		// without changing the launcher version or its catalog.
		if cycle == 2 {
			for _, asset := range assets {
				f, err := os.OpenFile(filepath.Join(cfg.Source, asset), os.O_APPEND|os.O_WRONLY, 0)
				if err != nil {
					t.Fatal(err)
				}
				_, err = f.WriteString("\n/* site-only update */\n")
				closeErr := f.Close()
				if err != nil || closeErr != nil {
					t.Fatalf("modify asset: %v %v", err, closeErr)
				}
			}
		}
		output, err := build(cfg, inspect)
		if err != nil {
			t.Fatal(err)
		}
		for _, asset := range assets {
			data, err := os.ReadFile(filepath.Join(cfg.Source, asset))
			if err != nil {
				t.Fatal(err)
			}
			digest := sha256.Sum256(data)
			ext := filepath.Ext(asset)
			fingerprinted := fmt.Sprintf("%s.%x%s", strings.TrimSuffix(asset, ext), digest[:8], ext)
			if cycle == 1 && fingerprinted != previous[asset] {
				t.Fatal("unchanged asset URL changed")
			}
			if cycle == 2 && fingerprinted == previous[asset] {
				t.Fatal("changed asset URL did not change")
			}
			previous[asset] = fingerprinted
			shipped, err := os.ReadFile(filepath.Join(output, fingerprinted))
			if err != nil || string(shipped) != string(data) {
				t.Fatalf("fingerprinted asset bytes: %s %v", asset, err)
			}
			if _, err := os.Stat(filepath.Join(output, asset)); !os.IsNotExist(err) {
				t.Fatalf("unversioned asset retained: %s", asset)
			}
			pages := 0
			err = filepath.WalkDir(output, func(path string, entry fs.DirEntry, err error) error {
				if err != nil || entry.IsDir() || filepath.Ext(path) != ".html" {
					return err
				}
				page, err := os.ReadFile(path)
				if err != nil {
					return err
				}
				if !strings.Contains(string(page), filepath.Base(fingerprinted)) || strings.Contains(string(page), asset+"\"") {
					t.Errorf("page %s does not use current fingerprint for %s", path, asset)
				}
				pages++
				return nil
			})
			if err != nil || pages < 3 {
				t.Fatalf("check generated homepage, tools, and setup guides: %d pages, %v", pages, err)
			}
		}
	}
}
