# kman

Manages Slack/human-triggered access to kranq (a separate repo at
`../kranq`): credentials, flows, groups, integrations, a cron scheduler, and
a web UI.

**The full design and phasing lives at** [docs/design.md](docs/design.md).
Read it before making architectural decisions; it records what was decided
and why.

## Where this is now

Phases 0-3. See [docs/status.md](docs/status.md) for what's done and
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
kman holds *other people's* long-lived credentials. There is no access
model yet (that's Phase 3, enforced at Phase 6): any flow can currently
bind any secret in the vault.

**A flow's `credentials:` block is resolved at push time and merged into
the same env as `args:`, with a hard error on collision.** `kman push`
refuses to push if an env var name is claimed by both an arg and a
credential — that's a flow-authoring mistake to catch, not something to
silently resolve one way. Like `args:`, `credentials:` is never rendered
into the kranq task-spec YAML; kranq has no concept of either.

**The web UI (Phase 4) is one more interface onto the config repo, never a
separate store.** It reads and writes the exact same `Config` type the CLI
uses; every save is a git commit, not a database write. Two admins editing
the same flow resolve the conflict the way two pushes to the same branch
always do here: a non-fast-forward write is rejected and retried.

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

**`kman grant`/`kman revoke` write local files only — they never commit or
push.** They edit `$KMAN_HOME/config/{users,groups}/<name>.yaml` directly
(the same directory `config.Load` reads, whether it's a plain local
directory or the checkout of a real config-repo URL) and leave committing
that change to the operator. Phase 4's web UI is the one that commits every
write automatically — see docs/design.md's reasoning for why that's a web
UI concern and not a CLI one.

**`flag.FlagSet.Parse` stops at the first non-flag argument.** Every kman
command that takes a flag after a required positional argument (`kman push
flow.yaml --kranq-url X`, `kman user set simon --slack-id X`) goes through
`parsePermuted` (`internal/cli/push.go`) instead of calling `fs.Parse`
directly, or the flag is silently dropped with no error — a real bug this
found twice already while wiring up new commands, worth checking for
before adding a ninth.

**No code comments in this repo, per the maintainer's standing instruction.**
If a construct needs a comment to be understood, it's the wrong construct —
rewrite it instead.
