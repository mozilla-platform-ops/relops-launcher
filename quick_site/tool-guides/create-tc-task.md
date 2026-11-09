## When to use it

Use a small diagnostic task to check that a worker queue accepts work, identify
the worker that claims it, or run a shell command on that worker. This tool
creates a fresh task rather than copying an existing Firefox CI job.

## Start with help

```sh
{{launch}} --help
```

*Shows the available payload, command, and monitoring options without submitting
a task. relops prepares the Python dependencies on the first run.*

Read the [requirements and credential setup](#tool-requirements) before building
a request. The tool reads its token file even for a dry run.

## Preview one task

```sh
{{launch}} --queue YOUR_PROVISIONER/YOUR_WORKER_TYPE --count 1 --dry-run
```

*Replace the destination with the queue you intend to test. This builds a request
without creating a task; it still needs the credential file.*

The default task prints an increasing counter while it sleeps. For a real
submission, remove `--dry-run`; add `--wait` to watch the task's status and worker
assignment. Use `--bash-command` or `--script-file` when the counter is not enough
for your diagnosis, choosing a payload format compatible with the destination.

## Know what the preview checks

A dry run does not establish that Taskcluster will accept the request or that
the worker can run the payload. Start with one task and inspect its result before
increasing the count. Quitting the monitoring display stops watching; it does
not cancel the tasks already submitted.
