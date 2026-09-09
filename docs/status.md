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

**Phase 8** — meta access and cron:
- `internal/meta` — `Store` mints a short-lived (`DefaultTokenTTL` = 2h),
  capability-scoped `Token{Value, FlowName, UserID, Grants, ExpiresAt}`,
  persisted to `$KMAN_HOME/meta-tokens.json` (0600) rather than kept
  in-memory, because the minting process (`kman push`, a one-shot CLI
  invocation, or `kman slack serve`) and the validating process (`kman meta
  serve`) are routinely different OS processes. `Server` exposes exactly
  one endpoint, `POST /meta/cron`, authenticated by `Authorization: Bearer
  <token>` — **the created cron entry's `flow` field is always the token's
  own `FlowName`, never anything the request body sends**, which is what
  makes this "the self-referential capability" for real: a flow can
  schedule *itself*, never an arbitrary other flow, no matter what a
  compromised or buggy VM-side caller puts in the request.
- **"Access back to kranq" narrowed to "back to kman," exactly per
  docs/design.md.** The VM never sees a kranq credential; it sees a
  kman-minted bearer token good only for the capabilities the *pushed
  flow's own* `access.meta` list named, re-checked by `Store.Validate` on
  every call. When a call succeeds, **kman** (not the VM) performs the
  actual `config.SaveCronEntry` git commit with kman's own attribution.
