# Python environments

[← README](../README.md)

Source tools need Git with GitHub SSH access and a compatible Python interpreter. For initial environment setup, install `uv` for locked uv projects or `pipenv` for Pipfile projects. relops reports missing managers but does not install them or download Python interpreters.

## uv projects

Target a repository (`relops repo fleetroll_mvp`) or a project subdirectory (`relops repo repo/path/to/project`) containing committed `pyproject.toml` and `uv.lock` files. relops uses installed uv to sync the locked runtime environment, then runs a console entry point from the installed project:

- Prefer the command matching the Python project name, not the GitHub repository name. For fleetroll_mvp, the project name is `fleetroll`, so `relops repo fleetroll_mvp` runs `fleetroll`.
- Otherwise use the sole entry point, or a deterministic alias when all entry points call the same function.
- If distinct commands remain ambiguous, list them and require `--command NAME` before the target. `--command fr` selects fleetroll's alias explicitly.

The project must be packaged with a build system to install console entry points. Python script targets also use uv automatically when the nearest project has both `pyproject.toml` and `uv.lock`. A nearer Pipfile takes precedence over an outer uv project; a locked uv project wins when both managers are defined in the same directory. An explicit `--runner` bypasses environment detection for scripts; project targets require `auto`.

Setup uses `uv sync --locked --no-default-groups --no-dev --no-editable --no-python-downloads`. Missing or stale locks and unavailable compatible Python versions stop execution. Dependencies are installed without development/default groups or optional extras. Project and local dependency installs are non-editable so the cached environment survives checkout cleanup. uv may execute package build backends during installation. relops does not load project `.env` files during setup; the tool still inherits your normal runtime environment.

Environments and installed entry-point metadata are cached per owner, repository, commit, and project path, separately from Pipenv environments. Setup is locked across concurrent invocations. Subsequent runs reuse the environment while each run gets its own checkout, and do not require uv on PATH. `--offline` requires a prepared environment and never invokes uv setup. Cache pruning remains manual. Build-time dependencies may require additional downloads during initial setup; the runtime lock does not pin every build backend input.

## Pipenv scripts

For Python scripts, `auto` searches upward from the script for the nearest `Pipfile`, stopping at the checkout root. It uses the installed Pipenv to verify the committed `Pipfile.lock` and sync runtime dependencies into a managed environment. You do not need an outer `pipenv run`, and detection does not depend on which directory you invoke `relops` from.

The environment is cached per owner, repository, commit, and project path. Subsequent runs at that commit reuse it without requiring Pipenv on PATH. Initialization is locked so concurrent invocations cannot install into the same environment simultaneously; each tool still has its own writable checkout. Preparation output goes to stderr. Pipenv itself and the required Python version must already be installed for setup; missing Pipenv produces an actionable error. Dev dependencies are not installed. Editable runtime dependencies are unsupported because their source checkout would be removed after execution. Use an explicit runner with your own environment for those projects.

Automatic setup does not load a project `.env` file; supply runtime configuration through your existing environment. `--offline` requires the managed environment to have been prepared already and never invokes Pipenv installation. A new commit gets a new environment even if its lockfile is unchanged. Environments persist until manually removed; automated pruning is deferred.
