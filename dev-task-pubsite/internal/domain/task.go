package domain

import "time"

// Task is a YouTrack task/issue.
type Task struct {
	ID          string    `json:"id"`
	ShortID     string    `json:"shortId"`
	Summary     string    `json:"summary"`
	Description string    `json:"description,omitempty"`
	State       string    `json:"state"`
	Priority    string    `json:"priority"`
	Type        string    `json:"type,omitempty"`
	Assignee    *Person   `json:"assignee,omitempty"`
	Project     string    `json:"project"`
	Sprint      string    `json:"sprint,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
}

// PullRequest is a GitHub pull request.
type PullRequest struct {
	Number      int        `json:"number"`
	Title       string     `json:"title"`
	State       string     `json:"state"`
	Author      string     `json:"author"`
	URL         string     `json:"url"`
	HeadBranch  string     `json:"headBranch"`
	BaseBranch  string     `json:"baseBranch"`
	Draft       bool       `json:"draft,omitempty"`
	CIStatus    string     `json:"ciStatus,omitempty"`
	ReviewState string     `json:"reviewState,omitempty"`
	DiffURL     string     `json:"diffUrl,omitempty"`
	Created     time.Time  `json:"created"`
	Updated     time.Time  `json:"updated"`
	MergedAt    *time.Time `json:"mergedAt,omitempty"`
}

// Issue is a GitHub issue.
type Issue struct {
	Number   int       `json:"number"`
	Title    string    `json:"title"`
	State    string    `json:"state"`
	Author   string    `json:"author"`
	URL      string    `json:"url"`
	Labels   []string  `json:"labels,omitempty"`
	Assignee string    `json:"assignee,omitempty"`
	Body     string    `json:"body,omitempty"`
	Created  time.Time `json:"created"`
	Updated  time.Time `json:"updated"`
}

// Sprint is a YouTrack sprint with aggregated state counts.
type Sprint struct {
	ID             string         `json:"id"`
	Name           string         `json:"name"`
	Goal           string         `json:"goal,omitempty"`
	Start          *time.Time     `json:"start,omitempty"`
	End            *time.Time     `json:"end,omitempty"`
	Completed      bool           `json:"completed"`
	IssueCount     int            `json:"issueCount"`
	StateBreakdown map[string]int `json:"stateBreakdown,omitempty"`
	Tasks          []Task         `json:"tasks,omitempty"`
}

// RepoStats holds aggregated GitHub repository statistics.
type RepoStats struct {
	Owner         string `json:"owner"`
	Repo          string `json:"repo"`
	Stars         int    `json:"stars"`
	Forks         int    `json:"forks"`
	OpenIssues    int    `json:"openIssues"`
	OpenPRs       int    `json:"openPRs"`
	DefaultBranch string `json:"defaultBranch"`
	Language      string `json:"language,omitempty"`
}

// CommitDiff holds a GitHub commit with its file-level diff.
type CommitDiff struct {
	SHA          string        `json:"sha"`
	Message      string        `json:"message"`
	Author       string        `json:"author"`
	Date         time.Time     `json:"date"`
	URL          string        `json:"url"`
	FilesChanged int           `json:"filesChanged"`
	Additions    int           `json:"additions"`
	Deletions    int           `json:"deletions"`
	Files        []ChangedFile `json:"files,omitempty"`
}

// ChangedFile is one file in a CommitDiff.
type ChangedFile struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Patch     string `json:"patch,omitempty"`
}

// Person holds basic user identity.
type Person struct {
	Login    string `json:"login"`
	FullName string `json:"fullName,omitempty"`
}

// ProviderStatus carries resilience metadata in every tool response.
type ProviderStatus struct {
	Source   string `json:"source"`
	Degraded bool   `json:"degraded"`
	StaleAge string `json:"staleAge,omitempty"`
}
