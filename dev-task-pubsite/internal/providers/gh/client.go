package gh

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/go-github/v68/github"
	"github.com/msw/dev-task-pubsite/internal/cache"
	"github.com/msw/dev-task-pubsite/internal/domain"
)

const (
	maxFailures      = 5
	cooldownDuration = 120 * time.Second
	cacheTTL         = 5 * time.Minute
	maxPageSize      = 100
)

// circuitBreaker trips after maxFailures consecutive errors.
type circuitBreaker struct {
	mu        sync.Mutex
	failures  int
	openUntil time.Time
}

func (cb *circuitBreaker) allow() bool {
	cb.mu.Lock()
	defer cb.mu.Unlock()
	return time.Now().After(cb.openUntil)
}

func (cb *circuitBreaker) success() {
	cb.mu.Lock()
	cb.failures = 0
	cb.mu.Unlock()
}

func (cb *circuitBreaker) failure() {
	cb.mu.Lock()
	cb.failures++
	if cb.failures >= maxFailures {
		cb.openUntil = time.Now().Add(cooldownDuration)
		cb.failures = 0
		slog.Warn("github circuit breaker opened", "cooldown", cooldownDuration)
	}
	cb.mu.Unlock()
}

// Client wraps the GitHub API with a circuit breaker and TTL cache.
type Client struct {
	gh    *github.Client
	cb    circuitBreaker
	cache *cache.Cache
}

// New creates a Client using a personal access token.
// Returns an error if token is empty.
func New(token string) (*Client, error) {
	if token == "" {
		return nil, fmt.Errorf("GITHUB_TOKEN must be set")
	}
	return &Client{
		gh:    github.NewClient(nil).WithAuthToken(token),
		cache: cache.New(cacheTTL),
	}, nil
}

// ---- nil-safe field helpers -------------------------------------------------

func strVal(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func intVal(n *int) int {
	if n == nil {
		return 0
	}
	return *n
}

func boolVal(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

func timeVal(t *github.Timestamp) time.Time {
	if t == nil {
		return time.Time{}
	}
	return t.Time
}

func timePtrVal(t *github.Timestamp) *time.Time {
	if t == nil {
		return nil
	}
	tt := t.Time
	return &tt
}

func userLogin(u *github.User) string {
	if u == nil {
		return ""
	}
	return strVal(u.Login)
}

// ---- resilience helpers -----------------------------------------------------

func (c *Client) staleStatus(key string) (domain.ProviderStatus, bool) {
	r := c.cache.Get(key)
	if r.Found {
		return domain.ProviderStatus{Source: "github", Degraded: true, StaleAge: r.Age.Round(time.Second).String()}, true
	}
	return domain.ProviderStatus{Source: "github", Degraded: true}, false
}

// ---- PR methods -------------------------------------------------------------

// PRListOpts filters for ListPRs.
type PRListOpts struct {
	State string // "open", "closed", "all"
}

// ListPRs returns pull requests for owner/repo. On failure, serves stale cache.
func (c *Client) ListPRs(ctx context.Context, owner, repo string, opts PRListOpts) ([]domain.PullRequest, domain.ProviderStatus, error) {
	state := opts.State
	if state == "" {
		state = "open"
	}
	cacheKey := fmt.Sprintf("prs:%s/%s:%s", owner, repo, state)

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.PullRequest), status, nil
		}
		return nil, domain.ProviderStatus{Source: "github", Degraded: true}, fmt.Errorf("github unavailable (circuit open, no cache)")
	}

	rawPRs, _, err := c.gh.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State:       state,
		ListOptions: github.ListOptions{PerPage: maxPageSize},
	})
	if err != nil {
		c.cb.failure()
		slog.Warn("github list_prs failed", "owner", owner, "repo", repo, "error", err)
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.PullRequest), status, nil
		}
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("github unavailable: %w", err)
	}
	c.cb.success()

	prs := make([]domain.PullRequest, 0, len(rawPRs))
	for _, p := range rawPRs {
		if p == nil {
			continue
		}
		head, base := "", ""
		if p.Head != nil {
			head = strVal(p.Head.Ref)
		}
		if p.Base != nil {
			base = strVal(p.Base.Ref)
		}
		prs = append(prs, domain.PullRequest{
			Number:     intVal(p.Number),
			Title:      strVal(p.Title),
			State:      strVal(p.State),
			Author:     userLogin(p.User),
			URL:        strVal(p.HTMLURL),
			HeadBranch: head,
			BaseBranch: base,
			Draft:      boolVal(p.Draft),
			DiffURL:    strVal(p.DiffURL),
			Created:    timeVal(p.CreatedAt),
			Updated:    timeVal(p.UpdatedAt),
			MergedAt:   timePtrVal(p.MergedAt),
		})
	}
	c.cache.Set(cacheKey, prs)
	return prs, domain.ProviderStatus{Source: "github"}, nil
}

