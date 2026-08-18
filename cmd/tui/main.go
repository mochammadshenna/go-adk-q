// Package main is the entry point for the go-adk-q Bubbletea TUI.
//
// It exposes two Cobra subcommands:
//
//	tui chat        – interactive Bubbletea chat UI
//	tui run <msg>   – one-shot: send a message and print the response
//
// Both subcommands share the same multi-provider ADK agent and failover chain
// defined in buildRunner. Environment variables are identical to the main binary.
package main

import (
	"context"
	"fmt"
	"log"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	localskilltoolset "go-adk-q/tool/skilltoolset"

	"github.com/spf13/cobra"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
	"google.golang.org/adk/tool/loadartifactstool"
	"google.golang.org/adk/tool/loadmemorytool"
	"google.golang.org/adk/tool/preloadmemorytool"
	"google.golang.org/adk/tool/skilltoolset/skill"
	"google.golang.org/genai"

	"go-adk-q/agents"
	"go-adk-q/model/catalog"
	"go-adk-q/model/chain"
	"go-adk-q/model/failover"
	"go-adk-q/model/githubmodels"
	"go-adk-q/model/groq"
	"go-adk-q/model/huggingface"
	"go-adk-q/model/nvidia"
	"go-adk-q/model/opencode"
	"go-adk-q/model/openrouter"
	"go-adk-q/tools"
)

const appName = "go-adk-q-tui"

// agentConfig holds the tool/instruction configuration captured by buildRunner
// so that rebuildRunnerWithModel can construct a new agent for the same setup
// with a different model (used by the /model picker).
var agentConfig struct {
	tools       []tool.Tool
	toolsets    []tool.Toolset
	instruction string
}

func init() {
	// Register all provider catalogs globally so the /model picker can list
	// them without requiring a network call.  Registration order matches the
	// failover priority so the picker shows providers in the same order.
	catalog.Register(githubmodels.KnownModels)
	catalog.Register(catalog.ProviderCatalog{
		Provider: "gemini",
		Label:    "Google Gemini",
		EnvVar:   "GOOGLE_API_KEY",
		Models: []catalog.ModelEntry{
			{ID: "gemini-2.0-flash", Label: "Gemini 2.0 Flash", Tags: []string{"fast"}, Default: true},
			{ID: "gemini-2.5-flash", Label: "Gemini 2.5 Flash", Tags: []string{"fast"}},
			{ID: "gemini-2.5-pro", Label: "Gemini 2.5 Pro"},
			{ID: "gemini-1.5-pro", Label: "Gemini 1.5 Pro", Tags: []string{"long-ctx"}},
			{ID: "gemini-1.5-flash", Label: "Gemini 1.5 Flash", Tags: []string{"fast"}},
		},
	})
	catalog.Register(groq.KnownModels)
	catalog.Register(nvidia.KnownModels)
	catalog.Register(openrouter.KnownModels)
	catalog.Register(opencode.KnownModels)
	catalog.Register(huggingface.KnownModels)
}

func main() {
	loadCredentialsIntoEnv()
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "layar-cli",
	Short: "Bubbletea/Lipgloss TUI for the go-adk-q ADK reference",
	// SilenceUsage: a config/runtime error (e.g. "no model providers
	// configured") is not a flag-parsing mistake — dumping the full
	// command-list usage block on top of it buries the one actionable line.
	// SilenceErrors: main() already prints the returned error to stderr;
	// without this cobra would print it a second time via its own default
	// error handler.
	SilenceUsage:  true,
	SilenceErrors: true,
	Long: `A terminal chat UI powered by Bubbletea and Lipgloss.

Talks to the same multi-provider ADK agent as 'go run . console', but
renders in a beautiful full-terminal layout with scrolling history,
a spinner while the agent thinks, and keyboard shortcuts.

Environment variables (same as the main binary):
  GOOGLE_API_KEY, GROQ_API_KEY, NVIDIA_API_KEY, OPENROUTER_API_KEY, HF_TOKEN

Set at least one to start.`,
	// Bare 'layar-cli' (no subcommand) launches the chat UI directly — matches
	// opencode-ai/opencode's root command, which also runs its TUI from a RunE
	// set directly on the root rather than requiring a 'chat' subcommand.
	// 'layar-cli chat' still works too (same RunE, kept for explicitness/scripts).
	RunE: func(cmd *cobra.Command, args []string) error {
		return chatCmd.RunE(cmd, args)
	},
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start the interactive terminal chat UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		r, sessionSvc, memorySvc, fm, modelName, err := buildRunner(ctx)
		if err != nil {
			return err // buildRunner already wraps this with "setup: "
		}
		return runChat(r, sessionSvc, memorySvc, modelName, fm)
	},
}

