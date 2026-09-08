# kman

Manages Slack/human-triggered access to kranq (a separate repo at
`../kranq`): credentials, flows, groups, integrations, a cron scheduler, and
a web UI.

**The full design and phasing lives at** [docs/design.md](docs/design.md).
Read it before making architectural decisions; it records what was decided
and why.

## Where this is now

Phase 0 only. See [docs/design.md](docs/design.md) for the phase-by-phase
plan.

```sh
go test ./...
go build -o /tmp/kman .
```

## Non-obvious facts

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

**Secrets, once Phase 2 lands, live encrypted at rest under
`$KMAN_HOME/secrets`, keyed by a machine key.** Only names, never values, go
into the committed config repo — unlike kranq's own single-operator, plain
`0600 $KRANQ_HOME/env`, because kman holds *other people's* long-lived
credentials.

**The web UI (Phase 4) is one more interface onto the config repo, never a
separate store.** It reads and writes the exact same `Config` type the CLI
uses; every save is a git commit, not a database write. Two admins editing
the same flow resolve the conflict the way two pushes to the same branch
always do here: a non-fast-forward write is rejected and retried.

**The flow schema grows one section only in the phase that enforces it.**
No inert, unimplemented fields — see docs/design.md's phasing notes for why.

**No code comments in this repo, per the maintainer's standing instruction.**
If a construct needs a comment to be understood, it's the wrong construct —
rewrite it instead.
