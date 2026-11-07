# relops site

Single-page download site for the `relops` launcher. The tracked HTML is a template; `just site` renders an unsigned preview, while `just signed-site` prepares Developer ID signed and notarized macOS downloads. Binaries and generated output are ignored by Git.

From `scripts/relops-launcher`:

```sh
just site-check
just serve
```

Open `http://localhost:8080`. `just site-check` requires Node for the download-selector/copy tests; `just site` requires just, Go, Git, and `shasum`. Go release builds use the compiler version recorded in `.go-version` in CI. For local reproduction, use that compiler version too. Override the release version with `just version=0.2.1 site` when needed.

For automatic rebuilds and serving in one terminal, stop any existing `just serve` process and run `just dev`. This requires `watchexec` (`brew install watchexec` on macOS). It builds unsigned binaries and the site immediately, then rebuilds and restarts the server when site or launcher sources change; refresh the browser after editing. It checks for changes every second. If you prefer separate terminals, use `just watch-site` alongside `just serve`. These commands update the local preview only; publishing still uses `just publish-site`.

Press Ctrl-C to stop the watcher and its server.

## Generated bundle

```text
dist/site/
  index.html
  css/ and js/                 current site assets
  releases.json               version, source commit/dirty flag, Go version,
                              build command, platform paths/sizes/checksums
  index.json                  inventory of every file in this bundle
  downloads/v0.1.1/
    relops-darwin-arm64
    relops-linux-amd64
    SHA256SUMS
```

`VERSION` supplies the release version; directory names follow it. The builder checks that all binaries have matching source/toolchain metadata and the expected platform, then hashes the exact downloadable bytes. A failed build leaves the previous generated site in place. Only the current homepage, CSS, and JS ship; concept pages, tests, documentation, source, and Git data do not.

`releases.json` records the actual embedded binary metadata, including whether the source checkout was dirty. Commit release source and rebuild before publishing so the source commit is reproducible and `source_dirty` is false. Release versions should identify fixed artifacts: bump `VERSION` for a changed published binary. Checksums detect corruption; they are not signatures. Quick's shared-write model still applies to the hosted files.

The footer places the lowercase relops wordmark with a static accent block on the left, with the program version, plain-text seven-character source revision, and a single Details link on the right. The version and revision come from the binary metadata rather than the website checkout's HEAD, so site-only deployments continue to identify the downloadable program accurately. Dirty builds append `-dirty` to the displayed revision. Details links to `releases.json`.

Go's dirty flag also includes untracked files elsewhere in the Git repository. Build official releases from a clean checkout or detached worktree when unrelated files are present. Check `source_dirty` in the prepared `releases.json` before deployment. Keep existing published bytes unchanged when correcting release provenance; publish a new version instead.

## macOS signing and notarization

Use a Mac with Xcode command-line tools and an available Developer ID Application identity. The configured publisher is `Developer ID Application: Andrew Erickson (W5BHZ9R8A3)`; private keys remain in Keychain. Ordinary builds and CI do not access signing credentials.

One-time setup:

1. In your Apple Developer Apple Account, generate an app-specific password at <https://account.apple.com/> under Sign-In and Security → App-Specific Passwords. Two-factor authentication must be enabled.
2. Run `just notary-setup` in your terminal. Enter your Apple ID and the app-specific password at the secure prompts. Credentials are saved under the `relops-notary` Keychain profile; never paste them into chat, files, or command arguments.

Equivalent direct setup command:

```sh
xcrun notarytool store-credentials relops-notary --team-id W5BHZ9R8A3
```

For each release, update `VERSION`, commit source, then run:

```sh
just signed-site
```

This builds macOS arm64 and Linux amd64 binaries, signs the macOS binary with Developer ID, hardened runtime, and a secure timestamp, and submits it in a ZIP to Apple. It records the submission ID before waiting, requires Apple's `Accepted` result, retrieves the log, and checks the online notarization ticket for the macOS binary. Then it exports the signed bytes, regenerates SHA-256 checksums, and assembles/tests `dist/site` with `--require-notarized`. That flag refuses missing, stale, rejected, or mismatched receipts. Linux binaries are unchanged by signing.

