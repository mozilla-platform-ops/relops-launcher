# RelOps Launcher

Run RelOps tools without cloning their repositories or setting up a Python environment by hand.

```sh
relops run fr --help
```

`relops` fetches source tools into temporary checkouts and caches their dependencies.
It also runs supported release binaries, such as fleetbench, and opens web tools.

## Install

Download the latest binary and `SHA256SUMS` from
[GitHub Releases](https://github.com/mozilla-platform-ops/relops-launcher/releases/latest)
(no sign-in required):

- macOS Apple silicon: `relops-darwin-arm64`
- Linux amd64: `relops-linux-amd64`

Verify the binary against the checksum file from the same release, then install it
as `relops` in a directory on your `PATH`. Prebuilt binaries do not require Go.

The [Quick download site](https://relops-launcher.quick.mozilla.cloud/)
also provides browser downloads and checksum-and-install instructions (Mozilla SSO).

Source tools need Git with working GitHub SSH access and the tool's interpreter.
Projects using `uv` or Pipenv need that manager for initial environment setup; see
[Python environments](docs/python-environments.md). Release binaries need none of these.

To build from this checkout, see [local installation](docs/development.md#local-installation).

## Find and run a tool

```sh
relops run                            # list tools
relops info fr                        # requirements and details
relops run fr --help                   # run fleetroll
relops run -b replay-tc-pool -h         # use a tool's beta target
relops run fb cpu --mode quick --json  # run the latest stable fleetbench binary
relops run pc                         # open Pool Classifier in a browser
```

`r` is short for `run`. Put launcher options before the tool name;
everything after it goes to the tool.

## Run from a branch or fork

Use `repo` to run source outside the catalog. For example, to try a pushed branch:

```sh
relops repo --ref my-branch android-tools/worker_health/create_tc_task.py --help
```

Repositories default to `mozilla-platform-ops`. Add `--owner USER_OR_ORG`
for a fork, or use `--ref` with a tag or full commit SHA.

For catalog source tools, you can override the default target's branch directly:

```sh
relops run --ref my-branch fr --help
```

## Things to know

- Source runs fetch on every launch. Use `--offline` before the target to reuse
  a previously fetched revision and prepared environment.
- Scripts start in your current directory; Python project commands start in the
  project directory. Use `--cwd caller` or `--cwd repo` to choose explicitly.
- Temporary checkouts are deleted when the tool exits. Save outputs outside them.
- Tools run with your environment and permissions. Only run code you trust.

Run `relops --help` for launcher options.

## Update

```sh
relops update --check
relops update
```

Updates download from GitHub Releases and run only when requested.
Older versions without the updater need a one-time manual installation.

## Documentation

- [Command reference](docs/usage.md) — catalog options, source execution, cache recovery
- [Python environments](docs/python-environments.md) — uv and Pipenv setup and reuse
- [Release binaries](docs/release-binaries.md) — fleetbench downloads, verification, offline use
- [Development](docs/development.md) — local builds, tests, and release checks
- [GitHub releases](docs/github-releases.md) — signing and publishing launcher updates
- [Download website](quick_site/README.md) — preview, macOS notarization, and publishing

## License and community

[Mozilla Public License 2.0](LICENSE) · [Mozilla Community Participation Guidelines](CODE_OF_CONDUCT.md)
