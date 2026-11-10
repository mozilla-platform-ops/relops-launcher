# Command reference

[← README](../README.md)

For a quick start, see the README. This page covers catalog selection, source execution, and cache recovery. See also [Python environments](python-environments.md) and [release binaries](release-binaries.md).

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

`relops info` also explains when to choose the tool. This guidance comes from the
catalog's tool-level `when_to_use` field, shared with the website's "When to use it"
section. Both channels use the same guidance; prerequisites remain specific to
each target. Catalog edits take effect in newly built launchers.

Use `relops repo` for arbitrary source paths and `relops run` (shortcut: `r`) for catalog tools. Bare targets are rejected with a suggested replacement:

```sh
relops repo android-tools/worker_health/create_tc_task.py --help
relops repo --ref my-branch android-tools/worker_health/create_tc_task.py --help
relops repo c/path/to/tool.sh --help          # repository named c
relops repo fleetbench                        # source project, rather than release binary
```

`run`, `r`, `c`, `catalog`, `info`, `i`, and `repo` are reserved as bare launcher commands. Use `relops repo c/path/to/tool.sh` for a repository named c. Flags may also precede the namespace, for example `relops --offline run fr --help`.

## Source tools

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
relops repo --owner YOUR_USERNAME --ref my-branch android-tools/worker_health/replay_tc_pool.py

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

## Execution and exit status

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
