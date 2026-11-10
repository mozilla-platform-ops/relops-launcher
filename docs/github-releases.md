# GitHub releases and launcher updates

GitHub Releases hosts public binaries. Signing and notarization remain local;
GitHub does not need the Apple private key or the update-signing private key.
Ordinary CI builds are previews and are never uploaded as official releases.

## One-time publisher setup

Generate an Ed25519 update-signing key pair. Choose a private key path outside
this checkout, in a protected directory that is backed up securely:

```sh
go run ./cmd/relops-release --keygen --private-key /secure/location/relops-update.key
```

This creates the private key with mode 0600 and `release-public-key.txt` in the
checkout. Existing files are never overwritten. Commit the public key before
building the first updater-enabled release. Keep that same public key in future
builds: replacing it makes existing installations unable to verify new releases.
Key rotation and revocation require a separate migration plan. A build without
a public key can check for updates but refuses to install them.

Enable release immutability in the GitHub repository settings. Prepare every
asset while the release is a draft, then publish after verification.

## Release preparation

1. Bump `VERSION`, run `just check`, and commit the reviewed source and public key.
   This commit must happen before building production binaries, including for a
   site-only deployment. Confirm `git status --short` is empty before running
   `just signed-site` or `just publish`: Go embeds the source commit and dirty
   status in each binary. Committing afterward does not fix existing binaries;
   rebuild, sign, and notarize them from the clean commit. Keep preview builds
   separate from retained production releases.
2. Create the matching `vVERSION` tag at that clean commit. Push the commit and tag
   through the normal reviewed release process.
3. On the signing Mac, run `just signed-site` to build, sign, notarize, and retain
   the approved bytes. Do not rebuild them before uploading.
4. Set `RELOPS_UPDATE_PRIVATE_KEY` to the private key's path and run
   `just github-release-draft`. This checks the retained notarization receipt,
   build commit, clean-source status, version, and embedded public key; generates
   `update.json` and its detached signature; and uploads a draft release using
   `gh`. It requires the version tag to exist remotely and never creates one.
5. Review the draft, download the assets, compare their checksums with the retained
   files, and verify the macOS signature/notarization. Publish the draft only
   after approval, marking the stable release as latest.

The uploaded assets are `relops-darwin-arm64`, `relops-linux-amd64`, `SHA256SUMS`,
`update.json`, `update.json.sig`, and `LICENSE`. Public downloads use
`https://github.com/mozilla-platform-ops/relops-launcher/releases/latest`.
Releases are published manually; rerunning the draft recipe does not overwrite
an existing release. If an upload partially fails, inspect and repair the draft
with `gh release upload` before publishing it.

## Updating installations

```sh
relops update --check
relops update
```

Checks query GitHub's latest stable release without downloading executable assets.
Installation authenticates the signed manifest with the embedded public key,
checks the selected binary's SHA-256, and on macOS verifies the expected Developer
ID Team ID and notarization. Only a strictly newer stable version is installed.
Development versions require a manual release installation.

The updater stages the verified executable in the installation directory and
atomically replaces the old file. Failed download or verification leaves the old
installation intact. Symlink installations update their resolved target. An
unwritable directory produces an error; the updater never invokes sudo. Other
running launcher processes continue using their existing executable.

Updates are explicit: normal tool runs and offline runs do not check for launcher
updates. GitHub API failures, including rate limits, fail the update command;
no GitHub token is required or sent. Linux amd64 and macOS arm64 are supported.
The macOS Developer ID is currently the same Team ID as the existing release
workflow. Changing publishers requires a deliberate trust migration too.

Versions predating the updater need a one-time manual installation. Existing
retained releases cannot be repackaged into updater-enabled releases: use a new
version and a clean tagged build with the public key embedded.
