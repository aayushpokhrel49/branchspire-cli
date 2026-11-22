# CI: checks, runs and logs

BranchSpire reports CI on a pull request as commit statuses ("checks") of its head commit. BranchSpire
Actions runs post one check per job, linked to the job.

## Is CI green?

```sh
bh pr checks 12 --no-input
echo $?   # 0 passed, 1 a check failed, 8 checks still pending
bh pr checks 12 --no-input --json name,state,description,url
```

- Without a number, `bh pr checks` takes the pull request of the current branch.
- `state` is `success`, `skipped` or `pending`, and anything else is a failure, such as
  `failure` or `error`.
- Checks belong to the head commit. After a push, re-read them; old results do not carry over.
- `bh pr view 12 --no-input --json head_sha,mergeable,review_decision` tells whether the pull
  request is mergeable once checks pass.
- `bh status --no-input --json failing_runs` lists failing runs of the current repository
  alongside your pull requests and assigned issues.

## Waiting

```sh
bh pr checks 12 --watch --no-input              # stops at the first failure
bh pr checks 12 --watch --timeout 20m --no-input
bh run watch 34 --timeout 10m --no-input        # fails when the run does not succeed
```

Both poll through an outage of the instance of up to 5 minutes. Prefer waiting over a loop
of repeated calls. A run whose jobs never start is waiting for a runner, or, with status
`waiting`, for approval in the web interface (a first-time contributor's fork); report that to
the user instead of waiting longer. `bh runner list --json name,status,labels` shows whether a
runner with the run's labels is online, but only among the repository's own runners: those of
its organization (`--org OWNER`) and of the instance (`--global`, administrators) take its jobs too.

## Finding the run

```sh
bh run list --branch feature/login -L 5 --no-input --json id,workflow,status,conclusion,commit
bh run list --commit "$(git rev-parse HEAD)" --no-input --json id,workflow,conclusion
bh run list --workflow ci.yml --status failure --since 24h --no-input --json id,title,branch
bh run view 34 --no-input --json status,conclusion,jobs --jq '.jobs[] | {id, name, status, conclusion}'
bh run jobs 34 --no-input --json id,name,conclusion,steps
```

- `--status` takes a status or a conclusion: `queued`, `in_progress`, `waiting`, `completed`,
  `success`, `failure`, `cancelled`, `skipped` and others.
- `--commit` takes a full SHA or a revision of the local clone.
- A run is given by its ID (`34`, `#34`) or its URL; a job URL selects one job.

## Reading logs

```sh
bh run logs 34 --failed --no-input                     # the failed steps of every failed or canceled job
bh run logs 34 --failed --tail 100 --no-input          # only the end of each
bh run logs 34 --job build --no-input                  # one whole job, by name or ID
bh run logs https://branchspire.example/OWNER/REPO/actions/runs/34/jobs/71 --no-input
bh run logs 34 --job build --follow --no-input         # stream until the job completes
bh run logs 34 --output logs.zip --no-input            # whole run as a ZIP archive
```

- The log goes to stdout and each job's name to stderr, with the names of its failed steps
  under `--failed`, so `2>/dev/null` leaves pure log.
- Lines print without the timestamp BranchSpire starts them with; `--timestamps` keeps it.
- `--failed` prints the whole log of a job that reports no failed step, such as a canceled one.
- Logs can be long. Search them (`| grep -n -i -E 'error|fail|panic'`) or read the end
  (`--tail 200`) before loading everything.
- `bh repo activity [REPOSITORY] --no-input --json type,actor,repository,created_at` lists what happened lately in a repository, and
  `bh activity [OWNER] --no-input --json type,actor,repository,created_at` the feed of a user (what they did and what happened in repositories they watch) or
  organization; `--date YYYY-MM-DD` keeps one day in the server's time zone.
- `bh run view 34 --log --no-input` prints the logs of every job instead of the run summary; `--log-failed`
  prints those of the failed steps, like `bh run logs --failed`, and `--job build` narrows the view or the logs to one job.
- Steps are in `bh run jobs 34 --json steps`: a failed step's name narrows where to look.

From a failing check straight to its log:

```sh
bh pr checks 12 --no-input --json state,url --jq '.[] | select(.state != "success" and .state != "skipped" and .state != "pending") | .url'
bh run logs https://branchspire.example/OWNER/REPO/actions/runs/34/jobs/71 --no-input
```

A check whose `url` points elsewhere comes from an external CI; bh cannot read its logs.

## Fixing and retrying

- A real failure: reproduce locally when possible, fix, commit and push, then wait on the new
  checks with `bh pr checks --watch`.
- A flaky failure: `bh run rerun 34 --failed --no-input` reruns the failed or canceled jobs,
  or one job with `--job build`. Rerun only completed runs; cancel a stuck one first with
  `bh run cancel 34 --no-input`.
- Ask the user before rerunning repeatedly or cancelling someone else's run.

## Artifacts

```sh
bh run artifact list 34 --no-input --json name,size_in_bytes,expired
bh run download 34 --name build --dir dist --no-input
bh run download 34 --zip --no-input
```
