# PLAN: ekko-github-action

## Overview

A GitHub Action that builds and pushes Docker images to Docker Hub and updates
manifest files in a separate GitOps repository to trigger Kubernetes
deployments. Modeled on [staffbase/gitops-github-action](https://github.com/staffbase/gitops-github-action)
(bash), but implemented in **Go**.

The action is operated by `ekko-github-bot[bot]` for all Git operations.

---

## Architecture

### Execution Model

The action is a **composite action** whose logic lives in a single Go CLI
binary with subcommands. Docker build/push stays delegated to the official
`docker/*` actions; the Go binary handles everything the bash scripts did:
tag generation, GitOps repo updates, retagging, and architecture verification.

The binary is invoked with `go run` from `action.yml` steps, using
`actions/setup-go` with caching so compilation adds only a few seconds on a
warm cache. Precompiled release binaries are a possible later optimization
(see Out of Scope).

Because composite steps run in the *caller's* workspace, all `go run`
invocations must use `${{ github.action_path }}` as the working directory or
module path.

### Why Go (vs. the reference bash implementation)

- Structured YAML editing with a real parser — no `yq` binary dependency
- Registry operations (retag, platform inspection) via API calls with
  `go-containerregistry` — no `docker pull` of full images
- Table-driven unit tests instead of bats; one toolchain for lint + test
- Typed input parsing and real error handling instead of `set -euo pipefail`

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
├── .golangci.yml                   # golangci-lint config
├── mise.toml                       # Dev tool versions (go, golangci-lint)
├── go.mod
├── go.sum
├── cmd/
│   └── ekko-action/
│       └── main.go                 # Subcommand dispatch: generate-tags, update-gitops, retag-image, verify-architecture
├── internal/
│   ├── ghaction/                   # Action I/O: inputs, GITHUB_OUTPUT, log annotations
│   │   ├── ghaction.go
│   │   └── ghaction_test.go
│   ├── tags/                       # Compute docker tag + push flag from git ref
│   │   ├── tags.go
│   │   └── tags_test.go
│   ├── gitops/                     # Clone gitops repo, patch YAML, commit, push
│   │   ├── clone.go                # git via os/exec, token via http.extraheader
│   │   ├── patch.go                # yaml.v3 node-level image field update
│   │   ├── gitops.go               # process file updates: route → patch → commit → push
│   │   └── *_test.go
│   └── registry/                   # go-containerregistry based operations
│       ├── retag.go                # crane.Copy: retag without pulling image
│       ├── verify.go               # assert manifest platforms match requested
│       └── *_test.go
└── .github/
    └── workflows/
        ├── ci.yml                  # golangci-lint + go test on every PR
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

1. **setup-go** — `actions/setup-go` with `go-version-file: ${{ github.action_path }}/go.mod` and module/build caching
2. **generate-tags** — `go run ./cmd/ekko-action generate-tags` (from action path), sets `tag`, `push`, `tag_list` step outputs
3. **docker-login** — `docker/login-action` with Docker Hub creds
4. **docker-setup-buildx** — `docker/setup-buildx-action`
5. **docker-build-push** — `docker/build-push-action` using outputs from step 2
6. **update-gitops** — `go run ./cmd/ekko-action update-gitops` (skipped if `push` output is `false`)

---

## Go Design

### `cmd/ekko-action`

Single binary, stdlib subcommand dispatch (no cobra — keep the dependency
tree small). Each subcommand reads its configuration from `INPUT_*` /
`GITHUB_*` env vars, exactly like the bash version, so `action.yml` wiring
stays identical to a script-based composite action.

### `internal/ghaction`

Thin layer over the Actions runtime contract (either hand-rolled or
`sethvargo/go-githubactions`):
- `Input(name string) string` / `RequireInput(name string) (string, error)` — read `INPUT_<NAME>` env vars
- `SetOutput(key, value string)` — append to `$GITHUB_OUTPUT`
- `Infof` / `Errorf` — workflow-command-formatted logging (`::error::` etc.)

### `internal/tags`

Pure function: `Generate(ref, sha, customTag string) (Result, error)` where
`Result{Tag, Push, TagList, Latest}`. No I/O — fully unit-testable against
the routing table above.

### `internal/gitops`

- `Clone(org, repo, token string) (dir string, err error)` — shallow clone
  via `os/exec` git; token passed with `-c http.extraheader=AUTHORIZATION: ...`
  (never embedded in the remote URL, so it can't leak into `.git/config` or logs)
- `PatchImage(file, image string) error` — decode into `yaml.Node`
  (gopkg.in/yaml.v3), update `.spec.template.spec.containers[].image`,
  re-encode. Node-level editing preserves comments, ordering, and formatting —
  the reason to avoid naive unmarshal/marshal
- `ProcessUpdates(dir string, files []string, image string, push bool) error` —
  patch each file, commit as the bot identity, push unless dry-run

### `internal/registry`

Built on `github.com/google/go-containerregistry`:
- `Retag(src, dst, user, pass string) error` — `crane.Copy` server-side,
  promotes a dev image to prod without pulling layers
- `VerifyPlatforms(image string, want []string) error` — fetch the manifest
  list, compare platform set against `docker-build-platforms`, error on mismatch

---

## Tooling

| Tool | Purpose |
|---|---|
| [Go](https://go.dev/) (1.24+) | Implementation language, `go test` for units |
| [golangci-lint](https://golangci-lint.run/) | Linting (replaces shellcheck) |
| [gopkg.in/yaml.v3](https://pkg.go.dev/gopkg.in/yaml.v3) | Comment-preserving YAML patching (replaces yq) |
| [go-containerregistry](https://github.com/google/go-containerregistry) | Registry API: retag + manifest inspection |
| [mise](https://mise.jdx.dev/) | Pin tool versions locally and in CI |

---

## CI / Release Workflows

### `ci.yml` (on pull_request + push to master/dev)
1. Install tools via `mise` (or `actions/setup-go` + golangci-lint action)
2. `golangci-lint run`
3. `go test ./...`
4. `go build ./...` (catches compile errors in untested paths)

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

- Precompiled release binaries downloaded by the action (skip `setup-go` +
  compile at runtime) — revisit if action startup time becomes a problem
- Multi-registry support (beyond Docker Hub)
- Upwind / SLSA provenance attestations
- Automatic PR creation in the gitops repo (direct push only)
- Helm chart value patching (only container image fields in raw manifests)
