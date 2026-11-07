package catalog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBundledLookupAndChannels(t *testing.T) {
	c, err := Bundled()
	if err != nil {
		t.Fatal(err)
	}
	fr, err := c.Lookup("fr")
	if err != nil || fr.ID != "fleetroll" {
		t.Fatalf("alias: %+v %v", fr, err)
	}
	if target, _, err := fr.Select(false); err != nil || target.Repo != "fleetroll_mvp" {
		t.Fatalf("default: %+v %v", target, err)
	}
	if _, _, err := fr.Select(true); err == nil {
		t.Fatal("missing beta fell back")
	}
	preview, err := c.Lookup("replay-tc-pool")
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := preview.Select(false); err == nil {
		t.Fatal("beta-only tool acquired a default")
	}
	if target, channel, err := preview.Select(true); err != nil || target.Owner != "aerickson" || channel != "beta" {
		t.Fatalf("beta: %+v %s %v", target, channel, err)
	}
}

func TestValidation(t *testing.T) {
	for _, tc := range []struct {
		name string
		edit func(*Catalog)
	}{
		{"schema", func(c *Catalog) { c.SchemaVersion = 2 }},
		{"revision", func(c *Catalog) { c.Revision = "" }},
		{"duplicate tool", func(c *Catalog) { c.Tools = append(c.Tools, c.Tools[0]) }},
		{"alias collision", func(c *Catalog) { c.Tools[0].Aliases = []string{"second"} }},
		{"duplicate alias", func(c *Catalog) { c.Tools[0].Aliases = []string{"first"} }},
		{"invalid name", func(c *Catalog) { c.Tools[0].ID = "../escape" }},
		{"no target", func(c *Catalog) { c.Tools[0].Default = nil }},
		{"path traversal", func(c *Catalog) { c.Tools[0].Default.Path = "../escape" }},
		{"absolute path", func(c *Catalog) { c.Tools[0].Default.Path = "/escape" }},
		{"invalid owner", func(c *Catalog) { c.Tools[0].Default.Owner = "-bad" }},
		{"invalid ref", func(c *Catalog) { c.Tools[0].Default.Ref = "-bad" }},
		{"invalid runner", func(c *Catalog) { c.Tools[0].Default.Runner = "guess" }},
		{"invalid cwd", func(c *Catalog) { c.Tools[0].Default.Cwd = "elsewhere" }},
		{"invalid command", func(c *Catalog) { c.Tools[0].Default.Command = "../bad" }},
		{"unsupported release", func(c *Catalog) { c.Tools[0].Default.Kind = "release" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := Catalog{SchemaVersion: 1, Revision: "test", Tools: []Tool{
				{ID: "first", Name: "First", Description: "First tool", Default: &Target{Kind: "source", Owner: "owner", Repo: "repo"}},
				{ID: "second", Name: "Second", Description: "Second tool", Beta: &Target{Kind: "source", Owner: "other", Repo: "another"}},
			}}
			tc.edit(&c)
			data, err := json.Marshal(c)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := Decode(data); err == nil {
				t.Fatal("invalid catalog accepted")
			}
		})
	}
	for _, suffix := range []string{` {}`, ` garbage`} {
		c, _ := Bundled()
		data, _ := json.Marshal(c)
		if _, err := Decode(append(data, suffix...)); err == nil {
			t.Fatal("accepted trailing data")
		}
	}
	c, _ := Bundled()
	data, _ := json.Marshal(c)
	data = []byte(strings.Replace(string(data), `"schema_version":1`, `"schema_version":1,"unknown":true`, 1))
	if _, err := Decode(data); err == nil {
		t.Fatal("accepted unknown schema field")
	}
}

func TestSameToolCanHaveIndependentTargets(t *testing.T) {
	c := Catalog{SchemaVersion: 1, Revision: "test", Tools: []Tool{{ID: "list", Aliases: []string{"beta"}, Name: "Example", Description: "Independent channels", Default: &Target{Kind: "source", Owner: "org", Repo: "main", Path: "tool.sh"}, Beta: &Target{Kind: "source", Owner: "person", Repo: "prototype", Path: "new/tool.sh", Ref: "preview"}}}}
	data, _ := json.Marshal(c)
	decoded, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	tool, err := decoded.Lookup("beta")
	if err != nil {
		t.Fatal(err)
	}
	target, _, err := tool.Select(true)
	if err != nil || target.Owner != "person" || target.Repo != "prototype" || target.Path != "new/tool.sh" {
		t.Fatalf("beta target: %+v %v", target, err)
	}
}
