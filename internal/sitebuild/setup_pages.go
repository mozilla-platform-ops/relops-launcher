package sitebuild

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"path/filepath"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

type setupPage struct {
	Guide             catalog.SetupGuide
	Content           template.HTML
	Tools             []tool
	Version, Revision string
}

func buildSetupPages(source, stage, version, revision string) error {
	root := filepath.Join(source, "requirements")
	info, err := os.Lstat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return fmt.Errorf("requirements must be a directory, not a symlink or file")
	}
	files, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, file := range files {
		if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".md" {
			return fmt.Errorf("setup guide must be a regular Markdown file: %s", file.Name())
		}
		if _, err := catalog.LookupSetupGuide(strings.TrimSuffix(file.Name(), ".md")); err != nil {
			return err
		}
	}
	entries, err := readTools(filepath.Join(source, "tools.json"))
	if err != nil {
		return err
	}
	tmpl, err := template.ParseFiles(filepath.Join(source, "requirement.html"))
	if err != nil {
		return err
	}
	for _, guide := range catalog.AllSetupGuides() {
		data, err := os.ReadFile(filepath.Join(root, guide.ID+".md"))
		if err != nil {
			return fmt.Errorf("setup guide %s: %w", guide.ID, err)
		}
		content, err := renderMarkdown(data, "")
		if err != nil {
			return fmt.Errorf("setup guide %s: %w", guide.ID, err)
		}
		page := setupPage{Guide: guide, Content: content, Version: version, Revision: revision}
		seen := make(map[string]bool)
		for _, entry := range entries {
			for _, linked := range entry.SetupGuides {
				if linked.ID == guide.ID && !seen[entry.Shortcut] {
					page.Tools = append(page.Tools, entry)
					seen[entry.Shortcut] = true
				}
			}
		}
		var out bytes.Buffer
		if err := tmpl.Execute(&out, page); err != nil {
			return err
		}
		if err := writeFile(stage, guide.Path()+"index.html", out.Bytes()); err != nil {
			return err
		}
	}
	return nil
}
