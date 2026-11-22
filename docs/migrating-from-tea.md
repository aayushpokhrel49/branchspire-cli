# Migrating from tea

bh replaces [tea](https://gitlab.com/aayushpokhrel49/branchspire-tea). Commands are singular and grouped by resource
(`bh pr`, not `tea pulls`), every `list` also answers to `ls` and every `delete` to `rm`, and
`--json` with `--jq`, or `--csv`, replaces `--output`. bh reads its own configuration, so log in
again with `bh auth login`.

## Global flags

| tea | bh |
| --- | --- |
| `--login`, `-l` | `--host` and `--user` |
| `--repo`, `-r` | `--repo`, `-R` |
| `--remote`, `-R` | `--remote` |
| `--output`, `-o` | `--json`, `--jq`, `--csv` |

## Commands

| tea | bh |
| --- | --- |
| `tea login add` | `bh auth login` |
| `tea login list`, `tea whoami` | `bh auth status` |
| `tea login default` | `bh auth switch` |
| `tea login delete`, `tea logout` | `bh auth logout` |
| `tea login oauth-refresh` | `bh auth refresh` |
| `tea login helper setup` | `bh auth setup-git` |
| `tea open` | `bh browse` |
| `tea clone` | `bh repo clone` |
| `tea repos create-from-template` | `bh repo create --template` |
| `tea repos search` | `bh search repo` |
| `tea issues` | `bh issue` |
| `tea comment` | `bh issue comment`, `bh pr comment` |
| `tea pulls` | `bh pr` |
| `tea pulls approve` | `bh pr review --approve` |
| `tea pulls reject` | `bh pr review --request-changes` |
| `tea pulls review-comments` | `bh pr review comments` |
| `tea pulls reply`, `resolve`, `unresolve` | `bh pr review reply`, `resolve`, `unresolve` |
| `tea labels list --save` | `bh label export` |
| `tea labels create --file` | `bh label import` |
| `tea labels update` | `bh label edit` |
| `tea milestones issues` | `bh milestone issue` |
| `tea releases assets` | `bh release asset` |
| `tea times add` | `bh time create` |
| `tea notifications read`, `unread` | `bh notification mark-read`, `mark-unread` |
| `tea organizations` | `bh org` |
| `tea ssh-keys add` | `bh ssh-key create` |
| `tea webhooks update` | `bh webhook edit` |
| `tea actions runs` | `bh run` |
| `tea actions workflows dispatch` | `bh workflow run` |
| `tea actions secrets`, `variables` | `bh secret`, `bh variable` |
| `tea admin users` | `bh admin user` |

Other tea commands keep their name in the singular, such as `tea branches` as `bh branch` and
`tea wiki` as `bh wiki`. `bh <command> --help` lists the flags of each command.
