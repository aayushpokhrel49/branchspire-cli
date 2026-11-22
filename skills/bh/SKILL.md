---
name: bh
description: >-
  Drives BranchSpire (hosted or self-hosted) through bh, the official BranchSpire CLI: checks whether a
  pull request's CI passed and reads the logs of the jobs that failed, reruns Actions jobs,
  reads an issue and opens a pull request for it, comments on issues and pull requests,
  reviews with inline comments, merges, searches issues and pull requests, manages releases
  and wiki pages, dispatches workflows, scripts bh in CI with GITEA_TOKEN, and makes raw BranchSpire
  API calls with bh api. Use for any BranchSpire task, or when the user mentions BranchSpire Actions,
  .gitea/workflows, or the older tea CLI.
license: MIT
---

# bh: the BranchSpire CLI for agents

`bh` drives a BranchSpire instance from the shell. Its commands follow `gh`, but flags,
JSON fields and defaults differ: check a command's `--help` (it lists `JSON FIELDS`)
instead of assuming `gh` behaviour.

## Contents

- [Rules for agents](#rules-for-agents)
- [Quick start](#quick-start): CI failures, issue to pull request, comments and reviews
- [Task reference](#task-reference)
- [Authentication and targeting](#authentication-and-targeting)
- [Exit codes](#exit-codes)
- [Validation checklist](#validation-checklist)
- [When not to use](#when-not-to-use)

## Rules for agents

- Pass `--no-input` on every command so bh never waits for a prompt or opens an editor on its
  own, and never pass `--editor` or `--web`, which open one even then.
- To extract values, name the fields: `--json number,title`, filtered with `--jq`. `--jq`
  without `--json` reads every field, so `--jq .` prints bodies too and costs many times the
  tokens of the same list as text. `--jq` runs inside bh, so `jq` need not be installed, and
  strings print without quotes. A field without a value is `null`, an empty list `[]`.
- To only look at something, the plain text output is the smallest: tab-separated rows without
  a header for lists, and `key:<TAB>value` lines, `--`, then the body for one item. It is for
  reading and may change, so never parse it in a script. Commit IDs are cut to twelve
  characters there; `--json` has them in full.
- Every bh command opens a new connection, so prefer one command to several: `bh pr view 18
  --comments` rather than a view and a comment list, `bh run view 34` for a run with its jobs.
  `--template` formats the JSON with a Go template when a layout is wanted rather than data
  (`bh help formatting` lists its functions); prefer `--jq` for scripts.
- `--json` without fields, or with an unknown one, fails and names the available ones; `bh help formatting`
  explains fields that cost extra requests.
- Without a terminal, confirmations fail with exit code 2. Pass `--yes` only after the user
  agreed to that action: deletes, merges, branch cleanup, pushing a new branch from
  `bh pr create`. Closing an issue or pull request asks no confirmation, so ask the user first.
- Pass multi-line bodies on stdin with `-F -` (`--body-file -`) and a quoted heredoc rather
  than `\n` inside `--body`.
- Lists stop at `-L 30` by default; raise `-L` (up to 1000) or page with `--page` before
  concluding something is missing.
- Pull request commands without a number act on the current branch's pull request.
- Unlike `gh`, the `comments` JSON field of issues and pull requests is a count. Read the
  comment text with `bh issue comment list` or `bh pr comment list`.

## Quick start

### Did CI pass, and why did it fail?

```sh
bh pr checks 12 --no-input                     # exit 0 passed, 1 failed, 8 still pending
bh pr checks 12 --no-input --json name,state,url --jq '.[] | select(.state != "success" and .state != "skipped" and .state != "pending")'
bh run logs https://branchspire.example/OWNER/REPO/actions/runs/34/jobs/71 --no-input
```

An Actions check's `url` is its job URL, and `bh run logs` accepts that URL to print just
that job. From a run ID instead, print the failed steps of every failed or canceled job;
`--tail 100` keeps only the end of each:

```sh
bh run list --branch feature/login --status failure -L 5 --no-input --json id,workflow,title,conclusion
bh run view 34 --no-input --json jobs --jq '.jobs[] | {name, conclusion}'
bh run logs 34 --failed --no-input
```

Wait for checks instead of polling: `bh pr checks 12 --watch --no-input`, or
`bh run watch 34 --no-input`. To retry a flaky job without a new commit,
`bh run rerun 34 --failed --no-input`.
More in [monitoring.md](references/monitoring.md).

### From an issue to a pull request

```sh
bh issue view 17 --no-input --json number,title,body,labels,state
bh issue comment list 17 -L 100 --no-input --json author,body,created_at
git switch -c fix/issue-17
# edit, test and commit
bh pr create --no-input --yes --title "fix: describe the change" --body-file - <<'EOF'
Explain what changed and why.

Closes #17
EOF
bh pr checks --watch --no-input
```

`bh pr create` pushes a branch that is not on the remote yet, after a confirmation that
`--yes` gives; ask the user before pushing. Details, forks and AGit in
[resources.md](references/resources.md).

### Comment and review

```sh
bh issue comment 17 --no-input --body-file - <<'EOF'
Reproduced on main; the fix is in #18.
EOF
bh pr comment 18 --edit-last --no-input --body "Updated after review."
bh pr review 18 --approve --no-input --body "Looks good."
bh pr review 18 --request-changes --no-input --comments-file inline.json --body "See the inline comments."
```

`inline.json` is an array of `{"path", "body", "new_position"}` (or `old_position`) on a line
of the diff; bh refuses lines outside the diff. See [resources.md](references/resources.md).

## Task reference

| Task | Command | Reference |
| --- | --- | --- |
| Checks of a pull request | `bh pr checks 12` | [monitoring.md](references/monitoring.md) |
| Wait for checks | `bh pr checks 12 --watch` | [monitoring.md](references/monitoring.md) |
| Logs of the failed steps | `bh run logs 34 --failed` | [monitoring.md](references/monitoring.md) |
| Follow a running job | `bh run logs 34 --job build --follow` | [monitoring.md](references/monitoring.md) |
| Rerun failed or canceled jobs | `bh run rerun 34 --failed` | [monitoring.md](references/monitoring.md) |
| Download artifacts | `bh run download 34 --name build` | [monitoring.md](references/monitoring.md) |
| Read an issue with comments | `bh issue view 17 --comments` | [resources.md](references/resources.md) |
| Open an issue | `bh issue create --title T --body-file -` | [resources.md](references/resources.md) |
| Close or reopen an issue | `bh issue close 17 --comment C` | [resources.md](references/resources.md) |
| Open a pull request | `bh pr create --title T --body-file -` | [resources.md](references/resources.md) |
| Read a pull request's diff | `bh pr diff 18` | [resources.md](references/resources.md) |
| Comment | `bh pr comment 18 --body-file -` | [resources.md](references/resources.md) |
| Review with inline comments | `bh pr review 18 --comment --comments-file inline.json` | [resources.md](references/resources.md) |
| Reply to an inline comment | `bh pr review reply 18 1234 --body-file -` | [resources.md](references/resources.md) |
| Merge | `bh pr merge 18 --squash --delete-branch` | [resources.md](references/resources.md) |
| Edit or delete a wiki page | `bh wiki edit Home --body-file -` | [resources.md](references/resources.md) |
| View or change a repository | `bh repo view --json full_name` | [resources.md](references/resources.md) |
| Find issues or pull requests | `bh search issue crash --owner OWNER` | [search.md](references/search.md) |
| My open work | `bh status` | [search.md](references/search.md) |
| Run bh in CI or a script | `GITEA_TOKEN` and `GITEA_CLI_HOST` | [automation.md](references/automation.md) |
| Dispatch a workflow | `bh workflow run ci.yml --ref main` | [automation.md](references/automation.md) |
| Anything without a command | `bh api 'repos/{owner}/{repo}/...'` | [api.md](references/api.md) |
| Write a BranchSpire Actions workflow | `.gitea/workflows/*.yaml` | [workflow-authoring.md](references/workflow-authoring.md) |

## Authentication and targeting

- `bh auth status --no-input --json host,user,credential_available,verified,error` reports each account; read `credential_available` and
  `verified` (add `--verify` to check against the server). With `--json` it exits 0 even when
  something is wrong; without it, missing or rejected credentials exit 4.
- In CI, set `GITEA_TOKEN` with `GITEA_CLI_HOST` (or `GITEA_INSTANCE_URL`); environment
  credentials are never saved. Logging in interactively (`bh auth login`) is for the user to
  run, not the agent.
- bh infers the repository from Git remotes and prefers `upstream` over `origin`, so a fork
  checkout targets the parent repository. Pass `-R OWNER/REPO` when that is not what you want.
- `bh help environment` lists the global flags and the selection order of repository and
  instance.

## Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Success, including an empty list |
| 1 | The command failed, `bh pr checks` found a failed check, or the instance did not respond within `http_timeout` |
| 2 | Usage error, or a confirmation that needs `--yes` |
| 3 | Canceled: a declined confirmation or prompt, or a wait's `--timeout` expired |
| 4 | Authentication missing or rejected, or the token lacks a needed scope |
| 5 | `bh run watch` and `bh workflow run --follow`: the run completed without success |
| 6 | `bh run watch --expect-head`: the run is for another commit; `bh pr merge`: the head is not the commit to merge, such as the one `--match-head-commit` names |
| 8 | `bh pr checks`: checks still pending |
| 130 | Interrupted with Ctrl-C |

`bh help exit-codes` has the full list. Branch on the code instead of parsing messages.

## Validation checklist

1. `bh --version` works and `bh auth status --no-input --json host,credential_available` shows a credential.
2. `bh repo view --no-input --json full_name` names the intended repository.
3. Reads use `--json`; every write was approved by the user.
4. After `bh pr create`, `bh pr view --no-input --json url` gives the link to report.

## When not to use

- Local Git work (commits, rebases, branches) needs `git`, not bh.
- GitHub, GitLab or other forges: bh only talks to BranchSpire instances.
- Administering the BranchSpire server itself (`app.ini`) is outside bh, except the API that
  `bh admin` and `bh api` expose and the Actions runners that `bh runner` manages.
