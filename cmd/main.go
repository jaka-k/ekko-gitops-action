package main

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/fatih/color"
	"github.com/google/go-github/v90/github"
	"github.com/sethvargo/go-githubactions"
)

type runFunc func(ctx context.Context, a *githubactions.Action) error

var commands = map[string]runFunc{
	"dump-context":        dumpContext,
	"generate-tags":       generateTags,
	"update-gitops":       updateGitops,
	"retag-image":         retagImage,
	"verify-architecture": verifyArchitecture,
}

func main() {
	// https://github.com/sethvargo/go-githubactions/blob/main/actions.go
	// Load envs from os
	// Setup everything you need
	// Runner logs render ANSI colors but aren't a TTY, so fatih/color would
	// silently disable itself without this.
	if os.Getenv("GITHUB_ACTIONS") == "true" {
		color.NoColor = false
	}
	color.Magenta("Starting Ekko Gitops Action")

	errLog := color.New(color.FgRed, color.Bold)
	if len(os.Args) < 2 {
		errLog.Fprintf(color.Error, "Usage: ekko-gitops-action <command>: %s\n", names(commands))
		os.Exit(2)
	}

	run, ok := commands[os.Args[1]]
	if !ok {
		errLog.Fprintf(color.Error, "Unknown command %q, expected one of: %s\n", os.Args[1], names(commands))
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	a := githubactions.New()
	if err := run(ctx, a); err != nil {
		a.Fatalf("%s: %v", os.Args[1], err)
	}
	color.Green("✔ %s finished", os.Args[1])

	// multistep
	// go run ./cmd generate-tags
	// docker/login-action
}

func generateTags(_ context.Context, a *githubactions.Action) error {
	return nil
}

// dumpContext is a learning/debugging command: it prints everything the two
// SDKs can see on a runner and makes a couple of read-only API calls.
func dumpContext(ctx context.Context, a *githubactions.Action) error {
	// Everything the runner exposes via GITHUB_* env vars, parsed into one
	// struct. Includes the full webhook payload in .Event.
	gctx, err := a.Context()
	if err != nil {
		return fmt.Errorf("reading GITHUB_* env (are we on a runner?): %w", err)
	}

	a.Group("GitHubContext — a.Context()")
	pretty, err := json.MarshalIndent(gctx, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(pretty))
	a.EndGroup()

	owner, repo := gctx.Repo()
	a.Infof("Repo() helper: owner=%q repo=%q", owner, repo)
	a.Infof("routing values: EventName=%q RefType=%q RefName=%q SHA=%q",
		gctx.EventName, gctx.RefType, gctx.RefName, gctx.SHA)

	// Runner facts live in RUNNER_* vars; the SDK has no struct for them,
	// a.Getenv is the escape hatch.
	a.Group("runner env — a.Getenv()")
	for _, key := range []string{"RUNNER_OS", "RUNNER_ARCH", "RUNNER_NAME", "RUNNER_TEMP", "RUNNER_DEBUG"} {
		fmt.Printf("%s=%q\n", key, a.Getenv(key))
	}
	a.EndGroup()

	// Inputs from action.yml arrive as INPUT_<NAME>. Secrets get masked
	// before anything else can log them.
	a.Group("inputs — a.GetInput()")
	fmt.Printf("ekkoRepository=%q\n", a.GetInput("ekkoRepository"))
	for _, name := range []string{"ghToken", "ghcrToken"} {
		v := a.GetInput(name)
		if v != "" {
			a.AddMask(v)
		}
		fmt.Printf("%s: %d chars\n", name, len(v))
	}
	a.EndGroup()

	token := a.GetInput("ghToken")
	if token == "" {
		a.Warningf("no ghToken input set, skipping go-github calls")
		return nil
	}

	client, err := github.NewClient(
		github.WithAuthToken(token),
		github.WithTimeout(15*time.Second),
	)
	if err != nil {
		return err
	}

	a.Group("go-github — what the token can reach")
	if limits, _, err := client.RateLimit.Get(ctx); err != nil {
		a.Errorf("rate limit: %v", err)
	} else {
		core := limits.GetCore()
		fmt.Printf("rate limit: %d/%d remaining, resets %s\n", core.Remaining, core.Limit, core.Reset)
	}

	if r, _, err := client.Repositories.Get(ctx, owner, repo); err != nil {
		a.Errorf("Repositories.Get(%s/%s): %v", owner, repo, err)
	} else {
		fmt.Printf("%s: default branch %q, private=%t, last push %s\n",
			r.GetFullName(), r.GetDefaultBranch(), r.GetPrivate(), r.GetPushedAt())
	}
	a.EndGroup()

	a.SetOutput("run-url", fmt.Sprintf("%s/%s/actions/runs/%d", gctx.ServerURL, gctx.Repository, gctx.RunID))
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
