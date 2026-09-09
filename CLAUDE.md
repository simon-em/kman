# kman

Manages Slack/human-triggered access to kranq (a separate repo at
`../kranq`): credentials, flows, groups, integrations, a cron scheduler, and
a web UI.

**The full design and phasing lives at** [docs/design.md](docs/design.md).
Read it before making architectural decisions; it records what was decided
and why.

## Where this is now

Phases 0-9 (Phase 5 landed and shipped in the kranq repo, not here). See
[docs/status.md](docs/status.md) for what's done and
[docs/design.md](docs/design.md) for the full phase-by-phase plan.

```sh
go test ./...
go build -o /tmp/kman .
```

## Non-obvious facts

**kman reimplements kranq's push wire format rather than importing it.**
`internal/kranqpush` re-derives the push option names, the synthetic
`refs/heads/task/<ts>-<nonce>` ref, and the `KRANQ-RESULT id=... status=...
exit=... result=...` line kranq's post-receive hook prints — by reading
kranq's `internal/cli/push.go` and `internal/gitsrv/options.go` and copying
the wire format, not the code (kman cannot import kranq's Go packages; see
the process-boundary fact below). A drift between kranq's real format and
kman's copy of it is caught only by `internal/kranqpush`'s tests against a
stub hook, never by the compiler — if kranq's push protocol changes, this
package needs a matching update, and there's no automated signal that it
does.

**`kman push`'s mirror-ref push-ahead (`kranqpush.UpdateMirror`) is
confirmed non-functional against a real kranq host, and has been since it
was written.** The idea was to best-effort-push `refs/heads/mirror/<slug>`
ahead of every task push so a `mirror/` ref (outside kranq's
`task/*`/`ok/*` sweep pattern) persists as a durable object anchor,
shrinking every later push to a delta. In practice, kranq's real
`pre-receive` hook rejects *any* push that carries no `task`/`task_file`/
`spec` option — including a plain ref update with no task at all — so
`UpdateMirror` fails every single time with "no task: push with -o
task_file=... or -o task=...". This was always caught (best-effort,
logged as `kman: warning:`, never fatal) and never blocked a real run, but
the optimization itself has never once succeeded outside a synthetic test
hook that doesn't enforce kranq's real pre-receive rule. Worth either a
kranq-side exception for a plain ref update with no task, or dropping the
mechanism from kman.

**`kman push --source` defaults to `.`, on purpose, matching kranq's own
`kranq push`.** kranq's own client just pushes whatever `HEAD` is in the
directory you run it from ("wherever you're standing") — no separate
clone step, because a CI pipeline has already checked the code out for
you. kman does the same by default. The `internal/gitcache` mirror only
comes into play when `--source` is given a remote URL instead of a local
path (detected by `://` or `@` in the string) — that's for a caller with no
local checkout yet, which Phase 0/1 doesn't exercise but Phase 7's
Slack-triggered blank VM will.

**kman cannot import any kranq Go package.** kranq's task schema lives under
`internal/task`, which Go's own visibility rules make unimportable outside
`github.com/simon-em/kranq`. The only interface between the two is a YAML
document plus `git push`/`git pull`/`git fetch`. `internal/flow` in this repo
independently re-declares the fields of kranq's `task.Spec`/`Step`/
`MCPServer` it needs to emit, by name, as data — not by type-sharing. This is
a process/code-coupling boundary, not a schema freeze: see the next fact.

**kranq's task YAML schema is not frozen.** It's a joint, evolving contract.
The one thing worth changing in kranq itself is how a task gets access to
tools (MCP servers) — see docs/design.md Phase 5. Nothing else about kranq
needs to change, and there's no requirement to preserve backward
compatibility for its own sake.

