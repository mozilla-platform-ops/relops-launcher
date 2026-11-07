# RelOps CLI design and color system

RelOps should make the next action easy to find, keep repeated launches compact,
and show enough context to understand what will run. ASCII art adds team identity
at the entrance. Root help uses the selected `small_slant` wordmark in green. Its artwork can be
refined independently of the help layout.

## Color roles

Use the terminal's ANSI palette so colors follow the user's theme. Do not set
background colors or assume a light or dark background.

| Role | ANSI SGR | Usage |
| --- | --- | --- |
| Heading | `1;32` (bold green) | Section titles, root wordmark, tool info title |
| Command | `1` (bold foreground) | Command names, option flags, launch commands |
| Metadata | `2` (dim foreground) | Version, catalog revision, aliases, preparation status |
| Beta | `33` (yellow) | Explicit beta channel label and launch marker |
| Error | `31` (red) | `error:` label; explanatory text remains normal |
| Provenance | `32` (green) | Compact launch header and verbose launcher diagnostics |
| Body | terminal default | Descriptions, examples, prerequisites, URLs, instructions |

Color reinforces explicit words and layout. A beta target always says `beta`;
errors always say `error:`. Never rely on color alone. Essential text must not be
dim. Do not color whole error explanations or introduce colored backgrounds,
emoji status indicators, or multiple competing accents.

## Terminal behavior

Styling is enabled only when the destination is a terminal, `TERM` is not `dumb`,
and `NO_COLOR` is not a nonempty value. Help and catalog discovery check stdout;
launcher diagnostics check stderr independently. Redirected output contains no
ANSI sequences. Each styled span resets its formatting. Tool output is passed
through unchanged, including the tool's own color choices.

Query terminal width using the output descriptor, with `COLUMNS` as a fallback.
Below 80 columns, help rows put descriptions on a separate line. Catalog prose
wraps to the available width (80 when unknown); URLs and identifiers stay intact.
Root help shows the wordmark on terminals at least 40 columns wide, or when width
is unknown. Narrow terminals, dumb terminals, and redirected help use a plain
`relops` title. `NO_COLOR` disables color but retains the terminal wordmark.
Subcommand help, launches, errors, and version output never show the wordmark.

## Help hierarchy

Root help leads with a one-line purpose and three quick-start actions: find,
inspect, run. Follow with commands, common options, argument forwarding, and
pointers to focused help. Avoid repeating every execution detail at the root.

- `relops catalog --help`: discovery and launch commands.
- `relops catalog list --help`: listing syntax and channel guidance.
- `relops catalog info --help`: inspection syntax and an example.
- `relops c --help` / `relops catalog launch --help`: catalog selection, common
  flags, source flags, release updates, examples, and execution rules.
- `relops repo --help`: source options, examples, environment selection, cleanup.

Launcher options go before the target. Every argument after the target belongs
to the tool, including `--help`. Examples must preserve that boundary. Explain
that source checkouts are removed on exit and outputs should be saved elsewhere.

## Catalog presentation

Use two-line entries for names and descriptions. Emphasize names, keep aliases
secondary, and label available channels in text. End lists with launch and
inspection hints; place the bundled revision last as metadata.

Tool details use readable labels, prerequisites, documentation links, and a
copyable launch command for each channel. Omit empty optional values and show
meaningful automatic defaults. Do not claim a resolved project directory or
entry point before the source has been inspected. Beta-only tools must show the
required `-b` in their launch command. Catalog display never launches a tool or
fetches its source.

## Validation

Check root and focused help, default and beta-only tools, source and release
info, errors, and version output. Verify a normal terminal, narrow terminal,
`NO_COLOR=1`, `TERM=dumb`, and redirection. Check stdout and stderr separately.
Keep argument forwarding and exit statuses unchanged. Run `just check` before
shipping. Palette and rendering live in `internal/launcher/style.go`,
`help.go`, `catalog.go`, and `provenance.go`.
