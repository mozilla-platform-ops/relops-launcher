# Release binaries (fleetbench)

[← README](../README.md)

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

For updates to the launcher itself, see [GitHub releases and launcher updates](github-releases.md).
