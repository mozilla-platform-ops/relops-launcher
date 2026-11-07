package launcher

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"path"
	"regexp"
	"strings"

	"github.com/mozilla-platform-ops/relops-launcher/internal/catalog"
)

const defaultOwner = "mozilla-platform-ops"

type options struct {
	owner, ref, runner, cwd, repo, path          string
	commandName                                  string
	catalogName, catalogChannel, catalogRevision string
	sourceTarget                                 bool
	cwdSet                                       bool
	args                                         []string
	offline, verbose, refresh                    bool
}

var repoName = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9._-]*$`)
var ownerName = regexp.MustCompile(`^[A-Za-z0-9]([A-Za-z0-9-]{0,37}[A-Za-z0-9])?$`)
var commitID = regexp.MustCompile(`^[0-9a-fA-F]{40}$`)

func parse(args []string, out io.Writer, version string) (options, bool, error) {
	return parseMode(args, out, version, "root")
}

func parseMode(args []string, out io.Writer, version, mode string) (options, bool, error) {
	o := options{sourceTarget: mode == "repo"}
	var beta bool
	fs := flag.NewFlagSet("relops", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	fs.StringVar(&o.owner, "owner", defaultOwner, "GitHub owner")
	fs.StringVar(&o.ref, "ref", "", "revision")
	fs.StringVar(&o.runner, "runner", "auto", "runner")
	fs.StringVar(&o.commandName, "command", "", "console entry point")
	fs.StringVar(&o.cwd, "cwd", "caller", "working directory")
	fs.BoolVar(&o.offline, "offline", false, "offline")
	fs.BoolVar(&o.refresh, "refresh", false, "refresh release")
	fs.BoolVar(&o.verbose, "verbose", false, "verbose")
	fs.BoolVar(&o.verbose, "v", false, "verbose")
	if mode == "catalog" {
		fs.BoolVar(&beta, "beta", false, "select beta target")
		fs.BoolVar(&beta, "b", false, "select beta target")
	}
	showVersion := fs.Bool("version", false, "version")
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			err = writeHelp(out, mode, version)
			return o, true, err
		}
		return o, false, err
	}
	if *showVersion {
		_, err := fmt.Fprintf(out, "relops %s\n", version)
		return o, true, err
	}
	if fs.NArg() == 0 {
		if (mode == "root" || mode == "repo") && len(args) == 0 {
			err := writeHelp(out, mode, version)
			return o, true, err
		}
		if mode == "catalog" {
			if len(args) == 0 {
				err := catalogShort(out)
				return o, true, err
			}
			return o, false, errors.New("expected a catalog tool name; use relops run --help")
		}
		if mode == "repo" {
			return o, false, errors.New("expected <repo> or <repo>/<path>; use relops repo --help")
		}
		return o, false, errors.New("expected a command; use relops --help")
	}
	if mode == "root" {
		prefix := args[:len(args)-fs.NArg()]
		rest := fs.Args()[1:]
		switch fs.Arg(0) {
		case "run", "r":
			return parseMode(append(append([]string{}, prefix...), rest...), out, version, "catalog")
		case "repo":
			return parseMode(append(append([]string{}, prefix...), rest...), out, version, "repo")
		case "info":
			if len(prefix) != 0 {
				return o, false, errors.New("launcher options do not apply to info; use relops info NAME")
			}
			if len(rest) == 1 && (rest[0] == "--help" || rest[0] == "-h") {
				err := writeHelp(out, "catalog-info", version)
				return o, true, err
			}
			err := catalogDisplay(append([]string{"info"}, rest...), out)
			return o, err == nil, err
		case "catalog", "c":
			if len(rest) > 0 && rest[0] == "launch" {
				return parseMode(append(append([]string{}, prefix...), rest[1:]...), out, version, "catalog")
			}
			if len(prefix) != 0 {
				return o, false, errors.New("launcher options only apply to catalog launch")
			}
			if len(rest) == 0 {
				err := catalogDisplay([]string{"list"}, out)
				return o, err == nil, err
			}
			if rest[0] == "--help" || rest[0] == "-h" {
				err := writeHelp(out, "catalog-index", version)
				return o, true, err
			}
			if (rest[0] == "list" || rest[0] == "ls" || rest[0] == "info") && len(rest) == 2 && (rest[1] == "--help" || rest[1] == "-h") {
				err := writeHelp(out, "catalog-"+rest[0], version)
				return o, true, err
			}
			err := catalogDisplay(rest, out)
			return o, err == nil, err
		}
		target := fs.Arg(0)
		replacement := append([]string{"relops", "repo"}, prefix...)
		if target == "fleetbench" {
			replacement = append(append([]string{"relops", "run"}, prefix...), "fb")
			replacement = append(replacement, rest...)
			return o, false, fmt.Errorf("release tools require run; use: %s", commandHint(replacement))
		}
		replacement = append(replacement, fs.Args()...)
		return o, false, fmt.Errorf("unknown command %q; source targets require repo; use: %s", target, commandHint(replacement))
	}
	fs.Visit(func(f *flag.Flag) {
		if f.Name == "cwd" {
			o.cwdSet = true
		}
	})
	targetName := fs.Arg(0)
	if mode == "catalog" {
		c, err := catalog.Bundled()
		if err != nil {
			return o, false, err
		}
		t, err := c.Lookup(targetName)
		if err != nil {
			return o, false, err
		}
		target, channel, err := t.Select(beta)
		if err != nil {
			return o, false, err
		}
		explicit := map[string]bool{}
		fs.Visit(func(f *flag.Flag) { explicit[f.Name] = true })
		if explicit["owner"] {
			return o, false, errors.New("--owner is not supported for catalog targets; use relops repo")
		}
		if beta && explicit["ref"] {
			return o, false, errors.New("--ref cannot be combined with --beta/-b")
		}
		o.owner = target.Owner
		if !explicit["ref"] {
			o.ref = target.Ref
		}
		if !explicit["runner"] && target.Runner != "" {
			o.runner = target.Runner
		}
		if !explicit["command"] {
			o.commandName = target.Command
		}
		if !explicit["cwd"] && target.Cwd != "" {
			o.cwd, o.cwdSet = target.Cwd, true
		}
		targetName = target.Repo
		if target.Path != "" {
			targetName += "/" + target.Path
		}
		o.sourceTarget = target.Kind == "source"
		o.catalogName, o.catalogChannel, o.catalogRevision = t.ID, channel, c.Revision
	}
	var hasPath bool
	o.repo, o.path, hasPath = strings.Cut(targetName, "/")
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
	if o.refresh && (o.offline || !isReleaseTarget(o)) {
		return o, false, errors.New("--refresh requires an online release target and cannot be combined with --offline")
	}
	if isReleaseTarget(o) {
		var invalid string
		fs.Visit(func(f *flag.Flag) {
			if f.Name == "ref" || f.Name == "runner" || f.Name == "command" {
				invalid = "--" + f.Name
			}
		})
		if invalid != "" {
			return o, false, fmt.Errorf("%s is not supported for release target %s; use relops run %s for the latest stable binary, or relops repo %s for source", invalid, o.repo, o.catalogName, o.repo)
		}
		if o.cwd == "repo" {
			return o, false, errors.New("--cwd repo is not supported for release targets; binaries run in the caller's directory")
		}
	}
	return o, false, nil
}

// Quote each argument so migration hints can be copied without changing tool input.
func commandHint(args []string) string {
	words := make([]string, len(args))
	for i, arg := range args {
		if arg != "" && strings.Trim(arg, "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789_./:@%+=,-") == "" {
			words[i] = arg
		} else {
			words[i] = "'" + strings.ReplaceAll(arg, "'", "'\"'\"'") + "'"
		}
	}
	return strings.Join(words, " ")
}
