# RelOps Launcher — Design v1.1

**Status:** Initial v1 implementation in this directory; release validation pending

**Design revision:** 1.1, describing the first product release, **v1**

**Command:** `relops`

**Predecessor:** [Original design](../../product-specs/relops-launcher-design.md)

## Outcome

A developer pushes a tool to a branch and gives a coworker one command to try it:

```sh
relops --ref my-feature relops-infra/path/to/check.sh --verbose
```

The coworker has installed `relops`, has Git access to the repository, and has the tool's runtime prerequisites. They do not need to clone the repository, change branches, or find the script locally. The branch must be pushed; local uncommitted changes are not shared.

This is a useful v1 by itself. Registration, a hosted catalog, and launcher self-update are subsequent releases. The installation step remains separate; this is one command to run a tool after installation, not a promise of zero prerequisites.

Examples use illustrative repository and script names.

## V1 scope

- Execute an explicit `repo/path` in `mozilla-platform-ops`, or select another GitHub owner with `--owner`.
- Resolve the repository's actual default branch, or an explicit branch, tag, or commit.
- Cache Git objects while giving each invocation a separate writable checkout.
- Preserve tool arguments, interactive input, output, exit status, and interruption behavior.
- Provide explicit working-directory and runner choices.
- Default to the caller's directory and automatically detect shebang/Python runners and Pipenv projects.
- Support intentional offline execution and report the executed commit.
- Ship a single launcher binary for macOS and Linux, with documented installation.

V1 does not include aliases, registry editing, Quick integration, automatic launcher updates, a general dependency manager, Windows, or execution of repository setup hooks. It is not a sandbox. Automatic Pipenv setup installs locked runtime dependencies through an already-installed Pipenv; tools can change external state.

## Command contract

```sh
# Repository's default branch
relops relops-infra/path/to/check.sh --verbose

# Share a pushed branch; launcher flags precede the target
relops --ref my-feature relops-infra/path/to/check.sh --verbose

# Share a branch from a personal fork; auto detects Python and Pipenv
relops --owner aerickson --ref 022526-tool_tweaks android-tools/worker_health/replay_tc_pool.py

# Share an exact revision for reproducibility
relops --ref <full-commit-sha> relops-infra/path/to/check.sh

# Select an interpreter for a script without an executable shebang
relops --runner python3 --ref my-feature relops-infra/path/to/check.py

# Caller-relative file arguments work by default
relops --ref my-feature relops-infra/path/to/check.sh ./hosts.csv

# Reuse cached refs and objects without attempting network access
relops --offline --ref my-feature relops-infra/path/to/check.sh
```

Syntax: `relops [launcher-options] <repo>/<path> [tool-arguments...]`.

The first positional argument ends launcher option parsing. Everything after it is passed unchanged to the tool, including `--help`, `--ref`, and `--`. `relops --help` describes the launcher; `relops repo/tool --help` asks the tool for help. Launcher errors exit with status 1 and a diagnostic on stderr; usage errors exit with status 2. Once launched, the tool's exit status is preserved. A signal-terminated tool uses the conventional `128 + signal` status.

Initial options:

| Option | Meaning |
| --- | --- |
| `--owner VALUE` | GitHub user or organization; default `mozilla-platform-ops` |
| `--ref VALUE` | Branch, tag, or commit; omitted means remote default branch |
| `--runner auto\|python3\|bash\|sh` | Execution rule; default `auto` |
| `--cwd repo\|caller` | Tool working directory; default `caller` |
| `--offline` | Make no network requests; require cached resolution and objects |
| `--verbose` | Additional cache and resolution diagnostics on stderr |
| `--version` | Launcher version |
| `--help` | Launcher usage |

Automatic detection honors shebangs, then falls back to Python for `.py` files. It never guesses a shell from `.sh`. Explicit runners override detection, including Pipenv environment selection. No shell command strings. More runners can be added when a real tool requires them; `uv` integration remains deferred.

## Revision resolution and updates

V1 favors predictable branch sharing over a TTL policy: every online invocation fetches before resolving the requested revision. Git object reuse avoids a fresh clone on each invocation. A five-minute TTL can be considered later if fetch latency justifies the extra behavior.

- No `--ref`: refresh remote metadata, discover the actual default branch, and resolve its fetched tip.
- Named branch or tag: fetch and resolve that remote ref, including branch names containing `/`.
- Full commit SHA: ensure that commit is locally available, fetching as necessary. Some server configurations may not allow fetching an otherwise unreachable commit; return an actionable error in that case.
- Bare names matching both a branch and tag: fail as ambiguous. Accept `refs/heads/...` and `refs/tags/...` to disambiguate.
- V1 requires a full SHA for commit selection; abbreviated SHAs are not part of the initial contract.
- A missing ref or failed online fetch is an error. Do not silently execute stale code. Suggest `--offline` when cached execution is possible.
- `--offline` resolves using the last successfully fetched refs/default-branch metadata and existing objects. Clearly label that resolution as cached.

