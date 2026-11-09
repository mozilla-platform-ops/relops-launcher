# Development and validation

[← README](../README.md)

## Local installation

From this checkout, build and install the current source (Go 1.23 or newer):

```sh
./install-local.sh --dev
```

The script installs to `~/.local/bin/relops`; ensure that directory is on your `PATH`. Run it again to install source changes.

On macOS Apple silicon, `./install-local.sh` without `--dev` installs the retained signed and notarized release matching `VERSION`. It verifies the checksum and Apple notarization, and reads `dist/releases/vVERSION` locally. If missing, prepare it with `just signed-site`. It does not download updates.

## Build and test

Run recipes from the repository root; `just` lists them. CI uses just 1.59.0. People installing a prebuilt launcher do not need just.

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

For shared macOS downloads, use `just signed-site` or `just publish`, on a Mac with Xcode tools and a Developer ID Application identity. `just site` is for unsigned previews and rebuilds ordinary binaries; do not run it after signing and then publish that preview. See the [macOS release workflow](../quick_site/README.md#macos-signing-and-notarization) for credential setup, retries, and publisher overrides.

All build outputs stay in ignored `bin/` and `dist/` directories; no binaries need committing. The [site README](../quick_site/README.md) describes the bundle, source/toolchain metadata, local preview, and later Quick publication. CI tests the launcher on Linux and macOS and uploads a generated site artifact without deploying it.

Integration tests use local Git repositories and temporary caches without accessing GitHub. They exercise owner isolation, ref changes, offline failures, ambiguous refs, concurrent runs, cwd/arguments/IO, runner detection, Pipenv/uv setup/reuse/failure, uv command selection and concurrent initialization, path containment, disabled checkout hooks, and process cleanup. Python runner checks skip if `python3` is unavailable. Normal tests use fake Pipenv/uv processes without accessing a package index; an opt-in `RELOPS_TEST_REAL_PIPENV=1 go test ./internal/launcher -run '^TestRealPipenv$'` checks the installed Pipenv using an empty dependency set.

Release tests use a fake HTTP transport and temporary caches to cover platform mapping, stable-release validation, checksums, updates, corruption repair, offline execution, concurrent installation, cancellation, and tool IO. An opt-in `RELOPS_TEST_REAL_FLEETBENCH=1 go test ./internal/launcher -run '^TestRealFleetbenchRelease$' -v` downloads a real release into a temporary cache and checks online/offline `--version` execution without running a benchmark.

Check terminal input, Ctrl-C, and foreground restoration interactively on each supported OS:

```sh
go test -c -o /tmp/relops-terminal-test ./internal/launcher
RELOPS_TEST_TERMINAL=1 /tmp/relops-terminal-test -test.run '^TestInteractiveTerminal$' -test.v
# Type hello and Enter, then press Ctrl-C at the prompt.
```

Before releasing, select and document three real organization tools (shell, Python with known prerequisites, and caller-relative file input), trial installation and SSH access with a coworker, and verify runtime behavior on macOS and Linux. Network catalog updates and broader Quick integration remain deferred. See [GitHub releases](github-releases.md) for launcher update signing and draft uploads.

## Release and presentation guides

- [Catalog onboarding audit and coworker trial](catalog-onboarding-audit.md)
- [GitHub releases and launcher updates](github-releases.md)
- [Website preview, signing, and publishing](../quick_site/README.md)
- [CLI design and color system](cli-design.md)
- [Design contract and roadmap](../design-v1.1.md)
- [Original design](original-design.md)
