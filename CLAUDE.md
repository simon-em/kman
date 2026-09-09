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

**`kman push`'s mirror ref survives kranq's ref sweep on purpose.** Before
every task push, `kranqpush.UpdateMirror` best-effort-pushes
`refs/heads/mirror/<slug>` (never forced, so a non-fast-forward there is a
loud warning, not a silent rewrite). kranq only sweeps `refs/heads/task/*`
and `refs/heads/ok/*` (it parses a timestamp out of the ref name to decide
what's stale) — a `mirror/` ref doesn't match that pattern, so it persists
indefinitely as a durable object anchor. That's what keeps repeat pushes to
the same external repo small instead of resending its whole history each
time, without needing anything from kranq itself.

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

**kman never talks to `limactl` and never manages a VM directly.** A "blank
VM" is a flow pushed to kranq with no Kranqfile — kranq's own base image and
layer cache are what actually provide it. Starting a VM is entirely kranq's
job, reached only through `git push kranq -o task_file=... -o
env.NAME=VALUE`.

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

**Bitbucket OAuth has been proven against a fake server, never against
real bitbucket.org.** `internal/integration/bitbucket` is fully tested
(`httptest`-based unit tests plus a manual `kman web` run: connect →
redirect → callback → token exchange → vault storage → `kman integration
bitbucket status` flipping to "connected") — but nothing in this repo or
session has a real OAuth consumer's client_id/secret or a browser to click
through Bitbucket's actual consent screen. `bitbucket/oauth/base_url`
(`internal/integration/bitbucket/provider.go`) exists for exactly this:
pointing the client at a stand-in server for tests today, and at Bitbucket
Server/Data Center for real on-prem use later — it is not test-only
scaffolding bolted onto production code.

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
into the rendered task YAML kman writes to a temp file and pushes as
`-o task_file=`. `ExtraEnv` instead flows through the same push-option env
path a resolved `credentials:` value does, so it only ever exists as a
shell-exported variable, inherited by the MCP server subprocess like any
other child process of the step.

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

**No code comments in this repo, per the maintainer's standing instruction.**
If a construct needs a comment to be understood, it's the wrong construct —
rewrite it instead.
