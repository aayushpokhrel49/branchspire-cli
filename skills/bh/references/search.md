# Finding issues, pull requests and your work

## Search or list?

- `bh issue list` and `bh pr list` read one repository and filter by state, labels, people and
  milestone. `-S` searches titles and bodies in that repository.
- `bh search issue`, `bh search pr` and `bh search repo` search the whole selected instance,
  or one owner with `--owner`, or one repository with `-R`.
- bh has no code search; to find code, clone or read files with `bh api` ([api.md](api.md)).

```sh
bh issue list --label bug --milestone v1.0 --state all -L 100 --no-input --json number,title,state
bh issue list -S "panic on start" --no-input --json number,title,url
bh pr list --author @me --state open --no-input --json number,title,head,review_decision
bh pr list --head feature/login --no-input --json number,url
bh search issue "login error" --owner OWNER --no-input --json repository,number,title,url
bh search pr --review-requested --no-input --json repository,number,title,url
bh search repo cli --owner gitea --no-input --json full_name,description
```

- The query is a plain search string. Filter with flags (`--label`, `--author`, `--state`)
  rather than GitHub-style qualifiers such as `is:open`; BranchSpire's search API takes these
  filters as separate parameters.
- `@me` stands for the signed-in user in `--author`, `--assignee` and `--mentions`, as in
  `bh search issue --assignee @me --no-input`. Without `--repo`, `bh search` takes only `@me`
  for `--assignee` and `--mentions`.
- Results stop at `-L` (default 30). An empty result exits 0; check the count, not the exit
  code, before saying nothing exists.
- `--since` and `--until` take a date, an RFC3339 timestamp or a duration ago (`24h`).

## Your work

```sh
bh status --no-input --json pull_requests,assigned_issues,mentions,failing_runs
bh pr status --no-input --json current_branch,authored,review_requested
bh notification list --no-input --json id,repository,type,number,title,unread
bh notification list --type pull --this-repo --no-input --json id,number,title
```

- `bh status` gathers your open pull requests, assigned issues, mentions and the failing
  Actions runs of the current repository.
- Notification IDs are thread IDs, not issue numbers; `number` and `repository` name the issue
  or pull request. `bh notification view` marks a notification as read, so prefer
  `bh notification list` for reading.
