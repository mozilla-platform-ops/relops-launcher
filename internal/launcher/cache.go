package launcher

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type checkout struct {
	repo, parent, dir, sha, ref string
}

func (a *app) git(ctx context.Context, dir string, args ...string) (string, error) {
	// Never run checkout hooks or recurse into submodules. LFS stays unhydrated.
	base := []string{"-c", "core.hooksPath=/dev/null", "-c", "core.fsmonitor=false", "-c", "submodule.recurse=false"}
	cmd := exec.CommandContext(ctx, "git", append(base, args...)...)
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	cmd.Env = replaceEnv(os.Environ(), "GIT_LFS_SKIP_SMUDGE", "1")
	// Do not inherit Git repository-selection variables from the caller's checkout.
	for _, key := range []string{"GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_INDEX_FILE", "GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES"} {
		cmd.Env = withoutEnv(cmd.Env, key)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	cmd.WaitDelay = 2 * time.Second
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("git %s: %w\n%s", strings.Join(args, " "), err, strings.TrimSpace(stderr.String()))
	}
	return strings.TrimSpace(stdout.String()), nil
}

func withoutEnv(env []string, key string) []string {
	result := make([]string, 0, len(env))
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return result
}

func (a *app) prepare(ctx context.Context, o options) (checkout, error) {
	c := checkout{}
	for _, part := range []string{"", "repos", "locks", "runs"} {
		dir := filepath.Join(a.cache, part)
		if err := os.MkdirAll(dir, 0700); err != nil {
			return c, err
		}
		if err := os.Chmod(dir, 0700); err != nil {
			return c, err
		}
	}
	key := strings.ToLower(o.repo)
	unlock, err := lock(ctx, filepath.Join(a.cache, "locks", key+".lock"), a.lockTimeout)
	if err != nil {
		return c, err
	}
	defer unlock()
	c.repo = filepath.Join(a.cache, "repos", key+".git")
	_, err = os.Stat(c.repo)
	if errors.Is(err, os.ErrNotExist) {
		if o.offline {
			return c, fmt.Errorf("no cached repository for %s; run once online first", o.repo)
		}
		if err := a.initialize(ctx, c.repo, o.repo); err != nil {
			return c, err
		}
	} else if err != nil {
		return c, err
	} else {
		bare, err := a.git(ctx, c.repo, "rev-parse", "--is-bare-repository")
		if err != nil || bare != "true" {
			return c, fmt.Errorf("invalid repository cache at %s; move it aside and retry", c.repo)
		}
		// Read the stored URL rather than get-url's expanded insteadOf rewrite.
		remote, err := a.git(ctx, c.repo, "config", "--get", "remote.origin.url")
		if err != nil || remote != a.remote(o.repo) {
			return c, fmt.Errorf("unexpected cache remote at %s; move it aside and retry", c.repo)
		}
		if !o.offline {
			if err := a.refresh(ctx, c.repo); err != nil {
				return c, fmt.Errorf("refresh %s: %w; use --offline explicitly to run cached code", a.remote(o.repo), err)
			}
		}
	}
	c.sha, c.ref, err = a.resolve(ctx, c.repo, o)
	if err != nil {
		return c, err
	}
	c.parent, err = os.MkdirTemp(filepath.Join(a.cache, "runs"), key+"-")
	if err != nil {
		return c, err
	}
	c.dir = filepath.Join(c.parent, "checkout")
	if _, err := a.git(ctx, c.repo, "worktree", "add", "--detach", c.dir, c.sha); err != nil {
		// A cancelled checkout can leave partially registered worktree metadata.
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_, _ = a.git(cleanupCtx, c.repo, "worktree", "remove", "--force", "--force", c.dir)
		_ = os.RemoveAll(c.parent)
		return c, fmt.Errorf("create isolated worktree: %w", err)
	}
	return c, nil
}

func (a *app) initialize(ctx context.Context, repoPath, name string) error {
	tmp, err := os.MkdirTemp(filepath.Join(a.cache, "repos"), ".initializing-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if _, err := a.git(ctx, tmp, "init", "--bare", "."); err != nil {
		return err
	}
	if _, err := a.git(ctx, tmp, "remote", "add", "origin", a.remote(name)); err != nil {
		return err
	}
	if err := a.refresh(ctx, tmp); err != nil {
		return fmt.Errorf("fetch %s: %w; verify access with git ls-remote %s", a.remote(name), err, a.remote(name))
	}
	return os.Rename(tmp, repoPath)
}

