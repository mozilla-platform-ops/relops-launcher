package launcher

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

type app struct {
	cache, caller string
	in            io.Reader
	out, errOut   io.Writer
	// Only tests substitute a local remote; production always constructs SSH URLs.
	remote      func(string, string) string
	lockTimeout time.Duration
}

type interrupted struct{ signal syscall.Signal }

func (e interrupted) Error() string { return e.signal.String() }

func Main(args []string, version string) int {
	o, done, err := parse(args, os.Stdout, version)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relops:", err)
		return 2
	}
	if done {
		return 0
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		fmt.Fprintln(os.Stderr, "relops: locate user cache:", err)
		return 1
	}
	caller, err := os.Getwd()
	if err != nil {
		fmt.Fprintln(os.Stderr, "relops: locate caller directory:", err)
		return 1
	}
	a := app{
		cache: filepath.Join(cache, "relops"), caller: caller,
		in: os.Stdin, out: os.Stdout, errOut: os.Stderr,
		remote:      githubRemote,
		lockTimeout: 30 * time.Second,
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	defer cancel(nil)
	signals := make(chan os.Signal, 4)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(signals)
	go func() {
		select {
		case sig := <-signals:
			cancel(interrupted{sig.(syscall.Signal)})
		case <-ctx.Done():
		}
	}()
	code, err := a.run(ctx, o)
	if err != nil {
		fmt.Fprintln(os.Stderr, "relops:", err)
	}
	if cause, ok := context.Cause(ctx).(interrupted); ok {
		return 128 + int(cause.signal)
	}
	return code
}

func (a *app) run(ctx context.Context, o options) (int, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return 1, errors.New("git is required; install Git and configure GitHub SSH access")
	}
	checkout, err := a.prepare(ctx, o)
	if err != nil {
		return 1, err
	}
	defer func() {
		// Cleanup must survive cancellation of the invocation.
		if err := a.cleanup(checkout); err != nil {
			fmt.Fprintf(a.errOut, "relops: cleanup failed: %v; retained worktree: %s\n", err, checkout.dir)
		}
	}()
	script, err := resolveScript(checkout.dir, o.path)
	if err != nil {
		return 1, err
	}
	fmt.Fprintf(a.errOut, "relops: %s/%s ref=%s commit=%s (%s)\n", o.owner, o.repo, checkout.ref, checkout.sha, resolutionMode(o.offline))
	command, virtualenv, err := a.command(ctx, o, checkout, script)
	if err != nil {
		return 1, err
	}
	command.Dir = a.caller
	if o.cwd == "repo" {
		command.Dir = checkout.dir
	}
	command.Env = replaceEnv(os.Environ(), "RELOPS_CALLER_CWD", a.caller)
	command.Env = replaceEnv(command.Env, "PWD", command.Dir)
	if virtualenv != "" {
		command.Env = replaceEnv(command.Env, "VIRTUAL_ENV", virtualenv)
		command.Env = replaceEnv(command.Env, "PATH", filepath.Join(virtualenv, "bin")+string(os.PathListSeparator)+os.Getenv("PATH"))
		command.Env = withoutEnv(command.Env, "PYTHONHOME")
		command.Env = withoutEnv(command.Env, "__PYVENV_LAUNCHER__")
	}
	command.Stdin, command.Stdout, command.Stderr = a.in, a.out, a.errOut
	if o.verbose {
		fmt.Fprintf(a.errOut, "relops: worktree=%s cwd=%s runner=%s\n", checkout.dir, command.Dir, o.runner)
	}
	return runTool(ctx, command)
}

func githubRemote(owner, repo string) string {
	return "git@github.com:" + owner + "/" + repo + ".git"
}

func resolutionMode(offline bool) string {
	if offline {
		return "offline cached"
	}
	return "fetched"
}

func resolveScript(root, path string) (string, error) {
	script, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(path)))
	if err != nil {
		return "", fmt.Errorf("locate tool %q: %w", path, err)
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(resolvedRoot, script)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return "", errors.New("tool symlink escapes the checkout")
	}
	info, err := os.Stat(script)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("tool must be a regular file")
	}
	return script, nil
}

func toolCommand(script, runner string, args []string) (*exec.Cmd, error) {
	if runner != "auto" {
		interpreter, err := exec.LookPath(runner)
		if err != nil {
			return nil, fmt.Errorf("runner %q is not on PATH; install it first: %w", runner, err)
		}
		// PATH may contain relative directories, while the tool runs in another cwd.
		interpreter, err = filepath.Abs(interpreter)
		if err != nil {
			return nil, err
		}
		return exec.Command(interpreter, append([]string{script}, args...)...), nil
	}
	info, err := os.Stat(script)
	if err != nil {
		return nil, err
	}
	interpreter, optional, err := shebang(script)
	if err != nil {
		return nil, err
	}
	if interpreter != "" {
		if info.Mode()&0111 != 0 {
			return exec.Command(script, args...), nil
		}
		argv := []string{}
		if optional != "" {
			// A shebang has a single optional argument; env -S can split it.
			argv = append(argv, optional)
		}
		argv = append(argv, script)
		return exec.Command(interpreter, append(argv, args...)...), nil
	}
	if strings.EqualFold(filepath.Ext(script), ".py") {
		return toolCommand(script, "python3", args)
	}
	return nil, errors.New("cannot detect a runner: add a shebang or select --runner python3, bash, or sh; .sh alone does not identify a shell")
}

func replaceEnv(env []string, key, value string) []string {
	result := make([]string, 0, len(env)+1)
	for _, entry := range env {
		if !strings.HasPrefix(entry, key+"=") {
			result = append(result, entry)
		}
	}
	return append(result, key+"="+value)
}
