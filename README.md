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
credential), and a built-in skills/MCP catalog: a flow can declare
`access.skills: [catalog:bitbucket@<ref>]` for a plain-markdown skill
kman stages for Claude to read and act on directly, `local:<path>` for
the same with a skill the flow brings along itself, or a `local:<path>`
pointing at an executable for a real MCP server kman wires into every
`claude:` step. See [docs/status.md](docs/status.md) for the
phase-by-phase state.

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
kman repo set <name> <url>              # a named source repo flows/dispatch can target by name
kman repo ls
kman repo rm <name>
kman grant <user/ID|group/NAME> <flow>
kman revoke <user/ID|group/NAME> <flow>
kman web [--addr 127.0.0.1:8080]        # a browser UI over flows/repos/users/groups/cron/integrations; every save is a git commit, pushed to the config repo's remote if it has one
kman integration bitbucket status [user-id...]
kman integration slack status
kman slack serve [--addr 0.0.0.0:8081] --kranq-url <url> [--meta-url <url>] [--default-flow <name>] [--dispatch] [--dispatch-model <name>]   # the Slack events endpoint; separate from kman web on purpose
kman meta serve [--addr 0.0.0.0:8082]   # the scoped callback endpoint a pushed VM calls through; separate listener again
kman cron set <name> --flow <flow> --schedule "<cron-expr>" [NAME=VALUE...]
kman cron ls
kman cron rm <name>
kman cron tick --kranq-url <url>        # fire whatever's due once, then exit
kman cron serve --kranq-url <url> [--interval 1m]   # loop, calling the same tick
kman skills ls                          # list the built-in and custom MCP/skill catalog
kman skills set <name> --kind doc|mcp --path <path> [--command <cmd>] [--description <desc>] [--source-url <url>] [--file <path>]   # content from stdin if --file is omitted
kman skills rm <name>
```

`kman web` is a real editor, not just flow/user/group/cron viewers: every
field of a flow (args, env, credentials, access, files, steps) has a form
control, with `<datalist>` suggestions drawn live from the repo registry,
the skills catalog, and the vault's own secret names — writing a flow's
YAML by hand is for the rare case the form doesn't cover (a step's own
`mcp_servers:`; use `access.skills:` instead, which the form does cover).
Repeatable sections (args, steps, files, ...) show existing entries plus
a few blank rows to fill in; save and reopen for more.

Whenever `$KMAN_HOME/config` is a git repo with an `origin` remote, every
save — from the CLI or the web UI — pushes to it automatically, pulling
first (`git pull --rebase`, correctly handling the ordinary case of two
admins each having committed locally before syncing) and retrying once on
a rejected push. No remote configured is not an error; the local-only and
local-git-no-remote modes work exactly as before. A push failure never
fails the save itself — the local commit already happened — it only
surfaces a warning (stderr on the CLI, a banner in the web UI).

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

That form mints a token for the human who ran `kman push --as <user-id>`
(or triggered it via Slack), carrying whatever scopes the OAuth consumer
itself was granted. For a task that shouldn't get that much, append the
Bitbucket scope it actually needs:

```yaml
credentials:
  BITBUCKET_TOKEN: integration:bitbucket:pullrequest
access:
  credentials: [integration:bitbucket:pullrequest]