func (a *app) refresh(ctx context.Context, repo string) error {
	head, err := a.git(ctx, repo, "ls-remote", "--symref", "origin", "HEAD")
	if err != nil {
		return err
	}
	var defaultRef string
	for _, line := range strings.Split(head, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 3 && fields[0] == "ref:" && fields[2] == "HEAD" && strings.HasPrefix(fields[1], "refs/heads/") {
			defaultRef = fields[1]
		}
	}
	if defaultRef == "" {
		return errors.New("remote does not advertise a default branch; check the repository's HEAD")
	}
	if _, err := a.git(ctx, repo, "fetch", "--atomic", "--prune", "origin", "+refs/heads/*:refs/remotes/origin/*", "+refs/tags/*:refs/tags/*"); err != nil {
		return err
	}
	// Store the default only after a successful fetch; all access holds the lock.
	return os.WriteFile(filepath.Join(repo, "relops-default-branch"), []byte(defaultRef+"\n"), 0600)
}

func (a *app) resolve(ctx context.Context, repo string, o options) (string, string, error) {
	ref := o.ref
	if ref == "" {
		data, err := os.ReadFile(filepath.Join(repo, "relops-default-branch"))
		if err != nil {
			return "", "", fmt.Errorf("read cached default branch: %w; run online to refresh", err)
		}
		ref = strings.TrimSpace(string(data))
	}
	if commitID.MatchString(ref) {
		sha := strings.ToLower(ref)
		if _, err := a.git(ctx, repo, "cat-file", "-e", sha+"^{commit}"); err != nil {
			if o.offline {
				return "", "", fmt.Errorf("commit %s is not cached; run online first", sha)
			}
			if _, err := a.git(ctx, repo, "fetch", "origin", sha); err != nil {
				return "", "", fmt.Errorf("commit %s is unavailable; push a branch containing it or check the SHA: %w", sha, err)
			}
		}
		kind, err := a.git(ctx, repo, "cat-file", "-t", sha)
		if err != nil || kind != "commit" {
			return "", "", fmt.Errorf("%s does not identify a commit", sha)
		}
		return sha, ref, nil
	}
	name := ref
	var candidates []string
	switch {
	case strings.HasPrefix(ref, "refs/heads/"):
		name = strings.TrimPrefix(ref, "refs/heads/")
		candidates = []string{"refs/remotes/origin/" + name}
	case strings.HasPrefix(ref, "refs/tags/"):
		name = strings.TrimPrefix(ref, "refs/tags/")
		candidates = []string{ref}
	case strings.HasPrefix(ref, "refs/"):
		return "", "", errors.New("only refs/heads/... and refs/tags/... are supported")
	default:
		candidates = []string{"refs/remotes/origin/" + name, "refs/tags/" + name}
	}
	if _, err := a.git(ctx, repo, "check-ref-format", "refs/heads/"+name); err != nil {
		return "", "", fmt.Errorf("invalid branch or tag %q", ref)
	}
	var sha string
	for _, candidate := range candidates {
		// Check existence separately: even a tag pointing to a blob is ambiguous
		// with a same-named branch, and must not silently select that branch.
		if _, err := a.git(ctx, repo, "show-ref", "--verify", "--", candidate); err == nil {
			if sha != "" {
				return "", "", fmt.Errorf("ambiguous ref %q; use refs/heads/%s or refs/tags/%s", ref, name, name)
			}
			sha = candidate
		}
	}
	if sha == "" {
		return "", "", fmt.Errorf("ref %q not found%s; use a pushed branch, tag, or full 40-character commit SHA", ref, offlineSuffix(o.offline))
	}
	resolved, err := a.git(ctx, repo, "rev-parse", "--verify", "--end-of-options", sha+"^{commit}")
	if err != nil {
		return "", "", fmt.Errorf("ref %q does not resolve to a commit: %w", ref, err)
	}
	return resolved, ref, nil
}

func offlineSuffix(offline bool) string {
	if offline {
		return " in the offline cache"
	}
	return ""
}

func (a *app) cleanup(c checkout) error {
	ctx, cancel := context.WithTimeout(context.Background(), a.lockTimeout+10*time.Second)
	defer cancel()
	key := strings.TrimSuffix(filepath.Base(c.repo), ".git")
	unlock, err := lock(ctx, filepath.Join(a.cache, "locks", key+".lock"), a.lockTimeout)
	if err != nil {
		return err
	}
	defer unlock()
	if _, err := a.git(ctx, c.repo, "worktree", "remove", "--force", "--force", c.dir); err != nil {
		return err
	}
	return os.Remove(c.parent)
}
