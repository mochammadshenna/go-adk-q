package yt

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/msw/dev-task-pubsite/internal/cache"
	"github.com/msw/dev-task-pubsite/internal/domain"
)

const (
	maxFailures      = 5
	cooldownDuration = 120 * time.Second
	cacheTTL         = 5 * time.Minute
	requestTimeout   = 15 * time.Second
	maxResults       = 200
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
		slog.Warn("youtrack circuit breaker opened", "cooldown", cooldownDuration)
	}
	cb.mu.Unlock()
}

// Client wraps the YouTrack REST API with a circuit breaker and TTL cache.
type Client struct {
	baseURL    string
	token      string
	httpClient *http.Client
	cb         circuitBreaker
	cache      *cache.Cache
}

// New creates a Client. Returns an error if baseURL or token is empty.
func New(baseURL, token string) (*Client, error) {
	if baseURL == "" || token == "" {
		return nil, fmt.Errorf("YOUTRACK_URL and YOUTRACK_TOKEN must be set")
	}
	return &Client{
		baseURL:    strings.TrimRight(baseURL, "/"),
		token:      token,
		httpClient: &http.Client{Timeout: requestTimeout},
		cache:      cache.New(cacheTTL),
	}, nil
}

// ---- internal HTTP helpers --------------------------------------------------

// issueFields is the fields projection sent on every issue request.
const issueFields = "id,idReadable,summary,description,state(name),priority(name),type(name),assignee(login,fullName),created,updated,project(id,name,shortName),customFields(name,value(name))"

type namedValue struct {
	Name string `json:"name"`
}

type ytUser struct {
	Login    string `json:"login"`
	FullName string `json:"fullName"`
}

type ytProject struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	ShortName string `json:"shortName"`
}

type ytCustomField struct {
	Name  string      `json:"name"`
	Value *namedValue `json:"value"`
}

type ytIssue struct {
	ID           string          `json:"id"`
	IDReadable   string          `json:"idReadable"`
	Summary      string          `json:"summary"`
	Description  string          `json:"description"`
	State        *namedValue     `json:"state"`
	Priority     *namedValue     `json:"priority"`
	Type         *namedValue     `json:"type"`
	Assignee     *ytUser         `json:"assignee"`
	Created      int64           `json:"created"`
	Updated      int64           `json:"updated"`
	Project      *ytProject      `json:"project"`
	CustomFields []ytCustomField `json:"customFields"`
}

func namedStr(n *namedValue) string {
	if n == nil {
		return ""
	}
	return n.Name
}

func (i *ytIssue) toDomain() domain.Task {
	var assignee *domain.Person
	if i.Assignee != nil {
		assignee = &domain.Person{Login: i.Assignee.Login, FullName: i.Assignee.FullName}
	}
	project := ""
	if i.Project != nil {
		project = i.Project.ShortName
	}
	sprint := ""
	for _, cf := range i.CustomFields {
		if strings.EqualFold(cf.Name, "sprint") && cf.Value != nil {
			sprint = cf.Value.Name
			break
		}
	}
	return domain.Task{
		ID:          i.ID,
		ShortID:     i.IDReadable,
		Summary:     i.Summary,
		Description: i.Description,
		State:       namedStr(i.State),
		Priority:    namedStr(i.Priority),
		Type:        namedStr(i.Type),
		Assignee:    assignee,
		Project:     project,
		Sprint:      sprint,
		Created:     time.UnixMilli(i.Created),
		Updated:     time.UnixMilli(i.Updated),
	}
}