Each run writes a short provenance line to stderr before starting the tool, including repository, requested ref, full resolved SHA, and whether resolution was offline. Keep stdout exclusively for tool output. This makes the executed SHA available in captured logs without requiring verbose mode. A branch or tag can move; a full SHA identifies the selected code. Neither pins external dependencies or external services.

V1 never automatically updates the launcher itself. Later registered operational commands should explicitly choose a moving branch or a pinned commit; adding an alias must not obscure that choice.

## Git access and cache

Use the installed `git` executable and the user's existing Git authentication. V1 uses `git@github.com:<owner>/<repo>.git`, with `mozilla-platform-ops` as the default owner, and therefore requires working SSH access. Do not add a second GitHub token store or invoke interactive authentication setup. Document how to verify repository access and report clone/fetch errors with the relevant repository URL.

Validate the repository as a single GitHub repository name and owner as a user/organization name; normalize both to lowercase. Construct the remote URL internally; reject user-supplied URLs, owner path components, and option-like names. Invoke Git using argument arrays and explicit option boundaries, never shell interpolation.

Suggested layout, using the platform's user cache directory:

```text
relops/
  repos/<owner>/<repo>.git/  # shared bare object store and fetched refs
  locks/<owner>/<repo>.lock  # repository mutation lock
  runs/<unique-run-id>/      # detached worktree for one invocation
  envs/<owner>/<repo>/<commit>/<project-hash>/  # managed Pipenv environments
```

Keep private cache directories accessible only to the current user. Lock clone/fetch/ref metadata/worktree bookkeeping mutations per repository, releasing the lock before running the tool. A partially initialized clone must never be reused as a healthy cache entry. Use bounded lock waiting and an actionable timeout error.

Create a fresh detached worktree at the resolved commit for each invocation, including concurrent invocations of the same SHA. Never check out or reset a shared runnable checkout. Remove the run's worktree after the child exits, including files the tool generated there. Document that files the user wants to retain must be written outside that temporary checkout. An uncatchable termination can leave an orphan; report its location and provide manual recovery instructions in v1. Automatic pruning can follow later.

Separate worktrees isolate repository-local writes. They do not isolate home-directory caches, credentials, external services, or other tool-managed shared state. Do not describe this as sandboxing.

V1 does not automatically initialize submodules or download Git LFS content. Tools needing those are outside the initial support contract; document that limitation in installation and usage guidance.

## Execution and working directory

Resolve the script within the run's checkout. Reject absolute paths, `..` components, missing/non-file targets, and symlinks whose resolved target escapes the checkout. Resolve the executable to an absolute path so changing the working directory does not change which file executes.

Runner behavior:

- `auto`: honor a shebang, invoking executable scripts directly or invoking the declared interpreter for non-executable scripts. Fall back to `python3` for `.py` without a shebang. For Python scripts, detect the nearest Pipfile within the checkout and use the managed Pipenv environment described below.
- `python3`, `bash`, `sh`: override detection, including Pipenv; look up that executable on the caller's `PATH`, then pass the absolute script path followed by the untouched tool arguments.
- Do not infer Bash compatibility from `.sh`, run repository setup hooks, or install missing interpreters/Pipenv.

Default `--cwd caller` preserves the directory from which the user invoked `relops`. `--cwd repo` starts the tool at the checkout root. Export `RELOPS_CALLER_CWD` with the original absolute directory in both cases. Relative tool arguments are passed unchanged: with the default working directory, `./hosts.csv` refers to a file in the caller's directory. The launcher never rewrites path arguments.

For automatic Pipenv execution, require an installed Pipenv and a committed, current `Pipfile.lock`. Verify the lockfile and sync runtime dependencies into an environment keyed by owner, repository, commit, and project path. Cache successful initialization and serialize setup with an environment-specific lock, released before running the tool. The environment persists independently of temporary checkouts. Do not install dev dependencies; reject editable runtime dependencies that would retain a link to a removed checkout. Offline mode requires an already-prepared environment and never performs setup. Missing Pipenv, missing/outdated lockfiles, or setup failures produce actionable errors before executing the tool. Explicit runners bypass this behavior. Automatic setup does not load project `.env` files. Environment pruning is deferred.

Inherit the environment and stdin/stdout/stderr. Do not capture or reformat tool output. Preserve terminal interaction and forward interruption/termination to the tool's process group, wait for it to exit, then clean up the worktree. Terminal foreground-process-group behavior must be verified with an interactive tool before declaring this complete.

## Implementation proposal

Use Go for a compiled launcher with a small dependency surface, delegating Git operations to the installed Git executable. Go is the accepted implementation choice. The initial implementation uses only the standard library, per-repository OS file locks, and a child process group with foreground terminal transfer when interactive.

Suggested modules:

