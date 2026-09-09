# kman status

## Done

**Phase 0** — CLI dispatch, the flow schema (`internal/flow`), config-repo
loading with a local-directory fallback (`internal/config`).
`kman version|validate|render`. No test suite beyond smoke tests, matching
kranq's own Phase 0 scope.

**Phase 1** — git mechanics against kranq:
- `internal/gitcache` — a host-side bare mirror per external repo, keyed by
  a hash of the remote URL, cloned once and `fetch --prune`'d incrementally
  after that.
- `internal/kranqpush` — reimplements kranq's own push wire format (push
  option names, the synthetic `refs/heads/task/<ts>-<nonce>` ref, the
  `KRANQ-RESULT id=... status=... exit=... result=...` line kranq's
  post-receive hook prints) without importing any kranq Go package, per the
  Phase 0 boundary. Also pushes a permanent `refs/heads/mirror/<slug>` ref
  before every task push, best-effort, as a durable object anchor kranq's
  own ref-sweep never touches (it only sweeps `task/`/`ok/` prefixes) — this
  is what keeps repeat pushes small instead of resending history.
- `internal/flow.ResolveArgs` — resolves a flow's typed `args:` block
  against caller-supplied values (provided wins, then default, then a
  required-but-missing arg is a named error), producing the env kman
  forwards as `-o env.NAME=VALUE` push options — kman's own `env forward`.
- `kman push <flow.yaml> [NAME=VALUE...] [--kranq-url] [--source]` —
  renders the flow, resolves a source directory (a local git dir, default
  `.`, matching kranq's own `kranq push` "wherever you're standing" model;
  or a remote URL, synced through `internal/gitcache` first), updates the
  mirror ref, pushes the task, and exits on the parsed `KRANQ-RESULT`.
- `kman mirror <remote-url>` — a thin CLI wrapper exposing `gitcache.Sync`
  directly, printing the resulting local mirror path.

Verified against real local bare git repos with a stub `post-receive` hook
that mimics kranq's contract (both in `go test ./...` and an end-to-end
manual run) — not against a real kranq instance yet. That's the next thing
worth doing once there's a real kranq host reachable to test against: push
a flow at it, confirm the rendered YAML round-trips through kranq's actual
`internal/task.Parse` without drift.

**Phase 2** — the encrypted secrets store and credential binding:
- `internal/vault` — secrets live encrypted (AES-256-GCM, stdlib only, no
  new dependency) under `$KMAN_HOME/secrets/vault.enc`, keyed by a 32-byte
  machine key generated on first `Open` and written to `$KMAN_HOME/vault.key`
  at 0600. "age-style" in docs/design.md described the shape (a local,
  file-based, machine-keyed store), not literally the `age` file format or
  library — this is a from-scratch stdlib implementation, a deliberate
  choice to keep kman's dependency count at zero, same as kranq's own.
  Worth revisiting only if something outside kman ever needs to decrypt the
  file directly (e.g. with the real `age` CLI for recovery).
- `kman secret set <name> [value]` (value read from stdin if omitted, so it
  never has to appear in shell history), `kman secret ls` (names only,
  never values — verified by a test that the encrypted file on disk never
  contains a set value or name in the clear), `kman secret rm <name>`.
- Flow schema addition `credentials: {ENV_NAME: secret-name}` — validated
  (no empty env name or secret name) but never rendered into the kranq
  task-spec YAML, same as `args:`: kranq has no concept of it. At push
  time, `kman push` resolves each binding against the vault and merges it
  into the same env `kman push` already forwards for `args:`, refusing to
  push if an env name is claimed by both an arg and a credential (an
  authoring mistake, not a runtime tiebreak) or if a bound secret is
  missing from the vault.

