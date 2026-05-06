package tools

// longtask.go demonstrates LONG-RUNNING TOOLS via IsLongRunning: true in functiontool.Config.
//
// What IsLongRunning does:
//
//   1. ADK appends a note to the tool's description in the LLM's JSON schema:
//      "NOTE: This is a long-running operation. Do not call this tool again
//       if it has already returned some intermediate or pending status."
//
//   2. The tool handler is expected to return immediately with a "pending" or
//      "in-progress" status (not block until completion).
//
//   3. The tool writes the task's ID or progress to session state so a
//      subsequent poll call (or the same tool with a task_id argument) can
//      check status without re-starting the operation.
//
// Pattern:
//
//	start_report (IsLongRunning) → returns {status: "PENDING", task_id: "abc123"}
//	check_report                 → reads state[task_id] → returns {status: "DONE", result: ...}
//
// The LLM understands from the description that it should NOT call start_report
// again once it receives a PENDING response, and instead call check_report to poll.
//
// Use long-running tools when:
//   - The operation takes longer than a typical LLM round-trip (>5s)
//   - You want to start work asynchronously and check later
//   - The tool triggers an external job (e.g., a Cloud Run job, Pub/Sub message)

import (
	"fmt"
	"math/rand/v2"
	"strings"
	"time"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ── start_report (long-running) ───────────────────────────────────────────────

type startReportArgs struct {
	Topic string `json:"topic" jsonschema:"The topic to generate a report on."`
	// Depth controls report thoroughness. Optional: defaults to "standard".
	Depth string `json:"depth,omitempty" jsonschema:"Report depth: 'quick', 'standard', or 'deep'. Default: standard."`
}

type startReportResult struct {
	Status  string `json:"status"`  // PENDING | ALREADY_RUNNING
	TaskID  string `json:"task_id"` // identifier to poll with check_report
	Message string `json:"message"`
}

// startReport initiates a simulated long-running report generation task.
// It returns immediately with PENDING status — it does NOT block.
// Progress is stored in session state under the task ID.
//
// In a real implementation this would submit a job to a queue or background
// worker and return the job ID for polling.
func startReport(ctx tool.Context, args startReportArgs) (startReportResult, error) {
	depth := args.Depth
	if depth == "" {
		depth = "standard"
	}

	// Check if a report for this topic is already running (idempotency guard).
	// Long-running tools must be idempotent: the LLM may call them again
	// after a context reset, so check state before re-starting work.
	runningKey := fmt.Sprintf("report:running:%s", strings.ToLower(args.Topic))
	if v, err := ctx.State().Get(runningKey); err == nil && v != nil {
		taskID, _ := v.(string)
		return startReportResult{
			Status:  "ALREADY_RUNNING",
			TaskID:  taskID,
			Message: fmt.Sprintf("Report for %q is already running as task %s. Use check_report to poll.", args.Topic, taskID),
		}, nil
	}

	// Generate a short random task ID.
	taskID := fmt.Sprintf("rpt-%06d", rand.IntN(1_000_000)) //nolint:gosec

	// Store task metadata in session state.
	// The check_report tool reads these keys to report progress.
	_ = ctx.State().Set(runningKey, taskID)
	_ = ctx.State().Set(fmt.Sprintf("report:topic:%s", taskID), args.Topic)
	_ = ctx.State().Set(fmt.Sprintf("report:depth:%s", taskID), depth)
	_ = ctx.State().Set(fmt.Sprintf("report:started:%s", taskID), time.Now().UTC().Format(time.RFC3339))
	_ = ctx.State().Set(fmt.Sprintf("report:status:%s", taskID), "PENDING")

	// Return immediately — the actual work happens asynchronously.
	// In a real system: publish to Pub/Sub, submit a Cloud Run Job, etc.
	return startReportResult{
		Status:  "PENDING",
		TaskID:  taskID,
		Message: fmt.Sprintf("Report generation started for %q (depth: %s). Task ID: %s. Call check_report with this task_id to poll for completion.", args.Topic, depth, taskID),
	}, nil
}

// NewStartReportTool creates the start_report FunctionTool with IsLongRunning: true.
// ADK adds a "do not re-call if already pending" note to its LLM description.
func NewStartReportTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "start_report",
		Description: "Starts a long-running report generation task on a given topic. Returns immediately with a task_id and PENDING status. Poll for completion using check_report.",
		IsLongRunning: true, // ← marks this as a long-running tool in the JSON schema
	}, startReport)
	if err != nil {
		panic(fmt.Sprintf("NewStartReportTool: %v", err))
	}
	return t
}

