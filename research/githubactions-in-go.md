# GitHub Actions Written in Go

`OVERVIEW.md` flagged shell vs. TypeScript as a tradeoff (takeaway 3) but didn't cover a third
option: Go. This doc looks at how Go-based actions are actually built and shipped, with real
examples, and how that would apply to `ekko-github-action` specifically.

---

## Why Go comes up for this action

`ekko-github-action` currently:
- shells out to `git clone https://TOKEN@github.com/...` (token embedded in the URL)
- shells out to `yq` for YAML patching
- shells out to `docker` for image build/push wiring

A Go rewrite would let it call the GitHub API directly (`google/go-github`), do typed YAML
patches (`gopkg.in/yaml.v3` / `sigs.k8s.io/yaml`) without a `yq` dependency, and keep the
static-binary distribution model that makes Go actions fast to run. The tradeoff is a build
step and a compiled-language dev loop instead of "edit the bash file and push."

---

## GitHub Actions has no native Go runtime

`runs.using` only accepts `node20`, `docker`, or `composite` — there is no `go` option. Every
Go action ends up in one of three shapes:

| Approach | `runs.using` | Cold start | Notes |
|---|---|---|---|
| Docker container action | `docker` | Slow if built from `Dockerfile` on every run (image build), fast if pulled from a registry | Simplest to write, worst cold-start unless pre-built |
| Composite action + shim | `node20`/`composite` invoking a prebuilt binary | Fast — no build, no image pull | What most serious Go actions use in practice |
| `go run` at workflow time | `composite`, checks out source and runs `go run` | Slow — compiles on every invocation | Fine for internal/dev-only actions, not for published ones |

---

## Approach 1 — Docker container action (simplest)

This is the pattern from Jacob Tomlinson's "Creating GitHub Actions in Go" writeup — a
minimal find-and-replace action.

**action.yml**
```yaml
name: "Find and Replace"
description: "Find and replace a string in your project files"
inputs:
  find:
    required: true
  replace:
    required: true
outputs:
  modifiedFiles:
    description: "The number of files which have been modified"
runs:
  using: "docker"
  image: "Dockerfile"
```

**Dockerfile** (multi-stage, so the shipped image is just the static binary)
```dockerfile
FROM golang:1.23 AS builder
WORKDIR /app
COPY . /app
RUN CGO_ENABLED=0 go build -ldflags="-w -s" -o app .

FROM gcr.io/distroless/static
COPY --from=builder /app/app /app
ENTRYPOINT ["/app"]
```

**main.go** — inputs arrive as `INPUT_<NAME>` env vars (uppercased), outputs are written to
the `$GITHUB_OUTPUT` file (the old `::set-output::` stdout command is deprecated):
```go
package main

import (
	"fmt"
	"os"
)

func main() {
	find := os.Getenv("INPUT_FIND")
	replace := os.Getenv("INPUT_REPLACE")

	modified := doReplace(find, replace) // your logic

	out, err := os.OpenFile(os.Getenv("GITHUB_OUTPUT"), os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Println("::error::failed to open GITHUB_OUTPUT:", err)
		os.Exit(1)
	}
	defer out.Close()
	fmt.Fprintf(out, "modifiedFiles=%d\n", modified)
}
```

Good for a first port, bad for production: if `image: Dockerfile` (rather than a
prebuilt registry image) is used, every job run rebuilds the image from scratch, which is
the exact "slow" complaint teams raise about container actions.

---

## Approach 2 — `sethvargo/go-githubactions` SDK