Submission state, signed bytes, and logs stay under ignored `dist/macos/`. A wait timeout or ticket propagation delay can be retried with the same `just signed-site` command. Identical inputs reuse the stored submission and signed artifacts. Changed inputs create a new signing cache. Rejected submissions fail and leave their log for diagnosis. Do not delete the cache during a pending submission. `just sign-macos` signs only and does not make a notarized release; `just notarize-macos` performs signing/notarization without assembling the website.

If Apple returns HTTP 403 saying a required agreement is missing or expired, the developer team must review pending agreements directly at <https://developer.apple.com/account/>. Credential storage can succeed and an upload can be accepted before this status-check error appears. After resolving the account agreement, rerun `just signed-site`; the saved submission ID is reused. The release recipe does not accept legal agreements on the account holder's behalf.

Standalone binaries cannot have a notarization ticket stapled to them. The ZIP is only for submission; the site still distributes individual binaries, and Gatekeeper retrieves their tickets online. First verification therefore needs internet access. An installer or disk image would be needed for a distribution with a stapled ticket. See [Apple's custom notarization workflow](https://developer.apple.com/documentation/security/customizing-the-notarization-workflow).

Signing adds secure timestamps, so a fresh signed build need not be byte-identical to a previous signed build. The receipt records the exact approved SHA-256 digests, and the cache retains those bytes for retries. Preserve published artifacts per version and use a new version for changed published bytes. `0.1.1` is the first release prepared through this workflow; previously published `0.1.0` artifacts are retained.

Publisher overrides use environment variables, not credentials in source:

```sh
RELOPS_SIGN_IDENTITY='Developer ID Application: Your Name (TEAMID)' \
RELOPS_TEAM_ID=YOURTEAMID \
RELOPS_NOTARY_PROFILE=your-keychain-profile \
  just signed-site
```

The public release manifest includes the signing identity, Team ID, submission ID, and approved hashes. The Keychain profile's credentials, submission ZIP, and private keys are not included in the website.

After validation, `just signed-site` also retains the two release binaries, notarization receipt, and checksums in gitignored `dist/releases/vVERSION/`. Preview builds can overwrite the top-level `dist/` binaries without touching these retained files. An existing version with different binary bytes is refused; use a new release version. Keep this directory for later site updates. It is local to this checkout and is not a backup shared through Git.

## Publishing

The site is published at <https://relops-launcher.quick.mozilla.cloud/>. For copy, style, or other website changes using the current release:

```sh
just publish-site
```

This rebuilds the site from current web sources and retained `dist/releases/vVERSION/` downloads, verifies checksums and the notarization receipt, runs the download/install tests, and deploys `dist/releases/vVERSION/site`. It does not rebuild, sign, notarize, or fetch binaries. `just site-from-release` performs the same preparation and checks without deploying. If the retained release is missing, preparation fails with instructions to run `just signed-site`. To retain already-prepared signed binaries from the top-level `dist/` directory, run `just retain-release`. Site assembly checks the version marker in each binary; retained releases before 0.2.1 lack this marker and cannot be assembled by the current builder.

For a new binary release, update `VERSION`, commit source, then sign, notarize, test, retain, and publish with:

```sh
just publish
```

That recipe uses `aerickson@mozilla.com` for Quick; other publishers can override `RELOPS_QUICK_ACCOUNT`. To deploy an already-prepared signed bundle directly, publish the generated folder, not this source folder or the whole project:

```sh
CLOUDSDK_CORE_ACCOUNT=aerickson@mozilla.com \
  quick deploy dist/site relops-launcher
```

This invocation was used successfully to publish the site. `CLOUDSDK_CORE_ACCOUNT` selects the publishing account for this command without changing the default gcloud account. Other publishers should substitute their own authenticated Mozilla account. Only `just publish` and `just publish-site` deploy automatically; build and signing recipes do not. Browser downloads use the staff member's Mozilla SSO session. Unauthenticated terminal downloads redirect to Google IAP sign-in HTML, even when the download command reports success. The site therefore requires a browser download, then supplies a command that verifies the checksum from `~/Downloads`, installs into `~/.local/bin`, sets PATH for the current terminal, and checks the version. Individual steps and persistent shell PATH setup remain in a disclosure. Custom download folders require editing the Downloads path. Launcher self-update is not provided. Quick publishers need its CLI/credentials; people downloading `relops` do not.

Use normal deployment without `--mirror` to retain older versioned downloads already on Quick. The local bundle and `index.json` describe the current release only. Do not republish different bytes under a previously shared version. If CI later owns deployment, use that workflow consistently rather than alternating manual and CI publishers.

After publishing, verify in an authenticated browser that each platform link downloads the correct filename, verify its checksum, install the native binary, and check `relops --version`. Verify macOS's first-download approval flow and trial the fork example with a coworker. These are still live-environment release checks; a local build cannot prove Quick authentication or download headers.

The repository workflow `.github/workflows/relops-launcher.yml` runs tests on Linux/macOS and saves an explicitly named unsigned preview site artifact. It does not access signing credentials, submit to Apple, publish to Quick, or commit binary files. Its CI runs will be verified after these changes are pushed.

## Source files

The Tools section is rendered at build time from `tools.json`, so it works without JavaScript; the existing copy buttons enhance it. This is also the launcher’s embedded catalog, loaded through `catalog.go` and validated by `internal/catalog`. Both consumers use schema version 1 and a human-readable catalog revision. Bump the revision when editing entries; a launcher rebuild is required to ship changes. The catalog is not downloaded at runtime.

Each tool supplies a unique `id` (the canonical shortcut), optional `aliases`, display name, description, and optional `default` and `beta` targets. At least one target is required; beta-only prototypes are supported. All IDs and aliases share one namespace and use lowercase letters, digits, dots, underscores, and hyphens, starting with a letter or digit. Names are claimed through Git review and should stay attached to the same tool.

Targets supply `kind` (`source` or `release`), GitHub `owner` and `repo`, optional `path` and `ref`, optional `command`, `runner`, and `cwd`, plus `example_args`, prerequisites, documentation, and an optional visible note. Source refs can be branches, tags, or full commit SHAs; omitted refs use the actual remote default branch. Each channel is a full independent target, allowing forks and different paths. Release targets currently support Fleetbench’s latest stable binary only.

Website commands use `relops run NAME` or `relops run -b NAME`, followed by shell-quoted `example_args`. Examples are never injected by the launcher. The list shows default targets first, followed by beta targets, preserving catalog order in each group. Aliases appear as comments above the example commands. Beta targets show a Beta badge; defaults make no stability guarantee. The source JSON is embedded in binaries, but is not copied into the generated website.

These shortcut commands require the launcher build containing catalog support. Publish them with a new launcher release; the builder rejects older or different retained binaries unless they embed the exact website catalog. Editing the catalog therefore requires rebuilding and releasing the launcher before publication.

Run `just check-tools` to check the catalog against GitHub using your existing Git SSH access. It resolves each branch, tag, full commit SHA, or default branch, fetches that commit into a temporary bare repository, and checks that the script exists. Project targets must contain `pyproject.toml` and `uv.lock`. Each source result includes the tool ID, repository, ref, path, and resolved commit; failures stop the command. Release entries report that assets are verified at launch; this command does not download or validate release assets. Tools are never executed. Each entry has a two-minute timeout, and temporary repositories are removed afterward. This checks availability, not runtime dependencies or behavior. Ordinary builds and tests remain offline; `just publish-site` and `just publish` run this live check before preparing and deploying the site. Direct `quick deploy` commands bypass it.

```text
index.html          current template: hero install command, browser download/install steps, usage examples, blob
tools.json          curated tool entries used to generate the Tools section
future.html         concept site for later releases (terminal demo, contract,
                    roadmap, and the living design system in section 06)
future2.html        earlier index: overview, install, usage, chrome blob
future3.html        moire-hero variant of the trimmed index
css/tokens.css      design tokens: color, type, space, motion, accent presets
css/components.css  components built only from tokens
css/moire.css       animated moiré hero object (pointer-reactive, uses --accent)
css/blob.css        mesh-gradient blob hero (complement of accent, drifting points)
js/main.js          progressive enhancement: copy, OS detection, theme/accent,
                    platform-specific Downloads install/checksum selection, live blob
                    silhouette, moire pointer hook, terminal demo
                    (future.html only). Each feature is guarded on missing elements.
tests/downloads.test.cjs   tests against the generated release data and JS
```

## Design system rules

- Monospace only. Square corners. Borders, not shadows.
- One accent at a time (default: volt). Re-theme by changing `--accent-h` or `data-accent`.
- Art uses the accent's complement (`--comp-h`), so it follows re-themes.
- Light theme: lime is fill-only (`--accent-fill`); never use it as text. Use `.mark` for emphasis.
- Persist user choices (theme/accent) only on explicit clicks, never defaults.
- Status colors (`--ok`, `--warn`, `--err`) carry meaning only, never decoration.
- Add a token before adding a raw value. Components never hardcode colors or sizes.
- New component: add it to `components.css`, then show it in the `#system` section.
- Plain voice. State limits up front.

## The blob

The hero blob is two layers working together.

**Inside (CSS, `css/blob.css`):** a mesh gradient made only of round balls.
Six color points tinted with the accent's complement (`--comp-h`), plus white
highlight balls, tinted shadow balls, and one roaming specular highlight.
Each point moves along its own irregular path using registered `@property`
variables. Loop lengths are coprime (29, 37, 43, 53, 59, 71s; highlight 47s), so
the combined pattern effectively never repeats. Points blend with `screen`.
Avoid conic gradients and pure black here: conic reads as a star/pinwheel, and
black makes the orb look dead. Shadows are deep violet instead.

**Outline (JS, `js/main.js`, "live silhouette"):** the edge is a closed curve
whose radius is a sum of slowly drifting sine harmonics, each with its own
speed and phase (`H` array). Every animation frame it is written as a
`clip-path: polygon(...)` of 160 points on `.blob__body`, so the shape never
loops. Each frame is rescaled so the shape touches the box edge, plus a slow
"breathing" scale.

- Tune the feel with `H`: `[harmonic, amplitude, speed rad/s, phase]`.
  Low harmonics (1-3) with small amplitudes give a soft pebble; high harmonics
  or large amplitudes make it lumpy and amoeba-like. Current values are
  deliberately gentle.
- JS adds `.blob--live` to `.blob`, which turns off the CSS `border-radius`
  morph. That morph is the fallback for no-JS and `prefers-reduced-motion`.
- The loop pauses when the blob scrolls offscreen (IntersectionObserver) and,
  via `requestAnimationFrame`, when the tab is hidden.
- Reduced motion: static shape, no color-point or highlight animation.
- Smooth color-point motion needs `@property` (Chrome, Safari 16.4+,
  Firefox 128+). Older browsers get a static blob.

## Concept pages

The `future*.html` pages are retained design experiments and do not ship. Their terminal demo output is illustrative and their download links are placeholders. Preview them separately from the generated release site.

Run `just inspect-tools` for an optional advisory inspection of catalog source targets. It uses the same Git SSH access and resolves the same refs as `just check-tools`, then reads committed source without executing the tools. Python 3 is required for Python AST analysis. Findings include the target, resolved commit, source location, and rule. Warnings do not fail the command; fetch, target validation, and inspector errors do.

The Python inspector scans the selected script (including extensionless Python shebangs), or all `.py` files under a selected project directory. It flags recognizable home-directory data files missing from the target's `data_files` list, writes using `__file__`, and literal relative writes when the default working directory is temporary. Declare user-facing file paths in `data_files`; they appear in `relops info`. List names only, never credential contents. Scope requirements still belong in prerequisites or linked documentation. Dynamic paths, imported code outside the selected project, aliases, control flow, and many file APIs are not fully analyzed, so a clean result is not a safety guarantee. Shell scripts, release binaries, and web targets are explicitly skipped. The inspector does not check scopes or execute help commands.