```

This mints a fresh, short-lived, genuinely restricted token via Bitbucket's
`client_credentials` grant, scoped to exactly what's asked for, not tied to
any user (no `--as` needed for this form) and never cached. Bitbucket
enforces the scope on every API call the token makes, not just at mint
time, confirmed against the real API: a `pullrequest`-scoped token can list
and comment on PRs but gets a hard `403` trying anything else, with the
response spelling out exactly what was required versus granted. A scope
name Bitbucket doesn't recognize fails the push immediately (Bitbucket
itself rejects it, `400 invalid_scope`) rather than minting something
broader than asked for. Scopes containing a colon work as-is, e.g.
`integration:bitbucket:repository:write`.

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
granted it, and replies in-thread with the result. A flow's `repo:` is
only its *default* source; `run deploy-review REPO=dx ENV=staging` (or a
flow that declares `args: REPO:` itself) overrides it, resolved as a name
from `kman repo ls` or a full URL directly.

A mention that isn't `run <flow>` needs routing to a flow (and, since a
flow no longer always implies one fixed repo, a target repo too). Two
ways to do that:

- `--default-flow <name>` (default `create-feature`): the whole message
  becomes that one flow's `TASK` argument, no routing decision made.
- `--dispatch`: a real routing step. kman runs `claude -p` on its own
  host (sandboxed: no tools, no filesystem, no MCP — `--allowedTools ""
  --strict-mcp-config`, a pure text-in/JSON-out classification call, not
  an agent) with the free text plus the *requesting user's own granted
  flows* (each flow's `description:` field) and `kman repo ls` as
  candidates, and it returns which flow, which repo, and the task text
  with routing phrases like "on dx" stripped out. If it can't tell,
  it replies with a clarifying question instead of guessing. `--dispatch`
  takes priority over `--default-flow` when both are set. Confirmed live
  end to end: a real Slack message with no flow name and no repo URL,
  routed through a real `claude -p` call, landed a real PR. Each dispatch
  call is real, billed Claude usage (haiku by default, ~$0.03-0.07 and
  5-15s per message once its prompt cache warms up) — `--dispatch-model`
  overrides the model. Requires `claude` on `kman slack serve`'s own PATH,
  logged in or with `CLAUDE_CODE_OAUTH_TOKEN` set in its environment.

Either way, a flow written for a routed message needs to read its own
`TASK` value at runtime (`` `echo "$TASK"` `` inside its own `claude:`
step), not reference `$TASK` in the prompt text itself, kranq compiles a
`claude:` prompt into a shell heredoc that doesn't expand variables. A
flow meant to work across repos should avoid hardcoding
`BITBUCKET_WORKSPACE`/`BITBUCKET_REPO_SLUG` in its own `env:` — kman
derives both automatically from whichever repo was actually targeted,
whenever the flow doesn't set them itself.

```sh
kman repo set dx https://bitbucket.org/effetmonstre/dx.git
kman repo ls
kman repo rm dx
```

If a flow's resolved repo (its own `repo:`, or a `REPO=`/dispatch
override) is a *private* `bitbucket.org` URL, kman mints its own
short-lived, read-only credential to clone it (separate from whatever the
flow's own `credentials:` declares for use inside the VM, that's a
different, narrower, host-side-only concern) — nothing to configure
beyond having Bitbucket set up already.

A flow that wants a
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

A flow that wants tool access declares it in `access.skills:` instead of
hand-writing `mcp_servers:` and hoping kranq happens to have the right
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
skill's content via `files:` automatically; the ref is a pin, not
"latest" — if the catalog's own copy of a skill ever changes, a flow
still naming the old ref fails loudly instead of silently running
different code.

A catalog (or `local:`) entry is one of two kinds. Most skills, including
the built-in `bitbucket` one, are plain markdown: a `SKILL.md` staged at
`.claude/skills/<name>/SKILL.md`, no process, no `mcp_servers:` entry,
just instructions Claude reads and acts on with `Bash`/`curl` the same way
it would follow any other skill. A `local:<path>` ref pointing at an
*executable* file (mode must be `0755` or similar) instead gets wired as
a real MCP server, `command` set to the file's own path — for the rarer
case where a flow genuinely needs a structured tool-call interface rather
than instructions.

The catalog isn't only what's compiled into the binary. `kman skills set`
(or `/skills/new` in the web UI) adds a real catalog entry backed by
config-repo content — pushed and versioned like everything else in
`~/.kman/config`, referenced the same way (`catalog:<name>@<ref>`), and
composed by the exact same code path as a built-in one. The web form has
a "fetch from a public repo" step: paste a URL (a plain raw URL, or an
ordinary `github.com/.../blob/...` link — it's rewritten to the raw form
for you) and it fetches the content into the form for you to review and
edit before saving anything; nothing is fetched again after that, so a
flow pinning `catalog:name@ref` stays exactly what it pinned even if the
original source later changes. A name already used by a built-in entry is
rejected, so a custom entry can never silently shadow one.

Writing a skill by hand doesn't require base64: any `files:` entry can use
`text:` instead of `content:` for plain UTF-8 content —

```yaml
files:
  - path: .claude/skills/my-thing/SKILL.md
    text: |
      ---
      name: my-thing
      description: how to do my thing
      ---
      # My thing
      ...
access:
  skills: [local:.claude/skills/my-thing/SKILL.md]
```

kman encodes it to what kranq's own wire format needs at push time; the
flow's own YAML never has to carry a base64 blob for something that's
just text.

## Commands

```sh
go build -o /tmp/kman .
go test ./...
```
