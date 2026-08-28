package lib

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-github/v90/github"
	"github.com/sethvargo/go-githubactions"
)

// Helpers around the action's inputs: token handling, client construction,
// and image resolution shared by all subcommands.

// NewGitHubClient builds an authenticated go-github client from the ghToken
// input, masking the token before anything can log it.
func NewGitHubClient(a *githubactions.Action) (*github.Client, error) {
	token := a.GetInput("ghToken")
	if token == "" {
		return nil, fmt.Errorf("missing required input %q", "ghToken")
	}
	a.AddMask(token)

	return github.NewClient(
		github.WithAuthToken(token),
		github.WithTimeout(30*time.Second),
	)
}

// NewRegistryAuth resolves GHCR credentials from the ghcrToken input. Empty
// creds behave as anonymous auth, which still works for public images.
func NewRegistryAuth(a *githubactions.Action) authn.Authenticator {
	token := a.GetInput("ghcrToken")
	if token == "" {
		Warn("no ghcrToken input set, using anonymous auth (public images only)")
	} else {
		a.AddMask(token)
	}
	return RegistryAuth(a.Getenv("GITHUB_ACTOR"), token)
}

// ResolveImage returns the image input, or the repo's own GHCR package when
// unset. GHCR paths must be lowercase, unlike GITHUB_REPOSITORY.
func ResolveImage(a *githubactions.Action) (string, error) {
	if image := a.GetInput("image"); image != "" {
		return image, nil
	}
	gctx, err := a.Context()
	if err != nil {
		return "", err
	}
	return "ghcr.io/" + strings.ToLower(gctx.Repository), nil
}

// SetOutput guards Action.SetOutput for local runs: the SDK panics when
// $GITHUB_OUTPUT is unset, which is only ever the case off-runner.
func SetOutput(a *githubactions.Action, k, v string) {
	if a.Getenv("GITHUB_OUTPUT") == "" {
		Info("(local run) skipping output %s=%s", k, v)
		return
	}
	a.SetOutput(k, v)
}
