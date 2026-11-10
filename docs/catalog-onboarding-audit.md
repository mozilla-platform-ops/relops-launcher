# Catalog onboarding audit

Reviewed on 2026-11-09 for epic `rlau-p66`. The launcher and website share
`quick_site/tools.json`; user-facing requirements belong there, with longer
instructions in each tool's own documentation.

## Source review

`just check-tools` resolved every configured source target successfully. The
requirements review read the following revisions without running the tools:

| Targets | Repository and revision | Evidence |
| --- | --- | --- |
| Taskcluster Web Launcher, both Moonshot wrappers | `mozilla-platform-ops/relops-infra` at `0a3a45bce128` | Selected scripts, oneshot/reimage helpers, and their READMEs |
| Pool and task replay | `aerickson/android-tools` at `5fc5f2fcc38f` | Replay scripts, controller, token reader, Pipfile, and README |
| Task creator and quarantine | `mozilla-platform-ops/android-tools` at `721bcf84e8cc` | Scripts, quarantine/Taskcluster helpers, Pipfile, and README |
| Fleetroll | `mozilla-platform-ops/fleetroll_mvp` at `13fc39bf06e6` | Python requirements, SSH and credential loaders, and README |

Hangar and Pool Classifier requirements describe using their hosted dashboards,
based on their repository READMEs and deployment documentation. Backend service
credentials and local development dependencies are not browser prerequisites.
Fleetbench's README separates host inspection/CPU workloads from ADB workloads;
the latter also need ADB and an authorized device.

Every catalog target now has a setup reference. Requirements distinguish initial
environment preparation, credentials, and operation-specific dependencies.
Declared files can be optional or created by a tool; the list is documentation,
not a mandatory-file preflight.

`just inspect-tools` completed with seven relative-write warnings in Fleetroll's
example/reference shell scripts. Those scripts are not run by the catalog's
console entry point. The inspector does not follow Moonshot helper calls, so the
reimage-log finding below came from reading the helper source.

## Remaining gaps

- `rlau-nps`: verify supported runtime versions and minimal Taskcluster grants
  where upstream docs currently omit them or list broad examples. Creator and
  quarantine entries retain the lock-compatible Python requirement until a
  supported minimum is established; their scope references are not a claim
  that minimal grants have been verified.
- `rlau-jk2`: Moonshot's reimage helper writes logs below its own checkout, so
  those logs disappear after a relops run. Bootstrap output already uses a
  separate temporary file; it does not preserve these reimage logs.

Tool pages added under `rlau-noi` now show the full catalog prerequisites and
upstream setup links alongside optional hand-written guides. The compact
homepage links to those pages through each tool's name.

No Taskcluster credentials were read, no tasks were submitted, and no worker
operations were performed. Source inspection and path checks do not establish
that a particular user has sufficient permissions or can install successfully.

## Coworker trial

Tracking: `rlau-p66.3`. **Prepared; not yet performed.**

Published v0.3.1 includes the updated catalog and shared setup guidance;
`rlau-h23` records the verified release and matching Quick site deployment.
Use v0.3.1 for this trial. The release prerequisite is satisfied; coworker
participation, trial results, and a recommendation for broader sharing remain
outstanding.

1. Install on a supported platform using the README instructions for the chosen
   build. Record OS/architecture, launcher version, and catalog revision. Check
   that `relops --version` works from outside the download/build directory.
2. Run `relops run`, then `relops info replay-tc-pool`. Ask whether the purpose,
   requirements, and setup reference are sufficient to get started.
3. Install the required Python and Pipenv versions, verify GitHub SSH access,
   and run `relops run -b replay-tc-pool -h`. Confirm that the launcher prepares
   dependencies without a manual clone or `pipenv shell`.
4. Follow the linked token setup and scope guidance. Record only whether setup
   succeeded; do not record tokens or credential-file contents.
5. With agreed source and destination pools, run one non-submitting cycle:

   ```sh
   relops run -b replay-tc-pool \
     --from-pool SOURCE_POOL --queue DESTINATION_POOL \
     --new-session --dry-run --once
   ```

   This still needs credentials and contacts Taskcluster. Record permission
   diagnostics and the saved session path. View it using
   `relops run -b replay-tc-pool --session SESSION_PATH`.
6. Record confusing steps, missing dependencies, and outcomes. Fix or file each
   actionable problem and give a recommendation for broader sharing. Actual
   submissions are a separate trial requiring an explicitly agreed target.

Coworker participation and any team announcement are arranged by the user. The
Slack draft remains outside Git at
`~/Desktop/relops-launcher-team-announcement-draft.md`.
