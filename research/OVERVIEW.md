# GitOps GitHub Action — Research Overview

Comparison of alternative approaches to the staffbase/gitops-github-action pattern.
Covers repos with meaningful star counts relative to the reference (staffbase: 19 ⭐).

---

## Reference: staffbase/gitops-github-action (19 ⭐)

A monolithic **composite shell action** that handles the full pipeline in one unit:

1. Generates a Docker tag from the git ref
2. Logs in to a private registry, builds and pushes the image
3. Clones a separate GitOps repo, patches YAML files with `yq`, commits and pushes as a bot

Key traits:
- 100% bash — no Node.js runtime
- Routing is branch-based and opinionated: `dev` → dev env, `main`/`master` → stage, tags → prod
- Direct push to the GitOps repo (no PR, no review step)
- Tightly couples Docker build and manifest update into a single action
- Bot identity is hard-coded around the team's internal tooling
- No PR creation; changes land in the GitOps repo immediately

---

## Repo 1 — `fjogeleit/yaml-update-action` (171 ⭐)

**Source:** https://github.com/fjogeleit/yaml-update-action  
**Language:** TypeScript (Node.js v24)  
**Scope:** YAML/JSON update only — no Docker build, no environment routing

### What it does

A focused, single-purpose action that updates one or more properties in YAML or JSON files
and optionally commits the result to a branch or opens a pull request. The caller controls
_what_ changes are made and _when_ by wiring this action into their own workflow.

### Key inputs

| Input | Purpose |
|---|---|
| `valueFile` | Path to the YAML/JSON file to modify |
| `propertyPath` | JSONPath expression pointing to the property to update |
| `value` | New value to write |
| `changes` | JSON blob for atomic multi-file / multi-property updates |
| `commitChange` | Whether to commit (default: true) |
| `createPR` | Open a PR instead of committing directly (default: false) |
| `targetBranch` | Branch the PR should target |
| `method` | `CreateOrUpdate` / `Update` / `Create` — controls behavior when key is absent |

### Key outputs

`commit` (SHA), `pull_request` (PR metadata)

### Main differences vs. staffbase

| Dimension | staffbase | fjogeleit/yaml-update-action |
|---|---|---|
| Scope | Full pipeline (build + deploy) | YAML patch only |
| Runtime | Bash | TypeScript / Node.js |
| YAML parsing | `yq` CLI | Proper JSONPath AST parsing |
| PR support | None — always direct push | First-class, configurable |
| Environment routing | Opinionated (dev/stage/prod by branch) | None — caller decides |
| Docker build | Bundled | Not included |
| Multi-file updates | Iterates a list | Atomic JSON `changes` block |
| Type safety | String replacement | Preserves YAML types |

### Design philosophy

Unix single-responsibility principle. This action only patches files; you compose it with
`docker/build-push-action` and your own routing logic. Because it is not opinionated about
_when_ or _what_ to update, it works for any environment model.

---

## Repo 2 — `quipper/monorepo-deploy-actions` (30 ⭐)

**Source:** https://github.com/quipper/monorepo-deploy-actions  
**Language:** TypeScript (100%)  
**Scope:** Full GitOps deployment for monorepos with ArgoCD — no Docker build

### What it does

A collection of 10+ coordinated GitHub Actions designed for teams that keep application
code and Kubernetes manifests in the same monorepo. Changes are promoted through environments
via pull requests; ArgoCD syncs those PRs to clusters.

Key actions in the collection:
- `create-deploy-pull-request` — opens a PR in the destination repo with updated manifests
- `bootstrap-pull-request` — spins up an ephemeral ArgoCD Application for a feature branch
- `git-push-service` — writes generated manifests to the destination repo's branch
- `environment-matrix` — computes which environments should receive a deployment
- `update-outdated-pull-request-branch` — rebases stale deployment PRs automatically

### Destination repository convention

The GitOps repo uses branches named `ns/<overlay>/<namespace>` (e.g., `ns/develop/my-service`).
ArgoCD's ApplicationSet watches these branches. Each PR to such a branch becomes a deployment
candidate; merging it is the deployment.

### Main differences vs. staffbase

| Dimension | staffbase | quipper/monorepo-deploy-actions |
|---|---|---|
| Runtime | Bash | TypeScript |
| Monorepo support | No | Yes — core design assumption |
| Merge strategy | Direct push | Mandatory PR review step |
| Ephemeral environments | No | Yes — per-PR namespaces via ArgoCD ApplicationSet |
| GitOps CD tool | Tool-agnostic | ArgoCD-specific |
| Docker build | Bundled | Not included (separate CI step) |
| Coupling | Tightly coupled | Loosely coupled collection |
| Environment routing | Branch name → env | Destination branch convention |

### Design philosophy

Assumes a mature platform engineering setup: ArgoCD already running, monorepo structure
established, and a requirement for human-review gates on environment promotions. Direct push
to prod is not possible by design — every environment change goes through a PR.

---

## Repo 3 — `peter-evans/create-pull-request` (2,776 ⭐)

**Source:** https://github.com/peter-evans/create-pull-request  
**Language:** TypeScript (Node.js)  
**Scope:** PR creation primitive — no YAML patching, no Docker build, no environment routing

### What it does

Detects _any_ changes present in the GitHub Actions workspace (new files, modified tracked
files) and commits them to a new or existing branch, then creates or updates a pull request.
It is the canonical building block for PR-based GitOps in GitHub Actions.

