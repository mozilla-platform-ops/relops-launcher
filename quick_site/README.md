# relops site

Single-page site for the `relops` launcher. Static: open `index.html` or serve the folder.

```text
index.html          the current site: hero, example command, install steps, blob
future.html         concept site for later releases (terminal demo, contract,
                    roadmap, and the living design system in section 06)
future2.html        earlier index: overview, install, usage, chrome blob
future3.html        moire-hero variant of the trimmed index
css/tokens.css      design tokens: color, type, space, motion, accent presets
css/components.css  components built only from tokens
css/moire.css       animated moiré hero object (pointer-reactive, uses --accent)
css/blob.css        mesh-gradient blob hero (complement of accent, drifting points)
js/main.js          progressive enhancement: copy, OS detection, theme/accent,
                    live blob silhouette, moire pointer hook, terminal demo
                    (future.html only). Each feature is guarded on missing elements.
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

## Open items

- Download links in `#install` point at a placeholder GitHub releases URL.
  Update them once the artifact host is chosen (design v1.1, gate 4).
- Terminal demo output is illustrative, not real tool output.
