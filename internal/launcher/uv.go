package launcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

type uvMetadata struct {
	Name    string            `json:"name"`
	Scripts map[string]string `json:"scripts"`
}

// Installed wheel metadata is the authoritative list of runnable entry points.
// Reading it with the managed Python avoids a TOML dependency or requiring a
// separate Python installation just to parse pyproject.toml. No package is imported.
const uvEntryPoints = `import importlib.metadata as m, json, sys
try:
    d = m.distribution(sys.argv[1])
    scripts = {e.name: e.value for e in d.entry_points if e.group == "console_scripts"}
except m.PackageNotFoundError:
    scripts = {}
print(json.dumps({"name": sys.argv[1], "scripts": scripts}))
`

func managedPythonCommand(script, interpreter, optional string, args []string, venv string) (*exec.Cmd, error) {
	if filepath.Base(interpreter) == "env" {
		return toolCommand(script, "auto", args)
	}
	pythonArgs := []string{}
	if optional != "" {
		pythonArgs = append(pythonArgs, optional)
	}
	pythonArgs = append(pythonArgs, script)
	return exec.Command(filepath.Join(venv, "bin", "python"), append(pythonArgs, args...)...), nil
}

// A nearer Pipfile owns the script. At the same level a locked uv project wins.
func uvProject(root, script string) (string, error) {
	root, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", err
	}
	for dir := filepath.Dir(script); ; dir = filepath.Dir(dir) {
		found := true
		for _, name := range []string{"pyproject.toml", "uv.lock"} {
			if _, err := os.Lstat(filepath.Join(dir, name)); errors.Is(err, os.ErrNotExist) {
				found = false
			} else if err != nil {
				return "", err
			}
		}
		if found {
			return dir, nil
		}
		if _, err := os.Lstat(filepath.Join(dir, "Pipfile")); err == nil {
			return "", nil
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

func selectUVCommand(meta uvMetadata, requested string) (string, error) {
	names := make([]string, 0, len(meta.Scripts))
	for name := range meta.Scripts {
		names = append(names, name)
	}
	sort.Strings(names)
	if requested != "" {
		if _, ok := meta.Scripts[requested]; ok {
			return requested, nil
		}
		return "", fmt.Errorf("unknown console command %q; available commands: %s", requested, strings.Join(names, ", "))
	}
	if len(names) == 0 {
		return "", errors.New("uv project has no installed console entry points; declare [project.scripts] and a build system in pyproject.toml, or target a Python script")
	}
	normalize := func(s string) string { return strings.NewReplacer("_", "-", ".", "-").Replace(strings.ToLower(s)) }
	// Prefer the exact name, then an unambiguous normalized match.
	if _, ok := meta.Scripts[meta.Name]; ok {
		return meta.Name, nil
	}
	var matching []string
	for _, name := range names {
		if normalize(name) == normalize(meta.Name) {
			matching = append(matching, name)
		}
	}
	if len(matching) == 1 {
		return matching[0], nil
	}
	entry := meta.Scripts[names[0]]
	for _, name := range names[1:] {
		if meta.Scripts[name] != entry {
			return "", fmt.Errorf("uv project has multiple console commands: %s; select one with --command NAME before the target", strings.Join(names, ", "))
		}
	}
	return names[0], nil
}

func (a *app) uvCommand(ctx context.Context, o options, c checkout, project string) (*exec.Cmd, string, error) {
	if o.runner != "auto" {
		return nil, "", errors.New("project targets require --runner auto; use a script path with an explicit runner")
	}
	venv, meta, err := a.uvEnvironment(ctx, o, c, project)
	if err != nil {
		return nil, "", err
	}
	name, err := selectUVCommand(meta, o.commandName)
	if err != nil {
		return nil, "", err
	}
	if !repoName.MatchString(name) || name == "." || name == ".." {
		return nil, "", fmt.Errorf("invalid console entry point name %q", name)
	}
	file := filepath.Join(venv, "bin", name)
	if !executable(file) {
		return nil, "", fmt.Errorf("uv did not install console command %q in %s", name, venv)
	}
	if o.verbose {
		a.provenancef("uv console command=%s", name)
	}
	return exec.Command(file, o.args...), venv, nil
}

func executable(file string) bool {
	info, err := os.Stat(file)
	return err == nil && info.Mode().IsRegular() && info.Mode()&0111 != 0
}

func (a *app) uvEnvironment(ctx context.Context, o options, c checkout, project string) (string, uvMetadata, error) {
	var meta uvMetadata
	root, err := filepath.EvalSymlinks(c.dir)
	if err != nil {
		return "", meta, err
	}
	rel, err := filepath.Rel(root, project)
	if err != nil {
		return "", meta, err
	}
	for _, name := range []string{"pyproject.toml", "uv.lock"} {
		if _, err := resolveScript(root, filepath.Join(rel, name)); err != nil {
			return "", meta, fmt.Errorf("automatic uv execution requires committed pyproject.toml and uv.lock in the project directory: %w", err)
		}
	}
	uv, err := exec.LookPath("uv")
	if err != nil {
		return "", meta, errors.New("this tool uses uv, but uv is not on PATH; install uv and retry, or use --runner python3 with a script path if you manage dependencies yourself")
	}
	uv, err = filepath.Abs(uv)
	if err != nil {
		return "", meta, err
	}
	projectID := sha256.Sum256([]byte(filepath.ToSlash(rel)))
	base := filepath.Join(a.cache, "envs", o.owner, o.repo, c.sha, hex.EncodeToString(projectID[:]), "uv-v1")
	if err := os.MkdirAll(base, 0700); err != nil {
		return "", meta, err
	}
	unlock, err := lock(ctx, filepath.Join(base, "setup.lock"), a.lockTimeout)
	if err != nil {
		return "", meta, err
	}
	defer unlock()
	venv := filepath.Join(base, "venv")
	python := filepath.Join(venv, "bin", "python")
	ready := filepath.Join(base, "ready.json")
	if data, err := os.ReadFile(ready); err == nil && json.Unmarshal(data, &meta) == nil && meta.Name != "" && executable(python) {
		if o.verbose {
			a.provenancef("reusing uv environment %s", venv)
		}
		return venv, meta, nil
	}
	if o.offline {
		return "", meta, errors.New("uv environment is not cached for this commit/project; run once online first")
	}
	env := os.Environ()
	for _, key := range []string{"VIRTUAL_ENV", "PYTHONHOME", "__PYVENV_LAUNCHER__", "UV_PROJECT", "UV_WORKING_DIR", "UV_ENV_FILE", "UV_FROZEN", "UV_NO_INSTALL_PROJECT", "UV_NO_INSTALL_WORKSPACE", "UV_NO_INSTALL_LOCAL", "UV_NO_INSTALL_PACKAGE"} {
		env = withoutEnv(env, key)
	}
	env = replaceEnv(env, "UV_PROJECT_ENVIRONMENT", venv)
	env = replaceEnv(env, "UV_NO_ENV_FILE", "1")
	// Preparatory subprocesses use the same cancellation and signal handling as tools.
	run := func(program string, args []string, capture bool) ([]byte, error) {
		cmd := exec.Command(program, args...)
		cmd.Dir, cmd.Env = project, env
		output, code, err := a.runSetup(ctx, o, cmd, capture)
		if err != nil || code != 0 {
			return nil, fmt.Errorf("%s %s failed (status %d): %v; check the lockfile and installed Python version; no tool was executed", filepath.Base(program), args[0], code, err)
		}
		return output, nil
	}
	a.statusf("preparing environment…")
	// --locked rejects stale locks rather than silently accepting them as --frozen does.
	if _, err := run(uv, []string{"sync", "--locked", "--no-default-groups", "--no-dev", "--no-editable", "--no-python-downloads", "--project", project}, false); err != nil {
		return "", meta, err
	}
	if !executable(python) {
		return "", meta, fmt.Errorf("uv did not create the expected interpreter at %s", python)
	}
	data, err := run(uv, []string{"version", "--project", project, "--output-format", "json", "--offline"}, true)
	if err != nil {
		return "", meta, err
	}
	var version struct {
		Name string `json:"package_name"`
	}
	if err := json.Unmarshal(data, &version); err != nil || version.Name == "" {
		return "", meta, errors.New("uv version returned invalid project metadata; update uv and retry")
	}
	data, err = run(python, []string{"-I", "-c", uvEntryPoints, version.Name}, true)
	if err != nil {
		return "", meta, err
	}
	if err := json.Unmarshal(data, &meta); err != nil || meta.Name != version.Name {
		return "", meta, errors.New("could not read installed uv project entry points")
	}
	if err := os.WriteFile(ready, data, 0600); err != nil {
		return "", meta, err
	}
	return venv, meta, nil
}
