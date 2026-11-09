# Standalone repository extraction (rlau-znf)

The local standalone checkout is `~/git/relops-launcher`. Use that checkout for
future launcher development. GitHub repository creation and publication are
separate follow-up actions; this extraction does not publish or deploy anything.

## History

The source is `~/git/df` (`git@github.com:aerickson/df.git`), branch `master`,
at `eac1dfa812c51d5947af3e70c28a18fb1f58bcc7`. The extraction retains 107
project commits, preserving authors, commit messages, and timestamps. No earlier
launcher paths or renames were found. Two project files outside the launcher
directory are included: its CI workflow and original design document.

These commands reproduce the committed baseline in a fresh destination:

```sh
git clone --no-local --single-branch --no-tags --branch master \
  ~/git/df ~/git/relops-launcher
cd ~/git/relops-launcher
git filter-repo \
  --path scripts/relops-launcher/ \
  --path .github/workflows/relops-launcher.yml \
  --path product-specs/relops-launcher-design.md \
  --path-rename scripts/relops-launcher/: \
  --path-rename product-specs/relops-launcher-design.md:docs/original-design.md
```

The commands assume the source branch still points at the revision above.
Only `master` is extracted; unrelated branches, tags, and cmux snapshot refs are
excluded. Commit IDs change when paths and parent histories change. The local
`.git/filter-repo/commit-map` records the old and new IDs. The extraction removes
the source remote; add the chosen destination remote only after it is created.

The initial extracted files were checked against the source HEAD byte for byte.
Path fixes, CI adjustments, this document, and the bead status updates are left
uncommitted for review. The Go module already names
`github.com/mozilla-platform-ops/relops-launcher` and is unchanged; the eventual
GitHub owner remains a publication decision.

## Local state and release provenance

The project's tracked `.beads` files move with the code, retaining `rlau` IDs.
The destination rebuilds its local issue database from `issues.jsonl`; SQLite
databases and locks from the old checkout are not copied. Existing ignored
`dist/` and `bin/` artifacts are copied separately to preserve retained signed
releases and local build outputs. They are not part of the filtered history.

Keep previously approved release bytes, receipts, and checksums unchanged.
Their embedded source revisions still refer to the original `df` history.
Use a new version for newly built release bytes. Signing, live release tests,
Quick deployment, and production website review remain separate release work.

## Cutover

1. Review the uncommitted changes in `~/git/relops-launcher` and run
   `just check build site-check` there. These checks build an unsigned local
   preview and do not deploy or run the opt-in live release tests.
2. Update terminal sessions, editor workspaces, and local preview/watch commands
   to use the standalone checkout. Use its beads database for future issues.
3. After explicit approval, commit the reviewed adjustments. Create the chosen
   GitHub repository separately, add its remote, and push only with approval.
   Verify the Linux/macOS CI jobs after publication.
4. Once the destination is accepted, remove `scripts/relops-launcher/`,
   `.github/workflows/relops-launcher.yml`, and
   `product-specs/relops-launcher-design.md` from `df` in a separate reviewed
   cutover change. Update any remaining `df` references to the published repo.
   The source checkout and history are preserved during local extraction.

Do not develop in both copies after switching: their issue databases and
uncommitted work are independent.
