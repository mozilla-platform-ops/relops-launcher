package sitebuild

import (
	"crypto/sha256"
	"fmt"
	"html"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Fingerprint the assembled assets, leaving source templates readable. Hashes
// depend only on asset bytes, so site-only edits need no launcher version bump.
func fingerprintAssets(root string) error {
	names := make(map[string]string)
	for _, dir := range []string{"css", "js"} {
		err := filepath.WalkDir(filepath.Join(root, dir), func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() {
				return err
			}
			ext := filepath.Ext(path)
			if ext != ".css" && ext != ".js" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			digest := sha256.Sum256(data)
			target := fmt.Sprintf("%s.%x%s", strings.TrimSuffix(path, ext), digest[:8], ext)
			if err := os.Rename(path, target); err != nil {
				return err
			}
			names[path] = target
			return nil
		})
		if err != nil {
			return err
		}
	}
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		page := siteLinkPattern.ReplaceAllStringFunc(string(data), func(link string) string {
			start := strings.IndexByte(link, '"') + 1
			u, err := url.Parse(html.UnescapeString(link[start : len(link)-1]))
			if err != nil || u.IsAbs() || u.Host != "" || u.Path == "" {
				return link
			}
			target := filepath.Join(filepath.Dir(path), filepath.FromSlash(u.Path))
			absolute := strings.HasPrefix(u.Path, "/")
			if absolute {
				target = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(u.Path, "/")))
			}
			renamed, ok := names[target]
			if !ok {
				return link
			}
			// Only the basename changes; preserve relative paths and URL suffixes.
			u.Path = strings.TrimSuffix(u.Path, filepath.Base(target)) + filepath.Base(renamed)
			return link[:start] + html.EscapeString(u.String()) + "\""
		})
		return os.WriteFile(path, []byte(page), 0644)
	})
}
