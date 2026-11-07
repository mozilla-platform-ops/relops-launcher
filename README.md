# RelOps Launcher

Give a coworker one command to run a tool from your pushed branch:

```sh
relops repo --ref my-feature relops-infra/path/to/check.sh --verbose
```

`relops` fetches the repository from `mozilla-platform-ops` by default, resolves the revision to a commit, and runs the script in its own temporary checkout. Use `--owner USER_OR_ORG` for a fork or another GitHub organization. Online runs fetch every time. Git objects are cached per owner/repository; separate invocations never share a writable checkout. The repository and script above are illustrative.

For supported binary tools, relops can instead download and run the latest stable GitHub release. The first supported tool is fleetbench: `relops run fb cpu --mode quick --json`. No Git checkout or Rust toolchain is needed for this target.

Published binaries target macOS Apple silicon (arm64) and Linux amd64. It has no Go package dependencies. See [design v1.1](design-v1.1.md) for the contract and release roadmap, or the [original design](../../product-specs/relops-launcher-design.md) for background.

## Install

Prerequisites for source targets: Git with working GitHub SSH access, plus the tool's interpreter. Automatic Python project execution also requires an installed `pipenv` for Pipfile projects or `uv` for locked uv projects. Missing managers produce an installation hint; relops does not install managers or download Python interpreters. Release binary targets use public HTTPS downloads instead and need none of those build tools. Verify source access to the actual owner/repository:

```sh
git ls-remote git@github.com:mozilla-platform-ops/REPOSITORY.git HEAD
```

For a personal macOS Apple silicon install, use the retained signed and notarized release for the version in `VERSION`:

```sh
./install-local.sh
```

The script checks the retained release checksum and Apple notarization before installing it to `~/.local/bin/relops`. It uses `dist/releases/vVERSION` from this checkout; if missing, prepare it with `just signed-site`. It does not download updates. To install a development build from the current checkout, including uncommitted changes, use `./install-local.sh --dev` (requires Go 1.23 or newer). Run the script again after updating the release or source.

Ensure `~/.local/bin` is on your `PATH`. Coworkers receiving a prebuilt binary do not need Go. The download site is published at <https://relops-launcher.quick.mozilla.cloud/> behind Mozilla SSO. Download the appropriate artifact in your browser, then use the site's checksum-and-install command from any terminal directory. It reads from `~/Downloads` and sets PATH for the current terminal; the site also explains persistent PATH setup. Terminal downloads without authentication return Google sign-in HTML. Generate the site with `just site`; see the [publishing instructions](quick_site/README.md#publishing) for the account-specific Quick deployment command. Live download verification, minimum OS versions, and real coworker installation trials remain release checks.

## Catalog

The launcher bundles a curated catalog from `quick_site/tools.json`; the website renders the same entries. Catalog lookup and discovery require no network access. Running a tool still fetches its source or checks its release normally, unless `--offline` is selected. Catalog updates currently require a new launcher build; remote catalog downloads are deferred.

```sh
relops run                                    # compact tool directory
relops catalog                                # descriptions and commands
relops info fr
relops run fleetroll --help
relops r ct --help                            # r = run; ct = create-tc-task
relops run fr --help                          # fr = fleetroll
relops run -b replay-tc-pool -h               # -b and --beta select the beta target
relops run --offline fr --help
relops run --ref my-branch fr --help          # override a default source target's ref
relops run fleetbench --version               # latest stable release binary
relops run pc                                 # open Pool Classifier in a browser
relops run --print-url hangar                 # print a web tool's URL
```

`run` is the primary launch command; `r` is its shortcut. `c` is the shortcut for `catalog`. `catalog launch` remains a compatibility alias for `run`. Bare `run` or `r` shows the compact tool directory.

Each entry has a unique shortcut ID and optional aliases, plus a default target, beta target, or both. Owners designate each target independently, including its GitHub owner, repository, path, and optional ref, console command, runner, and working directory. An omitted source ref follows the remote's default branch. Beta is a designated preview, not necessarily branch HEAD. Missing channels fail clearly; beta-only tools require `-b`. There is no fallback between channels. Launch diagnostics show the canonical shortcut, channel, catalog revision, and resolved source commit or release version.

