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
  for now, no live enforcement consumes `Registry.CanTrigger` yet (that
  starts at Phase 7's Slack allow-list check). Both `grant` and `revoke`
  refuse a flow name that doesn't exist in the current config, and
  `group add-member` refuses a user ID that doesn't exist, so the
  referential-integrity rule `Registry.Validate` enforces on load is also
  enforced proactively at write time. Every write is a plain file write
  under `$KMAN_HOME/config/{users,groups}/`, matching Phase 0's config
  directory layout exactly — no separate store, and if that directory is
  the checkout of a real config-repo URL, the change is a normal uncommitted
  edit an operator commits and pushes themselves. `kman grant`/`revoke`
  don't commit or push automatically; that's deliberately left to the
  operator (or, later, Phase 4's web UI, which does commit every write —
  see docs/design.md).

Not yet built, on purpose: nothing yet reads `Registry.CanTrigger` in
anger, because nothing yet triggers a flow on a user's behalf other than a
human running `kman push` directly at a terminal, which needs no
permission check of its own (the operator running the CLI already has
whatever access the machine's credentials give them, same as kranq's own
"if you can reach it, you're authorized" stance in `internal/gitsrv`'s
model — a permission model without a caller to check it against is just
data).

## Left to do

Everything from Phase 4 onward in [docs/design.md](design.md): the web UI,
kranq's own tool-passing generalization, the integrations (Bitbucket OAuth,
Slack), meta access, cron, declared MCP/skills access, and distribution
past the Homebrew stub.

Two things worth flagging now, before they're forgotten:
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
