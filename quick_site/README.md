# relops site

Single-page download site for the `relops` launcher. The tracked HTML is a template; `just site` renders an unsigned preview, while `just signed-site` prepares Developer ID signed and notarized macOS downloads. Binaries and generated output are ignored by Git.

From `scripts/relops-launcher`:

```sh
just site-check
python3 -m http.server 8080 --bind 127.0.0.1 --directory dist/site
```

Open `http://localhost:8080`. `just site-check` requires Node for the download-selector/copy tests; `just site` requires just, Go, Git, and `shasum`. Go release builds use the compiler version recorded in `.go-version` in CI. For local reproduction, use that compiler version too. Override the release version with `just version=0.2.0 site` when needed.

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
    relops-darwin-amd64
    relops-linux-arm64
    relops-linux-amd64
    SHA256SUMS
```

`VERSION` supplies the release version; directory names follow it. The builder checks that all binaries have matching source/toolchain metadata and the expected platform, then hashes the exact downloadable bytes. A failed build leaves the previous generated site in place. Only the current homepage, CSS, and JS ship; concept pages, tests, documentation, source, and Git data do not.

`releases.json` records the actual embedded binary metadata, including whether the source checkout was dirty. Commit release source and rebuild before publishing so the source commit is reproducible and `source_dirty` is false. Release versions should identify fixed artifacts: bump `VERSION` for a changed published binary. Checksums detect corruption; they are not signatures. Quick's shared-write model still applies to the hosted files.

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

This builds the four binaries, signs the two macOS binaries with Developer ID, hardened runtime, and a secure timestamp, and submits a ZIP of those binaries to Apple. It records the submission ID before waiting, requires Apple's `Accepted` result, retrieves the log, and checks online notarization tickets for both binaries. Then it exports the signed bytes, regenerates SHA-256 checksums, and assembles/tests `dist/site` with `--require-notarized`. That flag refuses missing, stale, rejected, or mismatched receipts. Linux binaries are unchanged by signing.

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

## Publishing

The site is published at <https://relops-launcher.quick.mozilla.cloud/>. From `scripts/relops-launcher`, sign, notarize, test, and publish with:

```sh
just publish
```

That recipe uses `aerickson@mozilla.com` for Quick; other publishers can override `RELOPS_QUICK_ACCOUNT`. To deploy an already-prepared signed bundle directly, publish the generated folder, not this source folder or the whole project:

```sh
CLOUDSDK_CORE_ACCOUNT=aerickson@mozilla.com \
  quick deploy dist/site relops-launcher
```

This invocation was used successfully to publish the site. `CLOUDSDK_CORE_ACCOUNT` selects the publishing account for this command without changing the default gcloud account. Other publishers should substitute their own authenticated Mozilla account. Only `just publish` deploys automatically; build and signing recipes do not. Browser downloads use the staff member's Mozilla SSO session; this flow does not promise unauthenticated `curl` installation or launcher self-update. Quick publishers need its CLI/credentials; people downloading `relops` do not.

Use normal deployment without `--mirror` to retain older versioned downloads already on Quick. The local bundle and `index.json` describe the current release only. Do not republish different bytes under a previously shared version. If CI later owns deployment, use that workflow consistently rather than alternating manual and CI publishers.

After publishing, verify in an authenticated browser that each platform link downloads the correct filename, verify its checksum, install the native binary, and check `relops --version`. Verify macOS's first-download approval flow and trial the fork example with a coworker. These are still live-environment release checks; a local build cannot prove Quick authentication or download headers.

The repository workflow `.github/workflows/relops-launcher.yml` runs tests on Linux/macOS and saves an explicitly named unsigned preview site artifact. It does not access signing credentials, submit to Apple, publish to Quick, or commit binary files. Its CI runs will be verified after these changes are pushed.

## Source files

```text
index.html          current template: hero, fork example, install steps, usage, blob
future.html         concept site for later releases (terminal demo, contract,
                    roadmap, and the living design system in section 06)
future2.html        earlier index: overview, install, usage, chrome blob
future3.html        moire-hero variant of the trimmed index
css/tokens.css      design tokens: color, type, space, motion, accent presets
css/components.css  components built only from tokens
css/moire.css       animated moiré hero object (pointer-reactive, uses --accent)
css/blob.css        mesh-gradient blob hero (complement of accent, drifting points)
js/main.js          progressive enhancement: copy, OS detection, theme/accent,
                    platform-specific install/checksum selection, live blob
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
