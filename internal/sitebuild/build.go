// Package sitebuild prepares a static download site from source and release
// binaries. Generated artifacts stay outside the tracked site source.
package sitebuild

import (
	"crypto/sha256"
	"debug/buildinfo"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Config struct {
	Source, Binaries, Version string
}

type Artifact struct {
	OS     string `json:"os"`
	Arch   string `json:"arch"`
	Path   string `json:"path"`
	Size   int    `json:"size"`
	SHA256 string `json:"sha256"`
}

type Release struct {
	SchemaVersion int        `json:"schema_version"`
	Version       string     `json:"version"`
	SourceCommit  string     `json:"source_commit"`
	SourceDirty   bool       `json:"source_dirty"`
	GoVersion     string     `json:"go_version"`
	BuildCommand  string     `json:"build_command"`
	Artifacts     []Artifact `json:"artifacts"`
}

type metadata struct {
	os, arch, commit, goVersion string
	dirty                       bool
}

var versionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?$`)

var platforms = []struct{ os, arch string }{
	{"darwin", "arm64"}, {"darwin", "amd64"}, {"linux", "arm64"}, {"linux", "amd64"},
}

// Build replaces Binaries/site only after preparing and validating a complete
// staged bundle. It never deploys or modifies the site source.
func Build(cfg Config) (string, error) { return build(cfg, inspectBinary) }

func build(cfg Config, inspect func(string) (metadata, error)) (string, error) {
	if !versionPattern.MatchString(cfg.Version) {
		return "", errors.New("version must be a semantic version such as 0.1.0")
	}
	source, err := filepath.Abs(cfg.Source)
	if err != nil {
		return "", err
	}
	binaries, err := filepath.Abs(cfg.Binaries)
	if err != nil {
		return "", err
	}
	output := filepath.Join(binaries, "site")
	if rel, err := filepath.Rel(source, output); err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", errors.New("generated output must be outside the site source")
	}
	stage, err := os.MkdirTemp(binaries, ".site-build-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(stage)
	// Keep concept pages and source documentation out of the deployed site.
	for _, item := range []string{"index.html", "css", "js"} {
		if err := copySource(filepath.Join(source, item), filepath.Join(stage, item)); err != nil {
			return "", err
		}
	}
	release := Release{SchemaVersion: 1, Version: cfg.Version, BuildCommand: "just version=" + cfg.Version + " site"}
	downloadPath := "downloads/v" + cfg.Version
	var checksums strings.Builder
	for i, platform := range platforms {
		name := "relops-" + platform.os + "-" + platform.arch
		file := filepath.Join(binaries, name)
		info, err := inspect(file)
		if err != nil {
			return "", fmt.Errorf("inspect %s: %w; run just release first", name, err)
		}
		if info.os != platform.os || info.arch != platform.arch {
			return "", fmt.Errorf("%s contains a %s/%s binary", name, info.os, info.arch)
		}
		if i == 0 {
			release.SourceCommit, release.SourceDirty, release.GoVersion = info.commit, info.dirty, info.goVersion
		} else if release.SourceCommit != info.commit || release.SourceDirty != info.dirty || release.GoVersion != info.goVersion {
			return "", errors.New("release binaries have different source/toolchain metadata; rebuild them together")
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return "", err
		}
		digest := sha256.Sum256(data)
		artifact := Artifact{OS: platform.os, Arch: platform.arch, Path: downloadPath + "/" + name, Size: len(data), SHA256: hex.EncodeToString(digest[:])}
		release.Artifacts = append(release.Artifacts, artifact)
		if err := writeFile(stage, artifact.Path, data); err != nil {
			return "", err
		}
		fmt.Fprintf(&checksums, "%s  %s\n", artifact.SHA256, name)
	}
	if err := writeFile(stage, downloadPath+"/SHA256SUMS", []byte(checksums.String())); err != nil {
		return "", err
	}
	manifest, err := json.MarshalIndent(release, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeFile(stage, "releases.json", append(manifest, '\n')); err != nil {
		return "", err
	}
	index, err := os.ReadFile(filepath.Join(stage, "index.html"))
	if err != nil {
		return "", err
	}
	page := strings.NewReplacer(
		"{{RELOPS_VERSION}}", cfg.Version,
		"__RELOPS_RELEASE_JSON__", string(manifest),
		"__RELOPS_SHA_DARWIN_ARM64__", release.Artifacts[0].SHA256,
	).Replace(string(index))
	if strings.Contains(page, "__RELOPS_") || strings.Contains(page, "{{RELOPS_VERSION}}") {
		return "", errors.New("unresolved site template placeholder")
	}
	if err := writeFile(stage, "index.html", []byte(page)); err != nil {
		return "", err
	}
	files := []string{"index.json"}
	if err := filepath.WalkDir(stage, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			rel, err := filepath.Rel(stage, path)
			if err != nil {
				return err
			}
			files = append(files, filepath.ToSlash(rel))
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	inventory, err := json.MarshalIndent(struct {
		SchemaVersion int      `json:"schema_version"`
		Files         []string `json:"files"`
	}{1, files}, "", "  ")
	if err != nil {
		return "", err
	}
	if err := writeFile(stage, "index.json", append(inventory, '\n')); err != nil {
		return "", err
	}
	// Refuse to replace arbitrary hand-maintained content or a symlink at the
	// generated destination. Our artifact inventory identifies previous builds.
	if info, err := os.Lstat(output); err == nil {
		if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return "", errors.New("site output already exists and is not a directory")
		}
		if _, err := os.Stat(filepath.Join(output, "index.json")); err != nil {
			return "", errors.New("refusing to replace site directory without a generated index.json")
		}
		if err := os.RemoveAll(output); err != nil {
			return "", err
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	if err := os.Rename(stage, output); err != nil {
		return "", err
	}
	return output, nil
}

func inspectBinary(path string) (metadata, error) {
	info, err := buildinfo.ReadFile(path)
	if err != nil {
		return metadata{}, err
	}
	if info.Path != "github.com/mozilla-platform-ops/relops-launcher/cmd/relops" {
		return metadata{}, errors.New("not a relops launcher binary")
	}
	settings := make(map[string]string)
	for _, setting := range info.Settings {
		settings[setting.Key] = setting.Value
	}
	if settings["CGO_ENABLED"] != "0" || settings["-trimpath"] != "true" {
		return metadata{}, errors.New("expected a CGO-disabled, trimpath release build")
	}
	if settings["vcs.revision"] == "" {
		return metadata{}, errors.New("source commit metadata is missing; build the release from a Git checkout")
	}
	return metadata{os: settings["GOOS"], arch: settings["GOARCH"], commit: settings["vcs.revision"], dirty: settings["vcs.modified"] == "true", goVersion: info.GoVersion}, nil
}

func writeFile(root, path string, data []byte) error {
	target := filepath.Join(root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	return os.WriteFile(target, data, 0644)
}

func copySource(source, target string) error {
	return filepath.WalkDir(source, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return os.MkdirAll(filepath.Join(target, rel), 0755)
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("site source must contain regular files, not symlinks: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.WriteFile(target, data, 0644)
		}
		return os.WriteFile(filepath.Join(target, rel), data, 0644)
	})
}