Not yet built, on purpose (per docs/design.md's phasing): enforcement of
*who* may bind which secret (that's Phase 3's access model, wired in at
Phase 6) and Bitbucket OAuth minting (Phase 6). Right now any flow can bind
any secret in the vault — there is no access model yet to gate that, which
is expected at this phase but worth remembering before this is exposed to
more than one operator.

**Phase 3** — users, groups, Slack-identity mapping, the flow access profile:
- `internal/access` — `User{ID, DisplayName, SlackUserID, Flows}` and
  `Group{Name, Members, Flows}`, a `Registry` over both.
  `Registry.CanTrigger(userID, flowName)` is true only via a direct grant on
  the user or a grant on a group the user belongs to — zero access by
  default, nothing inherited, nothing wildcard, matching kranq's own
  rejection of "principals, groups, grants... roles, scopes": groups here
  are a flat bag of users, not a hierarchy. `Registry.UserBySlackID` is the
  one-directional lookup Phase 7's Slack integration will use to resolve an
  inbound event to a kman user.
- Flow schema addition `access: {tools, meta, skills, credentials, flows}`
  (`internal/flow/access.go`) — the five dimensions the design doc named,
  each a plain allow-list, never rendered into the kranq task-spec YAML.
  `credentials:` (Phase 2's binding) and `access.credentials` (the grant)
  are cross-validated at parse time: every secret name a flow binds via
  `credentials:` must also be listed in `access.credentials`, so "this flow
  may use this secret" is a fact `flow.Parse` can already prove today, even
  though nothing enforces it against a live caller yet (that's Phase 6).
- `internal/config.Config` gains `Access access.Registry`, loaded from
  `config/users/*.yaml` and `config/groups/*.yaml` (one file per entity,
  same convention as `config/flows/*.yaml`) and validated for referential
  integrity on every load: a user's or group's grant must name a real flow,
  a group's member must be a real user, and a flow's own `access.flows`
  must name real flows too. A config directory that fails any of these
  checks fails to load at all, on purpose — better a loud error now than a
  grant that silently refers to nothing.
- `kman user set|ls`, `kman group set|ls|add-member|remove-member`,
  `kman grant <user/ID|group/NAME> <flow>`, `kman revoke ...` — CLI-only
  at this phase, no live enforcement consumes `Registry.CanTrigger` yet
  (that starts at Phase 7's Slack allow-list check). Both `grant` and
  `revoke` refuse a flow name that doesn't exist in the current config, and
  `group add-member` refuses a user ID that doesn't exist, so the
  referential-integrity rule `Registry.Validate` enforces on load is also
  enforced proactively at write time. **Since Phase 4** these commands
  write through the same `config.SaveUser`/`SaveGroup` path the web UI
  uses, so they now commit automatically too when `$KMAN_HOME/config` is a
  git repo — see Phase 4 below; this corrects what this section originally
  said (grant/revoke were CLI-only, uncommitted writes when Phase 3 shipped).

Not yet built, on purpose: nothing yet reads `Registry.CanTrigger` in
anger, because nothing yet triggers a flow on a user's behalf other than a
human running `kman push` directly at a terminal, which needs no
permission check of its own (the operator running the CLI already has
whatever access the machine's credentials give them, same as kranq's own
"if you can reach it, you're authorized" stance in `internal/gitsrv`'s
model — a permission model without a caller to check it against is just
data).

**Phase 4** — the web UI:
- `internal/config` gained a single-entity store shared by the CLI and the
  web UI, exactly as docs/design.md called for: `LoadFlow`/`LoadUser`/
  `LoadGroup`, `SaveFlow`/`SaveUser`/`SaveGroup`, `ListFlowNames`/
  `ListUserIDs`/`ListGroupNames`, and `ReadFlowRaw` (the exact bytes on
  disk, for round-trip-faithful editing — `LoadFlow` parses and would lose
  comments/formatting if used to redisplay a flow for editing).
  `internal/cli/access.go` was refactored to call these instead of writing
  files directly, which is what gave `kman grant`/`user set`/`group set`
  auto-commit for free — one save path, two front ends, per design.
- **Every `Save*` commits when `$KMAN_HOME/config/.git` exists, and is a
  silent no-op commit-wise otherwise.** Both are real, tested modes, not a
  fallback: a plain directory (no git at all) is Phase 0's local-testing
  mode; a git repo with no remote is "config alone can rebuild an
  identical install" made real without needing a server anywhere.
  Committer identity is always `kman <kman@kman.local>`; author identity is
  the acting operator's name if known, `kman` if not — see the next point.
- **No real login system, exactly as docs/design.md said there wouldn't
  be at this phase.** There is no session, no password, no per-request
  identity. What exists instead: a plain, unauthenticated "Acting as" text
  field on every form, remembered across page loads via a `kman_actor`
  cookie, used *only* to attribute the git commit's author — never checked
  against anything, never a permission gate. It makes `git log` on the
  config repo meaningful without pretending to be a security boundary.
  `kman web` binds `127.0.0.1` by default for the same reason design.md
  gave: this is not meant to be reachable beyond the operator's own
  machine until real authentication exists.
- **Flows are edited as raw YAML in a `<textarea>`; users and groups get
  real form fields (text inputs, and checkboxes for grants/membership
  built from `ListFlowNames`/`ListUserIDs`).** A flow's schema is deep
  (steps, args, credentials, access, mcp_servers); building a structured
  form for all of it now would be premature relative to how much the
  schema is still growing phase to phase. `access.User`/`Group` are simple
  enough that a structured form is strictly better UX for the same effort.
  On a validation error, the page re-renders with exactly what was typed
  (not the last saved version), so a mistake never costs the edit.
- Plain `net/http` + `html/template`, stdlib only — no new dependency, no
  JS, no CSS framework. Templates are `//go:embed`ded so `kman web` is
  still a single static binary, same as everything else in kman.
- **A known, documented limitation, not a bug**: saving a flow/user/group
  under a different name than the one it was opened with creates a new
  file rather than renaming; the old file is left behind. Renaming isn't
  built yet, and silently deleting the old file felt like the wrong
  default to guess at without deciding it on its own merits.

**Phase 5** — kranq-side: generalize tool passing. **This landed in the
kranq repo (`/Users/effetmonstre/workspace/kranq`), not here** — kman's
own tree is unchanged by it. kranq's `task.Spec` gained `files:` (a
`path`/`mode`/`content`-as-base64 list, `task.MaxFileSize` capped at
1MiB decoded), staged by a new bash block `BuildScript` emits before any
`run:`/`claude:` step executes (`internal/task/script.go`'s
`writeFileStaging`, committed as kranq `bff29da`). Verified with kranq's
own test suite (`go test ./...` unaffected, new tests for `File.validate`
and real-bash execution of the staging step) and a manual `kranq
validate`/`render`/execute smoke test. Pushed to kranq's remote as `bff29da`
after operator review. This is what Phase 7's `kman-ask` MCP relay and Phase 9's declared
skills/MCP access will build on: a flow can now carry its own tool
implementation in the push instead of depending on what kranq happened
to embed at build time.

**Phase 6** — the pluggable integration interface, Bitbucket OAuth first:
- `internal/integration` — `Integration` (a `Name()` marker),
  `CredentialProvider` (`Credential(ctx, userID) (Credential, error)`), and
  `TriggerChannel`, deliberately left as a bare marker interface for
  now — Slack (Phase 7) is what will actually shape its method set, and
  guessing that shape today risked designing something Phase 7 would just
  replace, the same "additive only in the phase that enforces it" discipline
  already applied to the flow schema.
- `internal/integration/bitbucket` — a real OAuth 2.0 Authorization Code
  client against Bitbucket Cloud's actual endpoints
  (`/site/oauth2/authorize`, `/site/oauth2/access_token`), tested against a
  fake server via `httptest` (`oauth_test.go`) covering the authorize URL,
  code exchange, refresh, and both error shapes (non-200, non-JSON). It has
  never been run against the real bitbucket.org — there's no live OAuth
  consumer registered anywhere this session could reach, and no browser to
  click through Bitbucket's own consent screen. Everything client-side of
  that consent screen is proven end to end, including a manual run: a real
  `kman` binary, a real local HTTP server standing in for Bitbucket,
  `kman web`'s actual connect → redirect → callback → token-exchange →
  vault-storage path, verified by `kman integration bitbucket status`
  flipping from "not connected" to "connected".
- `Provider` stores per-user tokens in the same vault Phase 2 built,
  namespaced `bitbucket/oauth/<user-id>/{access_token,refresh_token,
  expires_at}` — no second secrets store. `Credential` returns the cached
  access token if it has more than a minute of life left, otherwise
  refreshes and re-stores before returning. The OAuth app's own client
  credentials live at the fixed vault keys `bitbucket/oauth/client_id` /
  `client_secret` (`kman secret set` — nothing new to build for that), plus
  an optional `bitbucket/oauth/base_url` override for Bitbucket Server/Data
  Center or, as it happens, a test double.
- **`credentials:` entries now route two ways**, decided by the secret
  name's prefix: a plain name is a vault lookup (Phase 2, unchanged); a
  name prefixed `integration:<name>` (e.g. `integration:bitbucket`) resolves
  through that integration's `CredentialProvider` instead, for the flow's
  acting user. Nothing about Phase 2/3's schema or validation changed —
  `access.credentials` still has to list it, `credentials:` still binds it
  to an env var — only the resolution step at push time gained a branch.
- **`kman push` gained `--as <user-id>`** (default `$KMAN_ACTOR`, the same
  env var Phase 4 uses for commit attribution) because integration-routed
  credentials are inherently per-person: minting "the" Bitbucket token
  requires knowing whose. A push using an `integration:` credential with no
  acting user known is a named, immediate error, not a silent fallback.
- `kman integration bitbucket status [user-id...]` and a `/integrations`
  web panel (status per known user, a "Connect Bitbucket as X" link driven
  by the same actor-cookie convention Phase 4 established) — both read-only
  displays over the same `Provider.Connected`, no separate view logic.
- **Still exclusively the human/Slack path.** A CI pipeline's own
  step-level Bitbucket token (forwarded env, today's mechanism) is
  completely untouched; nothing in this phase changes how kranq itself
  receives credentials from a pipeline.

Not built, on purpose, per docs/design.md: Slack itself and GitHub as a
second `CredentialProvider` (Phase 6 only had to prove the interface
generalizes, not generalize it twice).

**Phase 7** — Slack integration, blank-VM-per-request, the AskUserQuestion
relay:
- `internal/trigger` — the compile-args-then-push logic that used to live
  entirely inside `cli.runPush` moved here (`Run`, `ResolveCredentials`,
  `MergeEnv`, `ParseAssignments`), so `kman push` and the new Slack handler
  share one implementation instead of two copies drifting apart. `Run`
  wraps a failure in a staged `*trigger.Error{Stage, Err}` so the CLI can
  still map it to the right POSIX exit code
  (`args`/`credentials`→`InvalidSpec`, `source`→`InternalError`,
  `push`→`Unreachable`) while a caller that doesn't care about exit codes
  (Slack) can just read the message. `Options.ExtraEnv` is new: a third env
  source alongside args/credentials, merged in through the same
  collision-checked `MergeEnv`, added so the Slack handler can inject
  `SLACK_BOT_TOKEN` without it going through `access.credentials`/the vault
  (it isn't a per-user credential, it's kman's own bot token).
- `flow.Spec` gained `files:` (`flow.File{Path,Mode,Content}`,
  `flow.MaxFileSize`), a direct mirror of kranq's own Phase-5 addition,
  passed straight through in `Render`. This is the first flow-schema growth
  since Phase 3, on purpose: nothing needed it until the ask relay did. A
  flow author can also declare `files:` directly, for the same reason
  kranq's own version exists — carrying a tool's implementation alongside
  the task that uses it.
- `flow.Access` gained `HasMeta(name string) bool`, a small generic
  addition (Phase 3's `Access.Meta` field already existed; this is just the
  first thing that reads it) — used to check for the `ask` capability that
  opts a flow into the relay.
- `internal/integration/slack` — `Client.PostMessage` (`chat.postMessage`),
  `VerifySignature` (Slack's `v0=` HMAC-SHA256 scheme, with the 5-minute
  replay-window check Slack's own docs call for), `ParseEnvelope`/`Event`
  (Events API JSON, including the `url_verification` handshake),
  `ParseCommand` (strips the `<@BOTID>` mention, expects `run <flow>
  [NAME=VALUE...]`), and `Handler` — the actual `POST /slack/events`
  endpoint: verify the signature, answer `url_verification`, ignore
  anything that isn't a human `app_mention`, ack within the request (Slack
  requires a reply inside 3s) and do the real work — resolve the Slack user
  to a kman `User` (`Registry.UserBySlackID`), check
  `Registry.CanTrigger`, load the flow, push it via `internal/trigger`,
  and post the result back — in a goroutine, since a push blocks until
  kranq's `KRANQ-RESULT` line comes back and that can take minutes. Every
  reply lands in a thread (the incoming message's own `ts` if it wasn't
  already in a thread), so a channel with several requests in flight stays
  readable.
- **Per-user allow-listing is exactly Phase 3's `CanTrigger`, no new
  mechanism** — a Slack user with no linked kman `User`, or a linked user
  with no grant for the named flow, gets a named reason back in the
  channel, never a silent no-op.
- **The "blank VM" path needed no new code.** A flow pushed with no
  `kranqfile:` already gets kranq's own base layer with nothing on top —
  that was already true before this phase; Slack triggering it is just
  another caller of `kman push`'s existing behavior.
- **Visibility is Slack's own channel model, not a second layer inside
  kman.** kman posts the result to whatever channel the request came from;
  it never decides who else can see that channel. The permission check is
  against the triggering user (`CanTrigger`), never against who else might
  read the reply.
- **The AskUserQuestion relay is an MCP server (`kman-ask`), not an SDK
  rewrite**, exactly as docs/design.md called for. `internal/integration/
  slack/assets/kman-ask.py` (go:embed'd) implements the same minimal
  stdio JSON-RPC shape as kranq's existing `assets/mcp/bitbucket-mcp.py` —
  `initialize`/`tools/list`/`tools/call` — with one tool,
  `ask_user_question`: posts to Slack, polls `conversations.replies` on the
  thread every 3s up to a timeout (default 10 minutes), returns the first
  human (non-bot) reply as the tool result. `InjectAskRelay` wires this in
  at push time, only when a flow declares `access.meta: [ask]`: it appends
  the script to `files:`, and on every `claude:` step it adds an
  `mcp_servers.kman-ask` entry (`command: python3`, `args: [the staged
  path]`, `env: {KMAN_RUN_ID, KMAN_FLOW_ID, KMAN_SLACK_CHANNEL,
  KMAN_SLACK_THREAD_TS}`) and appends `AskUserQuestion` to that step's
  `disallowed_tools`, so Claude has no choice but to go through the relay.
  The bot token itself is deliberately **not** in that env map — it travels
  as `SLACK_BOT_TOKEN` via `Options.ExtraEnv` (a push-option env var,
  exactly like a resolved credential), inherited by every step's shell and
  therefore by the MCP server subprocess too, rather than being baked into
  the `mcp_servers:` block of the rendered task YAML.
- A `/integrations` panel section for Slack (configured/not, and a table of
  users with their linked Slack ID) — Slack identity linking itself needed
  no new UI, since Phase 3/4 already put `slack_user_id` on the user edit
  form; this phase only had a connection-status panel to add, matching
  Bitbucket's.
- `kman slack serve [--addr] --kranq-url <url>` is a **separate** command
  from `kman web`, deliberately: `kman web` binds `127.0.0.1` by default
  specifically because it has no login (Phase 4's documented tradeoff), and
  Slack's Events API needs a publicly reachable endpoint. Splitting them
  keeps the unauthenticated admin UI off any address Slack (or anything
  else) can reach, while the Slack endpoint carries its own real
  authentication (the signature check) and does nothing else.
- Verified with `go test ./... -race` across the new/changed packages (unit
  tests for the signature scheme, command parsing, `InjectAskRelay`'s
  non-mutation of the input spec, and an end-to-end `Handler` test using a
  fake Slack API server and a real local bare git repo standing in for
  kranq, plus a manual run: a real `kman slack serve` process, a real
  Python fake-Slack HTTP server, a signature computed independently in
  Python (not reusing kman's own Go implementation) to catch any drift
  between the two, and a real local `post-receive` hook — the whole
  signature → allow-list → push → thread reply path observed working, plus
  the "no linked user" rejection path.

Not built, on purpose: meta access and cron (Phase 8).

## Left to do

Everything from Phase 8 onward in [docs/design.md](design.md): meta
access, cron, declared MCP/skills access, and distribution past the
Homebrew stub.

Worth flagging about Phase 7, before it's forgotten:
- **Never tested against real Slack.** Every piece proven so far is a fake
  HTTP server standing in for `slack.com/api` — there's no real Slack app,
  bot token, or signing secret in this environment. The signature scheme
  and Events API shapes are implemented from Slack's published docs, not
  verified against Slack's actual servers. Worth a real app before
  depending on this.
- **The AskUserQuestion relay has never run inside an actual kranq VM.**
  `kman-ask.py`'s JSON-RPC handshake and its `tools/call` error path are
  exercised directly (`python3 internal/integration/slack/assets/
  kman-ask.py` fed synthetic stdin), but the whole point — Claude actually
  invoking it mid-run inside a Lima VM, blocked on a real Slack thread
  reply — has no test and can't get one without a real kranq host and a
  real Slack app at the same time.
- **A Slack-triggered push requires `spec.Repo` to be a real remote URL.**
  Unlike `kman push`'s CLI default of `.` ("wherever you're standing"),
  there's no local checkout to fall back to inside a long-running `kman
  slack serve` daemon, so the handler refuses up front with a named error
  if `spec.Repo` isn't set to something `trigger.LooksLikeRemote` accepts,
  rather than silently resolving against the daemon's own working
  directory.

Two things worth flagging since Phase 1, before they're forgotten:
- **Artifact/output retrieval isn't built.** `kman push` reports the
  `KRANQ-RESULT` line (pass/fail, exit code, the verdict ref), but nothing
  yet fetches the ref's actual committed content back over git the way
  `infrastructure/ci/kranq-ci`'s `git pull --ff-only kranq "$KRANQ_TASK"`
  does. It's a natural extension of `internal/kranqpush` when it's needed —
  fetch `result.ResultRef` into the local mirror and let the caller inspect
  it with plain git — but nothing forced the decision yet, so it's not
  built ahead of a real need.
- **Never tested against a real kranq instance.** The stub-hook tests prove
  kman's own wire format is self-consistent; they can't catch a real drift
  against kranq's actual `internal/gitsrv`/`internal/task` code. Worth doing
  before relying on this for anything real.
