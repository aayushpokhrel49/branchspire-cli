# Issues, pull requests, comments and reviews

Issues and pull requests share one number sequence per repository. Commands take a number
(`17`, `#17`) or a URL; pull request commands also take a head branch name and default to the
current branch's pull request.

## Contents

- [Reading an issue](#reading-an-issue)
- [Opening and closing issues](#opening-and-closing-issues)
- [Working on an issue](#working-on-an-issue)
- [Opening the pull request](#opening-the-pull-request)
- [Updating the pull request](#updating-the-pull-request)
- [Reading a pull request](#reading-a-pull-request)
- [Comments](#comments)
- [Reviews](#reviews)
- [Merging and cleaning up](#merging-and-cleaning-up)
- [Repositories](#repositories)
- [Commits](#commits)
- [Releases and wiki](#releases-and-wiki)

## Reading an issue

```sh
bh issue view 17 --no-input --json number,title,body,state,labels,assignees
bh issue comment list 17 -L 100 --no-input --json id,author,body,created_at
bh issue view 17 --comments --no-input          # text: body, then the conversation
bh issue timeline 17 --no-input --json type,author,body,created_at
bh issue list --label bug --state open -L 50 --no-input --json number,title,labels
bh issue list --assignee @me --no-input --json number,title,updated_at
```

- The `comments` JSON field is the number of comments, not their text; read the text with
  `bh issue comment list`.
- `blocked_by` and `blocks` cost extra requests; request them only when needed.
- `bh issue list --kind all` includes pull requests; their `kind` reads `pull`.

## Opening and closing issues

```sh
bh issue create --no-input --title "Crash on empty config" --label bug --json number,url --body-file - <<'EOF'
Steps to reproduce, expected and actual behaviour.
EOF
bh issue create --template bug --title "Crash on empty config" --no-input --json number,url
bh issue create --title "Add a retry" --blocked-by 12 --blocking 15 --no-input --json number   # waits for #12, holds up #15
bh issue close 17 --comment "Fixed in #18." --no-input
bh issue reopen 17 --comment "Still happens on main." --no-input
bh issue lock 17 --reason Resolved --no-input     # unlock, pin and unpin work the same way
bh issue delete 17 --yes --no-input                # for good, with its comments; issues only
bh pr lock 18 --reason Resolved --no-input        # pr unlock, pin, unpin and delete too; pull requests only
```

- Labels, a milestone, assignees and a due date need write access to the repository's issues.
  Without it BranchSpire creates the issue without them, and bh names what it left out and fails.
- `--template` starts from a repository issue template; without a terminal it requires
  `--title`.
- Closing and reopening ask no confirmation and are visible to everyone: only when the user
  asks. Locking and pinning are too. Deleting asks first (`--yes` skips it) and cannot be undone.

## Working on an issue

```sh
bh issue edit 17 --add-assignee @me --no-input
git switch -c fix/issue-17
# change, test, commit with git
```

Assign yourself only when the user asks; it is visible to everyone.

## Opening the pull request

```sh
bh pr create --no-input --yes --base main --title "fix: crash on empty config" --body-file - <<'EOF'
Explain what changed and why.

Closes #17
EOF
bh pr create --fill --draft --no-input --yes      # title and body from the commits, as a draft
bh pr view --no-input --json number,url,state,draft
```

- Any flag of `bh pr create` skips its questions. The title defaults to the commit message of
  a single-commit branch, else the branch name, so pass `--title`.
- A branch not on the remote yet is pushed first, which needs `--yes` without a terminal.
  Ask the user before that push.
- Without push access, the branch goes to the user's fork when one is a remote. Otherwise ask
  the user to run `bh repo fork --add-remote` first; bh does not fork without a terminal.
- `Closes #17` (or `fixes`, `resolves`) in the body closes the issue when the pull request is
  merged.
- Follow the repository's own contribution rules (title style, sign-off, templates) when they
  exist; read `CONTRIBUTING.md` before writing the title.
- Useful flags: `--reviewer` (users, or teams as `ORG/TEAM`), `--label`, `--milestone`, `--assignee @me`, `--head OWNER:BRANCH`
  for a fork branch, `--agit` for AGit flow without a fork.

## Updating the pull request

```sh
bh pr edit 18 --title "fix: crash on empty config" --no-input
bh pr edit 18 --body-file - --no-input <<'EOF'
New description.
EOF
bh pr edit 18 --add-reviewer carol --add-label bug --no-input
bh pr ready 18 --no-input                       # draft to ready; --undo reverses it
bh pr update-branch 18 --no-input               # merge the base branch in on BranchSpire; --rebase rewrites the head
```

Pushing new commits with `git push` updates the pull request; then re-check CI
([monitoring.md](monitoring.md)).

## Reading a pull request

```sh
bh pr view 18 --no-input --json title,body,state,base,head,mergeable,review_decision,reviews
bh pr comment list 18 -L 100 --no-input --json id,author,body,created_at
bh pr view 18 --comments --no-input             # text: comments and reviews
bh pr diff 18 --name-only --no-input
bh pr diff 18 --no-input                        # unified diff, uncolored when piped
bh pr checkout 18 --no-input                    # switches branches; ask first
bh pr status --no-input --json current_branch,review_requested
```

Use `bh pr diff` or `bh pr view` to read; `bh pr checkout` changes the working tree.

## Comments

```sh
bh issue comment 17 --no-input --body-file - <<'EOF'
Comment text in Markdown.
EOF
bh pr comment 18 --no-input --body "Rebased on main."
bh pr comment 18 --edit-last --no-input --body "Rebased on main; CI is green."
bh pr comment list 18 --no-input --json id,author,body,created_at
bh issue comment edit 2001 --no-input --body "Corrected text."
```

- `--edit-last` and `--delete-last` act on your own latest comment.
- `--attachment FILE` (repeatable) uploads screenshots or logs with the comment.
- Comments on a pull request's conversation tab use `bh pr comment`; comments on lines of the
  diff are review comments, below.

## Reviews

```sh
bh pr review 18 --approve --no-input --body "Looks good."
bh pr review 18 --request-changes --no-input --body-file - <<'EOF'
Please handle the empty case.
EOF
bh pr review 18 --comment --comments-file inline.json --no-input
```

`inline.json` holds one JSON array:

```json
[
  {"path": "internal/config/load.go", "body": "This returns nil on an empty file.", "new_position": 42},
  {"path": "README.md", "body": "This line was removed but is still referenced.", "old_position": 10}
]
```

- `new_position` is a line number in the new file, `old_position` in the old one; exactly
  one per comment, on a line that is part of the diff. bh checks this before sending.
- `--comments-file` needs a real file, not `-`.
- `--request-changes` requires a body; `--comment` requires a body or inline comments.
- To collect comments first: `bh pr review 18 --pending --comments-file inline.json --no-input`,
  then `bh pr review submit 18 --approve --no-input`. A later review publishes a pending one,
  which needs `--yes` without a terminal.

Following up on reviews:

```sh
bh pr review list 18 --no-input --json id,author,state,body
bh pr review comments 18 --no-input --json id,path,line,body,author,resolver
bh pr review reply 18 1234 --no-input --body "Fixed in the latest commit."
bh pr review resolve 1234 --no-input
```

## Merging and cleaning up

```sh
bh pr merge 18 --squash --delete-branch --no-input --yes
bh pr merge 18 --auto --no-input --yes          # merge once required checks succeed
bh pr merge 18 --disable-auto --no-input         # cancel that scheduled merge
bh pr merge 18 --squash --match-head-commit 4b825dc --no-input --yes   # only the commit that was reviewed
bh pr checks 18 --required --watch --no-input   # only the checks the branch protection requires
bh pr clean 18 --no-input --yes                 # delete the branches of a merged pull request
bh pr backport 18 --to release/v1.2 --no-input --yes --json pull
```

Merging is irreversible and visible: only on the user's explicit request. Without a method bh
uses the repository's default merge style. A backport pushes a branch and opens a pull request,
so it also needs the user's request; on a conflict it changes nothing and fails.

## Repositories

```sh
bh repo view --no-input --json full_name,default_branch,private,actions_enabled
bh repo list OWNER -L 100 --no-input --json full_name,description,archived   # --visibility, --archived, --no-archived, --topic, --language
bh label list --search bug --sort name --order desc --no-input --json name,color   # --web opens the label page and takes no other flag but --org
bh repo fork --add-remote --no-input
bh repo edit --description "A short description" --add-topic cli --no-input
bh repo edit --enable-wiki=false --no-input
bh branch protection list --no-input --json rule,required_approvals,push_users,status_check_required,status_checks   # the first rule that matches a branch applies
```

- `bh repo edit` changes only the flags passed; on/off settings take `=false` to turn off.
- Changing settings, renaming, transferring, archiving and deleting affect everyone; run them
  only on the user's request. `bh repo archive`, `rename`, `transfer` and `delete` need
  `--yes` without a terminal.

## Commits

```sh
bh commit list main -L 20 --no-input --json sha,author_login,message
bh commit view 3f2a9c1 --no-input --json message,files,additions,deletions
bh commit diff 3f2a9c1 --stat                       # --path 'internal/' and --exclude '*_test.go' narrow it
bh commit compare main...feature/login --no-input --json total_commits
bh commit status main --no-input --json name,state  # exit 1 when a check failed, 8 while pending
bh commit pr 3f2a9c1 --no-input --json number,title # the pull request that merged it
```

- These read the repository on the server, so no clone is needed. `bh pr diff` takes the same
  `--path`, `--exclude`, `--name-only`, `--name-status` and `--stat`.

## Releases and wiki

```sh
bh release list -L 5 --no-input --json tag,title,draft,published_at   # --exclude-drafts, --exclude-pre-releases, --order asc
bh release create v1.2.0 dist/app.tar.gz --notes-file CHANGELOG.md --draft --no-input
bh tag list -L 5 --no-input --json name,commit,annotated
bh tag create v1.2.0 --target main --message "Version 1.2.0" --no-input   # a tag without a release
bh tag protection list --no-input --json name_pattern,users,teams
bh wiki list --no-input --json title
bh wiki view Home --no-input --json body
bh wiki create "Release process" --body-file docs/release.md --no-input
bh wiki edit Home --message "Document the release steps" --no-input --body-file - <<'EOF'
New page content.
EOF
bh wiki edit "Old title" --title "New title" --no-input   # renames, keeping the content
bh wiki revisions Home --no-input --json sha,author,message,created_at
bh wiki delete "Old Notes" --no-input --yes
```

`bh release create` creates the tag at `--target` (the default branch) when it does not exist;
pass `--verify-tag` to require an existing tag.
