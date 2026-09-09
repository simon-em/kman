---
name: bitbucket
description: Use Bitbucket's REST API via curl to inspect and comment on pull requests. Use this whenever a task needs to list, read, or comment on a Bitbucket pull request.
---

# Bitbucket pull requests

Bitbucket Cloud's REST API is reachable at `https://api.bitbucket.org/2.0`. Every
request needs `Authorization: Bearer $TOKEN`, where `$TOKEN` is the first of
`BITBUCKET_TOKEN`, `BITBUCKET_API_TOKEN`, or `BITBUCKET_STEP_OAUTH_TOKEN` that is
actually set in the environment.

Workspace and repo come from `BITBUCKET_WORKSPACE` (default `effetmonstre` if
unset) and `BITBUCKET_REPO_SLUG` (required). A pull request id comes from an
explicit argument if you have one, otherwise `BITBUCKET_PR_ID`.

Set these once per shell session so the examples below can stay short:

```sh
TOKEN="${BITBUCKET_TOKEN:-${BITBUCKET_API_TOKEN:-$BITBUCKET_STEP_OAUTH_TOKEN}}"
WORKSPACE="${BITBUCKET_WORKSPACE:-effetmonstre}"
REPO="$BITBUCKET_REPO_SLUG"
PR="${1:-$BITBUCKET_PR_ID}"
API="https://api.bitbucket.org/2.0/repositories/$WORKSPACE/$REPO"
AUTH="Authorization: Bearer $TOKEN"
```

## List open pull requests

```sh
curl -s -H "$AUTH" "$API/pullrequests?state=OPEN&pagelen=25"
```

Each item's `.title`, `.author.display_name`, and `.source.branch.name` are
usually what matters; the rest of the payload is noise for this purpose.

## Get one pull request

```sh
curl -s -H "$AUTH" "$API/pullrequests/$PR"
```

Read `.title`, `.state`, `.author.display_name`, `.source.branch.name`,
`.destination.branch.name`, `.description`, and `.links.html.href`.

## Get the diff

```sh
curl -sL -H "$AUTH" "$API/pullrequests/$PR/diff"
```

This endpoint redirects (HTTP 302) to the actual diff content, so `-L` is
required; without it, curl silently returns nothing instead of an error. The
response is a plain unified diff, not JSON. It can be large; if you only need
a summary, prefer the pull request's own `.description` first.

## List existing comments

```sh
curl -s -H "$AUTH" "$API/pullrequests/$PR/comments?pagelen=100"
```

The response is paginated: follow `.next` if present. Skip any comment where
`.deleted` is true. Check this before posting feedback, so the same point
isn't made twice.

## Add a comment

A general comment:

```sh
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"content":{"raw":"<markdown text>"}}' \
  "$API/pullrequests/$PR/comments"
```

An inline comment attached to a specific line of the diff:

```sh
curl -s -X POST -H "$AUTH" -H "Content-Type: application/json" \
  -d '{"content":{"raw":"<markdown text>"},"inline":{"path":"<file path>","to":<line number>}}' \
  "$API/pullrequests/$PR/comments"
```

Every response's `.links.html.href` is the comment's own URL, worth including
when reporting back what was posted.
