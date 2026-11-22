# BranchSpire CLI (`bh`)

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](https://opensource.org/licenses/MIT)
![CI](https://github.com/aayushpokhrel49/branchspire-cli/actions/workflows/test-pr.yml/badge.svg)
![Nightly](https://github.com/aayushpokhrel49/branchspire-cli/actions/workflows/release-nightly.yml/badge.svg)

`bh` is the command-line client for [BranchSpire](https://gitlab.com/aayushpokhrel49/branchspire),
a self-hosted Git service. It manages repositories, issues, pull requests, releases, Actions,
packages and the rest of the BranchSpire API with explicit commands that work equally well in a
terminal, a script or CI. BranchSpire 28 or newer is required.

```sh
bh auth login --host https://branchspire.example --oauth
bh repo list --limit 20
bh issue list --repo owner/repository --json number,title --jq '.[] | .title'
bh pr view 123 --repo owner/repository
bh pr merge 123 --repo owner/repository --squash --yes
```

Run `bh --help` or `bh <command> --help` for the full command reference.

## Why `bh`

- **Scriptable output.** `--json`, `--jq`, `--csv` and `--template` select and format data.
  `bh schema` lists every selectable field with its type. Fields are only ever added, so
  scripts keep working across releases.
- **Safe writes.** `--dry-run` shows the requests a write command would send without sending
  them. Destructive commands ask for confirmation unless `--yes` is given.
- **Machine-readable errors.** `GITEA_CLI_ERROR_FORMAT=json` writes one JSON object to stderr;
  exit codes distinguish usage errors, authentication failures and canceled runs.
- **Secure credentials.** Tokens live in the operating-system keyring by default. File storage
  is opt-in with private permissions. CI can use `GITEA_TOKEN` without persisting it.
- **Local awareness.** Repository, host and account are discovered from your Git remotes and
  configuration, and can always be overridden with `--repo`, `--host` and `--user`.

## Installation

### Install script

The script downloads a release, verifies its SHA-256 checksum and installs `bh`. Set
`GITEA_CLI_VERSION` or `GITEA_CLI_INSTALL_DIR` to choose a version or destination; `nightly`
installs the latest build of `main`. Later, `bh upgrade` upgrades it the same way.

```sh
curl -fsSL https://gitlab.com/aayushpokhrel49/branchspire-cli/-/raw/main/scripts/install.sh | sh
# latest nightly
curl -fsSL https://gitlab.com/aayushpokhrel49/branchspire-cli/-/raw/main/scripts/install.sh | sh -s -- --nightly
```

On Windows, in PowerShell (this also adds `bh` to your PATH):

```powershell
irm https://gitlab.com/aayushpokhrel49/branchspire-cli/-/raw/main/scripts/install.ps1 | iex
# latest nightly
$env:GITEA_CLI_VERSION = 'nightly'; irm https://gitlab.com/aayushpokhrel49/branchspire-cli/-/raw/main/scripts/install.ps1 | iex
```

### Other options

- Prebuilt binaries and packages: [releases](https://github.com/aayushpokhrel49/branchspire-cli/releases).
- Docker: `docker run --rm aayushpokhrel49/branchspire-cli --help` ([image](https://hub.docker.com/r/aayushpokhrel49/branchspire-cli)).
- From source: [build from source](#build-from-source).

## Authentication

Create a personal access token in your BranchSpire instance under **Settings → Applications**,
with at least `read:user` and the scopes for what you want to do. Store it in a private file and
run:

```sh
bh auth login --host https://branchspire.example --with-token < token-file
```

Alternatively, `bh auth login --host https://branchspire.example --oauth` authorizes the built-in
**BranchSpire CLI** application in your browser. OAuth availability depends on the instance.

Check the active account with `bh auth status --verify`. For CI, supply `GITEA_TOKEN` together
with `GITEA_CLI_HOST` or `GITEA_INSTANCE_URL`; environment credentials are never persisted.

If you came from `tea`, [the migration guide](docs/migrating-from-tea.md) maps its commands and
flags to `bh`.

## Scripting

```sh
# JSON with selected fields, filtered with jq
bh pr list --repo owner/repository --state open --json number,title --jq '.[] | "\(.number) \(.title)"'

# Tab-separated values on stdout when piped
bh issue list --repo owner/repository --limit 50 | cut -f1,2

# Render JSON with a Go template
bh release view v1.2.3 --repo owner/repository --template '{{.tag}}: {{.title}}{{"\n"}}'

# Preview a write without sending it
bh issue close 42 --repo owner/repository --dry-run
```

`--json`, `--csv`, `--jq` and `--template` are mutually exclusive. JSON fields are validated
before a request is sent; `bh schema` lists them. To report failures as a single line of JSON on
stderr, set `GITEA_CLI_ERROR_FORMAT=json`.

## Configuration

`bh` keeps preferences in `config.yml` and instances and accounts in `hosts.yml`, under the
`branchspire-cli` XDG directories. `bh config path` prints the location; `bh config list` shows
effective values with their sources.

The following environment variables are supported:

| Variable | Purpose |
| --- | --- |
| `GITEA_TOKEN` | Token for CI; never persisted |
| `GITEA_CLI_HOST`, `GITEA_INSTANCE_URL` | Instance that `GITEA_TOKEN` belongs to |
| `GITEA_CLI_USER`, `GITEA_CLI_REPO` | Default account and repository |
| `GITEA_CLI_CONFIG_DIR`, `GITEA_CLI_DATA_DIR` | Configuration and data locations |
| `GITEA_CLI_ERROR_FORMAT=json` | JSON errors on stderr |
| `GITEA_CLI_NO_UPDATE_NOTIFIER=1` | Disable the release check |

The `GITEA_*` names are kept for compatibility with existing deployments and BranchSpire's
Gitea-compatible configuration.

### Shell completion

```sh
# bash (~/.bashrc)
eval "$(bh completion bash)"

# zsh (~/.zshrc, after compinit)
source <(bh completion zsh)

# fish
bh completion fish > ~/.config/fish/completions/bh.fish

# PowerShell profile
bh completion powershell | Out-String | Invoke-Expression
```

### Man page

The hidden `bh man` command generates a man page:

```sh
man <(bh man)                  # bash or zsh
man (bh man | psub)            # fish
bh man --output-file ./bh.man  # write a file
```

### Agent skill

An [agent skill](https://agentskills.io) teaches coding agents to drive `bh`: inspecting CI,
opening pull requests from issues, reviewing and merging.

```sh
npx skills add https://gitlab.com/aayushpokhrel49/branchspire-cli.git --skill bh
npx skills update bh
```

## Build from source

Go 1.27 or newer and GNU Make (`gmake` on OpenBSD) are required.

```sh
git clone https://gitlab.com/aayushpokhrel49/branchspire-cli.git
cd branchspire-cli
make        # builds ./bh
make install
```

`make install` places the binary in the `bin` directory of `$(go env GOPATH)`; make sure that
directory is on your `PATH`.

## Contributing

Fork, patch, push, open a pull request. Useful targets:

```sh
make test       # unit tests
make vet        # go vet and gitea-vet
make lint       # golangci-lint
make build      # production binary
make docs       # regenerate reference, man pages and completions
```

CI runs from `.github/workflows`. See [CONTRIBUTING.md](CONTRIBUTING.md) for the code layout,
testing strategy and review process.

## License

MIT. See [LICENSE](LICENSE). Attribution to the original Gitea authors is preserved in the
source files.
