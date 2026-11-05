# RelOps Launcher

Give a coworker one command to run a tool from your pushed branch:

```sh
relops --ref my-feature relops-infra/path/to/check.sh --verbose
```

`relops` fetches the repository from `mozilla-platform-ops`, resolves the revision to a commit, and runs the script in its own temporary checkout. Online runs fetch every time. Git objects are cached; separate invocations never share a writable checkout. The repository and script above are illustrative.

The initial implementation supports macOS and Linux on arm64/amd64. It has no Go package dependencies. See [design v1.1](design-v1.1.md) for the contract and release roadmap, or the [original design](../../product-specs/relops-launcher-design.md) for background.

## Install

Prerequisites: Git with working GitHub SSH access, plus whatever interpreter and dependencies the tool needs. Verify access to the actual repository:

```sh
git ls-remote git@github.com:mozilla-platform-ops/REPOSITORY.git HEAD
```

Until binary hosting is selected, build locally using Go 1.23 or newer:

```sh
cd scripts/relops-launcher
make build
mkdir -p ~/.local/bin
install -m 755 bin/relops ~/.local/bin/relops
```

Ensure `~/.local/bin` is on your `PATH`. Coworkers receiving a prebuilt binary do not need Go. Copy the appropriate artifact to that directory using the same `install` command, then check `relops --version`. Artifact hosting, minimum OS versions, and real coworker installation trials remain release checks; this is not yet a published release.

## Use

```sh
# Default branch, discovered from the remote
relops repo/path/to/tool.sh --help

# Branch names can include slashes
relops --ref feature/my-change repo/path/to/tool.sh arg1

# Exact commit (full 40-character SHA); tags work too
relops --ref FULL_COMMIT_SHA repo/path/to/tool.sh
relops --ref refs/tags/v1.0 repo/path/to/tool.sh

# Explicit interpreter for a non-executable script
relops --runner python3 repo/path/to/tool.py
relops --runner bash repo/path/to/tool.sh

# Read a file relative to the directory you invoked relops from
relops --cwd caller repo/path/to/tool.sh ./hosts.csv

# Deliberately run the cached revision without fetching
relops --offline --ref feature/my-change repo/path/to/tool.sh
```

Launcher options must precede `repo/path`. Every argument after it belongs to the tool, including `--help`, `--ref`, and `--`. `relops --help` lists launcher options.

By default, scripts must be executable and have a shebang. There is no interpreter guessing by filename. Explicit runners are `python3`, `bash`, and `sh`; missing runtimes or dependencies must be installed separately. There are no automatic setup hooks, submodule initialization, or Git LFS downloads.

The tool starts at the repository root unless you pass `--cwd caller`. Relative arguments are not rewritten. `RELOPS_CALLER_CWD` always contains the original absolute directory. Save outputs outside the temporary checkout: the entire checkout, including generated files and local virtual environments, is removed when the tool exits.

The source, requested ref, and full commit SHA appear on stderr before execution; stdout belongs to the tool. Tools inherit your environment, input, output, and permissions. Git child processes ignore inherited Git repository-selection variables so launching from inside another checkout works. Normal user Git configuration, including transport rewrites, remains in effect; checkout hooks are disabled. This is trusted-code execution, not sandboxing.

Online fetch failures stop execution; use `--offline` explicitly if you want cached code. Bare names shared by a branch and tag are ambiguous: use `refs/heads/NAME` or `refs/tags/NAME`. Moving branches and tags can execute new code on each run. Use a full commit SHA when you want coworkers to try a particular revision; this does not pin the tool's dependencies.

Tool exit statuses pass through; signal termination returns `128 + signal`. Launcher errors return 1 and usage errors return 2. Ctrl-C and termination stop the tool's process group before cleanup. A tool ignoring a signal sent to the launcher gets five seconds before forced termination. Background processes in the tool's process group cannot outlive the invocation; tools should not detach daemons from it.

## Cache and recovery

Cache location is the platform's user cache directory plus `relops`: normally `~/Library/Caches/relops` on macOS and `$XDG_CACHE_HOME/relops` (or `~/.cache/relops`) on Linux. It contains shared bare repositories in `repos/`, lock files in `locks/`, and per-invocation directories in `runs/`. Cache directories have mode 0700.

`--verbose` prints the run's checkout path. Cleanup errors report the retained path. A force kill or machine crash can leave an orphan without an error message; inspect `runs/` when recovering. Only remove an orphan after verifying that no invocation is using it:

```sh
# Substitute actual absolute paths from your cache.
# Disable hooks consistently with the launcher.
git -c core.hooksPath=/dev/null --git-dir=/CACHE/relops/repos/REPO.git worktree list
git -c core.hooksPath=/dev/null --git-dir=/CACHE/relops/repos/REPO.git worktree remove --force --force /CACHE/relops/runs/REPO-ID/checkout
rmdir /CACHE/relops/runs/REPO-ID
```

Lock files persist intentionally; the OS releases their locks when the process exits. Do not delete a lock file to unblock a running invocation. A lock wait times out after 30 seconds with a retry diagnostic. Cache inspection/pruning commands are deferred.

## Development and validation

```sh
make test
make check                 # formatting, vet, race-enabled integration tests
make release VERSION=0.1.0 # four binaries and dist/SHA256SUMS
```

The release label above is illustrative. These commands build artifacts but do not publish them. Cross-compilation does not validate runtime behavior on the target OS. `SHA256SUMS` detects file corruption; it is not a signed release manifest.

Integration tests use local Git repositories and temporary caches without accessing GitHub. They exercise ref changes, offline failures, ambiguous refs, concurrent runs, cwd/arguments/IO, runners, path containment, disabled checkout hooks, and process cleanup. The Python runner check skips if `python3` is unavailable.

Check terminal input, Ctrl-C, and foreground restoration interactively on each supported OS:

```sh
go test -c -o /tmp/relops-terminal-test ./internal/launcher
RELOPS_TEST_TERMINAL=1 /tmp/relops-terminal-test -test.run '^TestInteractiveTerminal$' -test.v
# Type hello and Enter, then press Ctrl-C at the prompt.
```

Before releasing, select and document three real organization tools (shell, Python with known prerequisites, and caller-relative file input), trial installation and SSH access with a coworker, and verify runtime behavior on macOS and Linux. Aliases, a Git-backed registry, Quick integration, and self-update are deferred.

Initial local validation: integration tests, `go vet`, and race checks passed on macOS arm64; the interactive terminal check passed for input, Ctrl-C (status 130), and foreground restoration. All four target binaries cross-compiled. Linux execution and coworker installation have not yet been verified.