| Module | Responsibility |
| --- | --- |
| CLI | Option boundary, validation, help, exit status |
| Git cache | Remote access, locking, ref resolution, per-run worktrees |
| Execution | Runner, cwd, environment, process lifecycle, cleanup |
| Provenance | Consistent source/ref/SHA diagnostics |

Initial binaries: macOS arm64/amd64 and Linux arm64/amd64. Select and document a minimum supported OS version during the build spike. First distribution can use manually downloaded release artifacts with published SHA-256 checksums. Choose and verify an accessible artifact host before the coworker trial; Quick is not a prerequisite for the execution implementation.

## Points to resolve before implementation

These carry forward the review of the original design. Proposed resolutions are explicit so they can be accepted or changed before coding.

| Point | Proposed resolution | Gate |
| --- | --- | --- |
| Canonical registry and competing publishers | Store the registry in Git alongside launcher source. CI builds the catalog and publishes artifacts. Git review/history/reverts supply change review and rollback. Drop download–modify–redeploy editing. A future `registry` command can prepare a Git change. | Before registry implementation; no registry in v1 |
| Authentication for Quick runtime assets | Prototype unattended authenticated reads before choosing Quick for registry downloads or self-update. Quick may serve only a human-facing catalog if that is simpler. Do not assume browser authentication works for a CLI. | Before selecting those later-release backends; v1 artifact download must still be verified |
| Automatic updates to operational tools | Online v1 fetches each run; offline reuse is explicit. Always report SHA. Share a full SHA when the intended review is of exact code. Later registry entries must make moving versus pinned revisions visible. | Implemented in v1 |
| Writable state in commit-specific checkouts | One worktree per invocation; object storage is shared. Explain remaining external shared state and temporary output cleanup. | Implement in v1 |
| Working directory and relative arguments | Default caller directory; explicit repo mode; preserve original directory in `RELOPS_CALLER_CWD`. Never rewrite tool arguments. | Updated contract implemented in v1 |
| Interpreter guessing | Honor shebangs; `.py` fallback to Python; nearest Pipfile uses installed Pipenv. No shell guessing from `.sh`. Explicit runners override all detection. | Updated contract implemented in v1 |

Additional implementation gates:

1. Validate the SSH-only Git contract against the intended first coworkers. If HTTPS support is needed, choose a documented transport configuration before implementing cache URLs.
2. Confirm Go, minimum OS versions, and the process-group/locking approach with a small macOS/Linux spike.
3. Select three actual trial tools: an executable shell script, a Python script with known prerequisites, and a tool consuming caller-relative input. Record their real commands and expected results as fixtures/manual trial instructions. Do not promise arbitrary existing tools work before this trial.
4. Choose the initial binary host and verify a coworker can download and install an artifact with the documented permissions.

These are decisions and focused feasibility checks, not reasons to build the deferred registry/distribution platform first.

## Acceptance and validation

V1 is complete when a coworker can install a binary, receive a branch command, and run a real tool without managing a checkout.

Use local temporary Git remotes for automated integration tests, with a test-only transport seam rather than weakening production repository validation. Test meaningful contracts:

- First clone and later object reuse; non-`main` default branch; branch names with slashes; updated branch tips; tags; full SHA; ambiguous/missing refs.
- Offline reuse after a successful run, cache misses, and online fetch failure without silent stale execution.
- Two simultaneous runs at the same SHA cannot see each other's repository-local generated files; concurrent fetch/worktree operations leave a usable cache.
- Arguments containing spaces, option-like tool arguments, stdin, stdout/stderr, and nonzero exit statuses pass through unchanged.
- Working-directory choices resolve caller input as documented; escaping paths and symlinks are rejected.
- Unsupported scripts and missing interpreters produce actionable errors; no setup hooks run automatically.
- Interrupting a run stops its child processes and cleans up the worktree.

Manually verify the three real trial tools, SSH access, binary installation, and interactive Ctrl-C behavior on macOS and Linux. Cross-compilation alone does not validate execution on a platform.

## Tentative release roadmap

| Release | User outcome | Features / gates |
| --- | --- | --- |
| v1 | “Try this tool on my branch.” | Explicit repo/path, refs, shared object cache, per-run checkout, runners/cwd, offline mode, provenance, manual installation |
| v1.1 | “Find and run our usual tools.” | Git-backed curated registry, aliases, `list`/`info`, visible revision policy, cached registry; verify authenticated retrieval first |
| v1.2 | “Install and update the launcher easily.” | Verified binary hosting/authentication, release manifest, safe self-update, CI publication; optionally Quick-hosted catalog |
| v1.3 | “Keep everyday use convenient.” | Cache inspection/pruning, shell completion, optional TTL with explicit refresh controls; prioritize from actual usage |
| Later | “Add a shared tool with less ceremony.” | Registry editing that prepares reviewed Git changes; repository metadata and richer runners if justified |

Release assignments after v1 are planning placeholders. Keep them independently useful and adjust them from trial feedback. Direct registry mutation on a deployed site is superseded by the Git-backed registry proposal; it is not deferred implementation work.
