# PLAN: ekko-github-action

## Overview

A GitHub Action that builds and pushes Docker images to Docker Hub and updates
manifest files in a separate GitOps repository to trigger Kubernetes
deployments. Modeled on [staffbase/gitops-github-action](https://github.com/staffbase/gitops-github-action).

The action is operated by `ekko-github-bot[bot]` for all Git operations.

---

## Architecture

### Execution Model

The action is a **composite action** written entirely in **shell scripts**. No
Node.js runtime, no Docker container action — just bash called from `action.yml`
steps. This keeps the action fast and dependency-light.

### Environment Routing

| Git ref | Environment | Input file key |
|---|---|---|
| `refs/heads/dev` | dev | `gitops-dev` |
| `refs/heads/master` or `refs/heads/main` | stage | `gitops-stage` |
| `refs/tags/v*` or `refs/tags/*` | prod | `gitops-prod` |
| any other ref | dry-run (dev) | `gitops-dev` |

### Tag Generation Strategy

| Git ref | Docker tag | Push? |
|---|---|---|
| `refs/tags/v1.2.3` | `1.2.3` | yes |
| `refs/heads/main` / `master` | `main-<short-sha>` | yes |
| `refs/heads/dev` | `dev-<short-sha>` | yes |
| `INPUT_DOCKER_CUSTOM_TAG` set | custom value | yes |
| anything else | `<short-sha>` | no |

---

## File Structure

```
ekko-github-action/
├── action.yml                      # Composite action definition
├── README.md                       # Usage docs + examples
├── .gitignore
├── .editorconfig
├── .shellcheckrc                   # ShellCheck config
├── mise.toml                       # Dev tool versions (shellcheck, bats, yq)
├── scripts/
│   ├── generate-tags.sh            # Compute docker tag + push flag from git ref
│   ├── update-gitops.sh            # Clone gitops repo, patch YAML, push
│   ├── retag-image.sh              # Pull existing image, retag, push (no rebuild)
│   ├── verify-architecture.sh      # Assert image platforms match requested ones
│   └── lib/
│       ├── common.sh               # require_env, log_info, log_error, set_output
│       └── gitops-functions.sh     # clone_repo, update_file, process_file_updates
├── tests/
│   ├── generate-tags.bats
│   ├── update-gitops.bats
│   └── lib/
│       └── common.bats
└── .github/
    └── workflows/
        ├── ci.yml                  # Run shellcheck + bats on every PR
        └── release.yml             # Tag-triggered release (auto-move major tag)
```

---

## action.yml Design

### Inputs

**Docker**

| Input | Required | Default | Description |
|---|---|---|---|
| `docker-username` | yes | — | Docker Hub username |
| `docker-password` | yes | — | Docker Hub access token |
| `docker-image` | yes | — | Image name, e.g. `myorg/myapp` |
| `docker-file` | no | `./Dockerfile` | Path to Dockerfile |
| `docker-custom-tag` | no | — | Override generated tag |
| `docker-build-args` | no | — | Newline-separated build args |
| `docker-build-platforms` | no | `linux/amd64` | Comma-separated target platforms |
| `docker-build-secrets` | no | — | Build secrets |

**GitOps**

| Input | Required | Default | Description |
|---|---|---|---|
| `gitops-token` | yes | — | GitHub PAT with write access to gitops repo |
| `gitops-organization` | yes | — | GitHub org owning the gitops repo |
| `gitops-repository` | yes | — | Gitops repo name |
| `gitops-user` | no | `ekko-github-bot[bot]` | Committer name |
| `gitops-email` | no | `ekko-github-bot[bot]@users.noreply.github.com` | Committer email |
| `gitops-dev` | no | — | Newline-separated YAML paths to update for dev |
| `gitops-stage` | no | — | Newline-separated YAML paths to update for stage |
| `gitops-prod` | no | — | Newline-separated YAML paths to update for prod |

### Outputs

| Output | Description |
|---|---|
| `docker-tag` | The computed or custom tag applied to the image |
| `docker-digest` | SHA256 digest of the pushed image |

### Steps (composite)

1. **generate-tags** — run `scripts/generate-tags.sh`, sets `tag`, `push`, `tag_list` step outputs
2. **docker-login** — `docker/login-action` with Docker Hub creds
3. **docker-setup-buildx** — `docker/setup-buildx-action`
4. **docker-build-push** — `docker/build-push-action` using outputs from step 1
5. **update-gitops** — run `scripts/update-gitops.sh` (skipped if `push` output is `false`)

---

## Script Design

### `lib/common.sh`
- `require_env VAR` — exit 1 with message if env var unset/empty
- `log_info MSG` — prefixed stdout line
- `log_error MSG` — prefixed stderr line
- `set_output KEY VALUE` — writes to `$GITHUB_OUTPUT`

### `lib/gitops-functions.sh`
- `clone_repo ORG REPO TOKEN` — shallow clone via HTTPS with token, returns path
- `update_file FILE IMAGE` — use `yq` to update `.spec.template.spec.containers[].image` (or a configurable yq expression)
- `process_file_updates FILE_LIST PUSH` — iterate paths, call `update_file`, commit, optionally push

### `scripts/generate-tags.sh`
Reads `GITHUB_REF`, `GITHUB_SHA`, `INPUT_DOCKER_IMAGE`, `INPUT_DOCKER_CUSTOM_TAG`.
Writes to `$GITHUB_OUTPUT`: `tag`, `push`, `tag_list`, `latest`.

### `scripts/update-gitops.sh`
Reads all `INPUT_GITOPS_*` and `INPUT_DOCKER_*` env vars.
Routes to dev/stage/prod based on `GITHUB_REF`.
Clones the gitops repo, patches files with `yq`, commits as the bot, pushes.

### `scripts/retag-image.sh`
Pulls an existing image by digest, tags it with a new tag, and pushes without
rebuilding. Used for promoting a dev image to prod.

### `scripts/verify-architecture.sh`
Pulls the image manifest and asserts that the listed platforms match
`INPUT_DOCKER_BUILD_PLATFORMS`. Fails the action if there is a mismatch.

---

## Tooling

| Tool | Purpose |
|---|---|
| [shellcheck](https://github.com/koalaman/shellcheck) | Static analysis for all shell scripts |
| [bats-core](https://github.com/bats-core/bats-core) | Unit tests for shell scripts |
| [yq](https://github.com/mikefarah/yq) | YAML patching in update-gitops.sh |
| [mise](https://mise.jdx.dev/) | Pin tool versions locally and in CI |

---

## CI / Release Workflows

### `ci.yml` (on pull_request + push to master/dev)
1. Install tools via `mise`
2. Run `shellcheck` on all `.sh` files
3. Run `bats tests/`

### `release.yml` (on `push` to tags `v*`)
1. Move the major version tag (e.g. `v1`) to point at the new release
   so callers using `uses: org/ekko-github-action@v1` get the latest patch.

---

## Usage Example (caller workflow)

```yaml
- uses: your-org/ekko-github-action@v1
  with:
    docker-username: ${{ secrets.DOCKERHUB_USERNAME }}
    docker-password: ${{ secrets.DOCKERHUB_TOKEN }}
    docker-image: myorg/myservice
    gitops-token: ${{ secrets.GITOPS_TOKEN }}
    gitops-organization: myorg
    gitops-repository: k8s-manifests
    gitops-dev: |
      apps/myservice/dev/deployment.yaml
    gitops-stage: |
      apps/myservice/stage/deployment.yaml
    gitops-prod: |
      apps/myservice/prod/deployment.yaml
```

---

## Out of Scope (for now)

- Multi-registry support (beyond Docker Hub)
- Upwind / SLSA provenance attestations
- Automatic PR creation in the gitops repo (direct push only)
- Helm chart value patching (only raw YAML via yq)
