package sitebuild

import (
	"bytes"
	"errors"
	"fmt"
	"html"
	"html/template"
	"io/fs"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/text"
)

type toolPage struct {
	ID, Name, Description, Version, Revision, CatalogRevision string
	Targets                                                   []tool
	Guide                                                     template.HTML
}

func buildToolPages(source, stage, version, revision string) error {
	entries, err := readTools(filepath.Join(source, "tools.json"))
	if err != nil {
		return err
	}
	pages := make(map[string]*toolPage)
	var order []string
	for _, entry := range entries {
		if pages[entry.Shortcut] == nil {
			pages[entry.Shortcut] = &toolPage{ID: entry.Shortcut, Name: entry.Name,
				Description: entry.Description, Version: version, Revision: revision, CatalogRevision: entry.CatalogRevision}
			order = append(order, entry.Shortcut)
		}
		pages[entry.Shortcut].Targets = append(pages[entry.Shortcut].Targets, entry)
	}
	guides := filepath.Join(source, "tool-guides")
	if info, err := os.Lstat(guides); err == nil && !info.IsDir() {
		return errors.New("tool-guides must be a directory, not a symlink or file")
	} else if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	files, err := os.ReadDir(guides)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, file := range files {
		if file.Name() == "images" {
			continue
		}
		if !file.Type().IsRegular() || filepath.Ext(file.Name()) != ".md" {
			return fmt.Errorf("unexpected guide source: %s", file.Name())
		}
		id := strings.TrimSuffix(file.Name(), ".md")
		page := pages[id]
		if page == nil {
			return fmt.Errorf("guide %s has no catalog tool", file.Name())
		}
		data, err := os.ReadFile(filepath.Join(guides, file.Name()))
		if err != nil {
			return err
		}
		page.Guide, err = renderGuide(data, page.Targets[0])
		if err != nil {
			return fmt.Errorf("guide %s: %w", file.Name(), err)
		}
	}
	if err := copyGuideImages(guides, stage, pages); err != nil {
		return err
	}
	tmpl, err := template.ParseFiles(filepath.Join(source, "tool.html"))
	if err != nil {
		return err
	}
	for _, id := range order {
		var out bytes.Buffer
		if err := tmpl.Execute(&out, pages[id]); err != nil {
			return err
		}
		if err := writeFile(stage, "tools/"+id+"/index.html", out.Bytes()); err != nil {
			return err
		}
	}
	return nil
}

func renderGuide(data []byte, target tool) (template.HTML, error) {
	// A single convenience token keeps hand-written workflows tied to the
	// catalog's canonical ID and channel without making guides Go templates.
	data = bytes.ReplaceAll(data, []byte("{{launch}}"), []byte(target.Launch()))
	md := goldmark.New(goldmark.WithExtensions(extension.Table),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()))
	doc := md.Parser().Parse(text.NewReader(data))
	err := ast.Walk(doc, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		switch node := node.(type) {
		case *ast.Heading:
			if node.Level == 1 {
				return ast.WalkStop, errors.New("use level-2 or deeper headings; the page supplies its title")
			}
		case *ast.HTMLBlock, *ast.RawHTML:
			return ast.WalkStop, errors.New("use Markdown rather than raw HTML")
		case *ast.Image:
			prefix := "images/" + target.Shortcut + "/"
			if !strings.HasPrefix(string(node.Destination), prefix) {
				return ast.WalkStop, fmt.Errorf("guide images must use %s", prefix)
			}
			node.Destination = append([]byte("../../tool-guide-images/"), node.Destination[len("images/"):]...)
		}
		return ast.WalkContinue, nil
	})
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := md.Renderer().Render(&out, data, doc); err != nil {
		return "", err
	}
	// Only the Markdown renderer's HTML crosses this template boundary.
	return template.HTML(out.String()), nil
}

func copyGuideImages(guides, stage string, pages map[string]*toolPage) error {
	root := filepath.Join(guides, "images")
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if errors.Is(err, os.ErrNotExist) && path == root {
			return nil
		}
		if err != nil {
			return err
		}
		if path == root {
			if !entry.IsDir() {
				return errors.New("guide images must be a directory")
			}
			return nil
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		id := strings.Split(filepath.ToSlash(rel), "/")[0]
		if pages[id] == nil {
			return fmt.Errorf("image directory %s has no catalog tool", id)
		}
		if entry.IsDir() {
			return nil
		}
		if !entry.Type().IsRegular() {
			return fmt.Errorf("guide image must be a regular file: %s", path)
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".avif", ".svg":
		default:
			return fmt.Errorf("unsupported guide image: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return writeFile(stage, "tool-guide-images/"+filepath.ToSlash(rel), data)
	})
}

var siteLinkPattern = regexp.MustCompile(`(?:href|src)="([^"]+)"`)
var siteIDPattern = regexp.MustCompile(`\bid="([^"]+)"`)

// Validate the final bundle rather than source Markdown so image rewrites,
// cross-tool links, navigation anchors, and template assets are all covered.
func validateSiteLinks(root string) error {
	return filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || filepath.Ext(path) != ".html" {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		ids := make(map[string]bool)
		for _, match := range siteIDPattern.FindAllSubmatch(data, -1) {
			id := html.UnescapeString(string(match[1]))
			if ids[id] {
				return fmt.Errorf("duplicate anchor in %s: %s", path, id)
			}
			ids[id] = true
		}
		for _, match := range siteLinkPattern.FindAllSubmatch(data, -1) {
			href := html.UnescapeString(string(match[1]))
			u, err := url.Parse(href)
			if err != nil {
				return fmt.Errorf("invalid link in %s: %s", path, href)
			}
			if u.IsAbs() || u.Host != "" {
				continue
			}
			target := path
			if u.Path != "" {
				target = filepath.Join(filepath.Dir(path), filepath.FromSlash(u.Path))
				if strings.HasPrefix(u.Path, "/") {
					target = filepath.Join(root, filepath.FromSlash(strings.TrimPrefix(u.Path, "/")))
				}
			}
			rel, err := filepath.Rel(root, target)
			if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
				return fmt.Errorf("link leaves site in %s: %s", path, href)
			}
			info, err := os.Stat(target)
			if err == nil && info.IsDir() {
				target = filepath.Join(target, "index.html")
				_, err = os.Stat(target)
			}
			if err != nil {
				return fmt.Errorf("broken link in %s: %s: %w", path, href, err)
			}
			if u.Fragment != "" && filepath.Ext(target) == ".html" {
				destination, err := os.ReadFile(target)
				if err != nil {
					return err
				}
				found := false
				for _, id := range siteIDPattern.FindAllSubmatch(destination, -1) {
					found = found || html.UnescapeString(string(id[1])) == u.Fragment
				}
				if !found {
					return fmt.Errorf("missing anchor in %s: %s", path, href)
				}
			}
		}
		return nil
	})
}
