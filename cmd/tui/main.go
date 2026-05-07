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
	"log/slog"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/runner"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
	"google.golang.org/adk/tool/loadartifactstool"
	"google.golang.org/adk/tool/loadmemorytool"
	"google.golang.org/adk/tool/preloadmemorytool"
	"google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
	"google.golang.org/genai"

	"go-adk-q/agents"
	"go-adk-q/model/echo"
	"go-adk-q/model/failover"
	"go-adk-q/model/githubmodels"
	"go-adk-q/model/groq"
	"go-adk-q/model/huggingface"
	"go-adk-q/model/nvidia"
	"go-adk-q/model/openrouter"
	"go-adk-q/tools"
)

const appName = "go-adk-q-tui"

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

var rootCmd = &cobra.Command{
	Use:   "tui",
	Short: "Bubbletea/Lipgloss TUI for the go-adk-q ADK reference",
	Long: `A terminal chat UI powered by Bubbletea and Lipgloss.

Talks to the same multi-provider ADK agent as 'go run . console', but
renders in a beautiful full-terminal layout with scrolling history,
a spinner while the agent thinks, and keyboard shortcuts.

Environment variables (same as the main binary):
  GOOGLE_API_KEY, GROQ_API_KEY, NVIDIA_API_KEY, OPENROUTER_API_KEY, HF_TOKEN

Set at least one to start.`,
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start the interactive terminal chat UI",
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		r, sessionSvc, memorySvc, modelName, err := buildRunner(ctx)
		if err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		return runChat(r, sessionSvc, memorySvc, modelName)
	},
}

var runCmd = &cobra.Command{
	Use:   "run <message>",
	Short: "Send a single message and print the response (no TUI)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		ctx := context.Background()
		r, _, _, _, err := buildRunner(ctx)
		if err != nil {
			return fmt.Errorf("setup: %w", err)
		}
		return runOnce(ctx, r, args[0])
	},
}

func init() {
	rootCmd.AddCommand(chatCmd)
	rootCmd.AddCommand(runCmd)
}