Normal launches show one compact header, such as `relops: fleetroll · main@24f451fe`, in green. Beta selections show `(beta)` and offline runs show `(cached)`. Environment setup adds a dim `relops: preparing environment…` line only when needed. Successful dependency setup output is hidden; failures show diagnostics. Use `-v` or `--verbose` for the catalog revision, full repository/ref/commit provenance, and live setup logs. Launcher verbose diagnostics are green too; dependency and tool output retain their own formatting. Set `NO_COLOR=1` to disable styling; `TERM=dumb` and redirected stderr also produce plain text.

Launcher options go before the shortcut; all subsequent arguments belong to the tool. Website `example_args` are documentation only and are never injected into a launch. `--ref` can override the default source target but cannot be combined with `-b`/`--beta`. Catalog launches reject `--owner`; use repository execution for arbitrary forks. Release targets retain their existing option restrictions. `--command`, `--runner`, and `--cwd` override configured source defaults.

Web targets print their HTTPS URL and open it with `open` on macOS or `xdg-open` on Linux. If no graphical session is detected or opening fails, use the printed URL manually. Use `relops run --print-url NAME` to print only the URL without opening a browser. Web targets reject tool arguments and source or release options.

Catalog names and aliases share one namespace; duplicates fail validation. Claim names through Git review, and keep established names associated with the same tool. Because `run` selects execution specifically, `relops run list` launches a tool named `list` if one is registered. Catalog discovery uses `relops catalog` or `relops c`; `relops catalog list` and `relops catalog ls` remain aliases. Inspect requirements with `relops info NAME` (shortcut: `relops i NAME`; compatibility alias: `relops catalog info NAME`).

Source execution requires `relops repo`. Catalog tools, including release binaries, require `relops run` (shortcut: `r`). Bare targets are rejected with a suggested replacement:

```sh
relops repo android-tools/worker_health/create_tc_task.py --help
relops repo --ref my-branch android-tools/worker_health/create_tc_task.py --help
relops repo c/path/to/tool.sh --help          # repository named c
relops repo fleetbench                        # source project, rather than release binary
```

`run`, `r`, `c`, `catalog`, `info`, `i`, and `repo` are reserved as bare launcher commands. Use `relops repo c/path/to/tool.sh` for a repository named c. Flags may also precede the namespace, for example `relops --offline run fr --help`.

## Use

```sh
# Latest stable fleetbench release for this machine; options after the target
# belong to fleetbench, including --mode and --json
relops run fb cpu --mode quick --json

# Default branch, discovered from the remote
relops repo repo/path/to/tool.sh --help

# Run a uv project's default console command, with arguments
relops repo fleetroll_mvp host-monitor configs/host-lists/linux/all.list -h

# Select another console command or alias from the same project
relops repo --command fr fleetroll_mvp host-monitor -h

# Branch names can include slashes
relops repo --ref feature/my-change repo/path/to/tool.sh arg1

# Use a branch pushed to a personal fork
relops repo --owner aerickson --ref 022526-tool_tweaks android-tools/worker_health/replay_tc_pool.py

# Exact commit (full 40-character SHA); tags work too
relops repo --ref FULL_COMMIT_SHA repo/path/to/tool.sh
relops repo --ref refs/tags/v1.0 repo/path/to/tool.sh

# Override automatic detection and manage the Python environment yourself
relops repo --runner python3 repo/path/to/tool.py
relops repo --runner bash repo/path/to/tool.sh

# Relative files resolve from the caller's directory by default
relops repo repo/path/to/tool.sh ./hosts.csv

# Opt into running from the temporary repository root
relops repo --cwd repo repo/path/to/tool.sh

# Deliberately run the cached revision without fetching
relops repo --offline --ref feature/my-change repo/path/to/tool.sh
```

Launcher options must precede the repository or script target. Every argument after it belongs to the tool, including `--help`, `--ref`, and `--`. `relops --help` lists launcher options.

