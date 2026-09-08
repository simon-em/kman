# kman design

kman manages Slack/human-triggered access to kranq: credentials, flows
(kranq task specs plus argument/credential/access binding), groups,
integrations (Bitbucket OAuth, Slack, later GitHub), a cron scheduler, a
scoped "meta" capability so a granted flow can reach back into kman itself,
and a web UI that is nothing more than a convenient interface onto the same
config git repo the CLI and every integration read and write.

kranq deliberately has no credential vault, no groups/grants, and no
Slack/Bitbucket-OAuth integration — its own design doc explicitly deleted a
"principals, groups, grants, approval workflow" system, on the grounds that
env-forwarding from a pipeline already solves authority, and a vault would
make the build machine a bigger target. That doc also names, but declines to
build, the one case that breaks this model: *"a human triggering something
from a browser, with no env to send... stored per-user credentials come
back on the table... decided on its own merits."*

kman is that decision, made on its own merits, as a separate binary. It
pushes to kranq's default `kranq.git` repo using only `git push`/`git
pull`/`git fetch`, the same three operations
`infrastructure/ci/kranq-ci` already uses today — kman never imports a
kranq Go package and never drives `limactl`, so it stays a separate process
kranq knows nothing about. CI-pipeline-triggered kranq runs are explicitly
untouched: they keep forwarding env directly, exactly as today, with kman
nowhere in that path.

kranq's schema is not frozen, though. The process boundary (separate Go
module, separate binary, git + task YAML as the only interface) stays
fixed, but the YAML contract itself is a joint, evolving thing, not an
external API kman must always route around. kranq's current task schema is
good as-is with one exception: how a task gets access to tools —
concretely, MCP servers today only exist if kranq's binary happens to have
embedded them (`assets/mcp/bitbucket-mcp.py`, unconditionally copied into
every VM, referenced by a hardcoded absolute path). That's worth fixing in
kranq itself (Phase 5, below), not working around forever in kman.
Credentials-passing may get the same generalization later if it falls out
naturally, but it's lower priority — today's env-forwarding otherwise
works fine. No other part of kranq needs to change; backward compatibility
for its own sake is not a goal here, but nothing else in this design calls
for breaking it.

## Phasing overview

| Phase | Delivers | Excludes |
|---|---|---|
| 0 | New repo, CLI dispatch, flow schema v0, config-repo loading (+ local fallback). `kman version\|validate\|render`. | everything else |
| 1 | Git mechanics: host-side cache, incremental push/pull to kranq, `kman push` compiling a flow to a kranq task and submitting it. | credentials, Slack, cron, access control, web UI |
| 2 | Encrypted secrets store, credential binding in the flow schema. Scoped to human/Slack-triggered runs only. | Bitbucket OAuth itself (Phase 6 mints; Phase 2 only stores/binds) |
| 3 | Users, groups, Slack-identity mapping, the flow access-profile shape (tools, meta, skills, credentials, flows). Zero access by default. | live enforcement against a real integration |
| 4 | **Web UI**: server-rendered admin interface over the config repo — flows, users, groups, access grants, credential *names* (never values), every save a commit. | integration-specific panels (added incrementally as later phases land) |
| 5 | **kranq-side**: generalize tool passing — a pushed task carries its own tool/MCP-server file content instead of relying on kranq's embedded, unconditionally-copied `assets/mcp/*` at a fixed path. | rewriting kranq's headless-Claude invocation contract (script.go's exit-75/gate logic is untouched) |
| 6 | Pluggable integration interface. Bitbucket OAuth as its first instance: scoped token minting for the human path. | Slack, GitHub |
| 7 | Slack integration: bot, per-user allow-list, blank-VM-per-request, public/private visibility rule, the AskUserQuestion relay (built on Phase 5's generalized tool passing). | meta access, cron |
| 8 | Meta access (a flow calling back into kman) and the cron scheduler, incl. a flow granted the capability to create cron entries. | public MCP/skills catalogs |
| 9 | Flows declare access to specific MCP servers and skills, including references into public catalogs — using Phase 5's mechanism, superseding kranq's hardcoded `bitbucket-mcp.py` for kman-originated runs. | — |
| 10 | Homebrew distribution past the stub; config-only-bootstrap verified end to end; written note on further kranq-side cleanup (CLI surface CI no longer needs once kman is the only non-CI caller; optional credentials-passing generalization if it falls out cheaply). | — |

Ordering notes, matching how kranq's own doc reasoned about phase order:
**Phase 1 is second, not last** — every later phase submits work through
it, so it's worth proving against the real kranq shared repo early.
**Phases 2–9 are additive to the flow schema, never retrofitted** — a
schema field exists starting only in the phase that enforces it, avoiding
an inert field nothing reads yet. **The web UI (Phase 4) is slotted after
the access model (Phase 3)**, not before: a meaningfully useful admin
surface needs flows, users, groups, and grants to already exist as
concepts, and each later integration phase (6, 7, 8, 9) adds its own panel
to the same UI rather than the UI being a separate, later bolt-on. **The
kranq-side tool-passing change (Phase 5) is slotted right after the web UI
and before any integration that needs it (7, 9)**, once kman's own core is
proven end to end (Phases 0–4) rather than speculative — the change is
justified by a real, demonstrated need (the AskUserQuestion relay, declared
per-flow skills/MCP), not spun up in advance of one.

## Phase 0 — repo, CLI dispatch, flow schema v0, config loading

**Delivers**: new Go module; `internal/flow` (Flow schema, `Parse`,
`validate`, `Render` projecting a Flow to kranq task-spec YAML);
`internal/config` (`Load` against a git-repo checkout or a plain local
directory); `kman version|validate|render`; `Formula/kman.rb` stub.

**Decisions and why**:
- **kman cannot import any kranq Go package, ever — a process boundary, not
  a schema-freeze.** kranq's `internal/task` is Go-`internal/`-scoped to
  `github.com/simon-em/kranq`; the only interface is a YAML document plus
  `git push`. `internal/flow` re-declares (by name, as data) the fields of
  kranq's `task.Spec`/`Step`/`MCPServer` it needs to emit. Schema drift is
  caught by a Phase-1 integration test that pushes a rendered flow at a
  real/faked kranq, not by the compiler. This boundary is about code
  coupling, not about kranq's YAML schema being frozen — see Phase 5.
- **Flow, not Task, from day one** — the rename is adopted immediately
  since nothing external depends on the old name yet.
- **Args is a typed list, not kranq's flat `env: map[string]string`.**
  kranq's own doc already flagged this exact shortfall in its precedent
  (`inputs:` replacing `CI_FORWARD_ENV`). Shape:
  ```
  args:
    ENVIRONMENT: {default: staging, description: "target environment"}
    REPO_BRANCH: {required: true}
  ```
  Resolved client-side in kman before anything is pushed, so a missing
  required arg fails immediately, named.
- **`kman render` projects Flow → kranq task-spec YAML, not to bash.**
  kranq already owns spec-to-bash (`task.BuildScript`); kman must not
  duplicate it. What kman owns is the next compilation step down. Proving
  this projection in Phase 0 locks the architecture in as tested behavior,
  not aspiration. Render does not resolve `args:` against caller-supplied
  values (kranq doesn't know that field at all); that resolution happens in
  `kman push` (Phase 1), once real values exist to resolve against.
- **Config-repo model**: `Config{Flows []flow.Spec, ...}`. If a config-repo
  URL is set, kman clones/pulls it into `$KMAN_HOME/config`; otherwise it
  reads the same directory shape from a local path with no git remote —
  the "nice for testing" fallback, and what makes "web admin is just an
  interface onto the repo" literal later: the web UI in Phase 4 reads and
  writes through this exact same `Config` type, no second model.
- **`Config` never holds a value unsafe to commit** — secrets are excluded
  on principle now so Phase 2 is additive, not a schema revisit.

**Deferred**: push, VM, Slack, vault, cron, web UI — everything past
parse/validate/render.

## Phase 1 — git mechanics against kranq

**Delivers**: `internal/gitcache` (host-side bare mirror per external repo,
incrementally fetched); a kranq-push client (compiles Flow+args to kranq
task YAML, pushes to `kranq.git` with the right push options, parses
`KRANQ-RESULT`, pulls artifacts); `kman push <flow.yaml> [args...]`.

**Decisions and why**:
- **kman shells out to `git` for everything — cache, config repo, and the
  push to kranq. No Go git library, ever.** Same call kranq already made
  about `limactl`/`git`/`ssh`: an external tool's edge cases are someone
  else's solved problem.
- **The "avoid pushing all commits every time" problem is ref stability,
  not a new protocol.** git already negotiates deltas; the actual cause of
  "pushing everything every time" is a disconnected one-off branch with no
  shared history on the receiving side. Fix: kman keeps one stable ref per
  external repo inside kranq's repo, e.g. `refs/heads/mirror/<repo-slug>`,
  fast-forwarded from the local cache before every run. Once kranq has seen
  a repo's history once, every later push (mirror update, plus the
  ephemeral `task/<run>` branch kranq already layers on top) is a small
  delta — no kranq-side change needed for this part.
- **The host-side cache is a bare mirror keyed by remote URL,
  `git fetch --all --prune`'d incrementally.** Directly answers "avoid
  re-copying all code, so VMs start faster": the expensive clone from
  Bitbucket happens once, same principle as kranq's own layer cache paying
  for a base image once.
- **`kman push` never talks to Lima.** What starts a VM is kranq's own
  `git push kranq -o task_file=... -o env.NAME=VALUE`. "Blank VM" = a flow
  pushed with no Kranqfile, which is exactly kranq's own base layer with
  nothing on top — kman never re-implements VM orchestration.

**Deferred**: credentials in the push (forwards whatever the caller already
has, same as CI today), access control, Slack, web UI.

## Phase 2 — encrypted secrets store and credential binding

**Delivers**: `internal/vault` (age-encrypted file under `$KMAN_HOME/secrets`,
keyed by a machine key generated on first run, never committed); flow schema
addition `credentials:` (binds a step-env name to a vault secret name, never
its value); `kman secret set|ls|rm` (`ls` never prints a value).

**Decisions and why, including the tension with kranq's own principle**:
- **Scoped, narrowly, to human/Slack-triggered runs.** kranq's "not a
  credential vault" position stays correct for the CI-pipeline path —
  nothing about kman changes what a pipeline push looks like or reaches.
  kman fills exactly the gap kranq's doc named and declined to solve: a
  human with no env to forward. This doesn't reopen kranq's position; it
  answers the question kranq's doc left open, in a separate process.
- **Encrypted at rest, unlike kranq's plain 0600 env file** — kranq's file
  holds one operator's own tokens on a machine only they administer;
  kman's vault holds *other people's* credentials (Slack bot token, OAuth
  client secret/refresh tokens, per-user tokens), a qualitatively bigger
  blast radius if the home directory leaks.
- **Only names, never values, in the config repo** — `credentials:
  BITBUCKET_TOKEN: bitbucket/deploy-key` is a reference. Makes "boot an
  identical install from config alone, except secrets" literal: config
  gives shape, the vault (restored separately) gives values.
- **Identity-not-value hashing, reused from kranq's own `SECRET`
  precedent** — kranq's Kranqfile `SECRET` hashes by name only, never
  value, into the layer cache key. kman's credential binding follows the
  same shape for any future compiled-flow identity/cache/audit use.

**Deferred**: minting scoped tokens (Phase 6), enforcement of who may bind
which secret (Phase 3's model exists first, Phase 6 wires it in).

## Phase 3 — users, groups, Slack-identity mapping, flow access profile

**Delivers**: `internal/access` (`User{ID, DisplayName, SlackUserID, ...}`,
`Group{Name, Members}`); flow schema addition `access:` covering the five
dimensions named below; config-repo shape for both, so a grant is a commit;
`kman grant`/`revoke` editing config, never live state.

**Decisions and why**:
- **Zero access by default, unconditionally, on principle** — a
  Slack-recognized user has no groups and no grants until an admin adds
  them. Stated as a hard invariant: the vault becomes a more valuable
  target the moment it's easy to reach broadly.
- **The five dimensions are one struct, not five mechanisms**:
  ```
  access:
    tools:       [bash, read, write]
    meta:        [cron.create]
    skills:      [effetmonstre/deploy-review]
    credentials: [bitbucket/deploy-key]
    flows:       [notify-slack, run-tests]
  ```
  Every dimension is an explicit allow-list, nothing inherited, nothing
  wildcard. A flow's own `access:` is config-repo content — granting a flow
  more capability is a reviewable commit too, and (from Phase 4 on) an
  editable form in the web UI.
- **Groups exist to avoid repeating a Slack ID, not to build RBAC** —
  mirrors kranq's own rejection of "principals, groups, grants... roles,
  scopes." A flat bag of users, no hierarchy, no approval workflow.
- **Slack-identity mapping is a field on `User`, not a separate table** — a
  person has several external handles across integrations; the mapping
  needs to be trivially walkable one direction ("which User is this Slack
  ID") for every inbound event.

**Deferred**: any integration actually consulting this model (Phases 6–9
wire enforcement in one integration at a time); the web UI that edits this
model (Phase 4, immediately next).

## Phase 4 — web UI

**Delivers**: a server-rendered admin interface (part of kman's own daemon,
or a `kman web` command) over flows, users, groups, access grants, and
credential *names* (bound secrets, never their values); every save
round-trips through the same `Config` load/save path the CLI already uses,
producing a single git commit to the config repo (or the local-fallback
directory in no-git mode).

**Decisions and why**:
- **No separate database, ever.** The entire point of "a git repo as
  configuration... the web admin would basically just be an interface for
  this repo" is that there is exactly one source of truth. The web UI reads
  `Config.Load` and writes through a `Config.Save`/commit path that the CLI
  (`kman grant`, `kman flow edit`, etc.) uses too — two front ends, one
  model, one storage format. This is also what keeps "boot an identical
  install from config alone" true for anything ever touched through the UI.
- **Every write is a commit, authored as the acting user.** Once Phase 3's
  `User` model exists, a web edit is attributed to the mapped identity of
  whoever is logged in, making the config repo's own `git log` a real audit
  trail — no separate audit log to build or keep in sync.
- **Concurrent edits are a git conflict, not a lock.** Two admins editing
  overlapping flows resolve the same way two pushes to the same branch
  always do in this system: a non-fast-forward write is rejected and
  retried against the latest state. This reuses the same "let git do the
  compare-and-swap" philosophy kranq's own fence and kman's own config
  loading already lean on — no new locking primitive.
- **Auth to the web UI itself is Phase 3's access model, not a separate
  login system.** Before Phase 3 exists there is nothing to authenticate
  against beyond "reachable on localhost," which is an explicit, named
  limitation of an early build, not a design position — this UI is not
  meant to be exposed beyond the operator's own machine until real
  authentication exists.
- **Framework choice deferred to Phase 4's own implementation**, not fixed
  here: plain server-rendered HTML forms are almost certainly sufficient at
  this scale (a handful of admins, not a public product), and adding a
  frontend framework is a decision to make when Phase 4 is actually built,
  with real usage in hand.
- **Local-fallback mode still works.** With no config-repo URL configured,
  the web UI commits to a local-only repo (or the plain local directory),
  exactly matching Phase 0's config-loading fallback — so a contributor can
  run the whole admin experience on a laptop with zero external setup.

**Deferred**: integration-specific panels (Bitbucket connection status,
Slack identity linking, cron schedule editing, skills/MCP picker) — each
added by its own later phase (6, 7, 8, 9) once that integration exists to
have a panel about.

## Phase 5 — kranq-side: generalize tool passing

**Delivers** (this work lands in the **kranq repo**, not kman's — a scoped,
reviewed change, not part of kman's own build): a generic way for a pushed
task to carry its own tool/MCP-server implementation as file content,
materialized in the VM before any step runs, instead of relying on kranq's
build-time-embedded `assets/mcp/*` copied unconditionally into every VM at
a fixed path.

**Decisions and why**:
- **This is the one place kranq's current design is worth changing.**
  Today, `Step.MCPServers` (command, args, env) is already generic for
  *configuring* a server, but there is no generic way to *deliver* an
  arbitrary server's implementation into the VM — only kranq's own embedded
  assets, or a Kranqfile `COPY` (a build-time image layer, too heavy for a
  per-run script), or a hand-rolled heredoc smuggled into a `run:` step's
  bash (a real fallback that works today, and exactly the kind of
  accumulating hack this phase exists to avoid needing).
- **Concrete shape**: a task-level `files:` list (`path`, `mode`, `content`
  as base64, size-capped the same way `-o spec=` already caps at
  `MaxSpec = 1<<20`), staged by the compiled script (a new early step,
  `ci_stage_files`, alongside the existing `ci_step`/`ci_claude` helpers in
  `internal/task/script.go`) before any `run:`/`claude:` step executes.
  This is deliberately task-level and generic — not MCP-specific — so it
  also quietly fixes the "bitbucket-mcp.py path is hardcoded" complaint for
  kranq's own CI use: a CI task can carry its own MCP server file the same
  way, instead of depending on whatever kranq happened to embed at build
  time.
- **No change to kranq's headless-Claude invocation contract.** `script.go`'s
  exit-75 usage-gate protocol, `rate_limit_event` parsing, and the
  verdict/status-file convention are pinned by kranq's own tests as things
  that must never be "simplified" — this phase only adds a file-staging
  step ahead of the existing `claude`/`run` steps, it does not touch how
  either runs.
- **Backward compatibility is not a design constraint here** — kranq does
  not owe its current CI callers a frozen schema.
  `assets/mcp/bitbucket-mcp.py` can stay as a legacy embedded default for
  tasks that don't specify `files:`, or be migrated to use the new
  mechanism itself and dropped from the embedded binary entirely — worth
  deciding when this phase is actually built, not pre-committed here.
- **Enables, directly**: Phase 7's AskUserQuestion relay (kman's own
  `kman-ask` MCP server, delivered via `files:`, no longer needing to be
  pre-embedded in kranq) and Phase 9's per-flow declared skills/MCP access.

**Deferred**: any change to how credentials are passed (noted as an
optional, lower-priority follow-on in Phase 10 if it falls out cheaply, not
required here).

## Phase 6 — pluggable integration interface, Bitbucket OAuth first

**Delivers**: `internal/integration` (an `Integration` interface with two
optional facets: `CredentialProvider` mints/refreshes a scoped credential
for a user+flow, `TriggerChannel` is an external system that starts flows);
`internal/integration/bitbucket` (OAuth app registration, per-user token
exchange, scoped token minting on demand); enforcement wired so a flow's
`credentials:` entry referencing Bitbucket is resolved through OAuth for
the triggering user, not a static vault value; a web-UI panel (Phase 4)
showing connection status and per-user linked accounts.

**Decisions and why**:
- **Bitbucket and Slack are different shapes of "integration," so the
  interface says so.** Bitbucket here is purely a credential source
  (given a user, mint a scoped token). Slack (Phase 7) is purely a trigger
  source (an event says "run this flow, as this person"). GitHub could be
  both — which is why `CredentialProvider`/`TriggerChannel` are separable
  facets, not one monolithic interface every implementation stubs out.
- **Scoped, not blanket** — OAuth lets kman mint something narrower than
  "this user's full Bitbucket access" per request, short-lived where
  possible. Where Bitbucket's scopes are coarser than desired, that's
  documented as an accepted limit, not papered over.
- **Still exclusively the human/Slack path** — a CI pipeline's own
  step-level Bitbucket OAuth token (today's mechanism) is untouched.

**Deferred**: Slack itself, GitHub as a second `CredentialProvider` (left as
proof the interface generalizes, not built now).

## Phase 7 — Slack integration, blank-VM-per-request, AskUserQuestion relay

**Delivers**: `internal/integration/slack` (bot, event handling, per-user
allow-list check against Phase 3's model before anything runs); the
blank-VM request path (a Slack message becomes a flow push with no
Kranqfile, or the named repo's own flow); the AskUserQuestion relay as a
kman-provided MCP server (`kman-ask`), delivered into the VM via Phase 5's
`files:` mechanism and declared in the compiled task's `mcp_servers:`; the
public/private visibility rule enforced where kman posts output back; a
web-UI panel for linking a user's Slack identity and reviewing allow-lists.

**Decisions and why**:
- **Per-user allow-listing is a Phase-3 access check, not a new
  mechanism** — resolve the Slack user ID to a kman `User`, check that
  user's grant for the requested flow. No new authorization concept.
- **Visibility rule: permission check is against the triggering user; the
  output's audience is the channel.** A public/shared-channel request posts
  output visible to everyone in that channel — Slack's own channel model,
  not a second, weaker redaction layer inside kman. A DM stays private.
  Hard rule: kman never grants a *viewer* more or less than what the
  *triggering user* was authorized to see — it only decides who else is in
  the room.
- **AskUserQuestion relay: an MCP server, not an Agent SDK rewrite —
  recommendation and why.** Two options existed: (1) rewrite the headless
  invocation on the Agent SDK, intercepting `AskUserQuestion` in the SDK's
  own loop; (2) ship a small MCP server (`kman-ask`) declared in the flow's
  compiled `mcp_servers:` block, delivered via Phase 5's `files:` mechanism
  rather than pre-embedded in kranq, with the built-in `AskUserQuestion`
  disabled via `disallowed_tools` on that step. **Recommend (2).** kranq's
  task compiler already owns the entire headless-invocation contract
  (exit-75 usage protocol, `rate_limit_event` parsing, the
  verdict/status-file convention) pinned by kranq's own tests as things
  that must never be "simplified." An SDK rewrite re-derives that contract
  on a second code path kranq's tests know nothing about, and would touch
  how kranq drives Claude — a bigger, riskier change than Phase 5's narrow
  file-delivery addition. The MCP option, once Phase 5 lands, needs no
  further kranq change at all: kman populates `mcp_servers.kman-ask` with
  `command`, `args`, and `env: {KMAN_RUN_ID, KMAN_FLOW_ID,
  KMAN_SLACK_CHANNEL}`, and supplies the server's own source via `files:`.
  The tool posts to Slack, blocks with a timeout (a headless run can't wait
  forever), returns the human's answer as the tool result.

**Deferred**: meta access, cron.

## Phase 8 — meta access and cron

**Delivers**: a scoped HTTP endpoint kman exposes, reachable from a VM kman
started, authenticated by a short-lived single-purpose token kman mints per
run and injects as env (`KMAN_META_TOKEN`) — the same pattern kranq already
uses to forward `CLAUDE_CODE_OAUTH_TOKEN`; `internal/cron` (config-repo
schedule entries naming a flow + cron expression, a ticker in kman's
daemon); the self-referential capability (a flow granted `meta:
[cron.create]` calls back through the meta endpoint to create a cron
entry); a web-UI cron schedule editor.

**Decisions and why**:
- **"Access back to kranq" is narrowed to "back to kman" — here's why
  that's more restrictable.** kranq's own push/fetch surface has no scope
  narrower than "push, fetch" (by design). Handing a VM a real kranq
  credential means it can push anything that credential can. Instead the
  VM gets a kman-minted token good only for that flow's `access.meta`
  list, re-checked by kman on every call; when a call resolves to "submit
  a task for repo X" or "create a cron entry," **kman** performs the
  actual `git push`/config commit with kman's own credential. The VM never
  sees a kranq credential.
- **Guest→host reachability is unconfirmed and must be probed before this
  phase is built**, in the same spirit as kranq's own doc flagging
  unconfirmed writes to `refs/kranq/*` against Bitbucket Cloud. Verify
  whether a kranq-booted VM (per `assets/lima.yaml`) can reach a host
  endpoint (e.g. `host.lima.internal` under `vz` networking) before
  finalizing the design around it; the fallback is plain internet egress to
  a kman endpoint on its own public/LAN address, same posture as kranq's
  git server already has.
- **Cron is deliberately small** — no admission control or bidding, it just
  fires a flow push and lets Phase 1 do the rest. A missed tick is not
  retroactively retried (stated explicitly — silently "catching up" could
  double-run a side effect).
- **A meta-created cron entry is still config-repo content**, written by
  kman on the granted flow's behalf, so "config alone rebuilds an identical
  install" stays true even for self-created schedule entries, and it shows
  up in the web UI's cron editor the same as one an admin typed by hand.

**Deferred**: skills/MCP catalogs (Phase 9).

## Phase 9 — declared MCP servers and skills, public catalog references

**Delivers**: flow schema additions `access.skills`/MCP entries referencing
either a local resource (delivered via Phase 5's `files:` mechanism) or a
public catalog entry (name + pinned ref); kman composes each step's
`mcp_servers:` from what that flow was granted, superseding kranq's
unconditional `bitbucket-mcp.py` for kman-originated flows; a web-UI
skill/MCP picker.

**Decisions and why**:
- **"Too hardcoded, should be a skill" is resolved by moving the decision
  of which MCP servers/skills a run gets from kranq (unconditional, for
  everyone) to kman (per-flow, per-grant), using Phase 5's delivery
  mechanism.** kranq may keep its embedded default for its own CI use (a
  kranq-side cleanup to note, not required here); any kman-originated run
  gets exactly what its flow's `access:` names.
- **A public catalog reference is a name plus a pin, never "latest"** —
  consistent with config-repo-is-source-of-truth: "which skill did this
  flow run with" must be answerable from a commit alone.
- **Cross-flow access reuses Phase 3's declared-allow-list shape**,
  enforced here alongside skills/MCP — this is the phase where all five
  access dimensions are wired end to end for the first time.

## Phase 10 — distribution and cleanup

Homebrew formula past the stub (real tag, real sha256, kranq's exact
self-tap pattern); end-to-end verification that a fresh machine, given only
the config repo (secrets restored separately), reconstructs an identical
kman install; a written note (not code) on kranq-side cleanup: CLI surface
CI no longer needs once kman is the only non-CI caller of anything beyond
`git push`/`git pull`/`git fetch` for the verdict ref, and — optional, only
if it falls out cheaply — generalizing how credentials are named/passed
into a kranq task (today: ad hoc forwarded env plus one special-cased
`CLAUDE_CODE_OAUTH_TOKEN` variable name in the scheduler) into something as
structured as Phase 5 made tool passing, so kman's vault-resolved
credentials arrive as a first-class, named concept rather than flattened
`-o env.NAME=value` pairs kranq has no visibility into.