// GetPR returns a single pull request with review and CI status.
func (c *Client) GetPR(ctx context.Context, owner, repo string, number int) (*domain.PullRequest, domain.ProviderStatus, error) {
	cacheKey := fmt.Sprintf("pr:%s/%s:%d", owner, repo, number)

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			p := r.Value.(domain.PullRequest)
			return &p, status, nil
		}
		return nil, domain.ProviderStatus{Source: "github", Degraded: true}, fmt.Errorf("github unavailable (circuit open, no cache)")
	}

	p, _, err := c.gh.PullRequests.Get(ctx, owner, repo, number)
	if err != nil {
		c.cb.failure()
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			pr := r.Value.(domain.PullRequest)
			return &pr, status, nil
		}
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("github unavailable: %w", err)
	}

	// Fetch combined CI status.
	ciStatus := ""
	headSHA := ""
	if p.Head != nil {
		headSHA = strVal(p.Head.SHA)
	}
	if headSHA != "" {
		combined, _, csErr := c.gh.Repositories.GetCombinedStatus(ctx, owner, repo, headSHA, nil)
		if csErr == nil && combined != nil {
			ciStatus = strVal(combined.State)
		}
	}

	// Fetch latest review state.
	reviewState := ""
	reviews, _, rvErr := c.gh.PullRequests.ListReviews(ctx, owner, repo, number, nil)
	if rvErr == nil && len(reviews) > 0 {
		last := reviews[len(reviews)-1]
		if last != nil {
			reviewState = strVal(last.State)
		}
	}

	c.cb.success()

	head, base := "", ""
	if p.Head != nil {
		head = strVal(p.Head.Ref)
	}
	if p.Base != nil {
		base = strVal(p.Base.Ref)
	}

	pr := domain.PullRequest{
		Number:      intVal(p.Number),
		Title:       strVal(p.Title),
		State:       strVal(p.State),
		Author:      userLogin(p.User),
		URL:         strVal(p.HTMLURL),
		HeadBranch:  head,
		BaseBranch:  base,
		Draft:       boolVal(p.Draft),
		CIStatus:    ciStatus,
		ReviewState: reviewState,
		DiffURL:     strVal(p.DiffURL),
		Created:     timeVal(p.CreatedAt),
		Updated:     timeVal(p.UpdatedAt),
		MergedAt:    timePtrVal(p.MergedAt),
	}
	c.cache.Set(cacheKey, pr)
	return &pr, domain.ProviderStatus{Source: "github"}, nil
}

// ---- Issue methods ----------------------------------------------------------

// IssueListOpts filters for ListIssues.
type IssueListOpts struct {
	State  string
	Labels []string
}

