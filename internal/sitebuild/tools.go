package sitebuild

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"os"
	"strings"
)

// This catalog prototypes website discovery; it is not a launcher registry.
type tool struct {
	ID, Name, Description, Owner, Repo, Path, Ref string
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
	words := []string{"relops"}
	if t.Owner != "" && t.Owner != "mozilla-platform-ops" {
		words = append(words, "--owner", t.Owner)
	}
	if t.Ref != "" {
		words = append(words, "--ref", t.Ref)
	}
	target := t.Repo
	if t.Path != "" {
		target += "/" + t.Path
	}
	words = append(words, target)
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
	var catalog struct{ Tools []tool }
	if err := json.Unmarshal(data, &catalog); err != nil {
		return nil, fmt.Errorf("parse tools catalog: %w", err)
	}
	for _, t := range catalog.Tools {
		if t.Channel != "mainline" && t.Channel != "preview" && t.Channel != "release" {
			return nil, fmt.Errorf("tool %q: channel must be mainline, preview, or release", t.ID)
		}
		if (t.Channel == "release" || t.Channel == "preview") && t.Ref == "" {
			return nil, fmt.Errorf("tool %q: %s channel requires an explicit ref", t.ID, t.Channel)
		}
	}
	return catalog.Tools, nil
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
        <p class="prose">These tools use development branches. Try them out and share feedback in Slack’s #relops channel.</p>
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