The default runner is `auto`. It honors shebangs, including for non-executable files, and falls back to `python3` for `.py` scripts without a shebang. A `.sh` extension alone does not identify a shell. Explicit runners are `python3`, `bash`, and `sh`; they override detection and bypass Pipenv setup. There are no repository setup hooks, submodule initialization, or Git LFS downloads.

Script targets start in the caller's directory unless you pass `--cwd repo`. Project targets start in the project directory by default, so repository-relative inputs such as `configs/host-lists/linux/all.list` work. Explicit `--cwd caller` or `--cwd repo` selects the caller's directory or checkout root for source targets. Relative arguments are not rewritten. `RELOPS_CALLER_CWD` always contains the original absolute directory. Save outputs outside the temporary checkout: the entire checkout, including generated files, is removed when the tool exits.

### Release binaries (fleetbench)

The `relops run fb` command selects GitHub's latest published stable release. A successful release check is reused for 15 minutes; repeated launches in that interval verify and run the cached binary without making release requests. Once the interval expires, relops checks the release API and `SHA256SUMS` again. The version appears in the compact header on stderr; `--verbose` also shows the last successful check time. Stdout belongs to fleetbench. Downloads and cached binaries must match the published SHA-256 before execution. Checksums verify integrity against the release's checksum file; they are not a separate publisher signature.

Platform selection is automatic: macOS arm64 uses `fleetbench-<tag>-macos-aarch64`, and Linux amd64 uses `fleetbench-<tag>-linux-x86_64`. Unsupported platforms or a latest release missing its matching binary/checksum fail clearly; relops never substitutes a different architecture or an older release.

```sh
relops run fb --version
relops run fb cpu --mode quick --json > result.json
relops run --refresh fb --version
relops run --offline fb --version
```

Release binaries run in the caller's directory and inherit its environment, arguments, IO, and permissions. They use the same exit-status and signal handling as source tools. `--cwd caller` is allowed; `--cwd repo`, `--ref`, `--runner`, and `--command` are rejected for release targets. Branch builds and explicit release-version selection are deferred. Use `relops repo --ref BRANCH fleetbench/runner` for a source path.

`--offline` uses the last successfully prepared release for this owner/repository/platform, verifies the cached binary, and makes no network requests. A failed online check or download stops execution; there is no automatic fallback. Failed updates preserve the previous verified release for explicit offline use. Cache installation is locked across concurrent invocations and files are replaced atomically after verification. Old versions remain cached until manually removed.

`--refresh` bypasses the 15-minute interval but respects rate-limit cooldowns. It must precede the release target and cannot be combined with `--offline` or used with source targets. Normal concurrent launches share one check under the release cache lock; reading the cache does not extend its freshness. Existing caches without a check timestamp are refreshed on the next online launch. Refreshes send the saved ETag when available and accept `304 Not Modified`; checksums are still fetched to detect assets replaced under the same tag.

GitHub rate-limit failures report the server's message and retry time. Relops saves a cooldown shared by all release requests in this cache and makes no further release requests until that time, including requests with `--refresh`. It respects `Retry-After` and exhausted-quota reset headers; rate-limit responses without a usable future time pause requests for one minute. Fresh verified releases can still run during a cooldown; older prepared releases require explicit `--offline`.

Release owners are designated by the catalog; `run` rejects `--owner`. Use `repo --owner OWNER` for source execution from another owner. Optional API authentication uses `GH_TOKEN`, falling back to `GITHUB_TOKEN`; tokens are sent only to `api.github.com`, never to release download hosts or redirects to other hosts. Relops does not read GitHub CLI credentials or reuse SSH authentication for HTTPS. Release downloads remain public, so private releases are not supported. Adding other binary tools requires an explicit launcher definition rather than guessing asset names.

### Automatic uv projects

Target a repository (`relops repo fleetroll_mvp`) or a project subdirectory (`relops repo repo/path/to/project`) containing committed `pyproject.toml` and `uv.lock` files. relops uses installed uv to sync the locked runtime environment, then runs a console entry point from the installed project:

- Prefer the command matching the Python project name, not the GitHub repository name. For fleetroll_mvp, the project name is `fleetroll`, so `relops repo fleetroll_mvp` runs `fleetroll`.
- Otherwise use the sole entry point, or a deterministic alias when all entry points call the same function.
- If distinct commands remain ambiguous, list them and require `--command NAME` before the target. `--command fr` selects fleetroll's alias explicitly.

The project must be packaged with a build system to install console entry points. Python script targets also use uv automatically when the nearest project has both `pyproject.toml` and `uv.lock`. A nearer Pipfile takes precedence over an outer uv project; a locked uv project wins when both managers are defined in the same directory. An explicit `--runner` bypasses environment detection for scripts; project targets require `auto`.

Setup uses `uv sync --locked --no-default-groups --no-dev --no-editable --no-python-downloads`. Missing or stale locks and unavailable compatible Python versions stop execution. Dependencies are installed without development/default groups or optional extras. Project and local dependency installs are non-editable so the cached environment survives checkout cleanup. uv may execute package build backends during installation. relops does not load project `.env` files during setup; the tool still inherits your normal runtime environment.

Environments and installed entry-point metadata are cached per owner, repository, commit, and project path, separately from Pipenv environments. Setup is locked across concurrent invocations. Subsequent runs reuse the environment while each run gets its own checkout. `--offline` requires a prepared environment and never invokes uv setup. Cache pruning remains manual. Build-time dependencies may require additional downloads during initial setup; the runtime lock does not pin every build backend input.

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

For source targets, the tool, ref, and eight-character commit prefix appear in a compact header on stderr before execution. Use `--verbose` for the full repository, ref, and commit SHA; stdout belongs to the tool. Tools inherit your environment, input, output, and permissions. Git child processes ignore inherited Git repository-selection variables so launching from inside another checkout works. Normal user Git configuration, including transport rewrites, remains in effect; checkout hooks are disabled. This is trusted-code execution, not sandboxing.

Online fetch failures stop execution; use `--offline` explicitly if you want cached code. Bare names shared by a branch and tag are ambiguous: use `refs/heads/NAME` or `refs/tags/NAME`. Moving branches and tags can execute new code on each run. Use a full commit SHA when you want coworkers to try a particular revision; this does not pin the tool's dependencies.

Tool exit statuses pass through; signal termination returns `128 + signal`. Launcher errors return 1 and usage errors return 2. Ctrl-C and termination stop the tool's process group before cleanup. A tool ignoring a signal sent to the launcher gets five seconds before forced termination. Background processes in the tool's process group cannot outlive the invocation; tools should not detach daemons from it.

## Cache and recovery

Cache location is the platform's user cache directory plus `relops`: normally `~/Library/Caches/relops` on macOS and `$XDG_CACHE_HOME/relops` (or `~/.cache/relops`) on Linux. It contains shared bare repositories in `repos/OWNER/REPO.git`, repository locks in `locks/OWNER/REPO.lock`, per-invocation directories in `runs/`, and managed environments in `envs/OWNER/REPO/COMMIT/PROJECT_HASH/` (uv uses its `uv-v1/` subdirectory). Cache directories have mode 0700. Older flat repository caches are left untouched and are no longer used; run online once to populate the owner-specific cache before using offline mode.

Release binaries live separately in `releases/OWNER/REPO/OS-ARCH/TAG/SHA256/ASSET`. The platform directory also holds `latest.json` (the last verified release and checksum) and `release.lock`. Release binaries persist after execution; they do not use temporary checkouts. Including the checksum in the path keeps concurrent invocations on the bytes they verified even if an asset is replaced under the same tag.

`--verbose` prints the run's checkout path. Cleanup errors report the retained path. A force kill or machine crash can leave an orphan without an error message; inspect `runs/` when recovering. Only remove an orphan after verifying that no invocation is using it:

