package lib

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/go-github/v90/github"
)

// ---------------------------------------------------------------------------
// Client construction — github.NewClient + functional options
// ---------------------------------------------------------------------------

// NewGitHubClient showcases github.NewClient with its functional options.
// Since v69 the constructor returns an error and takes options instead of an
// *http.Client: WithAuthToken wraps the workflow's GITHUB_TOKEN (or the bot
// PAT for the GitOps repo), WithTimeout guards against hung API calls, and
// WithUserAgent identifies the action in audit logs. For GitHub Enterprise
// there is WithEnterpriseURLs(baseURL, uploadURL); for proxied runners,
// WithEnvProxy or WithTransport (e.g. DefaultTransport from registry.go).
func NewGitHubClient(token string) (*github.Client, error) {
	return github.NewClient(
		github.WithAuthToken(token),
		github.WithTimeout(30*time.Second),
		github.WithUserAgent("ekko-gitops-action"), // Are you sure?
	)
}

// ---------------------------------------------------------------------------
// Repositories service — contents API (single-file read/commit)
// ---------------------------------------------------------------------------

// ReadFileFromRepo showcases Repositories.GetContents plus the
// RepositoryContent.GetContent decoder (handles the base64 transport
// encoding). RepositoryContentGetOptions.Ref pins the read to a branch, tag,
// or SHA. The returned blob SHA is required later to update the same file.
// Passing a directory path fills the second (directoryContent) return instead.
func ReadFileFromRepo(ctx context.Context, client *github.Client, owner, repo, path, branch string) (content, blobSHA string, err error) {
	file, _, _, err := client.Repositories.GetContents(ctx, owner, repo, path,
		&github.RepositoryContentGetOptions{Ref: branch},
	)
	if err != nil {
		return "", "", fmt.Errorf("getting %s: %w", path, err)
	}

	content, err = file.GetContent()
	if err != nil {
		return "", "", err
	}
	return content, file.GetSHA(), nil
}

// UpdateFileInRepo showcases Repositories.UpdateFile: a one-call "edit file +
// commit" against the contents API — the simplest way for update-gitops to
// patch a single values.yaml in the GitOps repo without cloning. SHA must be
// the current blob SHA from GetContents (optimistic locking: a 409 means
// someone committed in between). Committer sets the bot identity so the
// commit shows up as ekko-github[bot]. Repositories.CreateFile is the
// counterpart when the file does not exist yet.
func UpdateFileInRepo(ctx context.Context, client *github.Client, owner, repo, path, branch, blobSHA string, newContent []byte) (string, error) {
	res, _, err := client.Repositories.UpdateFile(ctx, owner, repo, path,
		&github.RepositoryContentFileOptions{
			Message:   github.Ptr(fmt.Sprintf("chore: update %s", path)),
			Content:   newContent, // unencoded; the library base64-encodes it
			SHA:       github.Ptr(blobSHA),
			Branch:    github.Ptr(branch),
			Committer: &github.CommitAuthor{Name: github.Ptr(user), Email: github.Ptr(mail)},
		},
	)
	if err != nil {
		return "", fmt.Errorf("updating %s: %w", path, err)
	}
	return res.Commit.GetSHA(), nil
}

// GetDefaultBranch showcases Repositories.Get (repo metadata) and
// Repositories.GetBranch (tip SHA of a branch) — how the action discovers
// where to base its changes when the target branch input is left empty.
func GetDefaultBranch(ctx context.Context, client *github.Client, owner, repo string) (string, error) {
	r, _, err := client.Repositories.Get(ctx, owner, repo)
	if err != nil {
		return "", err
	}

	branch, _, err := client.Repositories.GetBranch(ctx, owner, repo, r.GetDefaultBranch(), 3)
	if err != nil {
		return "", err
	}
	fmt.Printf("%s tip is %s\n", branch.GetName(), branch.GetCommit().GetSHA())
	return r.GetDefaultBranch(), nil
}

// ---------------------------------------------------------------------------
// Git service — Git Data API (atomic multi-file commits, branches)
// ---------------------------------------------------------------------------

// CommitFiles showcases the full Git Data flow: GetRef → CreateBlob →
// CreateTree → CreateCommit → UpdateRef. Unlike UpdateFileInRepo this commits
// any number of files atomically — one commit patching every environment
// manifest at once, which is what update-gitops needs when a service has
// multiple YAML files. All of it happens API-side: no clone on the runner.
func CommitFiles(ctx context.Context, client *github.Client, owner, repo, branch, message string, files map[string][]byte) (string, error) {
	// 1. Tip of the target branch. Refs use the "heads/<name>" form.
	ref, _, err := client.Git.GetRef(ctx, owner, repo, "heads/"+branch)
	if err != nil {
		return "", fmt.Errorf("getting ref: %w", err)
	}
	parentSHA := ref.GetObject().GetSHA()

	// 2. One blob per file. Base64 encoding keeps arbitrary bytes safe.
	entries := make([]*github.TreeEntry, 0, len(files))
	for path, content := range files {
		blob, _, err := client.Git.CreateBlob(ctx, owner, repo, github.Blob{
			Content:  github.Ptr(string(content)),
			Encoding: github.Ptr("utf-8"),
		})
		if err != nil {
			return "", fmt.Errorf("creating blob for %s: %w", path, err)
		}
		entries = append(entries, &github.TreeEntry{
			Path: github.Ptr(path),
			Mode: github.Ptr("100644"), // regular file
			Type: github.Ptr("blob"),
			SHA:  blob.SHA,
		})
	}

	// 3. New tree on top of the parent commit's tree (unlisted files carry over).
	tree, _, err := client.Git.CreateTree(ctx, owner, repo, parentSHA, entries)
	if err != nil {
		return "", fmt.Errorf("creating tree: %w", err)
	}

	// 4. The commit itself, authored by the bot. CreateCommitOptions.Signer
	// accepts a github.MessageSigner for GPG-signed commits; commits made
	// with a GitHub App installation token are marked verified automatically.
	now := github.Timestamp{Time: time.Now()}
	commit, _, err := client.Git.CreateCommit(ctx, owner, repo, github.Commit{
		Message: github.Ptr(message),
		Tree:    tree,
		Parents: []*github.Commit{{SHA: github.Ptr(parentSHA)}},
		Author:  &github.CommitAuthor{Name: github.Ptr(user), Email: github.Ptr(mail), Date: &now},
	}, nil)
	if err != nil {
		return "", fmt.Errorf("creating commit: %w", err)
	}

	// I could use the installation to do all sorts of shenanigans on gh

	// 5. Fast-forward the branch. Force is left unset — if the branch moved
	// since GetRef, this fails instead of clobbering someone else's commit.
	_, _, err = client.Git.UpdateRef(ctx, owner, repo, "heads/"+branch, github.UpdateRef{
		SHA: commit.GetSHA(),
	})
	if err != nil {
		return "", fmt.Errorf("updating ref: %w", err)
	}
	return commit.GetSHA(), nil
}