var runCmd = &cobra.Command{
	Use:   "run <message>",
	Short: "Send a single message and print the response (no TUI)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		r, _, _, _, _, err := buildRunner(ctx)
		if err != nil {
			return err // buildRunner already wraps this with "setup: "
		}
		return runOnce(ctx, r, args[0])
	},
}

var acpCmd = &cobra.Command{
	Use:   "acp",
	Short: "Run as a real ACP agent over stdio (spec-conformant transport)",
	Long: `Runs go-adk-q as an Agent Client Protocol agent communicating over
stdin/stdout with newline-delimited JSON-RPC 2.0, per
https://agentclientprotocol.com/protocol/v1/transports.md. This is the
transport ACP-compliant clients (e.g. Zed) actually launch agents with —
the HTTP server started by '/acp' inside the interactive chat UI is a
non-spec convenience (ACP's only HTTP option, "Streamable HTTP", is itself
still "in discussion, draft proposal in progress" per the same spec page).

Do not run this manually in an interactive terminal: it expects to be
launched as a subprocess by an ACP client, which owns stdin/stdout for the
lifetime of the connection.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := cmd.Context()
		if ctx == nil {
			ctx = context.Background()
		}
		r, _, _, _, _, err := buildRunner(ctx)
		if err != nil {
			return err // buildRunner already wraps this with "setup: "
		}

		// stdout is the wire protocol itself here — every diagnostic must go
		// to the same log file the interactive TUI uses, never to stderr or
		// stdout, matching the spec's "MAY write to stderr for logging" (we
		// go one step further and avoid stderr too, keeping it free for the
		// client to display raw if it chooses).
		logPath := filepath.Join(os.TempDir(), "go-adk-q-tui.log")
		if lf, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600); err == nil {
			slog.SetDefault(slog.New(slog.NewTextHandler(lf, &slog.HandlerOptions{Level: slog.LevelDebug})))
			log.SetOutput(lf)
			defer lf.Close()
		}

		// stdio is referenced by bridge below before it's assigned — safe
		// because bridge is only ever invoked once stdio.Serve is running,
		// by which point stdio is non-nil. Same forward-reference pattern as
		// acp_stdio_test.go's TestACPStdio_EOFWhileOutboundRequestInFlight_UnblocksViaDone.
		var stdio *acpStdio
		bridge := func(ctx context.Context, input string) (string, error) {
			ctx, cancel := context.WithTimeout(ctx, 120*time.Second)
			defer cancel()
			// Unlike the HTTP-only /acp server (runAgentSync, chat.go), the
			// real stdio transport CAN service an Agent→Client
			// session/request_permission round trip mid-turn, so
			// exec_command's confirmation gate (tools/exec.go) is wired
			// straight through to the ACP client instead of failing.
			onConfirm := func(ctx context.Context, toolCallID, toolName string, args map[string]any) (bool, error) {
				title := "Run " + toolName
				if cmd, ok := args["command"].(string); ok && cmd != "" {
					title = "Run: " + cmd
				}
				outcome, err := stdio.requestPermission(ctx, "acp-session", toolCallUpdate{
					ToolCallID: toolCallID,
					Kind:       "execute",
					Title:      title,
				}, []permissionOption{
					{OptionID: "allow", Name: "Allow", Kind: "allow_once"},
					{OptionID: "reject", Name: "Reject", Kind: "reject_once"},
				})
				if err != nil {
					return false, err
				}
				return outcome.Outcome == "selected" && outcome.OptionID == "allow", nil
			}
			text, _, _, err := runTurnWithConfirmations(ctx, r, "acp-user", "acp-session", input, nil, onConfirm)
			return strings.TrimSpace(text), err
		}
		srv := newACPServer(bridge)
		stdio = newACPStdio(srv)
		return stdio.Serve(ctx, os.Stdin, os.Stdout)
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(runCmd)
	rootCmd.AddCommand(acpCmd)
}

// buildRunner constructs the failover model chain, a root agent with tools,
// and returns an ADK runner ready to use alongside the session and memory
// services needed by the TUI.
//
// The provider chain is built by the shared chain package — the single source
// of truth for provider priority (used by both binaries). This guarantees the
// TUI and the root binary expose the identical provider set, and that
// PROVIDER_SELECTED still re-orders the chain.
//
// chain.Build applies a per-attempt timeout (90s default) so a hung provider
// cannot block the whole request and defeat the fallback safety net.
//
// Tools and the memory/artifact/LLM-auditor integrations are gated on
// GOOGLE_API_KEY because non-Gemini models cannot reliably execute structured
// tool calls (Groq: 400 tool_use_failed; NVIDIA: 500 single-tool-only;
// HuggingFace/OpenRouter: silent JSON mangling).
func buildRunner(ctx context.Context) (*runner.Runner, session.Service, memory.Service, *failover.Model, string, error) {
	m, err := chain.Build(ctx)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("setup: %w", err)
	}
	chainName := m.Name() // full failover chain name, e.g. "failover(github-models/gpt-4o → groq/llama...)"
	slog.Info("model chain", "providers", chainName)

	// ── Tools, skills, memory, artifacts, and LLM Auditor ────────────────────
	//
	// Skills and basic tools (weather, time) work with ANY provider that
	// supports function calling — GitHub Models, OpenRouter, Groq, etc.
	// skilltoolset uses plain functiontool.New internally; it has no Gemini
	// dependency whatsoever.
	//
	// Only llm_auditor + memory/artifact tools remain gated on GOOGLE_API_KEY
	// because those involve multi-step tool chains that non-Gemini providers
	// struggle with (Groq: 400 tool_use_failed on complex chains).
	//
	// Tool inventory (all providers):
	//   weather, time                – FunctionTools (tools/)
	//   list_skills, load_skill,
	//   load_skill_resource          – via SkillToolset (if ./skills/ exists)
	//
	// Additional tools (GOOGLE_API_KEY only):
	//   preload_memory, load_memory  – memory recall across turns
	//   load_artifacts               – access saved artifact files
	//   llm_auditor (agenttool)      – fact-check via critic→reviser pipeline
	var agentTools []tool.Tool
	var agentToolsets []tool.Toolset

	// Basic tools — always enabled for any provider with function calling.
	// read_file/write_file/grep_search/fetch_url are the harness's local/
	// network capabilities: local-only (confined to cwd) or SSRF-guarded, so
	// they carry the same trust posture as the existing attachment reader —
	// safe to expose unconditionally, no GOOGLE_API_KEY gate needed.
	readFileTool := tools.NewReadFileTool()
	writeFileTool := tools.NewWriteFileTool()
	editFileTool := tools.NewEditFileTool()
	grepSearchTool := tools.NewGrepTool()
	fetchURLTool := tools.NewFetchURLTool()
	// execCommandTool requires human approval on every call (ADK's native
	// RequireConfirmation / Human-in-the-Loop flow, wired through the TUI's
	// y/n prompt in cmd/tui/chat.go) — that confirmation gate, not a
	// provider gate, is what makes it safe to expose unconditionally here.
	// See tools/exec.go's package doc comment for the full v1 security scope.
	execCommandTool := tools.NewExecCommandTool()
	agentTools = []tool.Tool{
		tools.NewWeatherTool(),
		tools.NewTimeTool(),
		readFileTool,
		writeFileTool,
		editFileTool,
		grepSearchTool,
		fetchURLTool,
		execCommandTool,
	}

	baseInstruction := "You are layar-cli, an expert AI assistant running in a terminal.\n\n" +
		"## Tools\n" +
		"- get_weather / get_current_time — use for weather and time questions\n" +
		"- read_file / write_file — read or write a file in the working directory\n" +
		"- edit_file — targeted find-and-replace edit to part of an existing file (prefer over write_file for partial changes)\n" +
		"- grep_search — search files for a regular expression\n" +
		"- fetch_url — fetch an http(s) URL\n" +
		"- exec_command — run a shell command; ALWAYS pauses for explicit human " +
		"approval first, so use it freely when it's the right tool, the human " +
		"decides whether it actually runs\n" +
		"- list_skills — call this (no arguments) to discover available skills\n" +
		"- load_skill — call with {\"name\": \"<skill_name>\"} to load a skill's instructions\n\n" +
		"## Skills workflow\n" +
		"If the user's request matches a specialized task (code review, architecture, " +
		"debugging, documentation, etc.), ALWAYS:\n" +
		"1. Call list_skills to see what is available\n" +
		"2. Call load_skill with the best matching skill name\n" +
		"3. Follow the loaded instructions exactly before replying\n\n" +
		"## Style\n" +
		"- Terminal output: prefer short, direct answers\n" +
		"- Use markdown sparingly — fenced code blocks for code, bold for emphasis\n" +
		"- Never pad responses with preamble or sign-offs"

	// Skills toolset — enabled for any provider when ./skills/ dir exists.
	//
	// skill.NewFileSystemSource does not scan recursively (confirmed by
	// reading its source, google.golang.org/adk's skill/filesystem_source.go:
	// "It does not traverse recursively" — fs.ReadDir(".") only, one level)
	// and this repo's skills are nested one level deeper under category
	// folders (skills/engineering/debug/SKILL.md, not skills/debug/SKILL.md),
	// so a single root-level source finds zero skills — a real, previously
	// undetected bug: list_skills always returned 0 despite 44 real SKILL.md
	// files on disk. Fixed by building one FileSystemSource per category
	// subdirectory and combining them with skill.NewMergedSource — no skill
	// files need to move. The root itself is also included so any future
	// skill placed directly under ./skills (no category folder) still works.
	if _, statErr := os.Stat("./skills"); statErr == nil {
		sources := []skill.Source{skill.NewFileSystemSource(os.DirFS("./skills"))}
		if entries, readErr := os.ReadDir("./skills"); readErr == nil {
			for _, e := range entries {
				if e.IsDir() {
					sources = append(sources, skill.NewFileSystemSource(os.DirFS("./skills/"+e.Name())))
				}
			}
		} else {
			slog.Warn("skills category scan failed — only root-level skills (if any) will be found", "error", readErr)
		}
		rawSource := skill.NewMergedSource(sources...)

		preloaded, _, preloadErr := skill.WithCompletePreloadSource(ctx, rawSource)
		if preloadErr != nil {
			slog.Warn("skills preload failed — running without skills", "error", preloadErr)
		} else {
			skillToolset, stErr := localskilltoolset.New(ctx, localskilltoolset.Config{Source: preloaded})
			if stErr != nil {
				slog.Warn("skills toolset init failed — running without skills", "error", stErr)
			} else {
				agentToolsets = append(agentToolsets, skillToolset)
				slog.Info("skills toolset enabled", "path", "./skills", "categories", len(sources)-1)
			}
		}
	}

	// Advanced tools — gated on GOOGLE_API_KEY because llm_auditor and memory
	// tools require reliable multi-step tool chains only Gemini handles well.
	if os.Getenv("GOOGLE_API_KEY") != "" {
		llmAuditor := agents.GetLLMAuditorAgent(ctx, m)
		// search_agent — see agents/search.go's doc comment for why Google
		// Search grounding must live in its own sub-agent rather than on the
		// root agent's own tool list (Gemini API constraint, not a design
		// choice).
		searchAgent := agents.GetSearchAgent(ctx, m)
		// Harness agents — advisor/judge/critique/review/critique_loop — gated
		// here for the same reason as llm_auditor: reliable multi-step
		// tool-calling (review_agent calls read_file/grep_search itself;
		// critique_loop drives a multi-iteration LoopAgent) needs Gemini.
		advisorAgent := agents.GetAdvisorAgent(ctx, m)
		judgeAgent := agents.GetJudgeAgent(ctx, m)
		critiqueAgent := agents.GetCritiqueAgent(ctx, m)
		reviewAgent := agents.GetReviewAgent(ctx, m, readFileTool, grepSearchTool)
		critiqueLoop := agents.GetCritiqueLoopAgent(ctx, m)
		// manager_agent coordinates the 30-role SDLC team (agents/roles.go)
		// plus cto_agent (agents/cto.go). tierA (advisory: read/grep/fetch,
		// no mutation) vs tierB (tierA + write/edit) is the mechanism that
		// keeps roles like cicd_agent/github_pr_agent advisory-only — see
		// agents/roles.go's RoleTier doc comment.
		// exec_command sits in tierB, not tierA: it is at least as risky as
		// write_file/edit_file (arbitrary shell, not just file mutation), so
		// cicd_agent/github_pr_agent (TierAdvisory → tierA) stay without it,
		// same structural-enforcement pattern as the write/edit split above.
		tierA := []tool.Tool{readFileTool, grepSearchTool, fetchURLTool}
		tierB := []tool.Tool{readFileTool, grepSearchTool, fetchURLTool, writeFileTool, editFileTool, execCommandTool}
		managerAgent := agents.GetManagerAgent(ctx, m, tierA, tierB)
		agentTools = append(agentTools,
			preloadmemorytool.New(),
			loadmemorytool.New(),
			loadartifactstool.New(),
			agenttool.New(llmAuditor, nil),
			agenttool.New(searchAgent, nil),
			agenttool.New(advisorAgent, nil),
			agenttool.New(judgeAgent, nil),
			agenttool.New(critiqueAgent, nil),
			agenttool.New(reviewAgent, nil),
			agenttool.New(critiqueLoop, nil),
			agenttool.New(managerAgent, nil),
		)
		baseInstruction = "You are layar-cli, an expert AI assistant running in a terminal.\n\n" +
			"## Tools\n" +
			"- get_weather / get_current_time — use for weather and time questions\n" +
			"- read_file / write_file — read or write a file in the working directory\n" +
			"- edit_file — targeted find-and-replace edit to part of an existing file (prefer over write_file for partial changes)\n" +
			"- grep_search — search files for a regular expression\n" +
			"- fetch_url — fetch an http(s) URL\n" +
			"- exec_command — run a shell command; ALWAYS pauses for explicit human " +
			"approval first, so use it freely when it's the right tool, the human " +
			"decides whether it actually runs\n" +
			"- list_skills — call this (no arguments) to discover available skills\n" +
			"- load_skill — call with {\"name\": \"<skill_name>\"} to load a skill's instructions\n" +
			"- preload_memory / load_memory — recall facts from past conversations\n" +
			"- load_artifacts — access previously saved files\n" +
			"- llm_auditor — delegate fact-checking and verification tasks here\n" +
			"- search_agent — delegate web-search questions here (real-time Google Search grounding)\n" +
			"- advisor_agent — get a second opinion on a plan or approach\n" +
			"- judge_agent — score content against a rubric (APPROVED/NEEDS_WORK)\n" +
			"- critique_agent — adversarially try to refute a claim or code\n" +
			"- review_agent — review real files (it reads/greps them itself)\n" +
			"- critique_loop — iteratively draft-and-critique something up to 3 cycles\n" +
			"- manager_agent — delegate SDLC/engineering-team tasks (backend, frontend, database, AI, " +
			"design, QA, PM, DevOps, security, etc.) here; it routes to the right specialist(s) and " +
			"consults cto_agent on high-stakes final calls\n\n" +
			"## Skills workflow\n" +
			"If the user's request matches a specialized task (code review, architecture, " +
			"debugging, documentation, etc.), ALWAYS:\n" +
			"1. Call list_skills to see what is available\n" +
			"2. Call load_skill with the best matching skill name\n" +
			"3. Follow the loaded instructions exactly before replying\n\n" +
			"## Style\n" +
			"- Terminal output: prefer short, direct answers\n" +
			"- Use markdown sparingly — fenced code blocks for code, bold for emphasis\n" +
			"- Never pad responses with preamble or sign-offs"
	} else {
		slog.Info("advanced tools disabled — GOOGLE_API_KEY not set (memory, artifacts, llm_auditor require Gemini)")
	}

	// Capture tool config so rebuildRunnerWithModel can reuse it.
	agentConfig.tools = agentTools
	agentConfig.toolsets = agentToolsets
	agentConfig.instruction = baseInstruction

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "layar-cli",
		Model:       m,
		Description: "Helpful AI assistant. Tools: weather, time, file read/write/edit, grep, fetch_url, exec_command (human-confirmed shell exec) — any provider; memory recall, artifact access, LLM fact-checker, search_agent (Google Search grounding), manager_agent (30-role SDLC team + CTO agent) — Gemini only.",
		Instruction: baseInstruction,
		Tools:       agentTools,
		Toolsets:    agentToolsets,
	})
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("create agent: %w", err)
	}

	// Memory service: backs preload_memory and load_memory tools.
	// AddSessionToMemory must be called after each turn for the tools to have
	// anything to search; the TUI does this in startAgentStream (chat.go).
	memorySvc := memory.InMemoryService()

	sessionSvc := session.InMemoryService()
	r, err := buildRunnerFromAgent(rootAgent, sessionSvc, memorySvc)
	if err != nil {
		return nil, nil, nil, nil, "", fmt.Errorf("create runner: %w", err)
	}

	return r, sessionSvc, memorySvc, m, chainName, nil
}

// buildRunnerFromAgent creates a new runner from an already-built agent,
// reusing existing session and memory services.  Used by buildRunner and by
// switchModelCmd when the user switches model via the /model picker.
func buildRunnerFromAgent(ag agent.Agent, sessionSvc session.Service, memorySvc memory.Service) (*runner.Runner, error) {
	return runner.New(runner.Config{
		AppName:           appName,
		Agent:             ag,
		SessionService:    sessionSvc,
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memorySvc,
		AutoCreateSession: true,
	})
}

// rebuildRunnerWithModel creates a new agent+runner for llm, reusing the
// session/memory services from the running TUI.  agentConfig must have been
// populated by buildRunner before this is called.
func rebuildRunnerWithModel(ctx context.Context, llm model.LLM, sessionSvc session.Service, memorySvc memory.Service) (*runner.Runner, error) {
	ag, err := llmagent.New(llmagent.Config{
		Name:        "layar-cli",
		Model:       llm,
		Description: "Helpful AI assistant. Tools: weather, time, file read/write/edit, grep, fetch_url, exec_command (human-confirmed shell exec) — any provider; memory recall, artifact access, LLM fact-checker, search_agent (Google Search grounding), manager_agent (30-role SDLC team + CTO agent) — Gemini only.",
		Instruction: agentConfig.instruction,
		Tools:       agentConfig.tools,
		Toolsets:    agentConfig.toolsets,
	})
	if err != nil {
		return nil, fmt.Errorf("create agent: %w", err)
	}
	return buildRunnerFromAgent(ag, sessionSvc, memorySvc)
}

// runOnce sends one message to the agent, prints the final response, and exits.
// This is the non-TUI path used by 'tui run <message>'.
func runOnce(ctx context.Context, r *runner.Runner, input string) error {
	msg := &genai.Content{
		Parts: []*genai.Part{{Text: input}},
		Role:  genai.RoleUser,
	}

	var sb strings.Builder
	for event, err := range r.Run(ctx, "cli-user", "cli-session", msg, agent.RunConfig{}) {
		if err != nil {
			return fmt.Errorf("agent error: %w", err)
		}
		if event != nil && event.IsFinalResponse() && event.LLMResponse.Content != nil {
			for _, part := range event.LLMResponse.Content.Parts {
				sb.WriteString(part.Text)
			}
		}
	}

	result := strings.TrimSpace(sb.String())
	if result == "" {
		result = "(no response)"
	}
	fmt.Println(result)
	return nil
}