**kman never talks to `limactl` and never manages a VM directly.** Starting
a VM is entirely kranq's job, reached only through `git push kranq -o
spec=<gzip+base64> -o env.NAME=VALUE`. There is no "blank VM" — confirmed
against a real kranq host (v0.2.0-21-gbff29da): `internal/project.Load`
requires a `Kranqfile` to exist in the pushed checkout at all, and
`internal/project` (`parse`) then requires it to contain at least one `RUN`
or `COPY` ("builds nothing" otherwise) — docs/design.md's Phase 1 assumption
that a flow with no Kranqfile gets kranq's bare base layer was never true of
the real binary. Every flow's source repo needs a real Kranqfile with at
least one instruction; `create-feature`'s demo repo
(`smntlbt/kman-demo`) carries a minimal `RUN true`.

**kman shells out to `git` for everything — the host-side cache, the config
repo, and the push to kranq. Never a Go git library.** Same reasoning kranq
itself used for `limactl`/`git`/`ssh`: an external tool's edge cases are
someone else's already-solved problem.

**kman is a credential vault only for the human/Slack-triggered path.**
CI-pipeline-triggered kranq runs (Bitbucket Pipelines pushing to kranq
today) are untouched — they keep forwarding env directly, kman is never in
that path. This is the specific case kranq's own design doc named and
declined to build ("a human triggering something from a browser, with no
env to send... decided on its own merits"), not a reversal of kranq's "not
a credential vault" position.

**Secrets live encrypted at rest under `$KMAN_HOME/secrets/vault.enc`,
keyed by a 32-byte machine key at `$KMAN_HOME/vault.key` (0600).**
`internal/vault` is AES-256-GCM via the Go standard library, not the real
`age` file format — docs/design.md's "age-style" described the shape (a
local, file-based, machine-keyed store), not a literal dependency on the
`age` tool. Only names, never values, go into the committed config repo —
unlike kranq's own single-operator, plain `0600 $KRANQ_HOME/env`, because
kman holds *other people's* long-lived credentials. The access model
(Phase 3) exists since Phase 3, but sat unconsulted by any live caller
until Phase 7: `Registry.CanTrigger` is now checked by the Slack handler
before a flow is pushed. Nothing else calls it yet — `kman push` from the
CLI still doesn't check whether `--as` is actually allowed to trigger the
flow it's pushing, only Slack-triggered pushes are gated. Per-user
Bitbucket OAuth tokens (Phase 6) live in this same vault, namespaced
`bitbucket/oauth/<user-id>/...` — no second secrets store for
integration-minted credentials.

**A flow's `credentials:` block is resolved at push time and merged into
the same env as `args:`, with a hard error on collision.** `kman push`
refuses to push if an env var name is claimed by both an arg and a
credential — that's a flow-authoring mistake to catch, not something to
silently resolve one way. Like `args:`, `credentials:` is never rendered
into the kranq task-spec YAML; kranq has no concept of either.

**The web UI is one more interface onto the config repo, never a separate
store.** `kman web` and the CLI both call the same `internal/config`
single-entity functions (`LoadUser`/`SaveUser`, `LoadGroup`/`SaveGroup`,
`LoadFlow`/`SaveFlow`/`ReadFlowRaw`, `ListFlowNames`/`ListUserIDs`/
`ListGroupNames`) — one storage format, two front ends, exactly per
docs/design.md. `Save*` commits automatically when `$KMAN_HOME/config` is
a git repo (checked via `.git`'s presence, not via whether a remote is
configured) and is a plain, uncommitted file write otherwise — both are
real supported modes, not one being a degraded fallback of the other.
Concurrent-edit conflict handling (design.md's "non-fast-forward is
rejected and retried") is *not* built: `internal/config`'s commit is local
only, nothing pushes or pulls, so two `kman web` processes against the
same directory can still race on the same file. That only becomes a real
problem once more than one person points a browser at the same
`$KMAN_HOME`, which isn't how this is used yet.

**The flow schema grows one section only in the phase that enforces it.**
No inert, unimplemented fields — see docs/design.md's phasing notes for why.

**A flow's `credentials:` binding and its `access.credentials` grant are two
different things, cross-validated at parse time.** `credentials:` (Phase 2)
says which env var a secret fills in; `access.credentials` (Phase 3) is the
grant that says the flow may use that secret at all. Every secret name
bound via `credentials:` must also appear in `access.credentials`, checked
in `flow.Spec.validate()` — but nothing outside `flow.Parse` enforces this
against a live caller yet. `internal/flow/access.go` holds the whole
`access:` struct (tools/meta/skills/credentials/flows), one file, since it
grows together with the config-repo referential-integrity checks in
`internal/config`.

**`config.Load` fails the whole config if a grant or membership points at
nothing real.** A user's or group's `flows:` grant must name a flow that
exists; a group's `members:` must name a user that exists; a flow's own
`access.flows` must name flows that exist. This is enforced on every load
(`internal/config/local.go`), not just at write time — so a config repo
that got corrupted by a hand-edit, not just a `kman grant` call, is caught
just as loudly. `kman grant`/`group add-member` re-check the same rules
before writing, so the common path never produces a config that would fail
to load.

**`kman grant`/`kman revoke`/`kman user set`/`kman group set` commit
automatically now, same as the web UI — this superseded what Phase 3 first
shipped.** Phase 3 had them write files directly with no commit step, on
the reasoning that auto-commit was "a web UI concern." Phase 4 built the
shared `config.Save*` path the design doc actually called for, and once it
existed there was no reason for the CLI not to use it too — so it does.
None of them push; that's still left to the operator (or a future phase).
Attribution on the CLI side is `internal/cli/access.go`'s `actor()`, which
just reads `$KMAN_ACTOR` — there's still no CLI flag for it on those
commands specifically, only on `kman push` (`--as`, added in Phase 6, for a
different reason: see the integration-credential fact below). The web UI's
per-request "Acting as" cookie has no CLI equivalent either way, since a
CLI invocation has no concept of "this session."

**`flag.FlagSet.Parse` stops at the first non-flag argument.** Every kman
command that takes a flag after a required positional argument (`kman push
flow.yaml --kranq-url X`, `kman user set simon --slack-id X`) goes through
`parsePermuted` (`internal/cli/push.go`) instead of calling `fs.Parse`
directly, or the flag is silently dropped with no error — a real bug this
found twice already while wiring up new commands, worth checking for
before adding a ninth.

**`kman web`'s "Acting as" field is attribution, never authorization.** It
sets a `kman_actor` cookie used only to name the git commit's author; there
is no session, password, or per-request identity check anywhere in
`internal/web`. Anyone who can reach the port can read and write anything —
`kman web` binds `127.0.0.1` by default specifically because of this, per
docs/design.md's explicit call that Phase 4 would ship with no login
system, not a placeholder for one.

**A flow/user/group saved under a new name creates a second file; it does
not rename the old one.** `internal/web`'s edit forms don't diff the
current name against what's being saved, so renaming via the UI (or via
`kman grant`'s target files) leaves an orphaned file behind under the old
name. Known, not yet built — see docs/status.md's Phase 4 entry.

**A `credentials:` secret name prefixed `integration:<name>` routes through
that integration's `CredentialProvider` instead of the vault.** Everything
else about it is unchanged from Phase 2/3: it still has to appear in
`access.credentials`, it still binds to an env var name, it's still never
rendered into kranq's YAML. Only `resolveCredentials`
(`internal/cli/push.go`) branches on the prefix. This is why `kman push`
gained `--as <user-id>` (default `$KMAN_ACTOR`) — minting "the" Bitbucket
token for a push requires knowing whose, something no kman command needed
to know before this phase. A push using an `integration:` credential with
no acting user known fails immediately, by name, rather than guessing.

**`internal/integration`'s `TriggerChannel` is still an empty marker
interface, even now that Slack (a real trigger source) exists.** `slack.
Provider` implements it trivially (it already has `Name()`), but nothing
calls through the interface — the Slack handler is concrete, calling
`slack.Provider`/`slack.Handler` directly, not driven through
`TriggerChannel` dispatch. There's still only one implementation; widening
the interface to something a second one could plug into is a decision to
make when there's a second one, not before.

**Bitbucket OAuth has now been proven against real bitbucket.org, not just
a fake server.** The maintainer supplied a real OAuth consumer (workspace
`smntlbt`) and clicked through the real consent screen once. That
confirmed, live, unmodified: `OAuth.AuthorizeURL`'s output is byte-for-byte
what Bitbucket's own "Authorization URL" field shows; the full
connect → redirect → real login/approve → callback → `Exchange` → vault
storage loop; and `Provider.Credential` handing back a real per-user token
that authenticated a real `GET /2.0/user` call. One real snag, since
fixed: the OAuth consumer's registered Callback URL has to be
`http://localhost:8080/integrations/bitbucket/callback`, kman's actual
route, not a bare `/callback` — Bitbucket redirects wherever the app is
configured to, regardless of what kman listens on, so a mismatched
consumer config 404s after a real login, not a kman bug. `bitbucket/oauth/
base_url` (`internal/integration/bitbucket/provider.go`) still exists for
Bitbucket Server/Data Center and as a test-server override; it just also
turned out to matter for nothing in the real-bitbucket.org path, since
that path uses the default. `Refresh` is still only proven against the
fake server (the real refresh_token has a multi-hour lifetime, nothing in
this session waited that out).

