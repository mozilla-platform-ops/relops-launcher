# Agent notes

<!-- tt-agent-instructions-v1 -->
## Taste

> Managed by `tt agents`; rerun it to refresh this block.

- Before starting work, read these files when available:
  `~/.config/tt/shared/scripts/tt/guidance.md`, `TASTE.md`, and optional
  `TASTE.local.md` at the project root. Expand `~` to your home directory.
  Follow the shared workflow guidance; explicit local
  preferences take precedence over general shared preferences.
- Shared guidance and taste are optional when unavailable. If shared guidance or
  `TASTE.md` is missing and `tt` is installed, attempt `tt install` once per task
  in the project, then read any restored files. If `tt` is unavailable or setup
  fails, continue using repository instructions and any available local taste.
  Do not install `tt`, repeatedly retry, or block the task on setup.
<!-- end-tt-agent-instructions -->

## Git operations

- Leave completed changes uncommitted for user review.
- Do not run `git commit` or `git push` without explicit user permission.
  Permission to implement changes does not authorize either action.
- When ready, summarize the changes and validation, and suggest a commit
  message. Include the relevant bead ID when applicable.
- Stage only task-related files when preparing an approved commit.
- Do not add `Co-Authored-By` trailers to commit messages.

## Validation

- During development, run focused tests and checks appropriate to the change.
  Documentation-only changes generally need only `git diff --check`.
- After changing CLI behavior or terminal output, run `just build` so the user
  can preview it with `./bin/relops` before committing. Include a relevant
  preview command in the handoff.
- Run `just check` before release and when broader validation is warranted;
  it is not required after every edit or session.
- `just check` runs formatting checks, `go vet`, and the full race-enabled test
  suite. Normal tests use local repositories and fake HTTP transports; they do
  not consume GitHub API requests.
- Live GitHub release tests are opt-in via `RELOPS_TEST_REAL_FLEETBENCH=1`.
  Run them only during release preparation, not for routine edits or commits,
  because they make real GitHub requests and download release assets.

## Website review and publishing

- Prepare website changes on a staging or local preview site for the user to review.
- Do not automatically commit changes or publish the production site. Wait until the user has verified the staging site and explicitly approved committing or publishing.
- A request to implement a change does not authorize committing or production publication. Keep changes uncommitted while staging review is pending.

<!-- br-agent-instructions-v1 -->

---

## Beads Workflow Integration

This project uses [beads_rust](https://github.com/Dicklesworthstone/beads_rust) (`br`/`bd`) for issue tracking. Issues are stored in `.beads/` and tracked in git.

### Essential Commands

```bash
# View ready issues (open, unblocked, not deferred)
br ready              # or: bd ready

# List and search
br list --status=open # All open issues
br show <id>          # Full issue details with dependencies
br search "keyword"   # Full-text search

# Create and update
br create --title="..." --description="..." --type=task --priority=2
br update <id> --status=in_progress
br close <id> --reason="Completed"
br close <id1> <id2>  # Close multiple issues at once

# Sync with git
br sync --flush-only  # Export DB to JSONL
br sync --status      # Check sync status
```

### Workflow Pattern

1. **Start**: Run `br ready` to find actionable work
2. **Claim**: Use `br update <id> --status=in_progress`
3. **Work**: Implement the task
4. **Complete**: Use `br close <id>`
5. **Sync**: Always run `br sync --flush-only` at session end

### Key Concepts

- **Dependencies**: Issues can block other issues. `br ready` shows only open, unblocked work.
- **Priority**: P0=critical, P1=high, P2=medium, P3=low, P4=backlog (use numbers 0-4, not words)
- **Types**: task, bug, feature, epic, chore, docs, question
- **Blocking**: `br dep add <issue> <depends-on>` to add dependencies

### Session Protocol

Before ending a session:

1. Run checks appropriate to the changes, following the validation guidance
   above. Keep live release tests limited to release preparation.
2. Run `br sync --flush-only` to export issue updates.
3. Review `git diff --check` and `git status`.
4. Summarize the changes, validation, and any outstanding work.
5. Suggest a commit message when changes are ready.

Leave changes uncommitted unless the user explicitly authorizes committing.
Push only when the user explicitly authorizes pushing.

### Best Practices

- Check `br ready` at session start to find available work
- Update status as you work (in_progress → closed)
- Create new issues with `br create` when you discover tasks
- Use descriptive titles and set appropriate priority/type
- Always sync before ending session

<!-- end-br-agent-instructions -->

