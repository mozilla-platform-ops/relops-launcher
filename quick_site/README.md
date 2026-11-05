# relops site

Single-page site for the `relops` launcher. Static: open `index.html` or serve the folder.

```text
index.html          the current site: hero, example command, install steps, chrome blob
future.html         concept site for later releases (terminal demo, contract,
                    roadmap, and the living design system in section 06)
future2.html        earlier index: overview, install, usage, chrome blob
future3.html        moire-hero variant of the trimmed index
css/tokens.css      design tokens: color, type, space, motion, accent presets
css/components.css  components built only from tokens
css/moire.css       animated moiré hero object (pointer-reactive, uses --accent)
css/blob.css        animated chrome-blob hero (complement of accent, drifting points)
js/main.js          progressive enhancement: copy, OS detection, theme/accent,
                    terminal demo (future.html only; guarded on other pages)
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

## Open items

- Download links in `#install` point at a placeholder GitHub releases URL.
  Update them once the artifact host is chosen (design v1.1, gate 4).
- Terminal demo output is illustrative, not real tool output.
