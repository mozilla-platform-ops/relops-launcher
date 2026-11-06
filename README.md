# RelOps Launcher

Give a coworker one command to run a tool from your pushed branch:

```sh
relops --ref my-feature relops-infra/path/to/check.sh --verbose
```

`relops` fetches the repository from `mozilla-platform-ops` by default, resolves the revision to a commit, and runs the script in its own temporary checkout. Use `--owner USER_OR_ORG` for a fork or another GitHub organization. Online runs fetch every time. Git objects are cached per owner/repository; separate invocations never share a writable checkout. The repository and script above are illustrative.

Published binaries target macOS Apple silicon (arm64) and Linux amd64. It has no Go package dependencies. See [design v1.1](design-v1.1.md) for the contract and release roadmap, or the [original design](../../product-specs/relops-launcher-design.md) for background.

## Install

Prerequisites: Git with working GitHub SSH access, plus the tool's interpreter. Automatic Python project execution also requires an installed `pipenv` when a `Pipfile` is found. Verify access to the actual owner/repository:

```sh
git ls-remote git@github.com:mozilla-platform-ops/REPOSITORY.git HEAD
```

For a local source install, install `just` and Go 1.23 or newer (release builds use the version pinned in `.go-version`):

```sh
cd scripts/relops-launcher
just build
mkdir -p ~/.local/bin
install -m 755 bin/relops ~/.local/bin/relops
```

Ensure `~/.local/bin` is on your `PATH`. Coworkers receiving a prebuilt binary do not need Go. The download site is published at <https://relops-launcher.quick.mozilla.cloud/> behind Mozilla SSO. Download the appropriate artifact in your browser, then use the site's checksum-and-install command from any terminal directory. It reads from `~/Downloads` and sets PATH for the current terminal; the site also explains persistent PATH setup. Terminal downloads without authentication return Google sign-in HTML. Generate the site with `just site`; see the [publishing instructions](quick_site/README.md#publishing) for the account-specific Quick deployment command. Live download verification, minimum OS versions, and real coworker installation trials remain release checks.

## Use

```sh
# Default branch, discovered from the remote
relops repo/path/to/tool.sh --help

# Branch names can include slashes
relops --ref feature/my-change repo/path/to/tool.sh arg1

# Use a branch pushed to a personal fork
relops --owner aerickson --ref 022526-tool_tweaks android-tools/worker_health/replay_tc_pool.py

# Exact commit (full 40-character SHA); tags work too
relops --ref FULL_COMMIT_SHA repo/path/to/tool.sh
relops --ref refs/tags/v1.0 repo/path/to/tool.sh

# Override automatic detection and manage the Python environment yourself
relops --runner python3 repo/path/to/tool.py
relops --runner bash repo/path/to/tool.sh

# Relative files resolve from the caller's directory by default
relops repo/path/to/tool.sh ./hosts.csv

# Opt into running from the temporary repository root
relops --cwd repo repo/path/to/tool.sh

# Deliberately run the cached revision without fetching
relops --offline --ref feature/my-change repo/path/to/tool.sh
```

Launcher options must precede `repo/path`. Every argument after it belongs to the tool, including `--help`, `--ref`, and `--`. `relops --help` lists launcher options.

The default runner is `auto`. It honors shebangs, including for non-executable files, and falls back to `python3` for `.py` scripts without a shebang. A `.sh` extension alone does not identify a shell. Explicit runners are `python3`, `bash`, and `sh`; they override detection and bypass Pipenv setup. There are no repository setup hooks, submodule initialization, or Git LFS downloads.

The tool starts in the caller's directory unless you pass `--cwd repo`. Relative arguments are not rewritten. `RELOPS_CALLER_CWD` always contains the original absolute directory. Save outputs outside the temporary checkout: the entire checkout, including generated files, is removed when the tool exits.

### Automatic Pipenv environments

For Python scripts, `auto` searches upward from the script for the nearest `Pipfile`, stopping at the checkout root. It uses the installed Pipenv to verify the committed `Pipfile.lock` and sync runtime dependencies into a managed environment. You do not need an outer `pipenv run`, and detection does not depend on which directory you invoke `relops` from.

The environment is cached per owner, repository, commit, and project path. Subsequent runs at that commit reuse it. Initialization is locked so concurrent invocations cannot install into the same environment simultaneously; each tool still has its own writable checkout. Preparation output goes to stderr. Pipenv itself and the required Python version must already be installed; missing Pipenv produces an actionable error. Dev dependencies are not installed. Editable runtime dependencies are unsupported because their source checkout would be removed after execution. Use an explicit runner with your own environment for those projects.

Automatic setup does not load a project `.env` file; supply runtime configuration through your existing environment. `--offline` requires the managed environment to have been prepared already and never invokes Pipenv installation. A new commit gets a new environment even if its lockfile is unchanged. Environments persist until manually removed; automated pruning is deferred.

For the replay tool, the full command is now:

```sh
/Users/aerickson/git/df/scripts/relops-launcher/bin/relops \
  --owner aerickson \
  --ref 022526-tool_tweaks \
  android-tools/worker_health/replay_tc_pool.py
```

Run it from whichever directory should hold relative inputs and outputs. No `pipenv run`, `--runner`, or `--cwd` is needed.

The source, requested ref, and full commit SHA appear on stderr before execution; stdout belongs to the tool. Tools inherit your environment, input, output, and permissions. Git child processes ignore inherited Git repository-selection variables so launching from inside another checkout works. Normal user Git configuration, including transport rewrites, remains in effect; checkout hooks are disabled. This is trusted-code execution, not sandboxing.