- `internal/trigger.Run` mints and injects the token itself, generically —
  not Slack-specific, unlike the ask relay — so any push path (`kman
  push`, Slack, and now cron's own re-firing of a flow) gets it the same
  way whenever `spec.Access.Meta` is non-empty. `KMAN_META_TOKEN`/
  `KMAN_META_URL` travel via the same `Options.ExtraEnv` push-option path
  the Slack bot token already used, so a meta token never gets baked into
  the rendered task YAML either. Missing `--meta-url`/`$KMAN_META_URL` is
  a loud stderr warning and the push proceeds without the token (a flow
  can declare `access.meta` defensively before an operator has wired cron
  up); missing `--as`/`$KMAN_ACTOR` on a flow that *does* have a meta URL
  configured is a hard `StageMeta` error, same shape as the existing
  `StageArgs`/`StageCredentials`/`StageSource`/`StagePush` stages — there's
  no one to attribute a minted token to otherwise.
- `internal/cron` — `Entry{Name, Flow, Schedule, Args, CreatedBy}` plus a
  hand-rolled 5-field (`minute hour dom month dow`) schedule parser/matcher
  (`*`, `*/N`, `A-B`, comma lists) — deliberately not a dependency, and
  deliberately simpler than real cron: **DOM and DOW are ANDed, not
  OR'd like POSIX cron's documented special case.** Config-repo storage
  (`config/cron/<name>.yaml`) follows the exact `Load*/Save*/List*` shape
  Phase 0's flows and Phase 3's users/groups already established, plus one
  new thing none of those had: `RemoveCronEntry`, because a stale schedule
  left with no way to cancel it is an operational hazard a flow/user/group
  edit form isn't (those still have no delete path anywhere in kman).
  `config.Load`'s referential-integrity pass now also rejects a cron entry
  naming an unknown flow, the same way it already does for a grant or an
  `access.flows` reference.
- **Cron is deliberately small, exactly per docs/design.md**: `kman cron
  tick --kranq-url <url>` loads the config once, matches every entry's
  schedule against `time.Now()`, and fires each due one with `Detach:
  true` (so a slow kranq run never stalls the rest of the tick) — no
  admission control, no bidding, and a missed tick is never retroactively
  caught up (each tick only ever asks "is *now* due", never "what did I
  miss"). `kman cron serve --interval 1m` is a thin loop around the exact
  same `tickCron` function `kman cron tick` calls once — the one-shot form
  exists so `tickCron` is testable without a long-running process, and so
  an operator can drive kman's cron off real OS cron/launchd instead of a
  daemon if they'd rather.
- A cron-fired flow needs `spec.Repo` to be a real remote URL, same
  constraint and same reasoning as Phase 7's Slack-triggered push: `kman
  cron serve` is a long-running daemon with no "wherever you're standing"
  checkout to fall back to, so `tickCron` skips (with a named stderr
  line) any entry whose flow isn't `trigger.LooksLikeRemote`, rather than
  resolving against the daemon's own cwd.
- `kman meta serve [--addr] ` is its own **third** separate listener,
  alongside `kman web` and `kman slack serve`, for the same reason those
  two are already split: it needs to be reachable from somewhere `kman
  web`'s login-less admin UI must not be — in this case, a kranq-booted
  VM, not the public internet. `kman cron serve`/`kman cron tick` need no
  inbound exposure at all (same as `kman push`), so they aren't a fourth
  listener, just another CLI command.
- A `/cron` web panel (list, and a new/edit form: name, flow dropdown,
  schedule, args as `NAME=VALUE` lines) — the design doc's own "shows up
  in the web UI's cron editor the same as one an admin typed by hand" is
  literal: `POST /cron/save` calls the identical `config.SaveCronEntry`
  a meta-endpoint-created entry goes through, no separate code path.
- Verified with `go test ./... -race` (unit tests for the schedule parser
  covering `*`, steps, ranges, comma lists and weekday matching; the token
  store's mint/validate/expire/prune behavior including that a token
  persists across separate `Store` instances backed by the same file; the
  meta server rejecting a missing/wrong-capability token and, pointedly,
  a test asserting the created entry's `flow` is the token's own even when
  the request body names a different one; `tickCron` firing a due entry
  and skipping a not-yet-due one and one whose flow has no remote repo)
  plus a full manual, no-mocks run: a real `kman meta serve` process, a
  real fake-kranq `post-receive` hook that reads `KMAN_META_TOKEN`/
  `KMAN_META_URL` out of its push options and `curl`s the meta endpoint
  exactly the way a VM-side tool call would, `kman push --meta-url ...`
  minting the token that made that callback succeed, `kman cron ls`
  showing the resulting entry correctly attributed to the acting user, and
  `kman cron tick` firing it through the same fake kranq repo — the entire
  mint → inject → callback → validate → create → tick chain observed
  working end to end.

Not built, on purpose, per docs/design.md: skills/MCP catalogs (Phase 9).

**Phase 9** — declared MCP servers and skills, catalog references:
- `flow.Access.Skills` entries now have real syntax, checked at parse
  time by `flow.ParseSkillRef`: `catalog:<name>@<ref>` (a name plus a
  pinned reference, resolved against `internal/catalog`) or
  `local:<path>` (a reference to a path the flow's own `files:` already
  declares). A malformed entry, a `local:` path with no matching `files:`
  entry, or a `local:` path whose declared mode isn't executable are all
  parse-time errors — `kman validate` catches all three before anything is
  pushed. Whether a `catalog:` name/ref actually exists is deliberately
  *not* checked at parse time, the same way a `credentials:` secret's
  actual presence in the vault isn't — both are resolved only at
  composition/push time, since the flow package itself stays decoupled
  from any specific catalog or vault content.
- `internal/catalog` — a small built-in registry (`go:embed`'d scripts,
  `Get`/`List`), **not a fetched-from-the-network registry**: "a public
  catalog entry (name + pinned ref)" from docs/design.md is delivered here
  as kman's own compiled-in set of maintained MCP servers, each one's
  `Ref` derived from a SHA-256 of its own embedded content rather than a
  hand-maintained version number — so the pin is automatically exact and
  automatically stale-checked: if the catalog's content for a name ever
  changes, every flow still referencing the old ref fails composition by
  name (`Compose` returns "the catalog's X entry is now pinned at Y, not
  Z"), never silently starts running different code. This is what "which
  skill did this flow run with must be answerable from a commit alone"
  means in practice.
- **The catalog's first, and so far only, entry is kman's own copy of
  kranq's `bitbucket-mcp.py`**, copied byte-for-byte (verified with `diff`
  against kranq's `assets/mcp/bitbucket-mcp.py` at the time of copying)
  into `internal/catalog/assets/`. This is the literal, concrete form of
  "supersedes kranq's unconditional bitbucket-mcp.py for kman-originated
  flows" — a kman-pushed flow that wants Bitbucket PR tools now declares
  `access.skills: [catalog:bitbucket@<ref>]` and gets its *own* copy
  staged via `files:` at push time, rather than depending on whatever
  kranq happened to embed at build time from a fixed, fixed-path,
  every-VM-unconditional asset. kranq's own embedded copy is untouched and
  still used by kranq's own CI callers — nothing about this phase changes
  kranq itself.
- `internal/skills.Compose(spec) (flow.Spec, error)` walks a flow's
  `access.skills`, and for each entry either (`catalog:`) resolves it
  against the catalog, verifies the ref matches exactly, stages the
  entry's content via `flow.WithFile`, and wires `mcp_servers.<name>` via
  `flow.WithMCPServer` with an explicit `command` kman controls (currently
  always `python3`, since that's what every catalog entry is written in);
  or (`local:`) wires `mcp_servers.<name>` with `command` set to the
  file's *own* path and no separate interpreter argument — kman doesn't
  know what language a flow author's own file is written in, so the
  convention is "make it executable (shebang + `0755` mode, already
  required by parse-time validation) and kman invokes it directly."
- **`flow.WithFile`/`WithMCPServer`/`WithDisallowedTool` are new,
  generically useful methods on `flow.Spec` itself**, factored out of what
  Phase 7's `slack.InjectAskRelay` used to do by hand (copy the Files
  slice, copy the Steps slice, walk claude steps, merge a map) — both
  `InjectAskRelay` and this phase's `skills.Compose` need the identical
  "stage a file, then wire an mcp server into every claude step" mechanics,
  a real shared reason to change together, so `InjectAskRelay` was
  refactored to call the same three methods instead of keeping a second,
  slightly-different copy of that logic. Both are still separate concepts
  (Slack's `access.meta: [ask]` opts into a specific behavior only Slack
  triggering can supply real values for — a channel, a thread; skills are
  generic and channel-agnostic) — only the low-level spec-mutation
  mechanics are shared, not the meta/skills dimensions themselves.
- `internal/trigger.Run` calls `skills.Compose` first, before anything
  else, so **any** push path (`kman push`, Slack, cron) gets a flow's
  declared skills composed the same way, not just one caller — same
  "generic, not bolted onto one trigger source" discipline Phase 8's meta
  token injection already established. A new `trigger.StageSkills` joins
  the existing stage set, mapped to `exitcode.InvalidSpec` in the CLI,
  same bucket as a bad arg or an ungranted credential.
- **`kman render` now composes skills before projecting to kranq shape**,
  which it didn't for Phase 7's ask relay (that composition needs a live
  Slack channel/thread, which `kman render` has no way to supply, so it
  genuinely can't be previewed statically). Skills composition needs no
  such runtime context, so leaving `kman render`'s output silently
  incomplete relative to what `kman push` actually sends would have been
  a real, avoidable gap — a user checking "what does kranq actually see"
  now gets an accurate answer for the skills dimension. This also means
  `kman render` fails loudly (exit 65) on a stale/unknown catalog ref,
  the same way `kman push` would, catching it before anything is pushed.
- A `/skills` web page listing the catalog (name, description, a
  copy-pasteable `catalog:<name>@<ref>` reference) and `kman skills ls`
  for the CLI-only path — read-only in both cases, since the catalog is
  compiled into the binary, not config-repo content; nothing to save.
  Deliberately not a structured picker wired into flow editing itself:
  `flow_edit.html`'s textarea *is* the flow's YAML, unlike the
  user/group/cron forms, and turning that into a structured editor was
  judged out of scope for what this phase asked for (a reference list, a
  link from the flow editor to it).
- `access.flows` (cross-flow access) and `access.tools` remain declared
  and referential-integrity-checked only, **not** further enforced by this
  phase, despite docs/design.md's "this is the phase where all five access
  dimensions are wired end to end for the first time." No existing
  mechanism in kman causes one flow to trigger or reference another at
  runtime (Phase 8's meta capability is deliberately self-referential
  only — a flow can reschedule itself, never a different one), so
  "enforcing" `access.flows` would mean inventing a new cross-flow
  triggering capability the design doc never actually specified the shape
  of. Building that on a guess risked designing something wrong; it's
  flagged here as a real, known gap instead — see "Left to do" below.
- Verified with `go test ./... -race`: `ParseSkillRef` and the new
  parse-time validations (malformed ref, unmatched `local:` path,
  non-executable `local:` mode); `flow.WithFile`'s path-based
  idempotency, `WithMCPServer`'s claude-steps-only scoping, and that
  neither mutates the receiver; the catalog's ref being genuinely
  content-derived and stable across calls; `Compose` adding the right
  file/mcp_servers for a catalog entry, rejecting an unknown name and a
  stale ref, and wiring a `local:` entry by its own path with no file
  added; a trigger-level test that captures the *actual* task file content
  a real `git push` sent to a fake kranq repo and confirms the composed
  skill's path and `mcp_servers` block are really in it (not just that
  `Compose` says so in isolation); plus a full manual run with the real
  binary — `kman skills ls`, a flow referencing the real emitted ref,
  `kman render` showing the composed YAML with the actual embedded
  `bitbucket-mcp.py` content (spot-checked as valid Python via
  `python3 -m py_compile`), `kman validate` rejecting a stale ref and a
  non-executable `local:` file mode, and a real `kman push` against a
  fake kranq repo succeeding with the composed spec.

**Phase 9 addendum** — pure-text skills, not just MCP servers. Raised
directly by the maintainer right after Phase 9 landed: "sometimes just a
skill md file is fine and we don't need a python script at all… I even
think bitbucket management could simply be a skill where we describe how
to use bitbucket." The original Phase 9 design conflated "skill" with
"MCP server" — every catalog/`local:` entry got wired into `mcp_servers:`
whether or not that made sense. Fixed:
- `catalog.Entry` gained `Kind` (`KindMCP` / `KindDoc`). A `KindDoc` entry
  is staged via `flow.WithFile` only — no `mcp_servers:` entry, nothing to
  execute. **The catalog's `bitbucket` entry was rewritten from a Python
  MCP server to a plain `SKILL.md`** (`internal/catalog/assets/
  bitbucket.md`, with the real Claude Code skill frontmatter shape: a
  `name`/`description` header, then markdown instructions for using
  Bitbucket's REST API via `curl` and `$BITBUCKET_TOKEN`) — replacing the
  MCP entry rather than adding a second one alongside it, at the
  maintainer's explicit direction. `internal/catalog/assets/
  bitbucket-mcp.py` was deleted, not kept dormant.
- `internal/skills.Compose` decides per `local:` ref by extension: a path
  ending `.md` is treated as a doc, composed with no `mcp_servers:` wiring
  at all (the flow author's own `files:` entry is left exactly as they
  wrote it); anything else is still treated as an executable, wired by its
  own path exactly as before. `flow.Spec.validateSkillRef`'s
  executable-mode requirement for `local:` refs is skipped for `.md`
  paths for the same reason — a doc has no business needing `chmod +x`.
- **`flow.File` gained a `Text` field, sibling to `Content`, exactly one
  of the two required.** This was the maintainer's second point in the
  same message: hand-writing a `SKILL.md` into a flow's own `files:`
  block shouldn't require pre-encoding it to base64 by hand. `Text` is
  the file's literal UTF-8 content (natural in YAML's block-scalar
  syntax, `text: |`); `flow.File.Base64Content()` returns `Content`
  as-is or encodes `Text` on demand, and `Render` calls it once, at the
  very last step, into a small `kranqFile{Path,Mode,Content}` wire type —
  **kranq's own task schema is completely unchanged**, it never sees
  anything but base64 `content:`, same as always. This authoring
  convenience is not skill-specific: any `files:` entry can use `text:`
  instead of `content:` now, e.g. a plain config file a task needs.
- `internal/skills.Compose` gained a `Lookup` parameter
  (`func(name string) (catalog.Entry, bool)`, satisfied directly by
  `catalog.Get` at every real call site) instead of calling the global
  catalog directly. This was needed for testing, not for production
  behavior: once the catalog's only entry became `KindDoc`, there was no
  real catalog entry left to exercise `Compose`'s `KindMCP` branch, and
  the catalog's own registry (`entries`) is an unexported package var no
  other package's tests can seed. `internal/skills`'s tests now construct
  a synthetic in-memory catalog to prove the MCP-kind path still works,
  without touching global state or needing a real MCP catalog entry to
  exist just to keep it covered.
- Verified with `go test ./... -race` across `flow`/`catalog`/`skills`/
  `trigger`/`cli` (all previously-passing tests that assumed bitbucket was
  MCP were updated to assert the new doc behavior, not skipped or
  deleted), plus a manual run with the real binary: `kman render` on the
  bitbucket entry decoding, byte for byte, back to the exact markdown
  source; and a from-scratch flow whose own `files:` entry uses `text:`
  directly (no base64 anywhere in the authored YAML) validating,
  rendering with correctly-encoded `content:`, and pushing successfully
  through a real fake-kranq repo.

## Left to do

Everything from Phase 10 onward in [docs/design.md](design.md):
distribution past the Homebrew stub, and the written note on further
kranq-side cleanup.

Worth flagging about Phase 9, before it's forgotten:
- **`access.flows` and `access.tools` are still declared-only.** See the
  bullet above — there's no cross-flow triggering mechanism in kman for
  `access.flows` to gate, and `access.tools` has had no consumer since
  Phase 3 defined it. Both are validated for referential shape
  (`access.flows` entries must name a real flow) but neither restricts
  anything live. Worth a real decision, not a guess, before either is
  built.
- **The catalog has exactly one entry, and it's `KindDoc`, not
  `KindMCP`.** The mechanism (`internal/catalog`, `internal/skills.
  Compose`) is generic and proven for both kinds, but `KindMCP`'s
  composition path is currently exercised only by `internal/skills`'
  own unit tests against a synthetic in-memory catalog entry (see the
  Phase 9 addendum above) and by a `local:` executable skill ref, never
  by a real, permanent catalog entry. Adding either kind of second entry
  is cheap (embed the file, `register` it with the right `Kind`) whenever
  a real need names one.

Worth flagging about Phase 8, before it's forgotten:
- **Guest→host reachability from a real kranq VM is still unconfirmed.**
  docs/design.md flagged this explicitly as needing a probe before the
  phase was finalized: whether a kranq-booted VM (per `assets/lima.yaml`)
  can actually reach a host address like `host.lima.internal:8082` under
  Lima's `vz` networking. Nothing in this session can run a real Lima VM
  to check. Everything meta-related is proven up to the boundary of "a
  process with the right token calls the endpoint" — the fake-kranq hook
  in the manual smoke test stands in for that call exactly the way a real
  VM's tool invocation would, but it isn't one. `--meta-url`/
  `$KMAN_META_URL` is deliberately just a plain configured address (no
  Lima-specific detection) precisely so the fallback docs/design.md named
  — plain internet/LAN egress to a kman endpoint on its own address, the
  same posture kranq's own git server already has — works without any
  code change once this is confirmed either way.
- **The meta token store has no locking.** `Store.Mint`/`Validate` do a
  plain read-modify-write of `meta-tokens.json`, so two concurrent mints
  (or a mint racing a validate's prune) could lose a write. Same
  known-and-accepted limitation `internal/vault` already has, for the same
  reason: a single-operator tool doesn't need a `flock` for this yet.
- **`kman cron serve`'s interval is real-time, not simulated**, so nothing
  exercises what happens across a restart that spans a missed tick, or two
  `kman cron serve` processes pointed at the same `$KMAN_HOME` racing each
  other. The design's "a missed tick is not retroactively retried" is true
  by construction (each tick only ever checks "is *now* due"), but running
  two schedulers against the same config has the same undocumented-race
  character as Phase 4's "two `kman web` processes" caveat below — not
  how this is meant to be run, but nothing currently stops it.

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