func (c *Client) get(ctx context.Context, path string, params url.Values) ([]byte, error) {
	reqURL := c.baseURL + "/api/" + strings.TrimLeft(path, "/")
	if len(params) > 0 {
		reqURL += "?" + params.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtrack %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func (c *Client) post(ctx context.Context, path string, payload []byte) ([]byte, error) {
	reqURL := c.baseURL + "/api/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, reqURL, strings.NewReader(string(payload)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("youtrack %s: %d %s", path, resp.StatusCode, strings.TrimSpace(string(body)))
	}
	return body, nil
}

// ---- resilience helpers -----------------------------------------------------

func (c *Client) staleStatus(key string) (domain.ProviderStatus, bool) {
	r := c.cache.Get(key)
	if r.Found {
		return domain.ProviderStatus{Source: "youtrack", Degraded: true, StaleAge: r.Age.Round(time.Second).String()}, true
	}
	return domain.ProviderStatus{Source: "youtrack", Degraded: true}, false
}

// ---- public API -------------------------------------------------------------

// TaskListOpts filters for ListTasks.
type TaskListOpts struct {
	Project string
	Sprint  string
	State   string
	Limit   int
}

// ListTasks returns tasks matching opts. If YouTrack is unreachable, stale
// cached results are returned with Degraded:true. If no cache exists, an
// error is returned.
func (c *Client) ListTasks(ctx context.Context, opts TaskListOpts) ([]domain.Task, domain.ProviderStatus, error) {
	cacheKey := fmt.Sprintf("tasks:%s:%s:%s", opts.Project, opts.Sprint, opts.State)

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.Task), status, nil
		}
		return nil, status, fmt.Errorf("youtrack unavailable (circuit open, no cache)")
	}

	limit := opts.Limit
	if limit <= 0 || limit > maxResults {
		limit = maxResults
	}

	query := buildQuery(opts.Project, opts.Sprint, opts.State)
	params := url.Values{
		"fields": {issueFields},
		"query":  {query},
		"$top":   {fmt.Sprintf("%d", limit)},
	}

	body, err := c.get(ctx, "issues", params)
	if err != nil {
		c.cb.failure()
		slog.Warn("youtrack list_tasks failed", "error", err)
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.Task), status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack unavailable: %w", err)
	}
	c.cb.success()

	var raw []ytIssue
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack parse error: %w", err)
	}

	tasks := make([]domain.Task, len(raw))
	for i, r := range raw {
		tasks[i] = r.toDomain()
	}
	c.cache.Set(cacheKey, tasks)
	return tasks, domain.ProviderStatus{Source: "youtrack"}, nil
}

// SearchTasks performs a free-text YouTrack query.
func (c *Client) SearchTasks(ctx context.Context, query string) ([]domain.Task, domain.ProviderStatus, error) {
	cacheKey := "search:" + query

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.Task), status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack", Degraded: true}, fmt.Errorf("youtrack unavailable (circuit open, no cache)")
	}

	params := url.Values{
		"fields": {issueFields},
		"query":  {query},
		"$top":   {fmt.Sprintf("%d", maxResults)},
	}

	body, err := c.get(ctx, "issues", params)
	if err != nil {
		c.cb.failure()
		slog.Warn("youtrack search_tasks failed", "error", err)
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			return r.Value.([]domain.Task), status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack search failed: %w", err)
	}
	c.cb.success()

	var raw []ytIssue
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack parse error: %w", err)
	}

	tasks := make([]domain.Task, len(raw))
	for i, r := range raw {
		tasks[i] = r.toDomain()
	}
	c.cache.Set(cacheKey, tasks)
	return tasks, domain.ProviderStatus{Source: "youtrack"}, nil
}

// GetTask fetches a single task by ID or readable ID (e.g. "PROJ-123").
func (c *Client) GetTask(ctx context.Context, id string) (*domain.Task, domain.ProviderStatus, error) {
	cacheKey := "task:" + id

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			t := r.Value.(domain.Task)
			return &t, status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack", Degraded: true}, fmt.Errorf("youtrack unavailable (circuit open, no cache)")
	}

	params := url.Values{"fields": {issueFields}}
	body, err := c.get(ctx, "issues/"+url.PathEscape(id), params)
	if err != nil {
		c.cb.failure()
		slog.Warn("youtrack get_task failed", "id", id, "error", err)
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			t := r.Value.(domain.Task)
			return &t, status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack unavailable: %w", err)
	}
	c.cb.success()

	var raw ytIssue
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack parse error: %w", err)
	}
	t := raw.toDomain()
	c.cache.Set(cacheKey, t)
	return &t, domain.ProviderStatus{Source: "youtrack"}, nil
}

// CreateTaskReq holds fields for creating a YouTrack task.
type CreateTaskReq struct {
	ProjectID   string
	Summary     string
	Description string
	Priority    string
	Type        string
}