**`integration:bitbucket:<scope>` mints a genuinely restricted,
per-push token via Bitbucket's `client_credentials` grant, never the
per-user OAuth path.** Confirmed against the real API: Bitbucket's token
endpoint accepts a `scope` param that narrows the returned token below
whatever the OAuth consumer itself is configured with, and *enforces* it
server-side on every later call, not just at mint time, a
`pullrequest`-scoped token gets a hard `403` (with `required`/`granted`
spelled out in the error body) trying anything outside that scope. This
is a different grant type from the `integration:bitbucket` (no scope)
form Phase 6 built: `client_credentials` has no human attached (Bitbucket
shows the actor as "Oauth client", not a person), never touches the
vault's per-user token storage, and is never cached, `Resolve
IntegrationCredential` calls `Provider.ScopedCredential` fresh on every
resolution rather than `Provider.Credential`'s load-or-refresh-and-cache
path. `--as`/`$KMAN_ACTOR` is **not** required for this form (there's no
user to attribute a self-contained app-level token to) — only the
unscoped, per-user form still requires it. The scope string can itself
contain colons (`integration:bitbucket:repository:write`);
`ResolveIntegrationCredential` splits on the *first* colon only
(`strings.Cut`), not the last, so this doesn't need special-casing.
kman deliberately doesn't validate scope names client-side — Bitbucket's
own `400 invalid_scope` is the source of truth, since hardcoding
Bitbucket's scope vocabulary into kman would drift the moment Bitbucket
changes it, the same reasoning that already applies to plain vault
secret names never being validated for existence until push time.

**`internal/trigger` exists because `kman push` and the Slack handler need
byte-identical arg/credential resolution and push semantics.** It used to
be one function, `cli.runPush`. Splitting it out only when a second real
caller (Slack) showed up, rather than pre-factoring it in Phase 1, is the
same "extract duplication when the two copies share a reason to change"
discipline the maintainer's global instructions call for — there was
nothing to extract until there were two callers. `trigger.Run` returns a
`*trigger.Error{Stage, Err}` specifically so the CLI can still choose a
POSIX exit code per failure stage even though the function itself is
caller-agnostic.

**A Slack-triggered push routes the bot token through `Options.ExtraEnv`,
not through `mcp_servers.kman-ask.env`.** docs/design.md's own Phase 7 text
lists only `KMAN_RUN_ID`/`KMAN_FLOW_ID`/`KMAN_SLACK_CHANNEL` in that env
block, deliberately omitting the token — putting it there would bake it
into the rendered task YAML itself (which travels as one `-o spec=`
push option and ends up in kranq's own task record). `ExtraEnv` instead
flows through the same push-option env path a resolved `credentials:`
value does, so it only ever exists as a shell-exported variable, inherited
by the MCP server subprocess like any other child process of the step.

**`internal/kranqpush` sends the rendered task inline via `-o
spec=<gzip+base64>`, matching kranq's real `gitsrv.EncodeSpec` wire format
— confirmed against a real kranq host, not assumed.** An earlier version
used `-o task_file=<host path>`, which is for a task file *already inside
the pushed commit*; kranq's real `pre-receive` hook rejected it outright
("... is not in the pushed commit") the first time this was tried against
a real daemon instead of a synthetic test hook. kman cannot import
kranq's `gitsrv` package (see the process-boundary fact above), so
`kranqpush.encodeSpec` is a second, independent implementation of the
same small gzip+base64 encoding — a drift in kranq's real format still
needs a matching update here, same as the rest of `internal/kranqpush`.

**`kman slack serve` is a separate command and a separate listener from
`kman web`, not a route mounted on it.** `kman web` binds `127.0.0.1` by
default specifically because it has no login (a documented Phase 4
tradeoff); Slack's Events API needs a URL it can reach from the outside.
Keeping them as two processes/ports means the unauthenticated admin UI
never has to be exposed for Slack to work — the Slack endpoint has its own
real authentication (`slack.VerifySignature`, Slack's HMAC scheme with a
5-minute replay window) and does nothing else.

**The AskUserQuestion relay (`internal/integration/slack/assets/
kman-ask.py`) is opt-in per flow, via `access.meta: [ask]`, checked with
`flow.Access.HasMeta`.** Nothing injects it unless a flow asks for it —
`InjectAskRelay` operates on an in-memory copy of the `flow.Spec` inside
the Slack handler at push time (never mutates the loaded spec, so a second
push of the same flow starts from the same base) and only touches
`claude:` steps, since `mcp_servers` is invalid on a `run:` step anyway.

**`kman-ask.py` mirrors kranq's own `assets/mcp/bitbucket-mcp.py` shape on
purpose** (the same `initialize`/`tools/list`/`tools/call` stdio JSON-RPC
loop, `urllib`-only, no dependencies) rather than inventing a second MCP
server pattern — it's proven directly (`python3 kman-ask.py` fed synthetic
stdin, both the handshake and a `tools/call` error path), but has never run
for real inside a kranq VM with a real Claude Code process talking to it
over stdio; that needs a real kranq host and a real Slack app at the same
time, neither of which exists in this environment yet.

**A minted meta token's `flow` is fixed at mint time and cannot be
overridden by the caller.** `meta.Server.createCron` builds the saved
`cron.Entry{Flow: tok.FlowName, ...}` from the *token*, not from anything
in the request body — a request body naming a different flow is silently
ignored for that field. This is what "the self-referential capability"
(docs/design.md) actually means in code: a flow can schedule itself, never
an arbitrary other flow, no matter what a compromised or buggy VM-side
caller sends.

**Meta tokens are file-backed (`$KMAN_HOME/meta-tokens.json`, 0600), not
in-memory.** `kman push` (a one-shot CLI process) mints the token; `kman
meta serve` (a separate long-running process) validates it later. An
in-memory map in either process wouldn't be visible to the other. Same
tradeoff `internal/vault` already made for secrets, extended to a second
thing: no locking beyond plain read-modify-write, acceptable for a
single-operator tool, not safe under real concurrent mints.

**`internal/trigger.Run` mints and injects `KMAN_META_TOKEN`/
`KMAN_META_URL` itself, generically, whenever `spec.Access.Meta` is
non-empty** — unlike the Slack-specific ask relay, meta access has nothing
to do with Slack, so it belongs in the shared push path both `kman push`
and every trigger source go through, not bolted onto one caller. Missing
`--meta-url`/`$KMAN_META_URL` is a loud stderr warning and the push still
proceeds (a flow can declare `access.meta` before an operator has wired
cron up); missing `--as`/`$KMAN_ACTOR` on a flow that *does* have a meta
URL configured is a hard `trigger.StageMeta` error — there's no one to
attribute a minted token to otherwise.

**`kman meta serve` is a third separate listener, alongside `kman web` and
`kman slack serve`, for the identical reason those two are split**: each
needs to be reachable from somewhere the others must not be. `kman web`
stays loopback-only (no login); `kman slack serve` needs the public
internet; `kman meta serve` needs to be reachable from a kranq-booted VM,
which — per docs/design.md's own caveat — is unconfirmed to even be
possible under Lima's networking in this environment. `kman cron
serve`/`kman cron tick` need no inbound exposure at all, so cron stays a
plain CLI command, not a fourth listener.

**`kman cron tick` and `kman cron serve` share one function
(`tickCron`), not two implementations of the same loop.** `serve` is
`for { tickCron(...); sleep(interval) }`; `tick` calls it once and exits.
The one-shot form exists specifically so the core logic is testable
without a long-running process, and so an operator can drive kman's cron
off real OS cron/launchd instead of running a kman daemon, if they'd
rather — matching kranq's own general preference for "an external tool's
scheduling is someone else's already-solved problem" where it reasonably
applies.

**`internal/cron`'s schedule matcher ANDs day-of-month and day-of-week,
unlike real POSIX cron's documented OR special-case.** A hand-rolled
5-field parser (`*`, `*/N`, `A-B`, comma lists) was worth writing to avoid
a dependency; matching cron's specific DOM/DOW-OR quirk exactly was judged
not worth the complexity for what this is used for. Known, simple,
documented — not a bug to "fix" without deciding it's worth the added
complexity first.

**A `catalog:` skill's ref is a content hash, not a hand-maintained version
number.** `internal/catalog.register` sets `Entry.Ref` from
`sha256(Entry.Content)[:12]` at init time — a flow's `access.skills:
[catalog:bitbucket@<ref>]` only composes if that ref exactly matches the
catalog's *current* hash for that name. This is what makes "a public
catalog entry is a name plus a pinned ref, never latest" (docs/design.md)
literally true rather than aspirational: there's no way to write a flow
that silently starts running different code because the catalog changed
underneath it — `internal/skills.Compose` returns a named error instead.

**kman's skills catalog is its own compiled-in registry, not a fetched
public one, despite docs/design.md's "public catalog" phrasing.** There's
no network fetch, no external registry service — `internal/catalog`'s
entries are `go:embed`'d into the binary. This is the concrete shape of
"supersedes kranq's unconditional bitbucket-mcp.py for kman-originated
flows": a kman-pushed flow gets kman's *own* skill staged via `files:`,
not a dependency on whatever kranq happened to embed at build time.
kranq's own embedded `assets/mcp/bitbucket-mcp.py` is untouched and still
serves kranq's own CI callers.

**`catalog.Entry` has a `Kind`: `KindMCP` (a real process, wired into
`mcp_servers:`) or `KindDoc` (plain text Claude reads and acts on, no
process at all).** The catalog's `bitbucket` entry started as `KindMCP`
(a Python script mirroring kranq's own `bitbucket-mcp.py`) and was
rewritten to `KindDoc` (`internal/catalog/assets/bitbucket.md`, a real
`SKILL.md`) at the maintainer's explicit direction, replacing the MCP
entry rather than keeping both — not every tool needs a structured
tool-call interface; Bitbucket's REST API is simple enough that
instructions plus `curl` are enough. `internal/skills.Compose` only adds
an `mcp_servers:` entry for `KindMCP`; `KindDoc` is staged via
`flow.WithFile` and nothing else. The same kind split applies to
`local:` refs, decided by the maintainer's own suggestion: a path ending
`.md` is treated as `KindDoc` (no mode requirement, no MCP wiring),
anything else is treated as an executable (must be `0755`-style,
`command` set to its own path).

**`bitbucket.md`'s "get the diff" curl command needs `-L`.** Found by
running the skill's own documented commands against the real API
(workspace `smntlbt`): `GET .../pullrequests/{id}/diff` returns an HTTP
302 to the actual diff content, and `curl` without `-L` silently returns
an empty body instead of erroring — nothing about it looks like a
failure. The original Python `bitbucket-mcp.py` never hit this because
`urllib.request.urlopen` follows redirects by default; writing the same
call as raw `curl` in a markdown skill surfaced a real gap a fake test
server never would have, since the fake server had no reason to redirect
anything. Every other endpoint in the skill (list PRs, get one PR, list
comments, add a comment) needed no such fix, confirmed live.

**`internal/skills.Compose` takes a `Lookup` function parameter
(`func(name string) (catalog.Entry, bool)`) instead of calling
`catalog.Get` directly.** Every real caller still just passes
`catalog.Get` (its signature matches exactly, no wrapper needed) — this
exists purely for testability. Once the catalog's only real entry became
`KindDoc`, there was no permanent `KindMCP` entry left to exercise
`Compose`'s MCP branch through, and `catalog.entries` is an unexported
package var no other package's tests can seed. `internal/skills`'s own
tests build a synthetic in-memory catalog instead of mutating global
state or requiring a real MCP entry to exist just to stay covered.

**`flow.File` has a `Text` field, sibling to `Content`, exactly one of the
two required.** `Content` stays base64 (kranq's own wire format never
changed and never needs to); `Text` is literal UTF-8, meant for exactly
what it looks like in YAML: `text: |` followed by real markdown, no
pre-encoding step a human has to run first. `File.Base64Content()` is the
single place that reconciles the two — returns `Content` as-is or encodes
`Text` on the way to kranq — called once, at the end of `Render`, into a
small kranq-shaped `kranqFile{Path,Mode,Content}` wire type distinct from
`flow.File` itself (they used to be the same type; they diverged the
moment kman's own authoring convenience and kranq's wire format needed to
say different things). Not skill-specific: any `files:` entry can use
`text:` instead of `content:`.

**`flow.WithFile`/`WithMCPServer`/`WithDisallowedTool` exist because
`slack.InjectAskRelay` (Phase 7) and `skills.Compose` (Phase 9) both need
identical spec-mutation mechanics** — stage a file idempotently by path,
wire an mcp server into every `claude:` step, append a disallowed tool
without duplicating it. `InjectAskRelay` was refactored to call these
instead of keeping its own copy once a second real caller showed up, the
same "extract only once there's a second copy with a real reason to
change together" discipline `internal/trigger` was split out under in
Phase 7. The meta (Slack-specific, needs a live channel/thread) and skills
(generic, channel-agnostic) dimensions themselves stay conceptually
separate — only the low-level mutation helpers are shared.

**`kman render` composes `access.skills` but not `access.meta`'s ask
relay, and that asymmetry is intentional, not a gap.** Skills composition
needs no runtime context, so `kman render`/`kman push` can and do preview
it identically; the ask relay needs a real Slack channel and thread `kman
render` has no way to supply, so it's composed only inside the Slack
handler at actual trigger time. Before this phase, `kman render` didn't
compose *anything* (Phase 7 shipped with this gap for the ask relay only,
which was unavoidable); Phase 9 closed it for the one dimension where
closing it was actually possible.

**`access.flows` and `access.tools` are still declared-and-validated-only,
despite docs/design.md's Phase 9 text calling it "the phase where all five
access dimensions are wired end to end for the first time."** Nothing in
kman causes one flow to trigger or reference another at runtime — Phase
8's meta capability is deliberately self-referential only (a flow can
reschedule *itself*, never a different one) — so there was no existing
mechanism for `access.flows` to gate, and inventing one on a guess risked
building the wrong shape. Left as a named, tracked gap (docs/status.md)
rather than guessed at.

**The Slack trigger path (`@mention` → push → threaded reply) has now
been proven against a real Slack workspace, not just a fake
`slack.com/api` server.** A real app (bot token, signing secret,
`app_mentions:read`/`chat:write`/`channels:history` scopes), a real
`kman slack serve` exposed through an `ngrok` tunnel so Slack's own
servers could reach it, a real signed `url_verification` challenge, and
a real `@Kman run hello` mention in an actual channel, all confirmed
working unmodified: signature check, event parse, Slack-ID-to-`User`
resolution, `CanTrigger`, command parse, a real push, two threaded
replies. No drift found between Slack's real Events API payload shape
and `ParseEnvelope`/`Event`. The AskUserQuestion relay specifically is
still unproven inside a real kranq VM (needs a real kranq host and a
real Slack app at the same time, which this session still doesn't have
both of at once).

**A Slack mention that isn't `run <flow>` falls back to a configurable
default flow, its whole text becoming that flow's `TASK` argument.**
`slack.ParseCommand(text, defaultFlow)` — `kman slack serve
--default-flow` (env `$KMAN_DEFAULT_FLOW`, defaults to `create-feature`)
sets it; empty disables the fallback and restores the old
`run`-only behavior. An explicit `run <flow>` always wins even when a
default is configured. `CanTrigger` still gates the resolved flow exactly
as before — free-form routing doesn't bypass authorization, it just
changes how the flow name is chosen. Because `args = []string{"TASK=" +
text}` reuses the *same* `trigger.ParseAssignments`/`strings.Cut`
machinery `run <flow> NAME=VALUE` already used, embedded `=` signs in the
free-form text survive intact (`strings.Cut` only ever splits on the
*first* `=`) with no new parsing path needed.

**A `claude:` step's prompt text cannot reference a shell variable like
`$TASK`, even though `TASK` really is exported in the process
environment.** kranq's `BuildScript` heredocs the prompt with a
*single-quoted* delimiter (`<<'CI_PROMPT_0'`), which disables all shell
expansion inside it, so `$TASK` in the prompt text reaches Claude
literally as the two characters `$` `T`, not the argument's value. The
correct pattern, used by the `create-feature` flow: tell Claude to read
the value itself at runtime (`` `echo "$TASK"` `` via its own Bash tool
call), which works, because *that* shell invocation is a real,
unquoted one, run by Claude after the prompt is already delivered, not
part of the heredoc kranq built.

**`internal/gitcache.Sync` takes an `authHeader` and injects it via
`-c http.extraHeader=...`, never into the remote URL itself.** Added
because `trigger.Run`'s own host-side "mirror the source repo before
pushing to kranq" step has *no* relationship to a flow's `credentials:`/
`BITBUCKET_TOKEN` (that's resolved for the VM's use, injected via push
options, never seen by kman's own git commands) — cloning a *private*
Bitbucket source repo from an unattended `kman slack serve` daemon (no
SSH agent, no stored git credentials) failed outright until this existed.
`http.extraHeader` was chosen specifically so the credential never lands
in the mirror's on-disk `.git/config`, unlike a `https://user:token@host`
URL, which git would otherwise happily persist as the `origin` remote.

