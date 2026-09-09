# kman

Manages Slack/human-triggered access to [kranq](https://github.com/simon-em/kranq):
credentials, flows, groups, integrations (Bitbucket OAuth, Slack), a cron
scheduler, and a web UI that is an interface onto a config git repo.

Read [docs/design.md](docs/design.md) before making architectural decisions —
it records the phasing and the reasoning behind each one, including why kman
exists as a separate binary from kranq rather than a feature of it.

## Where this is now

Phases 0-9 (Phase 5, kranq's own `files:` addition, landed and shipped in
the kranq repo, not here): CLI dispatch, the flow schema, config-repo
loading (with a local-directory fallback for testing), the git mechanics
to push a flow at kranq, an encrypted secrets store a flow can bind a
credential to, users/groups/grants (enforced for Slack-triggered pushes
via `CanTrigger`), a web UI over all of it, a Bitbucket OAuth integration
for per-user credentials, a Slack integration (allow-listed `@kman run
<flow>` triggering and an AskUserQuestion relay), meta access + cron (a
flow can be granted `access.meta: [cron.create]` to schedule itself
through a scoped callback endpoint, without ever holding a kranq
credential), and a built-in MCP/skills catalog: a flow can declare
`access.skills: [catalog:bitbucket@<ref>]` and kman stages that server's
implementation and wires it into every `claude:` step itself, or
`local:<path>` to wire in a tool the flow brings along in its own
`files:`. See [docs/status.md](docs/status.md) for the phase-by-phase
state.

```sh
kman version
kman validate <flow.yaml>...
kman render <flow.yaml>                 # the kranq task-spec YAML the flow projects to
kman mirror <remote-url>                # sync a host-side cache mirror of a repo
kman push <flow.yaml> [NAME=VALUE...] --kranq-url <url> --as <user-id> [--source DIR|URL] [--meta-url <url>]
kman secret set <name> [value]          # value read from stdin if omitted
kman secret ls                          # names only, never values
kman secret rm <name>
kman user set <id> [--display-name X] [--slack-id Y]
kman user ls
kman group set <name>
kman group add-member <group> <user-id>
kman group remove-member <group> <user-id>
kman group ls
kman grant <user/ID|group/NAME> <flow>
kman revoke <user/ID|group/NAME> <flow>
kman web [--addr 127.0.0.1:8080]        # a browser UI over flows/users/groups/cron/integrations; every save is a git commit
kman integration bitbucket status [user-id...]
kman integration slack status
kman slack serve [--addr 0.0.0.0:8081] --kranq-url <url> [--meta-url <url>]   # the Slack events endpoint; separate from kman web on purpose
kman meta serve [--addr 0.0.0.0:8082]   # the scoped callback endpoint a pushed VM calls through; separate listener again
kman cron set <name> --flow <flow> --schedule "<cron-expr>" [NAME=VALUE...]
kman cron ls
kman cron rm <name>
kman cron tick --kranq-url <url>        # fire whatever's due once, then exit
kman cron serve --kranq-url <url> [--interval 1m]   # loop, calling the same tick
kman skills ls                          # list the built-in MCP/skill catalog
```

To use Bitbucket OAuth, register an OAuth consumer in your Bitbucket
workspace settings, then:

```sh
kman secret set bitbucket/oauth/client_id <id>
kman secret set bitbucket/oauth/client_secret <secret>
```

A flow can then bind a credential to it instead of a static vault secret:

```yaml
credentials:
  BITBUCKET_TOKEN: integration:bitbucket
access:
  credentials: [integration:bitbucket]
```

To use Slack, create a Slack app with an `app_mention` Event Subscription
pointed at `kman slack serve`'s `/slack/events`, then:

```sh
kman secret set slack/bot_token <xoxb-token>
kman secret set slack/signing_secret <signing-secret>
kman user set simon --slack-id U0123456
kman grant user/simon deploy-review
```

Mentioning the bot with `@kman run deploy-review ENV=staging` in a channel
runs the flow as `simon`, if `simon` (or one of their groups) has been
granted it, and replies in-thread with the result. A flow that wants a
headless Claude step to be able to ask a human something mid-run declares
`access.meta: [ask]`; kman then delivers a small MCP server into the VM
(via `files:`) and disables the step's built-in `AskUserQuestion` in favor
of it, so the question and answer round-trip through the same Slack thread.

A flow granted `access.meta: [cron.create]` can schedule *itself* from
inside its own run — no code inside the VM ever sees a kranq credential.
`kman push`/`kman slack serve --meta-url <url>` mint a short-lived,
capability-scoped bearer token and inject it as `KMAN_META_TOKEN`
(`KMAN_META_URL` alongside it) whenever a pushed flow's `access.meta` is
non-empty; a tool running inside the VM calls back with that token:

```sh
curl -X POST "$KMAN_META_URL/meta/cron" \
  -H "Authorization: Bearer $KMAN_META_TOKEN" \
  -d '{"name":"nightly","schedule":"0 6 * * *"}'
```

`kman meta serve` validates the token and writes the resulting
`config/cron/<name>.yaml` entry itself, with `flow` always set to the
token's own flow — the request can't name a different one. From there,
`kman cron serve` (or `kman cron tick` on a real cron/launchd timer) picks
it up like any admin-authored entry.

A flow that wants MCP tool access declares it in `access.skills:` instead
of hand-writing `mcp_servers:` and hoping kranq happens to have the right
thing embedded:

```yaml
access:
  skills: [catalog:bitbucket@1ab934c12689]
steps:
  - name: review
    claude: review the open PR and leave comments
```

`kman skills ls` (or the `/skills` web page) lists what's in the catalog
and the exact reference to paste. `kman push`/`kman render` stage the
server's implementation via `files:` and add the `mcp_servers:` entry
automatically — the ref is a pin, not "latest": if the catalog's own copy
of a skill ever changes, a flow still naming the old ref fails loudly
instead of silently running different code. A flow can also bring its own
tool via `access.skills: [local:/path/to/tool]`, referencing a `files:`
entry it already declares (the file's mode must be executable); kman
wires it in by its own path, no catalog involved.

## Commands

```sh
go build -o /tmp/kman .
go test ./...
```
