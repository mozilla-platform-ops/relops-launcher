package sitebuild

import (
	"bytes"
	"html/template"
	"os"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

// tool is the website presentation of one catalog target.
type tool struct {
	ID, Name, Description, Owner, Repo, Path, Ref string
	Shortcut, Kind                                string
	Args                                          []string
	Channel, Prerequisites, Documentation, Note   string
}

func (t tool) ChannelLabel() string {
	switch t.Channel {
	case "preview":
		return "Preview"
	case "release":
		return t.Ref
	}
	return ""
}

func (t tool) Command() string {
	words := []string{"relops", "c"}
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
			name := entry.Name
			if len(entry.Aliases) > 0 {
				name += " (" + strings.Join(entry.Aliases, ", ") + ")"
			}
			entries = append(entries, tool{
				ID: id, Shortcut: entry.ID, Name: name, Description: entry.Description,
				Owner: target.Owner, Repo: target.Repo, Path: target.Path, Ref: target.Ref,
				Kind: target.Kind,
				Args: target.Args, Channel: channel, Prerequisites: target.Prerequisites,
				Documentation: target.Documentation, Note: target.Note,
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
	tmpl, err := template.New("tools").Parse(`{{define "list"}}
      <ul class="tool-list">{{range .}}
        <li class="tool-row">
          <div class="tool-row__heading">
            <h3>{{.Name}}</h3>
            {{if .ChannelLabel}}<span class="badge">{{.ChannelLabel}}</span>{{end}}
            <a href="{{.Documentation}}" aria-label="{{.Name}} documentation">docs ↗</a>
          </div>
          <p class="prose">{{.Description}}</p>
          <div class="cmd cmd--wrap"><code id="tool-{{.ID}}">{{.Command}}</code><button type="button" data-copy="#tool-{{.ID}}">copy</button></div>
          {{if .Note}}<p class="prose usage-note">{{.Note}}</p>{{end}}
        </li>
{{end}}
      </ul>{{end}}
{{if .Standard}}{{template "list" .Standard}}{{end}}
{{if .Preview}}
      <div class="tool-preview-intro">
        <h3 class="label">Tools in preview</h3>
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