**Bitbucket's git-over-HTTPS endpoint requires Basic auth
(`x-token-auth:<token>` as user:password), not a bare `Authorization:
Bearer <token>` header, even though the REST API accepts Bearer just
fine.** Found by testing directly: a Bearer header against `.git/info/
refs?service=git-upload-pack` gets a flat `401`; `-u x-token-auth:<token>`
(equivalently, `Authorization: Basic base64("x-token-auth:<token>")`)
gets a `200`. `trigger.sourceAuthHeader` builds the Basic form
specifically; getting this wrong is silent in the worst way; the header
gets sent, git just fails with `fatal: Authentication failed`.

**`trigger.sourceAuthHeader` auto-mints a `repository`-scoped
(Bitbucket's own name for read access, not `repository:read`) Bitbucket
credential whenever `Options.Source`/`spec.Repo` is a `bitbucket.org`
URL and Bitbucket is configured, with no flow YAML involved at all.**
This is deliberately separate from whatever credential a flow's own
`credentials:` block declares for *its* use inside the VM — kman's own
read of the source repo is a narrower, host-side-only concern, scoped to
exactly `repository` (read), never whatever broader scope the flow
itself might request for writing PRs. Best-effort: if Bitbucket isn't
configured, or the source isn't a Bitbucket URL, or minting fails,
`sourceAuthHeader` returns `""` and the clone proceeds unauthenticated,
exactly the pre-existing behavior for a public repo — the failure (if
any) still surfaces naturally as git's own authentication error, not a
new, separate one.

**No code comments in this repo, per the maintainer's standing instruction.**
If a construct needs a comment to be understood, it's the wrong construct —
rewrite it instead.

**`InjectAskRelay`'s staged file path must be relative, not absolute.**
kranq's `files:` staging does a plain `mkdir -p <dirname>` as the non-root
build user, no sudo — an absolute path like the original
`/kman/tools/kman-ask.py` fails outright ("Permission denied") since that
user can't create a directory at `/`. Confirmed against a real kranq VM.
`askRelayPath` is `.kman/kman-ask.py`, relative to the checkout, same as
every other staged file (the `catalog:bitbucket` skill's own
`.claude/skills/...` path).

**`kman-ask.py` form-encodes every Slack Web API call, not JSON — Slack's
own API is inconsistent about which it accepts.** `chat.postMessage`
accepts a JSON body; `conversations.replies` rejected the same shape of
well-formed request with `invalid_arguments`, confirmed twice against the
real API in a real run, before landing on `application/x-www-form-urlencoded`
as the encoding that actually works for both. Sending form data
everywhere sidesteps needing to track which method wants which.

**Guest→host reachability for `access.meta` is confirmed working under
kranq's default Lima networking, no extra config.** A real VM reached
`http://host.lima.internal:8082` (where `kman meta serve` was listening)
on the first try — `assets/lima.yaml` declares no `hostResolver`/
`hostNetworks` at all, and none was needed. `--meta-url` staying a plain
configured address rather than doing Lima-specific detection was the
right call.

**Proven end to end against a real kranq host, real Lima VM, real Claude
Code, real Bitbucket PR** — `smntlbt/kman-demo` pull request #1. kranq's
git-over-HTTP endpoint is off by default (`KRANQ_HTTP_ADDR` unset); to
point kman at a real local kranq for testing: `kranq config set
KRANQ_HTTP_ADDR=127.0.0.1:8420`, restart the daemon, `kranq token create
<name>` for a push credential (shown once, never stored/re-shown), then a
`KranqURL` of `http://<name>:<token>@127.0.0.1:8420/git/<repo-name>`
(`KRANQ_AUTO_CREATE_REPOS` defaults to true, so `<repo-name>` need not
exist yet — it's unrelated to `spec.Repo`, which is the *source* clone
URL). This surfaced the two real bugs described above (`-o spec=` and
the Kranqfile requirement); nothing about routing, credential minting,
or Slack event handling needed a further change once those two were
fixed.

**A flow's `repo:` is a default, not a fixed target — `internal/reporegistry`
plus `trigger.ResolveRepoRef` let it be overridden per push.** A `REPO=`
argument (explicit `run <flow> REPO=name`, or dispatch-resolved) is
resolved via `ResolveRepoRef`: a value that already looks like a remote
URL (`trigger.LooksLikeRemote`) passes through untouched, otherwise it's
looked up by name against `config.LoadRepo` (`kman repo set|ls|rm`). This
is deliberately scoped to the Slack path, not `kman push`'s own
`--source`: that flag already accepts any URL directly and defaults to
`.` ("wherever you're standing"), and applying alias resolution there
would make a literal local path like `.` an ambiguous lookup against the
registry. No such ambiguity exists for Slack, which has no local
checkout to fall back to at all.

**The `-o repo=` push option must reflect the actually-resolved source,
not `spec.Repo`, once a flow can target more than one repo.** Before this,
`Repo: spec.Repo` was always correct because a flow only ever had one
repo. `trigger.Run` now sends `firstNonEmpty(source-if-remote, spec.Repo)`
instead — otherwise every push from a multi-repo flow would misreport
its own identity as the flow's static default, which per kranq's own
docs is what a fence and a task's own repo association are keyed on.

**`BITBUCKET_WORKSPACE`/`BITBUCKET_REPO_SLUG` are auto-derived from
the resolved source when it's a `bitbucket.org` URL, unless the flow's
own `env:` already sets them.** `create-feature.yaml` used to hardcode
both to `kman-demo`; once the flow could target other repos, that
hardcoding became a real correctness bug (a PR opened against the wrong
repo's API), not just redundant. The conditional skip
(`trigger.autoRepoEnv`) exists so an older single-repo flow that still
sets these explicitly is left alone — this is additive, not a breaking
change to any flow written before it existed.

**Free-form Slack dispatch (`--dispatch`) shells out to `claude -p` on
kman's own host, sandboxed to a pure text-in/JSON-out classification
call — confirmed live, including the sandboxing.** `internal/dispatch`
passes `--allowedTools "" --strict-mcp-config` (no tools, no MCP servers
at all) plus `--json-schema` (a `{flow,repo,task,clarify}` object) and
`--output-format json`. This was not assumed safe: a first live test
*without* `--allowedTools ""` (only `--restricted`) had Claude actually
attempt to `Write` a file on the host machine in response to an
actionable-sounding message ("add a TEST.md file...") — blocked by the
permission system, but only after a 30s, $0.08, 6-turn detour, and a
useless routing answer. With `--allowedTools ""` the same message
completes in ~10s for ~$0.03-0.07 (haiku, the default — `--dispatch-model`
overrides it) with zero tool-use attempts, because there is no tool to
attempt. A completely empty (unset) `--allowedTools` value is what
disables tools entirely; `--restricted` alone is not enough for a
call this unsandboxed-by-default.

**Candidate flows offered to dispatch are pre-filtered by `CanTrigger`,
never the full flow list.** `Handler.candidateFlows` iterates
`config.ListFlowNames` and keeps only what `cfg.Access.CanTrigger(userID,
name)` already allows, before any of it reaches the `claude -p` prompt —
dispatch can never route a user into a flow they weren't already granted,
and `handle()` still re-checks `CanTrigger` after dispatch returns a
concrete flow name as defense in depth. Repos are not access-controlled
the same way; `config.ListRepos` offers the entire registry to every
dispatch call, on the reasoning that the actual write authority is
Bitbucket's own credential scoping, not a second ACL inside kman.

**`ParseCommand`'s "run <flow>" detection is shared with the dispatch
path via two unexported helpers (`strippedText`, `parseExplicitRun`),
not duplicated.** `ParseCommand` itself is unchanged in observable
behavior (same tests, same signature) — it's now built on top of the
same two functions `Handler.route` calls directly, so an explicit
`run <flow>` always wins over dispatch without a second, drifting copy
of that parsing logic.