Online fetch failures stop execution; use `--offline` explicitly if you want cached code. Bare names shared by a branch and tag are ambiguous: use `refs/heads/NAME` or `refs/tags/NAME`. Moving branches and tags can execute new code on each run. Use a full commit SHA when you want coworkers to try a particular revision; this does not pin the tool's dependencies.

Tool exit statuses pass through; signal termination returns `128 + signal`. Launcher errors return 1 and usage errors return 2. Ctrl-C and termination stop the tool's process group before cleanup. A tool ignoring a signal sent to the launcher gets five seconds before forced termination. Background processes in the tool's process group cannot outlive the invocation; tools should not detach daemons from it.

## Cache and recovery

Cache location is the platform's user cache directory plus `relops`: normally `~/Library/Caches/relops` on macOS and `$XDG_CACHE_HOME/relops` (or `~/.cache/relops`) on Linux. It contains shared bare repositories in `repos/OWNER/REPO.git`, repository locks in `locks/OWNER/REPO.lock`, per-invocation directories in `runs/`, and managed environments in `envs/OWNER/REPO/COMMIT/PROJECT/`. Cache directories have mode 0700. Older flat repository caches are left untouched and are no longer used; run online once to populate the owner-specific cache before using offline mode.

`--verbose` prints the run's checkout path. Cleanup errors report the retained path. A force kill or machine crash can leave an orphan without an error message; inspect `runs/` when recovering. Only remove an orphan after verifying that no invocation is using it:

```sh
# Substitute actual absolute paths from your cache.
# Disable hooks consistently with the launcher.
git -c core.hooksPath=/dev/null --git-dir=/CACHE/relops/repos/OWNER/REPO.git worktree list
git -c core.hooksPath=/dev/null --git-dir=/CACHE/relops/repos/OWNER/REPO.git worktree remove --force --force /CACHE/relops/runs/OWNER-REPO-ID/checkout
rmdir /CACHE/relops/runs/OWNER-REPO-ID
```

Lock files persist intentionally; the OS releases their locks when the process exits. Do not delete a lock file to unblock a running invocation. A lock wait times out after 30 seconds with a retry diagnostic. Cache inspection/pruning commands are deferred.

## Development and validation

Run recipes from this directory; `just` lists them. CI uses just 1.59.0. People installing a prebuilt launcher do not need just.

```sh
just test
just check                 # formatting, vet, race-enabled integration tests
just release               # unsigned release binaries and dist/SHA256SUMS
just site                  # unsigned preview site under dist/site
just site-check            # also test the site's download/copy behavior (Node)
just notary-setup          # one-time, interactive Apple credentials → Keychain
just signed-site           # sign, notarize, verify, and assemble the sharing bundle
just publish               # signed-site, then Quick deploy with the Mozilla account
just site-from-release     # rebuild/test website using retained signed downloads
just publish-site          # publish website changes without rebuilding binaries
```

Build recipes read the version from `VERSION` (currently `0.1.1`) and embed it in the executable; `relops --version` prints it. Override with `just version=... signed-site` when needed. Direct `go build`/`go run` without the justfile report `dev`. Only `just publish` and `just publish-site` deploy; other recipes prepare artifacts locally. Cross-compilation does not validate runtime behavior on the target OS. `SHA256SUMS` detects file corruption; it is not a signed release manifest.

For shared macOS downloads, use `just signed-site` or `just publish`, on a Mac with Xcode tools and a Developer ID Application identity. `just site` is for unsigned previews and rebuilds ordinary binaries; do not run it after signing and then publish that preview. See the [macOS release workflow](quick_site/README.md#macos-signing-and-notarization) for credential setup, retries, and publisher overrides. `0.1.1` introduces this workflow; the already-published `0.1.0` files are left unchanged.

All build outputs stay in ignored `bin/` and `dist/` directories; no binaries need committing. The [site README](quick_site/README.md) describes the bundle, source/toolchain metadata, local preview, and later Quick publication. CI tests the launcher on Linux and macOS and uploads a generated site artifact without deploying it.

Integration tests use local Git repositories and temporary caches without accessing GitHub. They exercise owner isolation, ref changes, offline failures, ambiguous refs, concurrent runs, cwd/arguments/IO, runner detection, Pipenv setup/reuse/failure, path containment, disabled checkout hooks, and process cleanup. Python runner checks skip if `python3` is unavailable. Normal tests use a fake Pipenv process without accessing a package index; an opt-in `RELOPS_TEST_REAL_PIPENV=1 go test ./internal/launcher -run '^TestRealPipenv$'` checks the installed Pipenv using an empty dependency set.

Check terminal input, Ctrl-C, and foreground restoration interactively on each supported OS:

```sh
go test -c -o /tmp/relops-terminal-test ./internal/launcher
RELOPS_TEST_TERMINAL=1 /tmp/relops-terminal-test -test.run '^TestInteractiveTerminal$' -test.v
# Type hello and Enter, then press Ctrl-C at the prompt.
```

Before releasing, select and document three real organization tools (shell, Python with known prerequisites, and caller-relative file input), trial installation and SSH access with a coworker, and verify runtime behavior on macOS and Linux. Aliases, a Git-backed registry, Quick integration, and self-update are deferred.

Initial local validation: integration tests, `go vet`, and race checks passed on macOS arm64; the interactive terminal check passed for input, Ctrl-C (status 130), and foreground restoration. All four target binaries cross-compiled. Linux execution and coworker installation have not yet been verified.

The `0.1.1` macOS release completed Developer ID signing and Apple notarization. Both architectures passed signature and online notarization-ticket checks, and the generated signed site passed its download-selector tests. Browser installation of the published signed release remains a separate verification step.