```sh
# Substitute actual absolute paths from your cache.
# Disable hooks consistently with the launcher.
git -c core.hooksPath=/dev/null --git-dir=/CACHE/relops/repos/OWNER/REPO.git worktree list
git -c core.hooksPath=/dev/null --git-dir=/CACHE/relops/repos/OWNER/REPO.git worktree remove --force --force /CACHE/relops/runs/OWNER-REPO-ID/checkout
rmdir /CACHE/relops/runs/OWNER-REPO-ID
```

Lock files persist intentionally; the OS releases their locks when the process exits. Do not delete a lock file to unblock a running invocation. A lock wait times out after 30 seconds with a retry diagnostic. Cache inspection/pruning commands are deferred.

## CLI presentation

Help, catalog discovery, and launcher diagnostics share a small ANSI palette.
See [CLI design and color system](docs/cli-design.md) for color roles, terminal
behavior, help structure, and the small-slant root-help ASCII wordmark.

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

Build recipes read the version from `VERSION` and embed it in the executable; `relops --version` prints it. Override with `just version=... signed-site` when needed. Direct `go build`/`go run` without the justfile report `dev`. Only `just publish` and `just publish-site` deploy; other recipes prepare artifacts locally. Cross-compilation does not validate runtime behavior on the target OS. `SHA256SUMS` detects file corruption; it is not a signed release manifest.

For shared macOS downloads, use `just signed-site` or `just publish`, on a Mac with Xcode tools and a Developer ID Application identity. `just site` is for unsigned previews and rebuilds ordinary binaries; do not run it after signing and then publish that preview. See the [macOS release workflow](quick_site/README.md#macos-signing-and-notarization) for credential setup, retries, and publisher overrides. `0.1.1` introduces this workflow; the already-published `0.1.0` files are left unchanged.

All build outputs stay in ignored `bin/` and `dist/` directories; no binaries need committing. The [site README](quick_site/README.md) describes the bundle, source/toolchain metadata, local preview, and later Quick publication. CI tests the launcher on Linux and macOS and uploads a generated site artifact without deploying it.

Integration tests use local Git repositories and temporary caches without accessing GitHub. They exercise owner isolation, ref changes, offline failures, ambiguous refs, concurrent runs, cwd/arguments/IO, runner detection, Pipenv/uv setup/reuse/failure, uv command selection and concurrent initialization, path containment, disabled checkout hooks, and process cleanup. Python runner checks skip if `python3` is unavailable. Normal tests use fake Pipenv/uv processes without accessing a package index; an opt-in `RELOPS_TEST_REAL_PIPENV=1 go test ./internal/launcher -run '^TestRealPipenv$'` checks the installed Pipenv using an empty dependency set.

Release tests use a fake HTTP transport and temporary caches to cover platform mapping, stable-release validation, checksums, updates, corruption repair, offline execution, concurrent installation, cancellation, and tool IO. An opt-in `RELOPS_TEST_REAL_FLEETBENCH=1 go test ./internal/launcher -run '^TestRealFleetbenchRelease$' -v` downloads a real release into a temporary cache and checks online/offline `--version` execution without running a benchmark.

Check terminal input, Ctrl-C, and foreground restoration interactively on each supported OS:

```sh
go test -c -o /tmp/relops-terminal-test ./internal/launcher
RELOPS_TEST_TERMINAL=1 /tmp/relops-terminal-test -test.run '^TestInteractiveTerminal$' -test.v
# Type hello and Enter, then press Ctrl-C at the prompt.
```

Before releasing, select and document three real organization tools (shell, Python with known prerequisites, and caller-relative file input), trial installation and SSH access with a coworker, and verify runtime behavior on macOS and Linux. Network catalog updates, Quick integration, and self-update are deferred.

Initial local validation: integration tests, `go vet`, and race checks passed on macOS arm64; the interactive terminal check passed for input, Ctrl-C (status 130), and foreground restoration. All four target binaries cross-compiled. Linux execution and coworker installation have not yet been verified.

The `0.1.1` macOS release completed Developer ID signing and Apple notarization. Both architectures passed signature and online notarization-ticket checks, and the generated signed site passed its download-selector tests. Browser installation of the published signed release remains a separate verification step.