// CreateBranch showcases Git.CreateRef: branch off a known SHA, e.g. an
// ephemeral "ekko/update-<service>" branch to hold changes destined for a PR
// instead of a direct push to the environment branch.
func CreateBranch(ctx context.Context, client *github.Client, owner, repo, name, fromSHA string) error {
	_, _, err := client.Git.CreateRef(ctx, owner, repo, github.CreateRef{
		Ref: "refs/heads/" + name,
		SHA: fromSHA,
	})
	return err
}

// ---------------------------------------------------------------------------
// PullRequests service — review-gated promotion
// ---------------------------------------------------------------------------

// OpenPullRequest showcases PullRequests.Create — the review-gated
// alternative to CommitFiles pushing straight to the environment branch
// (compare quipper/monorepo-deploy-actions in research/OVERVIEW.md).
// Head/Base are plain branch names; cross-fork heads use "owner:branch".
func OpenPullRequest(ctx context.Context, client *github.Client, owner, repo, head, base, title string) (*github.PullRequest, error) {
	pr, _, err := client.PullRequests.Create(ctx, owner, repo, github.CreatePullRequest{
		Title:               github.Ptr(title),
		Head:                head,
		Base:                base,
		Body:                github.Ptr("Automated image update by ekko-gitops-action."),
		MaintainerCanModify: github.Ptr(true),
	})
	if err != nil {
		return nil, fmt.Errorf("opening PR %s -> %s: %w", head, base, err)
	}
	fmt.Println("opened", pr.GetHTMLURL())
	return pr, nil
}

// FindOpenUpdatePRs showcases PullRequests.List with the standard pagination
// pattern: ListOptions embedded in the filter struct, Response.NextPage as
// the loop cursor. Used to detect an already-open update PR for a service so
// the action refreshes it instead of stacking duplicates. PullRequests.Merge
// is the counterpart for auto-merge setups.
func FindOpenUpdatePRs(ctx context.Context, client *github.Client, owner, repo, base string) ([]*github.PullRequest, error) {
	opts := &github.PullRequestListOptions{
		State:       "open",
		Base:        base,
		ListOptions: github.ListOptions{PerPage: 100},
	}

	var all []*github.PullRequest
	for {
		prs, resp, err := client.PullRequests.List(ctx, owner, repo, opts)
		if err != nil {
			return nil, err
		}
		all = append(all, prs...)
		if resp.NextPage == 0 {
			break
		}
		opts.Page = resp.NextPage
	}
	return all, nil
}

// ---------------------------------------------------------------------------
// Errors and rate limits — typed failures every call can return
// ---------------------------------------------------------------------------

// ClassifyGitHubError showcases the error types go-github returns:
// *github.ErrorResponse for any non-2xx (with the parsed status code),
// *github.RateLimitError when the primary limit is exhausted, and
// *github.AbuseRateLimitError with a RetryAfter hint for secondary limits.
// This is where update-gitops decides between "retry", "back off", and
// "fail the workflow with a useful message".
func ClassifyGitHubError(err error) string {
	var rateErr *github.RateLimitError
	if errors.As(err, &rateErr) {
		return fmt.Sprintf("rate limited until %s", rateErr.Rate.Reset)
	}

	var abuseErr *github.AbuseRateLimitError
	if errors.As(err, &abuseErr) {
		return fmt.Sprintf("secondary rate limit, retry after %s", abuseErr.GetRetryAfter())
	}

	var ghErr *github.ErrorResponse
	if errors.As(err, &ghErr) {
		switch ghErr.Response.StatusCode {
		case 404:
			return "not found (or token lacks repo access)"
		case 409:
			return "conflict — branch or blob SHA moved, re-read and retry"
		case 422:
			return fmt.Sprintf("rejected: %s", ghErr.Message)
		}
	}
	return err.Error()
}

// CheckRateLimit showcases RateLimit.Get and the Rate metadata that every
// *github.Response also carries (resp.Rate) — cheap to log at the start of a
// run so quota exhaustion in a busy org is visible before it bites.
func CheckRateLimit(ctx context.Context, client *github.Client) error {
	limits, _, err := client.RateLimit.Get(ctx)
	if err != nil {
		return err
	}
	core := limits.GetCore()
	fmt.Printf("rate limit: %d/%d remaining, resets %s\n",
		core.Remaining, core.Limit, core.Reset)
	return nil
}