// ── check_report (regular poll tool) ─────────────────────────────────────────

type checkReportArgs struct {
	TaskID string `json:"task_id" jsonschema:"The task ID returned by start_report."`
}

type checkReportResult struct {
	Status   string `json:"status"`    // PENDING | IN_PROGRESS | DONE | NOT_FOUND
	TaskID   string `json:"task_id"`
	Topic    string `json:"topic,omitempty"`
	Progress int    `json:"progress,omitempty"` // 0–100
	Summary  string `json:"summary,omitempty"`  // populated when DONE
	Message  string `json:"message"`
}

// checkReport polls the status of a previously started report task.
// It reads task metadata from session state and simulates progress
// by advancing the state on each call.
//
// In a real system this would query the job queue or database for status.
func checkReport(ctx tool.Context, args checkReportArgs) (checkReportResult, error) {
	statusKey := fmt.Sprintf("report:status:%s", args.TaskID)
	topicKey := fmt.Sprintf("report:topic:%s", args.TaskID)
	progressKey := fmt.Sprintf("report:progress:%s", args.TaskID)

	statusRaw, err := ctx.State().Get(statusKey)
	if err != nil || statusRaw == nil {
		return checkReportResult{
			Status:  "NOT_FOUND",
			TaskID:  args.TaskID,
			Message: fmt.Sprintf("No task found with ID %q. Did you call start_report first?", args.TaskID),
		}, nil
	}

	topic, _ := ctx.State().Get(topicKey)
	topicStr, _ := topic.(string)

	// Simulate incremental progress: each check_report call advances by 40%.
	progressRaw, _ := ctx.State().Get(progressKey)
	progress, _ := progressRaw.(int)
	progress += 40
	if progress > 100 {
		progress = 100
	}
	_ = ctx.State().Set(progressKey, progress)

	if progress >= 100 {
		// Mark complete; clean up the running key so re-start is allowed.
		_ = ctx.State().Set(statusKey, "DONE")
		runningKey := fmt.Sprintf("report:running:%s", strings.ToLower(topicStr))
		_ = ctx.State().Set(runningKey, nil)

		return checkReportResult{
			Status:   "DONE",
			TaskID:   args.TaskID,
			Topic:    topicStr,
			Progress: 100,
			Summary:  fmt.Sprintf("Report on %q is complete. [Simulated summary: key findings, trends, and recommendations for %s.]", topicStr, topicStr),
			Message:  fmt.Sprintf("Report %q is finished. See summary field.", args.TaskID),
		}, nil
	}

	_ = ctx.State().Set(statusKey, "IN_PROGRESS")
	return checkReportResult{
		Status:   "IN_PROGRESS",
		TaskID:   args.TaskID,
		Topic:    topicStr,
		Progress: progress,
		Message:  fmt.Sprintf("Report %q is %d%% complete. Call check_report again to poll.", args.TaskID, progress),
	}, nil
}

// NewCheckReportTool creates the check_report FunctionTool (not long-running).
func NewCheckReportTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "check_report",
		Description: "Polls the status of a report generation task started by start_report. Returns progress (0-100) and the final summary when complete.",
	}, checkReport)
	if err != nil {
		panic(fmt.Sprintf("NewCheckReportTool: %v", err))
	}
	return t
}
