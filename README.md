# RelOps Launcher

Run RelOps tools without cloning their repositories or setting up a Python environment by hand.

```sh
relops run fr --help
```

`relops` fetches source tools into temporary checkouts and caches their dependencies.
It also runs supported release binaries, such as fleetbench, and opens web tools.

## Install

Download the macOS Apple silicon or Linux amd64 binary from the
[download site](https://relops-launcher.quick.mozilla.cloud/) (Mozilla SSO).
Open it in your browser, then follow the site's checksum-and-install instructions.
Prebuilt binaries do not require Go.

Source tools need Git with working GitHub SSH access and the tool's interpreter.
Python projects also need `uv` or `pipenv` for their first run; see
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
