# kman

Manages Slack/human-triggered access to [kranq](https://github.com/simon-em/kranq):
credentials, flows, groups, integrations (Bitbucket OAuth, Slack), a cron
scheduler, and a web UI that is an interface onto a config git repo.

Read [docs/design.md](docs/design.md) before making architectural decisions —
it records the phasing and the reasoning behind each one, including why kman
exists as a separate binary from kranq rather than a feature of it.

## Where this is now

Phases 0-4 and 6: CLI dispatch, the flow schema, config-repo loading (with
a local-directory fallback for testing), the git mechanics to push a flow
at kranq, an encrypted secrets store a flow can bind a credential to,
users/groups/grants recording who may trigger which flow (not yet enforced
against a live caller — that starts with Phase 7's Slack integration), a
web UI over all of it, and a Bitbucket OAuth integration for per-user
credentials. Phase 5 (kranq's own `files:` addition) landed in the kranq
repo, not here. See [docs/status.md](docs/status.md) for the phase-by-phase
state.

```sh
kman version
kman validate <flow.yaml>...
kman render <flow.yaml>                 # the kranq task-spec YAML the flow projects to
kman mirror <remote-url>                # sync a host-side cache mirror of a repo
kman push <flow.yaml> [NAME=VALUE...] --kranq-url <url> --as <user-id> [--source DIR|URL]
kman secret set <name> [value]          # value read from stdin if omitted
kman secret ls                          # names only, never values
kman secret rm <name>
kman user set <id> [--display-name X] [--slack-id Y]
kman user ls
kman group set <name>
kman group add-member <group> <user-id>
kman group remove-member <group> <user-id>
kman group ls
kman grant <user/ID|group/NAME> <flow>
kman revoke <user/ID|group/NAME> <flow>
kman web [--addr 127.0.0.1:8080]        # a browser UI over flows/users/groups/integrations; every save is a git commit
kman integration bitbucket status [user-id...]
```

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

## Commands

```sh
go build -o /tmp/kman .
go test ./...
```
