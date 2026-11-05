package launcher

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

var pythonName = regexp.MustCompile(`^python([0-9]+(\.[0-9]+)*)?$`)

func shebang(script string) (string, string, error) {
	f, err := os.Open(script)
	if err != nil {
		return "", "", err
	}
	defer f.Close()
	line, err := bufio.NewReader(io.LimitReader(f, 4096)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", "", err
	}
	if !strings.HasPrefix(line, "#!") {
		return "", "", nil
	}
	line = strings.TrimSpace(strings.TrimPrefix(line, "#!"))
	interpreter, optional, _ := strings.Cut(line, " ")
	if i := strings.IndexAny(line, " \t"); i >= 0 {
		interpreter, optional = line[:i], strings.TrimSpace(line[i:])
	}
	if !filepath.IsAbs(interpreter) {
		return "", "", errors.New("shebang interpreter must be an absolute path; select --runner to override")
	}
	return interpreter, optional, nil
}

func pythonScript(script, interpreter, optional string) bool {
	if interpreter == "" {
		return strings.EqualFold(filepath.Ext(script), ".py")
	}
	if pythonName.MatchString(filepath.Base(interpreter)) {
		return true
	}
	if filepath.Base(interpreter) == "env" {
		args := strings.Fields(optional)
		if len(args) > 0 && args[0] == "-S" {
			args = args[1:]
		}
		return len(args) > 0 && pythonName.MatchString(args[0])
	}
	return false
}

func (a *app) command(ctx context.Context, o options, c checkout, script string) (*exec.Cmd, string, error) {
	if o.runner != "auto" {
		cmd, err := toolCommand(script, o.runner, o.args)
		return cmd, "", err
	}
	interpreter, optional, err := shebang(script)
	if err != nil {
		return nil, "", err
	}
	if pythonScript(script, interpreter, optional) {
		project, err := pipenvProject(c.dir, script)
		if err != nil {
			return nil, "", err
		}
		if project != "" {
			venv, err := a.pipenv(ctx, o, c, project)
			if err != nil {
				return nil, "", err
			}
			if filepath.Base(interpreter) == "env" {
				// Preserve env's shebang argument handling (including -S), with
				// the managed environment prepended to PATH by the caller.
				cmd, err := toolCommand(script, "auto", o.args)
				return cmd, venv, err
			}
			args := []string{}
			if optional != "" {
				args = append(args, optional)
			}
			args = append(args, script)
			return exec.Command(filepath.Join(venv, "bin", "python"), append(args, o.args...)...), venv, nil
		}
	}
	cmd, err := toolCommand(script, "auto", o.args)
	return cmd, "", err
}

func pipenvProject(root, script string) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	for dir := filepath.Dir(script); ; dir = filepath.Dir(dir) {
		file := filepath.Join(dir, "Pipfile")
		if _, err := os.Lstat(file); err == nil {
			rel, err := filepath.Rel(root, file)
			if err != nil {
				return "", err
			}
			if _, err := resolveScript(root, rel); err != nil {
				return "", fmt.Errorf("invalid Pipfile: %w", err)
			}
			return dir, nil
		} else if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		if dir == root {
			return "", nil
		}
		if filepath.Dir(dir) == dir {
			return "", errors.New("project search escaped the checkout")
		}
	}
}

func (a *app) pipenv(ctx context.Context, o options, c checkout, project string) (string, error) {
	pipenv, err := exec.LookPath("pipenv")
	if err != nil {
		return "", errors.New("this tool uses Pipenv, but pipenv is not on PATH; install Pipenv and retry, or use --runner python3 if you manage dependencies yourself")
	}
	pipenv, err = filepath.Abs(pipenv)
	if err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(c.dir)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(root, project)
	if err != nil {
		return "", err
	}
	lockPath := filepath.Join(project, "Pipfile.lock")
	if _, err := resolveScript(root, filepath.Join(rel, "Pipfile.lock")); err != nil {
		return "", fmt.Errorf("automatic Pipenv execution requires a committed Pipfile.lock next to the Pipfile: %w; use --runner python3 to manage dependencies yourself", err)
	}
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return "", err
	}
	var manifest struct {
		Default map[string]struct {
			Editable bool `json:"editable"`
		} `json:"default"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil || manifest.Default == nil {
		return "", errors.New("invalid Pipfile.lock: expected a default dependency section")
	}
	for name, pkg := range manifest.Default {
		if pkg.Editable {
			return "", fmt.Errorf("editable runtime dependency %q cannot be cached against a temporary checkout; use --runner python3 with your own environment", name)
		}
	}
	projectID := sha256.Sum256([]byte(filepath.ToSlash(rel)))
	base := filepath.Join(a.cache, "envs", o.owner, o.repo, c.sha, hex.EncodeToString(projectID[:]))
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", err
	}
	unlock, err := lock(ctx, filepath.Join(base, "setup.lock"), a.lockTimeout)
	if err != nil {
		return "", err
	}
	defer unlock()
	venv := filepath.Join(base, "venv")
	python := filepath.Join(venv, "bin", "python")
	ready := filepath.Join(base, "ready")
	if _, err := os.Stat(ready); err == nil {
		if info, err := os.Stat(python); err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0 {
			if o.verbose {
				fmt.Fprintf(a.errOut, "relops: reusing Pipenv environment %s\n", venv)
			}
			return venv, nil
		}
	}
	if o.offline {
		return "", errors.New("Pipenv environment is not cached for this commit/project; run once online first")
	}
	env := os.Environ()
	for _, key := range []string{"VIRTUAL_ENV", "PIPENV_ACTIVE", "PYTHONHOME", "__PYVENV_LAUNCHER__"} {
		env = withoutEnv(env, key)
	}
	for key, value := range map[string]string{
		"PIPENV_PIPFILE": filepath.Join(project, "Pipfile"), "WORKON_HOME": base,
		"PIPENV_CUSTOM_VENV_NAME": "venv", "PIPENV_VENV_IN_PROJECT": "0",
		"PIPENV_IGNORE_VIRTUALENVS": "1", "PIPENV_DONT_LOAD_ENV": "1",
		"PIPENV_NOSPIN": "1", "PIPENV_YES": "1", "PIPENV_DONT_USE_PYENV": "1", "PIPENV_DONT_USE_ASDF": "1",
	} {
		env = replaceEnv(env, key, value)
	}
	fmt.Fprintf(a.errOut, "relops: preparing Pipenv runtime dependencies for %s/%s/%s\n", o.owner, o.repo, filepath.ToSlash(rel))
	for _, args := range [][]string{{"verify"}, {"sync"}} {
		cmd := exec.Command(pipenv, args...)
		cmd.Dir, cmd.Env = project, env
		cmd.Stdout, cmd.Stderr = a.errOut, a.errOut
		code, err := runTool(ctx, cmd)
		if err != nil || code != 0 {
			return "", fmt.Errorf("pipenv %s failed (status %d): %v; check the lockfile and installed Python version; no tool was executed", args[0], code, err)
		}
	}
	if info, err := os.Stat(python); err != nil || info.Mode()&0111 == 0 {
		return "", fmt.Errorf("Pipenv did not create the expected interpreter at %s", python)
	}
	if err := os.WriteFile(ready, []byte("runtime dependencies synced\n"), 0600); err != nil {
		return "", err
	}
	return venv, nil
}
