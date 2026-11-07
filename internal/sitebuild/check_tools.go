package sitebuild

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

// CheckTools checks pushed refs and paths without executing any catalog tools.
// Like the launcher, it uses the caller's existing Git SSH access.
func CheckTools(ctx context.Context, path string, out io.Writer) error {
	return checkTools(ctx, path, out, func(t tool) string {
		return fmt.Sprintf("git@github.com:%s/%s.git", t.Owner, t.Repo)
	})
}

var catalogRepoName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
var catalogCommit = regexp.MustCompile(`^[a-fA-F0-9]{40}$`)

func checkTools(ctx context.Context, path string, out io.Writer, remote func(tool) string) error {
	entries, err := readTools(path)
	if err != nil {
		return err
	}
	var failures []error
	for _, t := range entries {
		if t.Kind == "web" {
			fmt.Fprintf(out, "OK %s URL=%s (not checked)\n", t.ID, t.URL)
			continue
		}
		if t.Owner == "" {
			t.Owner = "mozilla-platform-ops"
		}
		ref := t.Ref
		if ref == "" {
			ref = "HEAD (default branch)"
		}
		label := fmt.Sprintf("%s: %s/%s ref=%s path=%s", t.ID, t.Owner, t.Repo, ref, t.Path)
		toolCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		var sha string
		var err error
		if t.Kind == "release" {
			// The launcher checks current assets and checksums when executing releases.
			sha = "latest stable release (assets verified at launch)"
		} else {
			sha, err = checkTool(toolCtx, t, remote(t))
		}
		cancel()
		if err != nil {
			failure := fmt.Errorf("%s: %w", label, err)
			fmt.Fprintf(out, "FAIL %v\n", failure)
			failures = append(failures, failure)
		} else {
			fmt.Fprintf(out, "OK %s commit=%s\n", label, sha)
		}
	}
	return errors.Join(failures...)
}

func checkTool(ctx context.Context, t tool, remote string) (string, error) {
	return withCheckedTool(ctx, t, remote, nil)
}

func withCheckedTool(ctx context.Context, t tool, remote string, inspect func(string, string, string) error) (string, error) {
	if !catalogRepoName.MatchString(t.Owner) || !catalogRepoName.MatchString(t.Repo) {
		return "", errors.New("invalid repository owner or name")
	}
	if strings.HasPrefix(t.Path, "/") || strings.ContainsAny(t.Path, "\\\x00") {
		return "", errors.New("invalid repository path")
	}
	for _, part := range strings.Split(t.Path, "/") {
		if part == ".." || part == ".git" {
			return "", errors.New("invalid repository path")
		}
	}
	dir, err := os.MkdirTemp("", "relops-catalog-check-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(dir)
	git := func(args ...string) (string, error) {
		cmd := exec.CommandContext(ctx, "git", append([]string{"-c", "core.hooksPath=/dev/null", "-C", dir}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_SSH_COMMAND=ssh -o BatchMode=yes -o ConnectTimeout=15")
		data, err := cmd.CombinedOutput()
		if err != nil {
			return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(data)))
		}
		return strings.TrimSpace(string(data)), nil
	}
	if _, err := git("init", "--bare"); err != nil {
		return "", err
	}
	listing, err := git("ls-remote", "--symref", remote)
	if err != nil {
		return "", err
	}
	refs := make(map[string]string)
	for _, line := range strings.Split(listing, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && catalogCommit.MatchString(fields[0]) {
			refs[fields[1]] = fields[0]
		}
	}
	sha, err := resolveCatalogRef(t.Ref, refs)
	if err != nil {
		return "", err
	}
	if _, err := git("fetch", "--depth=1", "--no-tags", remote, sha); err != nil {
		return "", err
	}
	sha, err = git("rev-parse", "--verify", "FETCH_HEAD^{commit}")
	if err != nil {
		return "", err
	}
	kind, err := git("cat-file", "-t", sha+":"+t.Path)
	if err != nil {
		return "", fmt.Errorf("target does not exist at resolved commit: %w", err)
	}
	if kind != "blob" && kind != "tree" {
		return "", fmt.Errorf("target is %s, expected a script or project directory", kind)
	}
	if kind == "tree" {
		for _, file := range []string{"pyproject.toml", "uv.lock"} {
			prefix := strings.TrimSuffix(t.Path, "/")
			if prefix != "" {
				prefix += "/"
			}
			if kind, err := git("cat-file", "-t", sha+":"+prefix+file); err != nil || kind != "blob" {
				return "", fmt.Errorf("project target requires committed %s", file)
			}
		}
	}
	if inspect != nil {
		if err := inspect(dir, sha, kind); err != nil {
			return "", err
		}
	}
	return sha, nil
}

func resolveCatalogRef(ref string, refs map[string]string) (string, error) {
	if catalogCommit.MatchString(ref) {
		return ref, nil
	}
	candidates := []string{ref}
	if ref == "" {
		candidates = []string{"HEAD"}
	} else if !strings.HasPrefix(ref, "refs/") {
		candidates = []string{"refs/heads/" + ref, "refs/tags/" + ref}
	} else if !strings.HasPrefix(ref, "refs/heads/") && !strings.HasPrefix(ref, "refs/tags/") {
		return "", errors.New("only refs/heads/... and refs/tags/... are supported")
	}
	var sha string
	for _, candidate := range candidates {
		if value := refs[candidate]; value != "" {
			if sha != "" {
				return "", fmt.Errorf("ambiguous ref %q; specify refs/heads/ or refs/tags/", ref)
			}
			sha = value
			if peeled := refs[candidate+"^{}"]; peeled != "" {
				sha = peeled
			}
		}
	}
	if sha == "" {
		return "", fmt.Errorf("ref %q not found (empty ref selects the default branch)", ref)
	}
	return sha, nil
}
