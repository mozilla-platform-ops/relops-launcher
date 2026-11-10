package sitebuild

import (
	"bytes"
	"html/template"
	"net/url"
	"os"
	"sort"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

// tool is the website presentation of one catalog target.
type tool struct {
	ID, Name, Description, Owner, Repo, Path, Ref string
	Shortcut, Kind, URL, Cwd, EntryPoint, Runner  string
	Args, Aliases, DataFiles                      []string
	Channel, Prerequisites, Documentation, Note   string
	CatalogRevision                               string
	WhenToUse                                     string
	SetupGuides                                   []catalog.SetupGuide
}

type requirementFile struct {
	Path, GuidePath string
}

type requirementsView struct {
	Files  []requirementFile
	Guides []catalog.SetupGuide
}

func (t tool) Requirements() requirementsView {
	var view requirementsView
	for _, path := range t.DataFiles {
		view.Files = append(view.Files, requirementFile{Path: path})
	}
	for _, guide := range t.SetupGuides {
		linked := false
		for i := range view.Files {
			if guide.File != "" && view.Files[i].Path == guide.File {
				view.Files[i].GuidePath = guide.Path()
				linked = true
			}
		}
		if !linked {
			view.Guides = append(view.Guides, guide)
		}
	}
	return view
}

func (t tool) Launch() string {
	words := []string{"relops", "run"}
	if t.Channel == "preview" {
		words = append(words, "-b")
	}
	return strings.Join(append(words, t.Shortcut), " ")
}

func (t tool) SourceURL() string {
	if t.Kind != "source" {
		return ""
	}
	ref := strings.TrimPrefix(strings.TrimPrefix(t.Ref, "refs/heads/"), "refs/tags/")
	if ref == "" {
		ref = "HEAD"
	}
	// GitHub's tree route handles directories and redirects files to blob URLs.
	parts := []string{t.Owner, t.Repo, "tree", ref}
	if t.Path != "" {
		parts = append(parts, strings.Split(t.Path, "/")...)
	}
	for i := range parts {
		parts[i] = url.PathEscape(parts[i])
	}
	return "https://github.com/" + strings.Join(parts, "/")
}

func (t tool) ChannelLabel() string {
	switch t.Channel {
	case "preview":
		return "Beta"
	case "release":
		return t.Ref
	}
	return ""
}

func (t tool) Command() string {
	words := []string{"relops", "run"}
	if t.Channel == "preview" {
		words = append(words, "-b")
	}
	words = append(words, t.Shortcut)
	words = append(words, t.Args...)
	for i, word := range words {
		if strings.ContainsAny(word, " \t\n'\"\\$`;&|<>()*?[]{}!#~") {
			words[i] = "'" + strings.ReplaceAll(word, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(words, " ")
}

func readTools(path string) ([]tool, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	c, err := catalog.Decode(data)
	if err != nil {
		return nil, err
	}
	var entries []tool
	for _, entry := range c.Tools {
		for _, beta := range []bool{false, true} {
			target, _, err := entry.Select(beta)
			if err != nil {
				continue
			}
			channel, id := "mainline", entry.ID+"-default"
			if beta {
				channel, id = "preview", entry.ID+"-beta"
			}
			var guides []catalog.SetupGuide
			for _, guideID := range target.SetupGuides {
				guide, err := catalog.LookupSetupGuide(guideID)
				if err != nil {
					return nil, err
				}
				guides = append(guides, guide)
			}
			entries = append(entries, tool{
				ID: id, Shortcut: entry.ID, Name: entry.Name, Description: entry.Description, Aliases: entry.Aliases,
				WhenToUse: entry.WhenToUse,
				Owner:     target.Owner, Repo: target.Repo, Path: target.Path, Ref: target.Ref,
				Kind: target.Kind, URL: target.URL, Cwd: target.Cwd,
				EntryPoint: target.Command, Runner: target.Runner,
				Args: target.Args, Channel: channel, Prerequisites: target.Prerequisites,
				Documentation: target.Documentation, Note: target.Note, DataFiles: target.DataFiles,
				CatalogRevision: c.Revision,
				SetupGuides:     guides,
			})
		}
	}
	return entries, nil
}

func renderTools(path string) (string, error) {
	entries, err := readTools(path)
	if err != nil {
		return "", err
	}
	var groups struct{ Standard, Preview []tool }
	for _, entry := range entries {
		if entry.Channel == "preview" {
			groups.Preview = append(groups.Preview, entry)
		} else {
			groups.Standard = append(groups.Standard, entry)
		}
	}
	for _, group := range [][]tool{groups.Standard, groups.Preview} {
		sort.SliceStable(group, func(i, j int) bool {
			return strings.ToLower(group[i].Name) < strings.ToLower(group[j].Name)
		})
	}
	tmpl, err := template.New("tools").Parse(`{{define "list"}}
      <ul class="tool-list">{{range .}}
        <li class="tool-row">
          <div class="tool-row__heading">
            <h3><a class="tool-name" href="tools/{{.Shortcut}}/">{{.Name}}</a></h3>
            {{if eq .Kind "web"}}<span class="badge badge--web">Web</span>{{end}}
            {{if .ChannelLabel}}<span class="badge">{{.ChannelLabel}}</span>{{end}}
          </div>
          <p class="prose">{{if eq .Kind "web"}}Open {{.Name}} (<a href="{{.URL}}">{{.URL}}</a>) in your browser.{{else}}{{.Description}}{{end}}</p>
          <div class="cmd cmd--wrap"><div class="cmd__lines">{{$tool := .}}{{if .Aliases}}<span class="cmd__comment"># Short {{if eq (len .Aliases) 1}}alias{{else}}aliases{{end}}: {{range $i, $alias := .Aliases}}{{if $i}}, {{end}}relops run {{if eq $tool.Channel "preview"}}-b {{end}}{{$alias}}{{end}}</span>{{end}}<code id="tool-{{.ID}}">{{.Command}}</code></div><button type="button" data-copy="#tool-{{.ID}}">copy</button></div>
          {{if .Note}}<p class="prose usage-note">{{.Note}}</p>{{end}}
        </li>
{{end}}
      </ul>{{end}}
{{if .Standard}}{{template "list" .Standard}}{{end}}
{{if .Preview}}
      <div class="tool-preview-intro">
        <h3 class="label">Tools in beta</h3>
        <p class="prose">These tools use their owners’ designated beta targets. Try them out and share feedback in Slack’s #relops channel.</p>
      </div>
      {{template "list" .Preview}}
{{end}}`)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, groups); err != nil {
		return "", err
	}
	return out.String(), nil
}
