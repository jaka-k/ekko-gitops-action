# ekko-gitops-action

A specialized GitOps action that generates image tags, retags container images in
GHCR, and updates the manifests in [jaka-k/ekko](https://github.com/jaka-k/ekko)
(FluxCD). Written in Go, shipped as prebuilt binaries behind a Node launcher —
no `docker build` or `go run` on the consumer's runner.

## Commands

One action, multiple subcommands, selected via the `command` input. Each
invocation is one step (one row) in the job.

| Command | Status | What it does |
|---|---|---|
| `generate-images` | ✅ | Derives the image tag / full reference from the git context |
| `generate-preview` | 🚧 | Creates a preview environment in the GitOps repo |
| `retag-image` | 🚧 | Server-side retag (promote) of an existing GHCR image |
| `verify-architecture` | 🚧 | Verifies a pushed image contains the expected platforms |

## Inputs

| Input | Required | Default | Purpose |
|---|---|---|---|
| `command` | no | `generate-images` | Subcommand to run |
| `service` | no | `''` | Service name in a monorepo (see naming below) |
| `image` | no | `''` | Explicit image reference for `retag-image` / `verify-architecture` |
| `ghToken` | yes | — | Token with write access to the GitOps repo (use a GitHub App installation token) |
| `ghcrToken` | no | `${{ github.token }}` | Registry auth; the workflow's `GITHUB_TOKEN` works with `packages: write` |
| `ekkoRepository` | no | `https://github.com/jaka-k/ekko` | GitOps repository to update |

## Outputs

| Output | Example |
|---|---|
| `docker-tag` | `1667813-dev` |
| `docker-image` | `ghcr.io/jaka-k/my-app/backend:1667813` |

## Image naming

The scheme is *package-per-service*: in a monorepo every service gets its own
GHCR package under the repo path, so tags stay clean and each package has its
own access control and retention.

| Repo layout | Branch | Resulting image |
|---|---|---|
| single image | any | `ghcr.io/<owner>/<repo>:<repo>-<sha7>` |
| single image | `dev` | `ghcr.io/<owner>/<repo>:<sha7>-dev` |
| monorepo (`service: backend`) | any | `ghcr.io/<owner>/<repo>/backend:<sha7>` |
| monorepo (`service: backend`) | `dev` | `ghcr.io/<owner>/<repo>/backend:<sha7>-dev` |

## Usage

### Standard repo (one image)

```yaml
name: Deploy to Ekko

on:
  push:
    branches: ["**"]

jobs:
  deploy:
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write   # lets the default ghcrToken (GITHUB_TOKEN) push to GHCR

    steps:
      - uses: actions/checkout@v7

      # Mint a short-lived installation token for the GitOps repo.
      # GITHUB_TOKEN cannot cross repo boundaries, an app token can.
      - name: Get ekko app token
        id: app-token
        uses: actions/create-github-app-token@v2
        with:
          app-id: ${{ vars.EKKO_APP_ID }}
          private-key: ${{ secrets.EKKO_APP_PRIVATE_KEY }}
          owner: jaka-k
          repositories: ekko

      - name: Generate image tags
        id: tags
        uses: jaka-k/ekko-gitops-action@v0
        with:
          command: generate-images
          ghToken: ${{ steps.app-token.outputs.token }}

      # e.g. feed the result into docker/build-push-action:
      #   tags: ${{ steps.tags.outputs.docker-image }}
```

### Monorepo (matrix over changed services)

A cheap `changes` job detects which service paths were touched and feeds only
those into the matrix — an untouched service is neither rebuilt nor redeployed.
The filter names double as the `service` input values.

```yaml
name: Deploy to Ekko

on:
  push:
    branches: ["**"]

jobs:
  changes:
    runs-on: ubuntu-latest
    outputs:
      services: ${{ steps.filter.outputs.changes }}   # JSON array of matched filters
    steps:
      - uses: actions/checkout@v7
      - uses: dorny/paths-filter@v3
        id: filter
        with:
          filters: |
            backend:
              - 'backend/**'
            frontend:
              - 'frontend/**'
              - 'shared/**'

  deploy:
    needs: changes
    if: needs.changes.outputs.services != '[]'
    runs-on: ubuntu-latest
    permissions:
      contents: read
      packages: write
    strategy:
      matrix:
        service: ${{ fromJSON(needs.changes.outputs.services) }}

    steps:
      - uses: actions/checkout@v7

      - name: Get ekko app token
        id: app-token
        uses: actions/create-github-app-token@v2
        with:
          app-id: ${{ vars.EKKO_APP_ID }}
          private-key: ${{ secrets.EKKO_APP_PRIVATE_KEY }}
          owner: jaka-k
          repositories: ekko

      - name: Generate image tags
        id: tags
        uses: jaka-k/ekko-gitops-action@v0
        with:
          command: generate-images
          service: ${{ matrix.service }}
          ghToken: ${{ steps.app-token.outputs.token }}
```

A commit touching both services runs two parallel jobs that tag two packages
with the same `<sha7>`.

## Tokens

Two credentials, only one of which you create:

- **`ghcrToken`** — defaults to the workflow's `GITHUB_TOKEN`; just grant the
  job `packages: write`. Note that a GHCR package's ACL is bound to the repo
  whose token first pushed it — pushing the same package from another repo's
  workflow needs an explicit grant in the package settings.
- **`ghToken`** — must reach across repos into the GitOps repo, which
  `GITHUB_TOKEN` never can. Use the `ekko-github` GitHub App: store the app id
  as an `EKKO_APP_ID` variable and the PEM as an `EKKO_APP_PRIVATE_KEY` secret
  in the consuming repo, and mint a token per run with
  `actions/create-github-app-token` (see examples). Tokens live one hour,
  commits show up verified as `ekko-github[bot]`, and the installation is
  scoped to the `ekko` repo only.

## Development

```bash
go run ./cmd <command>            # run any command locally; GITHUB_*/INPUT_* env vars stand in for the runner
./scripts/build.sh                # build bin/main-<os>-<arch>-<sha> for all platforms + pin invoke-binary.js
```

The action runs as `using: node24` → `invoke-binary.js` picks the binary for
the current platform from `bin/` and execs it with the `command` input as
`argv[1]`. Inputs arrive as `INPUT_*` env vars, results leave via
`$GITHUB_OUTPUT` — see `cmd/main.go`.

## Releasing

Binaries are not committed to `master` (`bin/` is gitignored). The
[Release workflow](.github/workflows/release.yml) turns a source-only tag into
a consumable one:

```bash
gh release create v0.2.0 --generate-notes
```

The workflow checks out the tag, builds the binaries, commits them, and
force-moves both the version tag and the floating major tag (`v0`) onto that
commit. Consumers pin `@v0` for latest-in-major or `@v0.2.0` for
reproducibility. Bump `MAJOR_VERSION` in the workflow on breaking changes.
