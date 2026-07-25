# TODO: ekko-github-action

Tasks are ordered by dependency — complete each phase before starting the next.

---

## Phase 1 — Scaffolding

- [ ] `go mod init` (module path matching the GitHub repo)
- [ ] Create `.gitignore` (binaries, .env, *.swp, coverage output)
- [ ] Create `.editorconfig` (tabs for .go per gofmt, LF line endings)
- [ ] Create `.golangci.yml` (enable errcheck, govet, staticcheck, revive)
- [ ] Create `mise.toml` pinning go and golangci-lint versions
- [ ] Create `cmd/ekko-action/main.go` with subcommand dispatch skeleton
      (`generate-tags`, `update-gitops`, `retag-image`, `verify-architecture`)

---

## Phase 2 — Core Packages

- [ ] Write `internal/ghaction`
  - [ ] `Input(name)` / `RequireInput(name)` — read `INPUT_<NAME>` env vars
        (uppercase, `-` → `_`), error if required and empty
  - [ ] `SetOutput(key, value)` — append `KEY=VALUE` to `$GITHUB_OUTPUT`
        (heredoc syntax for multiline values)
  - [ ] `Infof` / `Errorf` — workflow-command logging (`::error::` etc.)
  - [ ] Unit tests with `t.Setenv` and temp `GITHUB_OUTPUT` files
- [ ] Write `internal/tags`
  - [ ] `Generate(ref, sha, customTag) (Result, error)` — pure, no I/O
  - [ ] Handle custom tag (highest priority)
  - [ ] Handle `refs/tags/v*` → version tag, push=true
  - [ ] Handle `refs/heads/main` / `master` → `main-<short-sha>`, push=true
  - [ ] Handle `refs/heads/dev` → `dev-<short-sha>`, push=true
  - [ ] Handle all other refs → `<short-sha>`, push=false
  - [ ] Table-driven tests covering every ref/tag scenario

---

## Phase 3 — GitOps Package

- [ ] Write `internal/gitops/clone.go`
  - [ ] Shallow clone via `os/exec` git into a temp dir
  - [ ] Auth via `-c http.extraheader` (token must never appear in remote URL or logs)
- [ ] Write `internal/gitops/patch.go`
  - [ ] Decode manifest into `yaml.Node` (gopkg.in/yaml.v3)
  - [ ] Update `.spec.template.spec.containers[].image`, preserve comments/formatting
  - [ ] Error clearly if the path doesn't exist in the document
  - [ ] Tests: golden-file comparison including comment preservation
- [ ] Write `internal/gitops/gitops.go`
  - [ ] Route to dev / stage / prod / dry-run file list based on `GITHUB_REF`
  - [ ] Patch each file, commit as bot identity, push unless dry-run
  - [ ] Integration test against a local bare git repo (no network)

---

## Phase 4 — Registry Package

- [ ] Write `internal/registry/retag.go`
  - [ ] `crane.Copy` src→dst with Docker Hub auth (no image pull)
- [ ] Write `internal/registry/verify.go`
  - [ ] Fetch manifest list, assert platforms match `docker-build-platforms`
  - [ ] Tests against `go-containerregistry`'s in-memory registry (`registry.New()`)

---

## Phase 5 — Subcommands + action.yml

- [ ] Wire subcommands in `cmd/ekko-action` to the internal packages
  - [ ] `generate-tags` — outputs `tag`, `push`, `tag_list`, `latest`
  - [ ] `update-gitops` — validates required inputs, runs the gitops flow
  - [ ] `retag-image`, `verify-architecture`
- [ ] Write `action.yml`
  - [ ] Metadata (name, description, author, branding)
  - [ ] All inputs with descriptions and defaults (see PLAN.md)
  - [ ] Outputs (`docker-tag`, `docker-digest`)
  - [ ] Step: `actions/setup-go` with `go-version-file` + cache pointing at `${{ github.action_path }}`
  - [ ] Step: `go run ./cmd/ekko-action generate-tags` (working-directory: action path)
  - [ ] Step: `docker/login-action` with Docker Hub creds
  - [ ] Step: `docker/setup-buildx-action`
  - [ ] Step: `docker/build-push-action` wired to generate-tags outputs
  - [ ] Step: `go run ./cmd/ekko-action update-gitops` with `if: steps.tags.outputs.push == 'true'`
  - [ ] Pin all third-party actions to full commit SHAs

---

## Phase 6 — CI Workflow

- [ ] Create `.github/workflows/ci.yml`
  - [ ] Trigger on `pull_request` and `push` to `master`/`dev`
  - [ ] `actions/setup-go` with caching
  - [ ] `golangci-lint run`
  - [ ] `go test ./...`
  - [ ] `go build ./...`

---

## Phase 7 — Release Workflow

- [ ] Create `.github/workflows/release.yml`
  - [ ] Trigger on `push` to tags matching `v*`
  - [ ] Step: move major version tag (e.g. `v1`) to HEAD
  - [ ] Step: create GitHub Release with auto-generated changelog

---

## Phase 8 — Documentation

- [ ] Update `README.md`
  - [ ] Add badges (CI status, license)
  - [ ] Describe purpose in 2–3 sentences
  - [ ] Document all inputs and outputs in a table
  - [ ] Add usage example workflow snippet
  - [ ] Add branch → environment routing table
  - [ ] Add contributing / local dev section (mise, golangci-lint, go test)
- [ ] Add `LICENSE` file (choose license — Apache-2.0 recommended)
- [ ] Add `CONTRIBUTING.md` with local dev setup instructions

---

## Phase 9 — End-to-End Validation

- [ ] Create a test repository with a dummy `deployment.yaml` manifest
- [ ] Wire a test caller workflow against the action
- [ ] Measure action startup overhead (setup-go + go run) on cold and warm cache;
      if unacceptable, revisit precompiled release binaries
- [ ] Verify dev push updates the dev manifest file
- [ ] Verify master push updates the stage manifest file
- [ ] Verify tag push updates the prod manifest file and produces correct docker-digest output
- [ ] Confirm commits are authored as `ekko-github-bot[bot]`
- [ ] Confirm the gitops token never appears in workflow logs