// CreateTask creates a new issue in YouTrack.
func (c *Client) CreateTask(ctx context.Context, req CreateTaskReq) (*domain.Task, domain.ProviderStatus, error) {
	if req.Summary == "" || req.ProjectID == "" {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("summary and project_id are required")
	}
	if !c.cb.allow() {
		return nil, domain.ProviderStatus{Source: "youtrack", Degraded: true}, fmt.Errorf("youtrack unavailable (circuit open)")
	}

	type createBody struct {
		Summary     string         `json:"summary"`
		Description string         `json:"description,omitempty"`
		Project     map[string]any `json:"project"`
		Priority    *namedValue    `json:"priority,omitempty"`
		Type        *namedValue    `json:"type,omitempty"`
	}
	reqBody := createBody{
		Summary:     req.Summary,
		Description: req.Description,
		Project:     map[string]any{"id": req.ProjectID},
	}
	if req.Priority != "" {
		reqBody.Priority = &namedValue{Name: req.Priority}
	}
	if req.Type != "" {
		reqBody.Type = &namedValue{Name: req.Type}
	}

	payload, err := json.Marshal(reqBody)
	if err != nil {
		return nil, domain.ProviderStatus{Source: "youtrack"}, err
	}

	resp, err := c.post(ctx, "issues?fields="+issueFields, payload)
	if err != nil {
		c.cb.failure()
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack create failed: %w", err)
	}
	c.cb.success()

	var raw ytIssue
	if err := json.Unmarshal(resp, &raw); err != nil {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack parse error: %w", err)
	}
	t := raw.toDomain()
	return &t, domain.ProviderStatus{Source: "youtrack"}, nil
}

// GetSprintSummary returns the summary for a sprint. If sprintName is empty,
// the active sprint is used via YouTrack query language.
func (c *Client) GetSprintSummary(ctx context.Context, projectID, sprintName string) (*domain.Sprint, domain.ProviderStatus, error) {
	if projectID == "" {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("project_id is required")
	}

	sprintFilter := "Sprint: {Active sprint}"
	if sprintName != "" {
		sprintFilter = fmt.Sprintf("Sprint: {%s}", sprintName)
	}
	query := fmt.Sprintf("project: %s %s", projectID, sprintFilter)
	cacheKey := "sprint:" + projectID + ":" + sprintName

	if !c.cb.allow() {
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			s := r.Value.(domain.Sprint)
			return &s, status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack", Degraded: true}, fmt.Errorf("youtrack unavailable (circuit open, no cache)")
	}

	params := url.Values{
		"fields": {"id,idReadable,summary,state(name),priority(name),customFields(name,value(name))"},
		"query":  {query},
		"$top":   {"500"},
	}

	body, err := c.get(ctx, "issues", params)
	if err != nil {
		c.cb.failure()
		status, hasCached := c.staleStatus(cacheKey)
		if hasCached {
			r := c.cache.Get(cacheKey)
			s := r.Value.(domain.Sprint)
			return &s, status, nil
		}
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack unavailable: %w", err)
	}
	c.cb.success()

	var raw []ytIssue
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, domain.ProviderStatus{Source: "youtrack"}, fmt.Errorf("youtrack parse error: %w", err)
	}

	breakdown := make(map[string]int)
	tasks := make([]domain.Task, len(raw))
	for i, r := range raw {
		tasks[i] = r.toDomain()
		breakdown[tasks[i].State]++
	}

	resolvedSprintName := sprintName
	if resolvedSprintName == "" {
		resolvedSprintName = "Active Sprint"
	}
	sprint := domain.Sprint{
		Name:           resolvedSprintName,
		IssueCount:     len(tasks),
		StateBreakdown: breakdown,
		Tasks:          tasks,
	}
	c.cache.Set(cacheKey, sprint)
	return &sprint, domain.ProviderStatus{Source: "youtrack"}, nil
}

// buildQuery constructs a YouTrack query string from optional filters.
func buildQuery(project, sprint, state string) string {
	var parts []string
	if project != "" {
		parts = append(parts, "project: "+project)
	}
	if sprint != "" {
		parts = append(parts, fmt.Sprintf("Sprint: {%s}", sprint))
	}
	if state != "" {
		parts = append(parts, fmt.Sprintf("State: {%s}", state))
	}
	return strings.Join(parts, " ")
}
