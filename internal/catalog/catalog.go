// Package catalog defines the curated, bundled tool directory.
package catalog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"regexp"
	"strings"

	quicksite "github.com/mozilla-platform-ops/relops-launcher/quick_site"
)

type Catalog struct {
	SchemaVersion int    `json:"schema_version"`
	Revision      string `json:"revision"`
	Tools         []Tool `json:"tools"`
}

type Tool struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Aliases     []string `json:"aliases,omitempty"`
	Description string   `json:"description"`
	Default     *Target  `json:"default,omitempty"`
	Beta        *Target  `json:"beta,omitempty"`
}

type Target struct {
	Kind          string   `json:"kind"` // source, release, or web
	URL           string   `json:"url,omitempty"`
	Owner         string   `json:"owner"`
	Repo          string   `json:"repo"`
	Path          string   `json:"path,omitempty"`
	Ref           string   `json:"ref,omitempty"`
	Command       string   `json:"command,omitempty"`
	Runner        string   `json:"runner,omitempty"`
	Cwd           string   `json:"cwd,omitempty"`
	Args          []string `json:"example_args,omitempty"`
	Prerequisites string   `json:"prerequisites,omitempty"`
	DataFiles     []string `json:"data_files,omitempty"`
	Documentation string   `json:"documentation,omitempty"`
	Note          string   `json:"note,omitempty"`
}

var name = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]*$`)
var repo = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
var owner = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)

func Decode(data []byte) (Catalog, error) {
	var c Catalog
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err := d.Decode(&c); err != nil {
		return c, fmt.Errorf("parse catalog: %w", err)
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return c, fmt.Errorf("catalog must contain one JSON object")
	}
	if c.SchemaVersion != 1 || strings.TrimSpace(c.Revision) == "" {
		return c, fmt.Errorf("catalog requires schema_version 1 and a revision")
	}
	names := map[string]bool{}
	for _, t := range c.Tools {
		for _, n := range append([]string{t.ID}, t.Aliases...) {
			if !name.MatchString(n) || names[n] {
				return c, fmt.Errorf("invalid or duplicate catalog name %q", n)
			}
			names[n] = true
		}
		if t.Name == "" || t.Description == "" || (t.Default == nil && t.Beta == nil) {
			return c, fmt.Errorf("tool %q requires name, description, and at least one target", t.ID)
		}
		for _, target := range []*Target{t.Default, t.Beta} {
			if target == nil {
				continue
			}
			if err := validateTarget(*target); err != nil {
				return c, fmt.Errorf("tool %q: %w", t.ID, err)
			}
		}
	}
	return c, nil
}

func validateTarget(t Target) error {
	if t.Kind == "web" {
		u, err := url.Parse(t.URL)
		if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || strings.ContainsAny(t.URL, "\r\n\t") {
			return fmt.Errorf("web target requires an HTTPS URL")
		}
		if t.Owner != "" || t.Repo != "" || t.Path != "" || t.Ref != "" || t.Command != "" || t.Runner != "" || t.Cwd != "" || len(t.Args) != 0 {
			return fmt.Errorf("web target cannot specify source or release fields")
		}
		return nil
	}
	if t.URL != "" {
		return fmt.Errorf("URL is only supported for web targets")
	}
	if !owner.MatchString(t.Owner) || !repo.MatchString(t.Repo) || t.Repo == "." || t.Repo == ".." {
		return fmt.Errorf("invalid target owner/repository")
	}
	if strings.HasPrefix(t.Path, "/") || strings.ContainsAny(t.Path, "\\\x00") {
		return fmt.Errorf("invalid target path")
	}
	for _, part := range strings.Split(t.Path, "/") {
		if part == ".." || part == ".git" {
			return fmt.Errorf("invalid target path")
		}
	}
	if strings.HasPrefix(t.Ref, "-") || strings.ContainsAny(t.Ref, "\x00\r\n") {
		return fmt.Errorf("invalid target ref")
	}
	if t.Command != "" && (!repo.MatchString(t.Command) || t.Command == "." || t.Command == "..") {
		return fmt.Errorf("invalid console command")
	}
	if t.Runner != "" && t.Runner != "auto" && t.Runner != "python3" && t.Runner != "bash" && t.Runner != "sh" {
		return fmt.Errorf("invalid runner")
	}
	if t.Cwd != "" && t.Cwd != "repo" && t.Cwd != "caller" {
		return fmt.Errorf("invalid cwd")
	}
	switch t.Kind {
	case "source":
	case "release":
		if t.Repo != "fleetbench" || t.Path != "" || t.Ref != "" || t.Command != "" || t.Runner != "" || (t.Cwd != "" && t.Cwd != "caller") {
			return fmt.Errorf("unsupported release target")
		}
	default:
		return fmt.Errorf("target kind must be source, release, or web")
	}
	return nil
}

func Bundled() (Catalog, error) { return Decode(quicksite.Catalog) }

func (c Catalog) Lookup(n string) (Tool, error) {
	for _, t := range c.Tools {
		if t.ID == n {
			return t, nil
		}
		for _, alias := range t.Aliases {
			if alias == n {
				return t, nil
			}
		}
	}
	return Tool{}, fmt.Errorf("unknown catalog tool %q; use relops catalog", n)
}

func (t Tool) Select(beta bool) (*Target, string, error) {
	target, channel := t.Default, "default"
	if beta {
		target, channel = t.Beta, "beta"
	}
	if target == nil {
		if !beta && t.Beta != nil {
			return nil, channel, fmt.Errorf("tool %q has no default target; launch its beta with relops run -b %s", t.ID, t.ID)
		}
		return nil, channel, fmt.Errorf("tool %q has no %s target", t.ID, channel)
	}
	return target, channel, nil
}
