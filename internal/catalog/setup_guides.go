package catalog

import "fmt"

// SetupGuide is shared documentation, not a runtime prerequisite check.
type SetupGuide struct {
	ID, Title, Summary, File string
}

var setupGuides = []SetupGuide{
	{ID: "taskcluster-credentials", Title: "Taskcluster credentials", File: "~/.tc_token",
		Summary: "Create a Taskcluster client, save its credentials, and choose scopes for the tools you use."},
}

func (g SetupGuide) Path() string { return "requirements/" + g.ID + "/" }

func (g SetupGuide) URL() string {
	return "https://relops-launcher.quick.mozilla.cloud/" + g.Path()
}

func AllSetupGuides() []SetupGuide { return append([]SetupGuide(nil), setupGuides...) }

func LookupSetupGuide(id string) (SetupGuide, error) {
	for _, guide := range setupGuides {
		if guide.ID == id {
			return guide, nil
		}
	}
	return SetupGuide{}, fmt.Errorf("unknown setup guide %q", id)
}