// ListIssues returns GitHub issues for owner/repo.
func (c *Client) ListIssues(ctx context.Context, owner, repo string, opts IssueListOpts) ([]domain.Issue, domain.ProviderStatus, error) {
	state := opts.State
	if state == "" {
		state = "open"
	}
	labelStr := fmt.Sprintf("%v", opts.Labels)
	cacheKey := fmt.Sprintf("issues:%s/%s:%s:%s", owner, repo, state, labelStr)

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.Issue), status, nil
		}
		return nil, domain.ProviderStatus{Source: "github", Degraded: true}, fmt.Errorf("github unavailable (circuit open, no cache)")
	}

	rawIssues, _, err := c.gh.Issues.ListByRepo(ctx, owner, repo, &github.IssueListByRepoOptions{
		State:       state,
		Labels:      opts.Labels,
		ListOptions: github.ListOptions{PerPage: maxPageSize},
	})
	if err != nil {
		c.cb.failure()
		slog.Warn("github list_issues failed", "owner", owner, "repo", repo, "error", err)
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.Issue), status, nil
		}
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("github unavailable: %w", err)
	}
	c.cb.success()

	issues := make([]domain.Issue, 0, len(rawIssues))
	for _, i := range rawIssues {
		if i == nil || i.PullRequestLinks != nil {
			continue // skip PRs listed as issues by GitHub API
		}
		labels := make([]string, 0, len(i.Labels))
		for _, l := range i.Labels {
			if l != nil {
				labels = append(labels, strVal(l.Name))
			}
		}
		issues = append(issues, domain.Issue{
			Number:   intVal(i.Number),
			Title:    strVal(i.Title),
			State:    strVal(i.State),
			Author:   userLogin(i.User),
			URL:      strVal(i.HTMLURL),
			Labels:   labels,
			Assignee: userLogin(i.Assignee),
			Body:     strVal(i.Body),
			Created:  timeVal(i.CreatedAt),
			Updated:  timeVal(i.UpdatedAt),
		})
	}
	c.cache.Set(cacheKey, issues)
	return issues, domain.ProviderStatus{Source: "github"}, nil
}

// CreateIssueReq holds fields for creating a GitHub issue.
type CreateIssueReq struct {
	Title    string
	Body     string
	Labels   []string
	Assignee string
}

// CreateIssue creates a new GitHub issue.
func (c *Client) CreateIssue(ctx context.Context, owner, repo string, req CreateIssueReq) (*domain.Issue, domain.ProviderStatus, error) {
	if req.Title == "" {
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("title is required")
	}
	if !c.cb.allow() {
		return nil, domain.ProviderStatus{Source: "github", Degraded: true}, fmt.Errorf("github unavailable (circuit open)")
	}

	ir := &github.IssueRequest{
		Title:  &req.Title,
		Body:   &req.Body,
		Labels: &req.Labels,
	}
	if req.Assignee != "" {
		ir.Assignee = &req.Assignee
	}

	raw, _, err := c.gh.Issues.Create(ctx, owner, repo, ir)
	if err != nil {
		c.cb.failure()
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("github create_issue failed: %w", err)
	}
	c.cb.success()

	labels := make([]string, 0, len(raw.Labels))
	for _, l := range raw.Labels {
		if l != nil {
			labels = append(labels, strVal(l.Name))
		}
	}
	issue := domain.Issue{
		Number:   intVal(raw.Number),
		Title:    strVal(raw.Title),
		State:    strVal(raw.State),
		Author:   userLogin(raw.User),
		URL:      strVal(raw.HTMLURL),
		Labels:   labels,
		Assignee: userLogin(raw.Assignee),
		Body:     strVal(raw.Body),
		Created:  timeVal(raw.CreatedAt),
		Updated:  timeVal(raw.UpdatedAt),
	}
	return &issue, domain.ProviderStatus{Source: "github"}, nil
}

// ---- Repo methods -----------------------------------------------------------

