package main

import (
	"context"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"

	"github.com/jaka-k/ekko-gitops-action/lib"
	"github.com/sethvargo/go-githubactions"
)

type runFunc func(ctx context.Context, a *githubactions.Action) error

var commands = map[string]runFunc{
	"generate-images":     generateImages,
	"generate-preview":    updateGitops,
	"retag-image":         retagImage,
	"verify-architecture": verifyArchitecture,
}

func main() {
	lib.StepBold("Starting Ekko Gitops Action")

	if len(os.Args) < 2 {
		lib.Error("Usage: ekko-gitops-action <command>: %s", names(commands))
		os.Exit(2)
	}

	run, ok := commands[os.Args[1]]
	if !ok {
		lib.Error("Unknown command %q, expected one of: %s", os.Args[1], names(commands))
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := githubactions.New()
	if err := run(ctx, a); err != nil {
		a.Fatalf("%s: %v", os.Args[1], err) // ::error:: annotation + exit 1
	}
	lib.Success("✔ %s finished", os.Args[1])
}

func generateImages(_ context.Context, a *githubactions.Action) error {
	gctx, err := a.Context()
	if err != nil {
		return err
	}
	if len(gctx.SHA) < 7 {
		return fmt.Errorf("no usable GITHUB_SHA (got %q)", gctx.SHA)
	}

	_, repo := gctx.Repo()
	short := gctx.SHA[:7]

	// GHCR paths must be lowercase; GITHUB_REPOSITORY preserves the repo's casing.
	pkg := strings.ToLower(gctx.Repository)
	tag := fmt.Sprintf("%s-%s", strings.ToLower(repo), short)

	// Monorepo: each service gets its own package under the repo path, and the
	// repo name drops out of the tag since the path already carries it.
	if service := strings.ToLower(strings.TrimSpace(a.GetInput("service"))); service != "" {
		pkg = fmt.Sprintf("%s/%s", pkg, service)
		tag = short
	}

	if gctx.RefName == "dev" {
		tag = fmt.Sprintf("%s-dev", short)
	}

	image := fmt.Sprintf("ghcr.io/%s:%s", pkg, tag)

	lib.Step("generating image reference for %s", gctx.Repository)
	lib.Sub("ref %s/%s @ %s", gctx.RefType, gctx.RefName, short)
	lib.StepBold("image: %s", image)
	lib.SetOutput(a, "docker-tag", tag)
	lib.SetOutput(a, "docker-image", image)
	return nil
}

func names(m map[string]runFunc) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ", ")
}

func updateGitops(ctx context.Context, a *githubactions.Action) error {
	client, err := lib.NewGitHubClient(a)
	if err != nil {
		return err
	}
	_ = client // TODO: patch the service's manifests in the ekko repo
	return nil
}

func retagImage(ctx context.Context, a *githubactions.Action) error {
	image, err := lib.ResolveImage(a)
	if err != nil {
		return err
	}

	digest, err := lib.GetDigest(image, lib.NewRegistryAuth(a))
	if err != nil {
		return err
	}
	lib.Step("retagging %s", image)
	lib.Sub("digest %s", digest)
	lib.SetOutput(a, "docker-digest", digest)

	// TODO: crane.Copy the digest to the target tag (server-side, no pull)
	return nil
}

func verifyArchitecture(ctx context.Context, a *githubactions.Action) error {
	image, err := lib.ResolveImage(a)
	if err != nil {
		return err
	}

	platforms, err := lib.ImagePlatforms(ctx, image, lib.NewRegistryAuth(a))
	if err != nil {
		return err
	}
	lib.Step("verifying architectures of %s", image)
	lib.Sub("platforms: %s", strings.Join(platforms, ", "))

	// TODO: compare against an expected-platforms input and fail on mismatch
	return nil
}
