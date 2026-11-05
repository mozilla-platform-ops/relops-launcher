# RelOps Launcher — Design (v1)

**Status:** Proposed
**Repository:** `mozilla-platform-ops/relops-launcher`
**Quick site:** `https://relops-launcher.quick.mozilla.cloud/`
**Command:** `relops`

## Summary

`relops` is a small, `uvx`-inspired CLI launcher for scripts and tools stored in repositories under the `mozilla-platform-ops` GitHub organization. Users install one executable, then invoke tools without manually cloning or updating repositories. A curated registry adds friendly aliases and descriptions, but **registration is not required** to execute an existing script.

A Mozilla Quick site distributes launcher binaries, installation instructions, a browsable catalog, the machine-readable registry, and a bounded registry revision history. The `relops` CLI abstracts Quick away, including registry editing and publishing.

## Goals

- Run existing executable scripts in any organization repository with one command and no repository changes.
- Clone repositories lazily, cache them, and refresh them automatically with a configurable TTL.
- Support default-branch execution plus explicit branch, tag, or commit overrides.
- Offer friendly registered commands and basic discovery (`list`, `info`, eventually `search`).
- Make registry changes and rollbacks through `relops`, without requiring users to know Quick's deployment workflow.
- Distribute and update the launcher itself from the Quick site.
- Preserve normal CLI behavior: argument passthrough, stdout/stderr, exit codes, and signals.

## Non-goals (v1)

- A general-purpose dependency/package manager or sandbox.
- Automatically executing repository setup scripts.
- Requiring repository-local metadata or modifying existing tools.
- A browser-based registry editor or backend service.
- Searching every file in every organization repository.
- Strong concurrent-edit coordination for registry publishing.

## User experience

```sh
# Unregistered tool: repo/path, using the repository's default branch
relops relops-infra/path/to/check.py --verbose

# Registered alias
relops fleetroll status

# Override a tool's revision (branch, tag, or commit)
relops --ref feature-branch fleetroll status
relops --ref abc1234 relops-infra/path/to/check.py

# Discover curated tools
relops list
relops info fleetroll

# Manage the registry (requires publishing access and Quick tooling)
relops registry add fleetroll
relops registry edit fleetroll
relops registry remove fleetroll
relops registry history
relops registry rollback 3

# Refresh cached repositories / launcher
relops update
relops update fleetroll
relops self-update
```

Exact command flags and interactive edit UX can be refined during implementation. `repo/path` uses actual GitHub repository names; examples above are illustrative.

## Architecture

### 1. Resolution

For each invocation:

1. Parse launcher options separately from tool arguments.
2. If the target is an explicit `repo/path`, resolve it directly against `mozilla-platform-ops`.
3. Otherwise, look up the alias in the downloaded registry.
4. Resolve the repository, requested revision, executable path, runner, and working directory.
5. Ensure the requested repository revision is available in the local cache.
6. Execute the tool, passing arguments and propagating exit status/signals.

A Quick outage must not prevent execution of explicit `repo/path` targets when the required Git objects are cached. Registered aliases use a locally cached registry when offline.

### 2. Git checkout cache

Suggested layout:

```text
~/.local/share/relops/
  repos/              # cached Git repositories / shared object stores
  worktrees/          # isolated checkouts keyed by resolved commit
  registry.json       # cached registry
  releases.json       # cached launcher release manifest
```

- Clone **on first use**, not during installation.
- Refresh remote refs only when the cache TTL expires (initial default: five minutes), or on explicit `relops update`.
- Use the repository's actual default branch; never assume `master` or `main`.
- Resolve mutable refs to commit SHAs and execute against isolated, commit-specific worktrees. Never switch a shared checkout underneath another invocation.
- Support concurrent invocations, offline reuse, and cache pruning later.
- Display the resolved revision in verbose output for troubleshooting and reproducibility.

### 3. Execution rules

For unregistered scripts:

1. If executable and has a valid shebang, invoke it directly.
2. Otherwise, use a small documented mapping for recognized extensions (for example `.py` → `python3`, `.sh` → `sh` or `bash` as appropriate).
3. If no safe execution rule applies, return an actionable error.

Registered tools can specify an explicit runner and working directory. Default working directory: repository root; registered tools may override it. Do not silently install arbitrary dependencies or execute setup hooks. Tools remain responsible for their own runtime prerequisites; `uv run` can be an explicitly configured runner.

**Execution safety:** Treat code in the organization as trusted only within the organization's normal code-review/security model. Show the resolved source and revision on request; do not silently execute code merely because it appears in a registry. Registry edits can redirect execution, so publishing access and review/audit expectations matter.

### 4. Curated registry

The Quick site serves a static, versioned JSON registry. Example:

```json
{
  "schema_version": 1,
  "tools": {
    "fleetroll": {
      "description": "Inspect fleet configuration rollouts",
      "repo": "fleetroll_mvp",
      "path": "cli/fleetroll.py",
      "ref": "main",
      "runner": ["uv", "run", "python", "cli/fleetroll.py"],
      "cwd": "repo"
    }
  }
}
```

`runner` is an argument vector rather than a shell command string to avoid accidental shell interpretation. Define clearly whether the script path is included in the runner or appended by the launcher; the example uses the former. Validate aliases, repo ownership, paths, and schema before publishing. Registry revisions can pin a branch, tag, or commit; users can override with `--ref`.

