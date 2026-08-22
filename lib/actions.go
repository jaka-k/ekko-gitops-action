package lib

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/sethvargo/go-githubactions"
)

// ---------------------------------------------------------------------------
// Construction — githubactions.New + options (testability)
// ---------------------------------------------------------------------------

// NewTestableAction showcases githubactions.New with its functional options.
// WithGetenv replaces os.Getenv and WithWriter replaces os.Stdout, so unit
// tests can feed fake INPUT_* / GITHUB_* variables and assert on the exact
// workflow commands the action emits — no real runner needed. Every method
// also exists as a package-level function (githubactions.GetInput, ...) that
// operates on a default Action, which is what quick scripts use.
func NewTestableAction(env map[string]string, out *bytes.Buffer) *githubactions.Action {
	return githubactions.New(
		githubactions.WithGetenv(func(key string) string { return env[key] }),
		githubactions.WithWriter(out),
	)
}

// ---------------------------------------------------------------------------
// Inputs — reading and validating action.yml inputs
// ---------------------------------------------------------------------------

// RequireInput showcases Action.GetInput, which reads INPUT_<NAME> (the
// name uppercased, spaces to underscores — so GetInput("docker-image") reads
// INPUT_DOCKER-IMAGE exactly as the runner sets it). The library has no
// required-input concept, so this wrapper is the pattern parseUpdateConfig
// builds on: empty means the workflow didn't pass it. Action.Getenv is the
// raw escape hatch for non-input variables.
func RequireInput(a *githubactions.Action, name string) (string, error) {
	v := strings.TrimSpace(a.GetInput(name))
	if v == "" {
		return "", fmt.Errorf("missing required input %q", name)
	}
	return v, nil
}

// MaskSecret showcases Action.AddMask: registers a value so the runner
// replaces it with *** in all subsequent log output. Must be called before
// the value can leak — first thing after reading the gitops-token input.
func MaskSecret(a *githubactions.Action, secret string) {
	a.AddMask(secret)
}

// ---------------------------------------------------------------------------
// Outputs, env, state — passing data to later steps
// ---------------------------------------------------------------------------

// PublishTagOutputs showcases Action.SetOutput (appends to $GITHUB_OUTPUT —
// how generate-tags exposes docker-tag/docker-digest to later workflow steps
// via ${{ steps.<id>.outputs.docker-tag }}) and Action.SetEnv (appends to
// $GITHUB_ENV, making the value an environment variable in every subsequent
// step). Action.AddPath is the same mechanism for $GITHUB_PATH.
func PublishTagOutputs(a *githubactions.Action, tag, digest string) {
	a.SetOutput("docker-tag", tag)
	a.SetOutput("docker-digest", digest)
	a.SetEnv("EKKO_DOCKER_TAG", tag)
}

// SaveForPostStep showcases Action.SaveState: writes to $GITHUB_STATE, which
// reappears as the STATE_<key> environment variable in the action's post:
// step — e.g. remembering a temporary branch that post-cleanup must delete.
func SaveForPostStep(a *githubactions.Action, branch string) {
	a.SaveState("cleanup-branch", branch)
}

// ---------------------------------------------------------------------------
// Logging — leveled output, annotations, grouping
// ---------------------------------------------------------------------------

// LogLevels showcases the leveled loggers. Debugf only shows up when the
// runner has step debugging enabled (ACTIONS_STEP_DEBUG=true); Noticef,
// Warningf and Errorf also create annotations on the workflow summary and
// the PR checks tab. Fatalf is Errorf + os.Exit(1) — main.go already uses it
// as the single exit point for failed subcommands.
func LogLevels(a *githubactions.Action) {
	a.Debugf("resolved registry auth for %s", "ghcr.io")
	a.Infof("retagging %s -> %s", "dev-abc1234", "prod-v1.2.3")
	a.Noticef("image already up to date, nothing to do")
	a.Warningf("gitops branch is %d commits ahead, retrying", 2)
	a.Errorf("manifest missing platform %s", "linux/arm64")
}

