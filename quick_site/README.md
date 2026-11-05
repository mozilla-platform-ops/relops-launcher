# relops site

Single-page download site for the `relops` launcher. The tracked HTML is a template; `just site` renders the release version and checksums and assembles real downloads. Binaries and generated output are ignored by Git.

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
  downloads/v0.1.0/
    relops-darwin-arm64
    relops-darwin-amd64
    relops-linux-arm64
    relops-linux-amd64
    SHA256SUMS
```

`VERSION` supplies the release version; directory names follow it. The builder checks that all binaries have matching source/toolchain metadata and the expected platform, then hashes the exact downloadable bytes. A failed build leaves the previous generated site in place. Only the current homepage, CSS, and JS ship; concept pages, tests, documentation, source, and Git data do not.

`releases.json` records the actual embedded binary metadata, including whether the source checkout was dirty. Commit release source and rebuild before publishing so the source commit is reproducible and `source_dirty` is false. Release versions should identify fixed artifacts: bump `VERSION` for a changed published binary. Checksums detect corruption; they are not signatures. Quick's shared-write model still applies to the hosted files.

## Publishing

Publish the generated folder, not this source folder or the whole project:

```sh
quick deploy dist/site relops-launcher
```

This command is documented for the later publishing step; neither build target deploys. Review the bundle first. Browser downloads use the staff member's Mozilla SSO session; this flow does not promise unauthenticated `curl` installation or launcher self-update. Quick publishers need its CLI/credentials; people downloading `relops` do not.

Use normal deployment without `--mirror` to retain older versioned downloads already on Quick. The local bundle and `index.json` describe the current release only. Do not republish different bytes under a previously shared version. If CI later owns deployment, use that workflow consistently rather than alternating manual and CI publishers.

After publishing, verify in an authenticated browser that each platform link downloads the correct filename, verify its checksum, install the native binary, and check `relops --version`. Verify macOS's first-download approval flow and trial the fork example with a coworker. These are still live-environment release checks; a local build cannot prove Quick authentication or download headers.

The repository workflow `.github/workflows/relops-launcher.yml` runs tests on Linux/macOS and saves the generated site as a GitHub Actions artifact. It does not publish to Quick or commit binary files. Its CI runs will be verified after these changes are pushed.

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