Registry access should be cached and independent of the Quick publishing implementation. Explicit repository paths need no registry entry.

### 5. Quick distribution site

Proposed deployed layout:

```text
https://relops-launcher.quick.mozilla.cloud/
  index.html                 # installation guide + curated tool catalog
  index.json                 # manifest of downloadable site artifacts
  registry.json              # current tool definitions
  releases.json              # versions, binary paths, SHA-256 digests
  binaries/
    relops-darwin-arm64
    relops-darwin-amd64
    relops-linux-amd64
    relops-linux-arm64
  history/
    2026-11-05T20-42-00Z.json
    ...
```

The root document should link to **all artifacts required to reconstruct the deployment**; `index.json` provides a straightforward machine-readable inventory. If `index.html` is the canonical root, it should link to `index.json` and every other published artifact, including history and binaries. The site can display the curated tool catalog directly from `registry.json`.

Quick sites are staff-only behind Identity-Aware Proxy; do not present the site or binaries as publicly downloadable. Browser downloads are the initial installation path unless an authenticated CLI download mechanism is verified. Quick is suitable only for the permitted Mozilla data classification, not secrets or load-bearing services.

### 6. Registry edit / publish lifecycle

`relops registry ...` hides the Quick-specific workflow:

1. Fetch the current artifact manifest and download **all listed files** into a temporary working directory.
2. Validate the downloaded registry and manifest.
3. Apply the requested change to a local copy.
4. Archive the previous registry in `history/` with a timestamp (optionally include editor and change description).
5. Keep the latest configurable number of history snapshots (suggested default: 20).
6. Regenerate the artifact manifest and homepage/catalog as needed.
7. Validate that every manifest entry exists and all retained binaries are preserved.
8. Publish the entire directory using `quick deploy .`.

**Important:** Confirm Quick's supported deployment-download, authentication, and update semantics against the version-matched `.claude/skills/quick/SKILL.md` generated by `quick init`. The manifest approach assumes that authenticated clients can fetch linked site artifacts. No database or browser editing UI is required.

Quick's instructions say any Mozilla staff member can overwrite a site. Concurrent editors may overwrite one another; v1 should warn about this and re-fetch/revalidate immediately before deploy where possible. Git remains the source of truth for the launcher and site-building code; published registry snapshots provide lightweight operational history, not a substitute for protected change review.

### 7. Binary distribution and self-update

- Publish platform-specific binaries and a `releases.json` manifest with version and SHA-256 digest.
- Provide browser-based installation instructions for each supported platform.
- `relops self-update` checks the release manifest, downloads the appropriate binary using a **verified authentication method**, checks its digest, and replaces the installed binary safely.
- Keep binary release publishing separate from registry edits: registry publication must preserve existing binaries, and binary releases must preserve the registry/history.
- Users running tools do **not** need the Quick CLI. Registry publishers need the Quick CLI and valid `gcloud` credentials.

## Implementation modules

| Module | Responsibility |
| --- | --- |
| `launcher` | Parse commands, resolve aliases and repo paths, execute tools |
| `gitcache` | Lazy clone/fetch, resolve revisions, isolated worktrees |
| `registry` | Schema, validation, cache, history, editing |
| `publisher` | Download Quick artifact inventory, rebuild, deploy |
| `release` | Binary manifest, checksums, self-update |

The launcher must not depend on Quick's SDK or deployment mechanism for ordinary execution; Quick is a replaceable registry/distribution backend.

## MVP acceptance criteria

- A user can install `relops` from the internal Quick site and run an unregistered script from an organization repository.
- The first invocation clones the needed repository; subsequent invocations use the cache and refresh on TTL expiry.
- Branch and commit overrides work without mutating a shared checkout.
- Tool arguments, output, exit status, and interruption behavior are preserved.
- Registered aliases resolve from a Quick-hosted JSON registry; `list` and `info` work with a cached copy.
- `registry add/edit/remove/history/rollback` can publish a complete site without losing unrelated artifacts or retained history.
- Launcher binary releases are downloadable with documented authentication requirements and verifiable checksums.
- Offline cached execution works where the required registry/revision/runtime is already present.

## V2 possibilities

- Repository-local `.relops.yaml` metadata to advertise commands and runners, without central registration.
- Cross-repository discovery/search and shell completion.
- Richer dependency/environment support where justified.
- Cache inspection/pruning and optional revision pinning policies.
- Stronger optimistic concurrency checks or reviewed registry publishing if edit frequency grows.

## Open decisions

- Implementation language and cross-platform packaging (a single compiled binary is desirable).
- Exact CLI option ordering and syntax for unregistered repo paths versus aliases.
- Supported extension/interpreter mappings and whether non-executable `.py` files run by default.
- Authentication mechanism for fetching Quick assets outside a browser, particularly for self-update and registry reads.
- Whether Quick deployment exposes a reliable way to detect intervening edits before publication.
- Where the canonical editable registry lives: Quick snapshots only, or mirrored to Git for stronger auditability.

## Guiding principle

> Install one binary. Run any RelOps tool. Registration improves discoverability, but is never a prerequisite for execution.