// GetRepoStats returns aggregated statistics for owner/repo.
func (c *Client) GetRepoStats(ctx context.Context, owner, repo string) (*domain.RepoStats, domain.ProviderStatus, error) {
	cacheKey := fmt.Sprintf("stats:%s/%s", owner, repo)

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			s := r.Value.(domain.RepoStats)
			return &s, status, nil
		}
		return nil, domain.ProviderStatus{Source: "github", Degraded: true}, fmt.Errorf("github unavailable (circuit open, no cache)")
	}

	repoData, _, err := c.gh.Repositories.Get(ctx, owner, repo)
	if err != nil {
		c.cb.failure()
		slog.Warn("github get_repo failed", "owner", owner, "repo", repo, "error", err)
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			s := r.Value.(domain.RepoStats)
			return &s, status, nil
		}
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("github unavailable: %w", err)
	}

	// Count open PRs separately — OpenIssuesCount includes PRs.
	openPRCount := 0
	prs, _, prErr := c.gh.PullRequests.List(ctx, owner, repo, &github.PullRequestListOptions{
		State:       "open",
		ListOptions: github.ListOptions{PerPage: 1},
	})
	if prErr == nil {
		openPRCount = len(prs)
	}

	c.cb.success()

	openIssues := intVal(repoData.OpenIssuesCount)
	if openIssues > openPRCount {
		openIssues -= openPRCount
	}

	stats := domain.RepoStats{
		Owner:         owner,
		Repo:          repo,
		Stars:         intVal(repoData.StargazersCount),
		Forks:         intVal(repoData.ForksCount),
		OpenIssues:    openIssues,
		OpenPRs:       openPRCount,
		DefaultBranch: strVal(repoData.DefaultBranch),
		Language:      strVal(repoData.Language),
	}
	c.cache.Set(cacheKey, stats)
	return &stats, domain.ProviderStatus{Source: "github"}, nil
}

// GetCommitDiff returns a commit with its per-file diff.
func (c *Client) GetCommitDiff(ctx context.Context, owner, repo, sha string) (*domain.CommitDiff, domain.ProviderStatus, error) {
	if sha == "" {
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("sha is required")
	}
	cacheKey := fmt.Sprintf("commit:%s/%s:%s", owner, repo, sha)

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			cd := r.Value.(domain.CommitDiff)
			return &cd, status, nil
		}
		return nil, domain.ProviderStatus{Source: "github", Degraded: true}, fmt.Errorf("github unavailable (circuit open, no cache)")
	}

	rc, _, err := c.gh.Repositories.GetCommit(ctx, owner, repo, sha, nil)
	if err != nil {
		c.cb.failure()
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			cd := r.Value.(domain.CommitDiff)
			return &cd, status, nil
		}
		return nil, domain.ProviderStatus{Source: "github"}, fmt.Errorf("github unavailable: %w", err)
	}
	c.cb.success()

	author := ""
	date := time.Time{}
	message := ""
	if rc.Commit != nil {
		message = strVal(rc.Commit.Message)
		if rc.Commit.Author != nil {
			author = strVal(rc.Commit.Author.Name)
			if rc.Commit.Author.Date != nil {
				date = rc.Commit.Author.Date.Time
			}
		}
	}

	files := make([]domain.ChangedFile, 0, len(rc.Files))
	for _, f := range rc.Files {
		if f == nil {
			continue
		}
		files = append(files, domain.ChangedFile{
			Filename:  strVal(f.Filename),
			Status:    strVal(f.Status),
			Additions: intVal(f.Additions),
			Deletions: intVal(f.Deletions),
			Patch:     strVal(f.Patch),
		})
	}

	additions, deletions := 0, 0
	if rc.Stats != nil {
		additions = intVal(rc.Stats.Additions)
		deletions = intVal(rc.Stats.Deletions)
	}

	cd := domain.CommitDiff{
		SHA:          strVal(rc.SHA),
		Message:      message,
		Author:       author,
		Date:         date,
		URL:          strVal(rc.HTMLURL),
		FilesChanged: len(files),
		Additions:    additions,
		Deletions:    deletions,
		Files:        files,
	}
	c.cache.Set(cacheKey, cd)
	return &cd, domain.ProviderStatus{Source: "github"}, nil
}