Typical GitOps usage:
1. Checkout the GitOps repo in the same job
2. Run `yq` / `sed` / `kustomize edit set image` to patch manifests
3. Call `create-pull-request` to open a PR — no git plumbing needed

### Key inputs

`token`, `branch`, `title`, `body`, `add-paths`, `draft`, `labels`, `reviewers`, `assignees`,
`base`, `delete-branch`

### Main differences vs. staffbase

| Dimension | staffbase | peter-evans/create-pull-request |
|---|---|---|
| Stars | 19 | 2,776 |
| Scope | Full pipeline | PR creation only |
| Deployment model | Direct push (no review) | PR with mandatory review |
| YAML patching | Built-in (`yq` scripts) | Not included — caller patches first |
| Environment routing | Built-in | Not included |
| Composability | Low (all-in-one) | Maximum (pure primitive) |
| Auditability | Commit in GitOps repo | Full PR history + review trail |

### Design philosophy

Extreme single-responsibility. The action does exactly one thing. Because it uses the standard
GitHub Actions `GITHUB_TOKEN`, it requires zero bot credentials beyond what is already in the
workflow. The tradeoff: the caller must handle all the domain-specific logic (what to patch,
which files, what routing rules apply). Many teams pair it with `fjogeleit/yaml-update-action`
or inline `yq` steps to form a complete pipeline.

---

## Repo 4 — `werf/actions` (85 ⭐)

**Source:** https://github.com/werf/actions  
**Language:** JavaScript (Node.js, 99.9%)  
**Scope:** Full build → push → deploy pipeline via the `werf` CLI

### What it does

Not a manifest-patching action, but a wrapper for [werf](https://werf.io/), a comprehensive
CLI tool that handles the entire Docker image lifecycle and Kubernetes deployment in one
invocation. The GitHub Action installs werf and exposes it to subsequent steps.

```yaml
- uses: werf/actions/install@v2
# then in a later step:
- run: werf converge --repo ghcr.io/myorg/myapp
```

`werf converge` builds missing images, pushes them to the registry, and applies the resulting
Kubernetes manifests — all in one command with built-in content-addressable caching.

### Main differences vs. staffbase

| Dimension | staffbase | werf/actions |
|---|---|---|
| Runtime | Bash | JavaScript wrapping a Go CLI |
| Build system | `docker/build-push-action` | werf (custom builder with layer caching) |
| GitOps update mechanism | Clone repo + `yq` patch + push | `werf converge` applies manifests directly |
| Deployment model | Indirect (update GitOps repo, CD tool syncs) | Direct apply to Kubernetes cluster |
| Separation of concerns | Separate GitOps repo | Single repo (app code + manifests) |
| Registry | Any (configurable) | Any OCI-compliant registry |
| Learning curve | Low (pure bash) | High (learn werf's model) |
| Content-addressing | No | Yes — only rebuilt layers are pushed |

### Design philosophy

Vertical integration. Rather than composing several independent tools, werf owns the entire
delivery pipeline. This eliminates the coordination problem between build and deploy steps but
trades flexibility for convention — the team must adopt werf's project structure and concepts.
The GitOps side is implicit (manifests are kept in the same repo) rather than an explicit
separate-repo commit.

---

## Summary Comparison Table

| Repo | Stars | Runtime | Docker build | GitOps update | PR support | Env routing | Coupling |
|---|---|---|---|---|---|---|---|
| staffbase/gitops-github-action | 19 | Bash | ✅ | Direct push | ❌ | Branch-based (built-in) | High |
| fjogeleit/yaml-update-action | 171 | TypeScript | ❌ | Optional direct or PR | ✅ | Caller decides | Low |
| quipper/monorepo-deploy-actions | 30 | TypeScript | ❌ | PR mandatory | ✅ required | ArgoCD branch convention | Medium |
| peter-evans/create-pull-request | 2,776 | TypeScript | ❌ | PR only | ✅ only option | Caller decides | Minimal |
| werf/actions | 85 | JavaScript | ✅ (werf) | Direct to k8s | ❌ | werf's own model | Very high |

---

## Key Takeaways for ekko-gitops-action

1. **Direct push vs. PR gate.** Staffbase (and most low-star actions) push directly to the
   GitOps repo. The more popular and mature approaches (`peter-evans`, `quipper`) require PR
   review. Consider adding optional PR creation as a first-class mode.

2. **Separation of Docker build and manifest update.** The three highest-starred repos here
   deliberately exclude Docker build. Building in a separate action (`docker/build-push-action`)
   and updating manifests in another step is the dominant pattern in the ecosystem because it
   lets teams swap either piece independently. Staffbase (and our plan) bundles both — this is
   an ergonomic shortcut that reduces configurability.

3. **Shell vs. TypeScript.** All non-staffbase repos here use TypeScript/JavaScript. TypeScript
   gives structured YAML parsing (no `yq` dependency), better error messages, and native GitHub
   API access (no raw `git clone` with token-in-URL). The tradeoff is a Node.js build step and
   a more complex developer setup.

4. **Environment routing should be decoupled or at least configurable.** Staffbase hard-codes
   `dev` → dev, `main` → stage, `tag` → prod. Other approaches let the caller define routing
   entirely. Adding a `gitops-ref-override` or dry-run mode input would make ekko's action more
   portable.

5. **Bot credentials are friction.** `peter-evans/create-pull-request` uses only `GITHUB_TOKEN`
   — no separate bot account needed. Our plan requires a separate `gitops-token` PAT and a bot
   identity. This is necessary for cross-repo writes but worth documenting clearly as a setup
   cost.