// buildRunner constructs the failover model chain, a root agent with tools,
// and returns an ADK runner ready to use alongside the session and memory
// services needed by the TUI.
//
// The model chain is built from environment variables in priority order:
// Gemini → Groq → NVIDIA → OpenRouter → HuggingFace → echo (test only).
// At least one provider must be configured or buildRunner returns an error.
//
// Tools and the memory/artifact/LLM-auditor integrations are gated on
// GOOGLE_API_KEY because non-Gemini models cannot reliably execute structured
// tool calls (Groq: 400 tool_use_failed; NVIDIA: 500 single-tool-only;
// HuggingFace/OpenRouter: silent JSON mangling).
func buildRunner(ctx context.Context) (*runner.Runner, session.Service, memory.Service, string, error) {
	var candidateLLMs []model.LLM

	// GitHub Models — enabled when GITHUB_PAT is set. Highest priority:
	// supports GPT-4o, LLaMA 4, Claude, Gemini, DeepSeek and more via a
	// single GitHub PAT. Set GITHUB_MODEL to override the default (gpt-4o).
	if cfg := githubmodels.ConfigFromEnv(); cfg.PAT != "" {
		m, err := githubmodels.NewModel(ctx, cfg)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("githubmodels: %w", err)
		}
		candidateLLMs = append(candidateLLMs, m)
	}

	// Gemini — enabled when GOOGLE_API_KEY is set.
	if key := os.Getenv("GOOGLE_API_KEY"); key != "" {
		name := os.Getenv("GOOGLE_MODEL")
		if name == "" {
			name = "gemini-2.0-flash"
		}
		m, err := gemini.NewModel(ctx, name, &genai.ClientConfig{APIKey: key})
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("gemini: %w", err)
		}
		candidateLLMs = append(candidateLLMs, m)
	}

	// Groq — enabled when GROQ_API_KEY is set.
	if cfg := groq.ConfigFromEnv(); cfg.APIKey != "" {
		m, err := groq.NewModel(ctx, cfg)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("groq: %w", err)
		}
		candidateLLMs = append(candidateLLMs, m)
	}

	// NVIDIA NIM — enabled when NVIDIA_API_KEY is set.
	if cfg := nvidia.ConfigFromEnv(); cfg.APIKey != "" {
		m, err := nvidia.NewModel(ctx, cfg)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("nvidia: %w", err)
		}
		candidateLLMs = append(candidateLLMs, m)
	}

	// OpenRouter — enabled when OPENROUTER_API_KEY is set.
	if cfg := openrouter.ConfigFromEnv(); cfg.APIKey != "" {
		m, err := openrouter.NewModel(ctx, cfg)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("openrouter: %w", err)
		}
		candidateLLMs = append(candidateLLMs, m)
	}

	// HuggingFace — enabled when HF_TOKEN is set.
	if cfg := huggingface.ConfigFromEnv(); cfg.Token != "" {
		m, err := huggingface.NewModel(ctx, cfg)
		if err != nil {
			return nil, nil, nil, "", fmt.Errorf("huggingface: %w", err)
		}
		candidateLLMs = append(candidateLLMs, m)
	}

	// Echo stub — zero-credential fallback; activated by ECHO_FALLBACK_ENABLED=1.
	if echo.Enabled() {
		candidateLLMs = append(candidateLLMs, echo.Default())
		slog.Warn("echo fallback enabled — local testing only")
	}

	if len(candidateLLMs) == 0 {
		return nil, nil, nil, "", fmt.Errorf(
			"no model providers configured — set at least one of: " +
				"GITHUB_PAT, GOOGLE_API_KEY, GROQ_API_KEY, NVIDIA_API_KEY, OPENROUTER_API_KEY, HF_TOKEN",
		)
	}

	m := failover.New(candidateLLMs[0], candidateLLMs[1:]...)
	slog.Info("model chain", "providers", m.Name())

	// ── Tools, skills, memory, artifacts, and LLM Auditor ────────────────────
	// All gated on GOOGLE_API_KEY; see function comment for rationale.
	//
	// Tool inventory when Gemini is configured:
	//   weather, time, calculator    – FunctionTools (tools/)
	//   preload_memory               – auto-injects past-turn context into every request
	//   load_memory                  – lets the model explicitly search past turns
	//   load_artifacts               – lets the model access saved artifact files
	//   llm_auditor (via agenttool)  – fact-checks answers via critic→reviser pipeline
	var agentTools []tool.Tool
	var agentToolsets []tool.Toolset
	geminiInstruction := "You are a helpful, concise AI assistant in a terminal chat UI. " +
		"Answer questions directly and briefly. " +
		"Keep responses short — the user is reading in a terminal window."
	if os.Getenv("GOOGLE_API_KEY") != "" {
		llmAuditor := agents.GetLLMAuditorAgent(ctx, m)
		agentTools = []tool.Tool{
			tools.NewWeatherTool(),
			tools.NewTimeTool(),
			tools.NewCalculatorTool(),
			// Memory tools: preload_memory runs automatically on every request;
			// load_memory is called explicitly by the model to search past turns.
			preloadmemorytool.New(),
			loadmemorytool.New(),
			// Artifact tool: lets the model list and load named artifact files.
			loadartifactstool.New(),
			// LLM Auditor: exposed as a sub-agent tool; delegate to it when
			// the user asks to fact-check or verify an answer.
			agenttool.New(llmAuditor, nil),
		}
		geminiInstruction = "You are a helpful, concise AI assistant in a terminal chat UI. " +
			"Answer questions directly and briefly. " +
			"Use tools when the user asks about weather, time, or arithmetic. " +
			"Use load_memory or preload_memory context to recall past conversations. " +
			"Delegate to llm_auditor when the user asks to fact-check or verify an answer. " +
			"Keep responses short — the user is reading in a terminal window."
	} else {
		slog.Info("tools disabled — GOOGLE_API_KEY not set; set it to enable weather/time/calculator/memory/artifact tools")
	}

	if os.Getenv("GOOGLE_API_KEY") != "" {
		if _, statErr := os.Stat("./skills"); statErr == nil {
			rawSource := skill.NewFileSystemSource(os.DirFS("./skills"))
			preloaded, _, preloadErr := skill.WithCompletePreloadSource(ctx, rawSource)
			if preloadErr != nil {
				slog.Warn("skills preload failed — running without skills", "error", preloadErr)
			} else {
				skillToolset, stErr := skilltoolset.New(ctx, skilltoolset.Config{Source: preloaded})
				if stErr != nil {
					slog.Warn("skills toolset init failed — running without skills", "error", stErr)
				} else {
					agentToolsets = append(agentToolsets, skillToolset)
					slog.Info("skills toolset enabled", "path", "./skills")
				}
			}
		}
	} else {
		slog.Info("skills toolset disabled — GOOGLE_API_KEY not set; set it to enable skills")
	}

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "cli-q",
		Model:       m,
		Description: "Helpful AI assistant. Tools: weather, time, calculator, memory recall, artifact access, LLM fact-checker (Gemini only).",
		Instruction: geminiInstruction,
		Tools:       agentTools,
		Toolsets:    agentToolsets,
	})
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("create agent: %w", err)
	}

	// Memory service: backs preload_memory and load_memory tools.
	// AddSessionToMemory must be called after each turn for the tools to have
	// anything to search; the TUI does this in startAgentStream (chat.go).
	memorySvc := memory.InMemoryService()

	sessionSvc := session.InMemoryService()
	r, err := runner.New(runner.Config{
		AppName:           appName,
		Agent:             rootAgent,
		SessionService:    sessionSvc,
		ArtifactService:   artifact.InMemoryService(),
		MemoryService:     memorySvc,
		AutoCreateSession: true, // creates the session on first Run() call
	})
	if err != nil {
		return nil, nil, nil, "", fmt.Errorf("create runner: %w", err)
	}

	return r, sessionSvc, memorySvc, m.Name(), nil
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
