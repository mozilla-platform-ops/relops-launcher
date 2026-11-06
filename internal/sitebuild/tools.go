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
	case "mainline":
		return "Mainline"
	case "preview":
		return "Preview"
	case "release":
		return "Release · " + t.Ref
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

func renderTools(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	var catalog struct{ Tools []tool }
	if err := json.Unmarshal(data, &catalog); err != nil {
		return "", fmt.Errorf("parse tools catalog: %w", err)
	}
	for _, t := range catalog.Tools {
		if t.ChannelLabel() == "" {
			return "", fmt.Errorf("tool %q: channel must be mainline, preview, or release", t.ID)
		}
		if (t.Channel == "release" || t.Channel == "preview") && t.Ref == "" {
			return "", fmt.Errorf("tool %q: %s channel requires an explicit ref", t.ID, t.Channel)
		}
	}
	tmpl, err := template.New("tools").Parse(`{{range .Tools}}
        <li class="tool-row">
          <div class="tool-row__heading">
            <h3>{{.Name}}</h3>
            <span class="badge">{{.ChannelLabel}}</span>
            <a href="{{.Documentation}}" aria-label="{{.Name}} documentation">docs ↗</a>
          </div>
          <p class="prose">{{.Description}}</p>
          <div class="cmd cmd--wrap"><code id="tool-{{.ID}}">{{.Command}}</code><button type="button" data-copy="#tool-{{.ID}}">copy</button></div>
          {{if .Note}}<p class="prose usage-note">{{.Note}}</p>{{end}}
        </li>
{{end}}`)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	if err := tmpl.Execute(&out, catalog); err != nil {
		return "", err
	}
	return out.String(), nil
}
