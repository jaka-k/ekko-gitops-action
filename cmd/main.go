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

	"github.com/sethvargo/go-githubactions"
)

type runFunc func(ctx context.Context, a *githubactions.Action) error

var commands = map[string]runFunc{
	"generate-tags":       generateTags,
	"update-gitops":       updateGitops,
	"retag-image":         retagImage,
	"verify-architecture": verifyArchitecture,
}

func main() {
	// https://github.com/sethvargo/go-githubactions/blob/main/actions.go
	// Load envs from os
	// Setup everything you need
	fmt.Println("Starting Ekko Gitops Action")

	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "Usage: ekko-gitops-action <command>: %s\n", names(commands))
		os.Exit(2)
	}

	run, ok := commands[os.Args[1]]
	if !ok {
		fmt.Fprintf(os.Stderr, "Unknown command %q, expected one of: %s\n", os.Args[1], names(commands))
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := githubactions.New()
	if err := run(ctx, a); err != nil {
		a.Fatalf("%s: %v", os.Args[1], err)
	}

	// multistep
	// go run ./cmd generate-tags
	// docker/login-action
}

func generateTags(_ context.Context, a *githubactions.Action) error {
	return nil
}

func updateGitops(ctx context.Context, a *githubactions.Action) error {
	return nil
}

func names(m map[string]runFunc) string {
	return strings.Join(slices.Sorted(maps.Keys(m)), ", ")
}

func retagImage(ctx context.Context, a *githubactions.Action) error {
	return nil
}

func verifyArchitecture(ctx context.Context, a *githubactions.Action) error {
	return nil

}
