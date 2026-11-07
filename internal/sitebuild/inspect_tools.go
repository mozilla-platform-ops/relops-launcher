package sitebuild

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path"
	"strings"
	"time"
)

//go:embed inspect_python.py
var pythonInspector string

//go:embed inspect_shell.py
var shellInspector string

type pythonInput struct {
	Path         string   `json:"path"`
	Source       string   `json:"source"`
	DataFiles    []string `json:"data_files"`
	TemporaryCwd bool     `json:"temporary_cwd"`
}
type finding struct {
	Line   int    `json:"line"`
	Rule   string `json:"rule"`
	Detail string `json:"detail"`
}

// InspectTools fetches catalog revisions and reports advisory static findings. It never runs catalog code.
func InspectTools(ctx context.Context, catalogPath string, out io.Writer) error {
	return inspectTools(ctx, catalogPath, out, func(t tool) string { return fmt.Sprintf("git@github.com:%s/%s.git", t.Owner, t.Repo) })
}

func inspectTools(ctx context.Context, catalogPath string, out io.Writer, remote func(tool) string) error {
	entries, err := readTools(catalogPath)
	if err != nil {
		return err
	}
	var failures []error
	for _, t := range entries {
		if t.Kind != "source" {
			fmt.Fprintf(out, "SKIP %s: %s targets are not inspected\n", t.ID, t.Kind)
			continue
		}
		toolCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		var results []string
		sha, err := withCheckedTool(toolCtx, t, remote(t), func(dir, sha, kind string) error {
			var err error
			results, err = inspectSources(toolCtx, dir, sha, kind, t)
			return err
		})
		cancel()
		if err != nil {
			fmt.Fprintf(out, "FAIL %s: %v\n", t.ID, err)
			failures = append(failures, fmt.Errorf("%s: %w", t.ID, err))
			continue
		}
		fmt.Fprintf(out, "INSPECT %s: %s/%s commit=%s\n", t.ID, t.Owner, t.Repo, sha)
		for _, result := range results {
			fmt.Fprintln(out, result)
		}
	}
	return errors.Join(failures...)
}

func inspectSources(ctx context.Context, dir, sha, kind string, t tool) ([]string, error) {
	git := func(args ...string) ([]byte, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=/dev/null", "-C", dir}, args...)...)
		var stderr bytes.Buffer
		cmd.Stderr = &stderr
		data, err := cmd.Output()
		if err != nil {
			return nil, fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(stderr.String()))
		}
		return data, nil
	}
	var files []string
	if kind == "tree" {
		args := []string{"ls-tree", "-r", "-z", "--name-only", sha}
		if t.Path != "" {
			args = append(args, "--", t.Path)
		}
		data, err := git(args...)
		if err != nil {
			return nil, err
		}
		for _, file := range strings.Split(string(data), "\x00") {
			if strings.HasSuffix(strings.ToLower(file), ".py") || strings.HasSuffix(strings.ToLower(file), ".sh") {
				files = append(files, file)
			}
		}
	} else {
		data, err := git("show", sha+":"+t.Path)
		if err != nil {
			return nil, err
		}
		first, _, _ := strings.Cut(string(data), "\n")
		if strings.HasSuffix(strings.ToLower(t.Path), ".py") || strings.HasSuffix(strings.ToLower(t.Path), ".sh") || (strings.HasPrefix(first, "#!") && (strings.Contains(first, "python") || shellShebang(first))) {
			files = append(files, t.Path)
		}
	}
	if len(files) == 0 {
		return []string{"  SKIP: no Python or shell files selected (other languages are not yet inspected)"}, nil
	}
	var pythonInputs, shellInputs []pythonInput
	for _, file := range files {
		data, err := git("show", sha+":"+file)
		if err != nil {
			return nil, err
		}
		if len(data) > 2*1024*1024 {
			return nil, fmt.Errorf("source file %s exceeds 2 MiB", file)
		}
		input := pythonInput{Path: file, Source: string(data), DataFiles: t.DataFiles, TemporaryCwd: t.Cwd == "repo" || (kind == "tree" && t.Cwd != "caller")}
		first, _, _ := strings.Cut(string(data), "\n")
		if strings.HasSuffix(strings.ToLower(file), ".sh") || (!strings.HasSuffix(strings.ToLower(file), ".py") && shellShebang(first)) {
			shellInputs = append(shellInputs, input)
		} else {
			pythonInputs = append(pythonInputs, input)
		}
	}
	var results []string
	for _, group := range []struct {
		name, script string
		inputs       []pythonInput
	}{{"Python", pythonInspector, pythonInputs}, {"Shell", shellInspector, shellInputs}} {
		if len(group.inputs) == 0 {
			continue
		}
		lines, err := runInspector(ctx, group.name, group.script, group.inputs)
		if err != nil {
			return nil, err
		}
		results = append(results, lines...)
	}
	return results, nil
}

func shellShebang(first string) bool {
	if !strings.HasPrefix(first, "#!") {
		return false
	}
	fields := strings.Fields(strings.TrimPrefix(first, "#!"))
	for _, field := range fields {
		switch path.Base(field) {
		case "sh", "bash", "dash", "zsh", "ksh":
			return true
		}
	}
	return false
}

func runInspector(ctx context.Context, name, script string, inputs []pythonInput) ([]string, error) {
	payload, err := json.Marshal(inputs)
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, "python3", "-I", "-c", script)
	cmd.Stdin = bytes.NewReader(payload)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	data, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("%s inspector: %w: %s", name, err, strings.TrimSpace(stderr.String()))
	}
	var findings [][]finding
	if err := json.Unmarshal(data, &findings); err != nil || len(findings) != len(inputs) {
		return nil, fmt.Errorf("invalid %s inspector output: %v", name, err)
	}
	results := []string{fmt.Sprintf("  %s: scanned %d file(s)", name, len(inputs))}
	for i, group := range findings {
		for _, f := range group {
			results = append(results, fmt.Sprintf("  WARN %s:%d [%s] %s", path.Clean(inputs[i].Path), f.Line, f.Rule, f.Detail))
		}
	}
	return results, nil
}
