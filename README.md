# kman

Manages Slack/human-triggered access to [kranq](https://github.com/simon-em/kranq):
credentials, flows, groups, integrations (Bitbucket OAuth, Slack), a cron
scheduler, and a web UI that is an interface onto a config git repo.

Read [docs/design.md](docs/design.md) before making architectural decisions —
it records the phasing and the reasoning behind each one, including why kman
exists as a separate binary from kranq rather than a feature of it.

## Where this is now

Phase 0 only: CLI dispatch, the flow schema, and config-repo loading (with a
local-directory fallback for testing). Nothing pushes to kranq yet.

```sh
kman version
kman validate <flow.yaml>...
kman render <flow.yaml>   # prints the kranq task-spec YAML the flow projects to
```

## Commands

```sh
go build -o /tmp/kman .
go test ./...
```
