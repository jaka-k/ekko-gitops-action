# TODO: ekko-github-action

Tasks are ordered by dependency — complete each phase before starting the next.

---

## Phase 1 — Scaffolding

- [ ] Create `.gitignore` (node_modules, .env, *.swp)
- [ ] Create `.editorconfig` (2-space indent, LF line endings)
- [ ] Create `.shellcheckrc` (`shell=bash`, `external-sources=true`)
- [ ] Create `mise.toml` pinning shellcheck, bats, yq versions
- [ ] Create empty `scripts/lib/` directory with a `.gitkeep` (or first lib file)
- [ ] Create empty `tests/lib/` directory

---

## Phase 2 — Core Library

- [ ] Write `scripts/lib/common.sh`
  - [ ] `require_env VAR` — exits with error if var is unset or empty
  - [ ] `log_info MSG` — `[INFO] MSG` to stdout
  - [ ] `log_error MSG` — `[ERROR] MSG` to stderr
  - [ ] `set_output KEY VALUE` — appends `KEY=VALUE` to `$GITHUB_OUTPUT`
- [ ] Write `scripts/lib/gitops-functions.sh`
  - [ ] `clone_repo ORG REPO TOKEN` — shallow HTTPS clone authenticated with token
  - [ ] `update_file FILE IMAGE` — use `yq` to patch container image reference
  - [ ] `process_file_updates FILE_LIST PUSH` — loop files, update, commit, conditionally push
- [ ] Write `tests/lib/common.bats` — unit tests for common.sh helpers

---

## Phase 3 — Scripts

- [ ] Write `scripts/generate-tags.sh`
  - [ ] Handle `INPUT_DOCKER_CUSTOM_TAG` (highest priority)
  - [ ] Handle `refs/tags/v*` → version tag, push=true
  - [ ] Handle `refs/heads/main` / `master` → `main-<short-sha>`, push=true
  - [ ] Handle `refs/heads/dev` → `dev-<short-sha>`, push=true
  - [ ] Handle all other refs → `<short-sha>`, push=false
  - [ ] Output: `tag`, `push`, `tag_list`, `latest`
- [ ] Write `tests/generate-tags.bats` — test all ref/tag scenarios
- [ ] Write `scripts/update-gitops.sh`
  - [ ] Validate all required env vars via `require_env`
  - [ ] Route to dev / stage / prod / dry-run based on `GITHUB_REF`
  - [ ] Call `process_file_updates` with correct file list and push flag
- [ ] Write `tests/update-gitops.bats` — test routing logic with mocked git
- [ ] Write `scripts/retag-image.sh`
  - [ ] Pull image by digest, apply new tag, push to Docker Hub
- [ ] Write `scripts/verify-architecture.sh`
  - [ ] Inspect pushed manifest, assert platforms match `INPUT_DOCKER_BUILD_PLATFORMS`

---

## Phase 4 — action.yml

- [ ] Define composite action metadata (name, description, author, branding)
- [ ] Declare all inputs with descriptions and defaults (see PLAN.md)
- [ ] Declare outputs (`docker-tag`, `docker-digest`)
- [ ] Add step: run `scripts/generate-tags.sh` (shell: bash)
- [ ] Add step: `docker/login-action` with Docker Hub creds
- [ ] Add step: `docker/setup-buildx-action`
- [ ] Add step: `docker/build-push-action` wired to generate-tags outputs
- [ ] Add step: run `scripts/update-gitops.sh` with `if: steps.tags.outputs.push == 'true'`
- [ ] Pin all third-party actions to full commit SHAs

---

## Phase 5 — CI Workflow

- [ ] Create `.github/workflows/ci.yml`
  - [ ] Trigger on `pull_request` and `push` to `master`/`dev`
  - [ ] Job: install tools with `mise install`
  - [ ] Job step: `shellcheck scripts/**/*.sh`
  - [ ] Job step: `bats tests/`

---

## Phase 6 — Release Workflow

- [ ] Create `.github/workflows/release.yml`
  - [ ] Trigger on `push` to tags matching `v*`
  - [ ] Step: move major version tag (e.g. `v1`) to HEAD
  - [ ] Step: create GitHub Release with auto-generated changelog

---

## Phase 7 — Documentation

- [ ] Update `README.md`
  - [ ] Add badges (CI status, license)
  - [ ] Describe purpose in 2–3 sentences
  - [ ] Document all inputs and outputs in a table
  - [ ] Add usage example workflow snippet
  - [ ] Add branch → environment routing table
  - [ ] Add contributing / local dev section (mise, shellcheck, bats)
- [ ] Add `LICENSE` file (choose license — Apache-2.0 recommended)
- [ ] Add `CONTRIBUTING.md` with local dev setup instructions

---

## Phase 8 — End-to-End Validation

- [ ] Create a test repository with a dummy `deployment.yaml` manifest
- [ ] Wire a test caller workflow against the action
- [ ] Verify dev push updates the dev manifest file
- [ ] Verify master push updates the stage manifest file
- [ ] Verify tag push updates the prod manifest file and produces correct docker-digest output
- [ ] Confirm commits are authored as `ekko-github-bot[bot]`
