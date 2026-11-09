## Create a client

Open the [Firefox CI Taskcluster clients page](https://firefox-ci-tc.services.mozilla.com/auth/clients),
sign in, and create a client for the work you intend to do. Copy its client ID
and access token. If you cannot create a client or grant the required scopes,
ask the team for the appropriate access.

## Save the credentials

Several RelOps tools read a JSON file named `~/.tc_token`:

```json
{
  "clientId": "YOUR_CLIENT_ID",
  "accessToken": "YOUR_ACCESS_TOKEN"
}
```

*Replace both placeholders with the credentials from your client. These are the
file's two fields; scopes are configured on the Taskcluster client.*

Save the file in your home directory, then restrict access to your account:

```sh
chmod 600 ~/.tc_token
```

Keep the access token out of Git, screenshots, and shared logs. relops does not
create this file or provision a Taskcluster client. Tools read the credentials
when their operation needs them.

## The token needs Taskcluster scopes

Your client's scopes determine what its token can do. A token must have the
scopes required by the selected tool and operation; creating a client alone
does not establish that it has those permissions.

Check the tool's Requirements section, upstream setup guide, or help for its
scope requirements. Task submission can need permission for the destination
worker pool as well as the scopes requested by the task. Quarantine operations
need permission to change the target workers' quarantine state. Those operations
have different requirements, so there is no universal scope list for this file.

Configure the grants the operation needs. If a tool reports missing scopes,
compare them with the client's effective scopes and the intended workload.
Replay tools also explain workload-dependent grants in their help and permissions
views. The tool's own guide is the place for exact scope names and examples.

## Check when credentials are needed

Credential requirements vary even between tools with similar names. Pool replay
needs the file for active monitoring, including `--dry-run`; viewing a saved
session does not. Task creator loads the file for a dry run too. Consult the
individual tool page before assuming that a preview is credential-free.

Some tools support a different file path. Fleetroll accepts a credential-file
path through `TC_TOKEN`; others read `~/.tc_token` directly. Follow the selected
tool's requirements for overrides. Environment variables supported by one tool
do not automatically apply to the others.

## Troubleshooting

- **Cannot read the token file:** check the file path, JSON syntax, and the
  `clientId` and `accessToken` field names locally.
- **Authentication fails:** check that the client ID and token belong together
  and that the client is still valid.
- **`InsufficientScopes` or a missing-scope message:** compare the operation's
  required scopes with the client's grants. A readable token file does not
  demonstrate permission to submit or modify anything.

When asking for help, share the error and relevant scope names, leaving the
access token out of the report.
