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

const usage = `Usage: relops [options] <repo>/<path> [tool-arguments...]

Run a tool from a mozilla-platform-ops repository using existing Git SSH access.
Launcher options must precede the target. All following arguments go to the tool.

Options:
  --ref VALUE       Pushed branch, tag, or full commit SHA (default: default branch)
  --runner VALUE    auto, python3, bash, or sh (default: auto)
  --cwd VALUE       repo or caller (default: repo)
  --offline         Use cached refs and objects; do not access the network
  --verbose         Show cache and worktree diagnostics on stderr
  --version         Print launcher version
  --help            Show this help

Examples:
  relops --ref my-feature relops-infra/path/to/check.sh --verbose
  relops --cwd caller --runner python3 repo/check.py ./hosts.csv

Online runs fetch before executing. Each run uses a temporary checkout that is
removed on exit. Save outputs outside that checkout. Tools run with your normal
permissions; relops is not a sandbox and does not install dependencies.
`

type options struct {
	ref, runner, cwd, repo, path string
	args                         []string
	offline, verbose             bool
}

var repoName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
var commitID = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func parse(args []string, out io.Writer, version string) (options, bool, error) {
	o := options{}
	fs := flag.NewFlagSet("relops", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.ref, "ref", "", "revision")
	fs.StringVar(&o.runner, "runner", "auto", "runner")
	fs.StringVar(&o.cwd, "cwd", "repo", "working directory")
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
		return o, false, errors.New("expected <repo>/<path>; use relops --help")
	}
	o.repo, o.path, _ = strings.Cut(fs.Arg(0), "/")
	if !repoName.MatchString(o.repo) || o.path == "" {
		return o, false, errors.New("target must be <repo>/<path> within mozilla-platform-ops")
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
	if strings.HasPrefix(o.ref, "-") || strings.ContainsAny(o.ref, "\x00\r\n") {
		return o, false, errors.New("invalid revision")
	}
	o.args = fs.Args()[1:]
	return o, false, nil
}