[`sethvargo/go-githubactions`](https://github.com/sethvargo/go-githubactions) is the
Go equivalent of the `@actions/core` npm package: it wraps the `INPUT_*` / `GITHUB_*` env var
and workflow-command conventions in an idiomatic API, with no external dependencies.

```go
package main

import (
	"github.com/sethvargo/go-githubactions"
)

func main() {
	action := githubactions.New()

	image := action.GetInput("docker-image")
	customTag := action.GetInput("docker-custom-tag")

	tag, push := computeTag(action.Context, image, customTag) // port of generate-tags.sh

	action.SetOutput("docker-tag", tag)
	action.SetOutput("push", fmt.Sprintf("%t", push))

	action.Group("resolved tag", func() {
		action.Infof("image=%s tag=%s push=%t", image, tag, push)
	})
}
```

Key API surface: `GetInput`, `SetOutput`, `SetEnv`, `AddPath`, `Group`, `Infof`/`Warningf`/
`Errorf`, `Fatalf`, and `action.Context` for a typed view of `GITHUB_REF`, `GITHUB_SHA`,
`GITHUB_EVENT_NAME`, etc. — this is a direct replacement for `scripts/lib/common.sh`'s
`require_env`/`log_info`/`set_output` helpers, minus the hand-rolled bash.

This SDK is orthogonal to the three deployment shapes above — you'd still choose Docker vs.
prebuilt-binary-shim for how the binary using this SDK actually ships.

---

## Approach 3 — Prebuilt binary + JS shim (what serious Go actions use)

Documented in [Blend's "How We Write GitHub Actions in Go"](https://full-stack.blend.com/how-we-write-github-actions-in-go.html).
Avoids both Docker's build-on-every-run cost and `go run`'s compile-on-every-run cost by
shipping compiled binaries for each OS/arch alongside a tiny JS shim that just execs the
right one:

```
├── action.yml
├── invoke-binary.js
├── main-linux-amd64
├── main-linux-arm64
├── main-windows-amd64
└── main-windows-arm64
```

```yaml
# action.yml
runs:
  using: "node20"
  main: "invoke-binary.js"
```

`invoke-binary.js` picks the binary for `process.platform`/`process.arch` and spawns it,
piping through env vars and exit code. The actual logic lives entirely in Go; the shim is
~20 lines of JS that never changes.

Why this wins over Docker for a published action: no image pull/build latency, no registry
auth friction for private images, and it matches the perceived speed of a native JS action.
The cost is a release pipeline that cross-compiles (`GOOS`/`GOARCH` matrix) and commits or
attaches binaries per release — more moving parts than `git push` for a bash script.

---

## Other things worth knowing about

- **`posener/goaction`** — lets you write a plain `go run`-able script and annotates
  inputs/outputs via comments/flags; the framework generates `action.yml` for you. Good for
  quick internal tooling, less control over the release/distribution story than approach 3.
- **`google/go-github`** — the API client to use instead of shelling out to `git clone
  https://TOKEN@...`. Lets `ekko-github-action`'s gitops update step create commits via the
  Contents API (or a real PR via the Pulls API) without ever writing the token into a URL or
  shelling out to `git`.
- **Cross-compiling** — Go's `GOOS=linux GOARCH=arm64 go build` (no cgo) is what makes
  approach 3 practical; this is the same reason Blend picked Go over other compiled options.

---

## If `ekko-github-action` were ported to Go

| Current (bash) | Go equivalent |
|---|---|
| `scripts/lib/common.sh` (`require_env`, `log_info`, `set_output`) | `go-githubactions` SDK (`GetInput`, `Infof`, `SetOutput`) |
| `scripts/generate-tags.sh` | plain Go function over `action.Context` (`GITHUB_REF`, `GITHUB_SHA`) |
| `scripts/lib/gitops-functions.sh` (`git clone` with token-in-URL, `yq` patch) | `google/go-github` Contents/Git API calls + `sigs.k8s.io/yaml` typed patch |
| `.bats` tests | plain `go test` — no bats/shellcheck toolchain needed |
| Composite action calling `docker/build-push-action` | unchanged — Docker build/push is still delegated to the existing marketplace actions; only the tag-generation and gitops-update logic move to Go |

Recommended shape if this path is taken: **approach 3** (prebuilt binary + JS shim), released
via the existing `release.yml` tag-triggered flow, with `goreleaser` handling the
`GOOS`/`GOARCH` build matrix and attaching binaries to the GitHub Release. This keeps the
composite-action call sites for `docker/login-action`, `docker/setup-buildx-action`, and
`docker/build-push-action` from `PLAN.md` unchanged, and only replaces the two custom bash
scripts with one Go binary.

---

## Sources

- [sethvargo/go-githubactions](https://github.com/sethvargo/go-githubactions)
- [How We Write GitHub Actions in Go — Blend Engineering](https://full-stack.blend.com/how-we-write-github-actions-in-go.html)
- [Creating GitHub Actions in Go — Jacob Tomlinson](https://jacobtomlinson.dev/posts/2019/creating-github-actions-in-go/)
- [posener/goaction](https://github.com/posener/goaction)
- [google/go-github](https://github.com/google/go-github)
