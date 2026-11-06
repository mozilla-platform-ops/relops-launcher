package launcher

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"
)

const usage = `Usage: relops [options] <repo>[/<path>] [tool-arguments...]

Run a tool from a GitHub repository using existing Git SSH access.
Launcher options must precede the target. All following arguments go to the tool.

Options:
  --owner VALUE     GitHub user or organization (default: mozilla-platform-ops)
  --ref VALUE       Pushed branch, tag, or full commit SHA (default: default branch)
  --runner VALUE    auto, python3, bash, or sh (default: auto)
  --command VALUE   Select a uv project's console entry point
  --cwd VALUE       repo or caller (default: caller for scripts, project for projects)
  --offline         Use cached refs and objects; do not access the network
  --verbose         Show cache and worktree diagnostics on stderr
  --version         Print launcher version
  --help            Show this help

Examples:
  relops --ref my-feature relops-infra/path/to/check.sh --verbose
  relops --cwd caller --runner python3 repo/check.py ./hosts.csv
  relops fleetroll_mvp host-monitor configs/host-lists/linux/all.list
  relops --command fr fleetroll_mvp -h

Online runs fetch before executing. Each run uses a temporary checkout that is
removed on exit. Save outputs outside that checkout. Tools run with your normal
permissions; relops is not a sandbox. Auto detects shebangs and Python scripts;
Python projects with a Pipfile use installed Pipenv to sync locked dependencies.
Projects with pyproject.toml and uv.lock use installed uv. A repository or project
directory target runs its console entry point matching the project name, or its
sole distinct entry point. Use --command when selection is ambiguous.
An explicit --runner bypasses project-environment detection.
`

const defaultOwner = "mozilla-platform-ops"

type options struct {
	owner, ref, runner, cwd, repo, path string
	commandName                         string
	cwdSet                              bool
	args                                []string
	offline, verbose                    bool
}

var repoName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
var ownerName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
var commitID = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func parse(args []string, out io.Writer, version string) (options, bool, error) {
	o := options{}
	fs := flag.NewFlagSet("relops", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.owner, "owner", defaultOwner, "GitHub owner")
	fs.StringVar(&o.ref, "ref", "", "revision")
	fs.StringVar(&o.runner, "runner", "auto", "runner")
	fs.StringVar(&o.commandName, "command", "", "console entry point")
	fs.StringVar(&o.cwd, "cwd", "caller", "working directory")
	fs.BoolVar(&o.offline, "offline", false, "offline")
	fs.BoolVar(&o.verbose, "verbose", false, "verbose")
	showVersion := fs.Bool("version", false, "version")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			_, err = io.WriteString(out, usage)
			return o, true, err
		}
		return o, false, err
	}
	if *showVersion {
		_, err := fmt.Fprintf(out, "relops %s\n", version)
		return o, true, err
	}
	if fs.NArg() == 0 {
		return o, false, errors.New("expected <repo> or <repo>/<path>; use relops --help")
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "cwd" {
			o.cwdSet = true
		}
	})
	var hasPath bool
	o.repo, o.path, hasPath = strings.Cut(fs.Arg(0), "/")
	if !ownerName.MatchString(o.owner) {
		return o, false, errors.New("--owner must be a GitHub user or organization name (letters, digits, and internal hyphens; up to 39 characters)")
	}
	o.owner = strings.ToLower(o.owner)
	if !repoName.MatchString(o.repo) || (hasPath && o.path == "") {
		return o, false, errors.New("target must be <repo> or <repo>/<path>; select a GitHub user or organization with --owner")
	}
	o.repo = strings.ToLower(o.repo)
	if path.IsAbs(o.path) || strings.ContainsAny(o.path, "\\\x00") {
		return o, false, errors.New("script path must be a relative repository path")
	}
	for _, part := range strings.Split(o.path, "/") {
		if part == ".." || part == ".git" {
			return o, false, errors.New("script path must not contain '..' or '.git' components")
		}
	}
	if o.runner != "auto" && o.runner != "python3" && o.runner != "bash" && o.runner != "sh" {
		return o, false, fmt.Errorf("unsupported runner %q; use auto, python3, bash, or sh", o.runner)
	}
	if o.cwd != "repo" && o.cwd != "caller" {
		return o, false, errors.New("--cwd must be repo or caller")
	}
	if o.commandName != "" && (!repoName.MatchString(o.commandName) || o.commandName == "." || o.commandName == "..") {
		return o, false, errors.New("--command must be a console entry point name, not a path")
	}
	if strings.HasPrefix(o.ref, "-") || strings.ContainsAny(o.ref, "\x00\r\n") {
		return o, false, errors.New("invalid revision")
	}
	o.args = fs.Args()[1:]
	return o, false, nil
}