// AnnotateFile showcases WithFieldsMap (and its sibling WithFieldsSlice):
// returns a derived Action whose annotations carry file/line properties, so
// an Errorf points at the exact line in the values.yaml that failed to parse
// — rendered inline when the GitOps change comes from a PR.
func AnnotateFile(a *githubactions.Action, path string, line int) {
	a.WithFieldsMap(map[string]string{
		"file": path,
		"line": fmt.Sprintf("%d", line),
	}).Errorf("image tag not found at this line")
}

// GroupedLogs showcases Action.Group/EndGroup, which collapse log sections in
// the runner UI — one group per updated environment keeps update-gitops runs
// scannable.
func GroupedLogs(a *githubactions.Action, env string) {
	a.Group(fmt.Sprintf("updating %s", env))
	a.Infof("...noisy per-file details...")
	a.EndGroup()
}

// RawCommand showcases Action.IssueCommand, the low-level primitive behind
// everything above: it prints "::name key=value::message" to the runner.
// Only needed for workflow commands the library has no wrapper for.
func RawCommand(a *githubactions.Action) {
	a.IssueCommand(&githubactions.Command{
		Name:       "notice",
		Message:    "deployed",
		Properties: githubactions.CommandProperties{"title": "ekko"},
	})
}

// ---------------------------------------------------------------------------
// Step summary — markdown report on the workflow run page
// ---------------------------------------------------------------------------

// WriteRunSummary showcases Action.AddStepSummary and AddStepSummaryTemplate
// (text/template over the same $GITHUB_STEP_SUMMARY file). This is where the
// action renders its "what got deployed where" table.
func WriteRunSummary(a *githubactions.Action, image, env string) error {
	a.AddStepSummary("## Ekko GitOps update")

	return a.AddStepSummaryTemplate(
		"| image | environment |\n|---|---|\n| {{.Image}} | {{.Env}} |\n",
		struct{ Image, Env string }{image, env},
	)
}

// ---------------------------------------------------------------------------
// GitHub context — everything the runner knows about the trigger
// ---------------------------------------------------------------------------

// DescribeTrigger showcases Action.Context, which parses all GITHUB_* runner
// variables into a GitHubContext, and its Repo() helper (owner/name split,
// falling back to the event payload). RefType/RefName/SHA are exactly the
// inputs generate-tags needs for the staffbase routing rules: branch "dev" →
// dev, "main" → stage, tag → prod. Context.Event is the decoded webhook
// payload (contents of $GITHUB_EVENT_PATH) for anything deeper.
func DescribeTrigger(a *githubactions.Action) (string, error) {
	gctx, err := a.Context()
	if err != nil {
		return "", err // an *githubactions.EnvMissingError outside a runner
	}

	owner, repo := gctx.Repo()
	a.Infof("%s: %s/%s @ %s (run %d, attempt %d)",
		gctx.EventName, owner, repo, gctx.SHA, gctx.RunID, gctx.RunAttempt)

	switch {
	case gctx.RefType == "tag":
		return "prod-" + gctx.RefName, nil
	case gctx.RefName == "main" || gctx.RefName == "master":
		return "stage-" + gctx.SHA[:7], nil
	default:
		return "dev-" + gctx.SHA[:7], nil
	}
}

// ---------------------------------------------------------------------------
// OIDC — short-lived cloud credentials without stored secrets
// ---------------------------------------------------------------------------

// FetchOIDCToken showcases Action.GetIDToken: asks the runner for a signed
// JWT proving "this is workflow X in repo Y", exchangeable at a cloud
// provider or registry for temporary credentials — the secretless
// alternative to the docker-password input. Requires `permissions:
// id-token: write` on the job; without it the runner doesn't set
// ACTIONS_ID_TOKEN_REQUEST_URL and this returns an error.
func FetchOIDCToken(ctx context.Context, a *githubactions.Action) (string, error) {
	return a.GetIDToken(ctx, "sts.amazonaws.com")
}
