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
	"google.golang.org/adk/tool/skilltoolset/skill"
	localskilltoolset "go-adk-q/tool/skilltoolset"
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

// applyProviderSelected reads PROVIDER_SELECTED and moves the matching
// provider to the front of the candidateLLMs slice so it becomes the
// primary model in the failover chain. All other providers remain as
// ordered fallbacks.
//
// Valid values (case-insensitive):
//
//	github, gemini, groq, nvidia, openrouter, huggingface, echo
//
// Example:
//
//	export PROVIDER_SELECTED=openrouter
func applyProviderSelected(llms []model.LLM) []model.LLM {
	sel := strings.ToLower(strings.TrimSpace(os.Getenv("PROVIDER_SELECTED")))
	if sel == "" || len(llms) <= 1 {
		return llms
	}
	for i, m := range llms {
		if strings.Contains(strings.ToLower(m.Name()), sel) {
			if i == 0 {
				return llms // already first
			}
			reordered := make([]model.LLM, 0, len(llms))
			reordered = append(reordered, llms[i])
			reordered = append(reordered, llms[:i]...)
			reordered = append(reordered, llms[i+1:]...)
			slog.Info("PROVIDER_SELECTED applied", "provider", sel, "model", m.Name())
			return reordered
		}
	}
	slog.Warn("PROVIDER_SELECTED: no configured provider matched", "value", sel)
	return llms
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

	// PROVIDER_SELECTED moves the named provider to the front of the chain.
	// Valid values: github, gemini, groq, nvidia, openrouter, huggingface, echo.
	// Example: PROVIDER_SELECTED=openrouter
	candidateLLMs = applyProviderSelected(candidateLLMs)

	primaryName := candidateLLMs[0].Name() // primary model name for display
	m := failover.New(candidateLLMs[0], candidateLLMs[1:]...)
	slog.Info("model chain", "providers", m.Name())

	// ── Tools, skills, memory, artifacts, and LLM Auditor ────────────────────
	//
	// Skills and basic tools (weather, time, calculator) work with ANY provider
	// that supports function calling — GitHub Models, OpenRouter, Groq, etc.
	// skilltoolset uses plain functiontool.New internally; it has no Gemini
	// dependency whatsoever.
	//
	// Only llm_auditor + memory/artifact tools remain gated on GOOGLE_API_KEY
	// because those involve multi-step tool chains that non-Gemini providers
	// struggle with (Groq: 400 tool_use_failed on complex chains).
	//
	// Tool inventory (all providers):
	//   weather, time, calculator    – FunctionTools (tools/)
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
	agentTools = []tool.Tool{
		tools.NewWeatherTool(),
		tools.NewTimeTool(),
		tools.NewCalculatorTool(),
	}

	baseInstruction := "You are cli-q, an expert AI assistant running in a terminal.\n\n" +
		"## Tools\n" +
		"- get_weather / get_current_time — use for weather and time questions\n" +
		"- calculator — use for arithmetic\n" +
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
	if _, statErr := os.Stat("./skills"); statErr == nil {
		rawSource := skill.NewFileSystemSource(os.DirFS("./skills"))
		preloaded, _, preloadErr := skill.WithCompletePreloadSource(ctx, rawSource)
		if preloadErr != nil {
			slog.Warn("skills preload failed — running without skills", "error", preloadErr)
		} else {
			skillToolset, stErr := localskilltoolset.New(ctx, localskilltoolset.Config{Source: preloaded})
			if stErr != nil {
				slog.Warn("skills toolset init failed — running without skills", "error", stErr)
			} else {
				agentToolsets = append(agentToolsets, skillToolset)
				slog.Info("skills toolset enabled", "path", "./skills")
			}
		}
	}

	// Advanced tools — gated on GOOGLE_API_KEY because llm_auditor and memory
	// tools require reliable multi-step tool chains only Gemini handles well.
	if os.Getenv("GOOGLE_API_KEY") != "" {
		llmAuditor := agents.GetLLMAuditorAgent(ctx, m)
		agentTools = append(agentTools,
			preloadmemorytool.New(),
			loadmemorytool.New(),
			loadartifactstool.New(),
			agenttool.New(llmAuditor, nil),
		)
		baseInstruction = "You are cli-q, an expert AI assistant running in a terminal.\n\n" +
			"## Tools\n" +
			"- get_weather / get_current_time — use for weather and time questions\n" +
			"- calculator — use for arithmetic\n" +
			"- list_skills — call this (no arguments) to discover available skills\n" +
			"- load_skill — call with {\"name\": \"<skill_name>\"} to load a skill's instructions\n" +
			"- preload_memory / load_memory — recall facts from past conversations\n" +
			"- load_artifacts — access previously saved files\n" +
			"- llm_auditor — delegate fact-checking and verification tasks here\n\n" +
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

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:        "cli-q",
		Model:       m,
		Description: "Helpful AI assistant. Tools: weather, time, calculator, memory recall, artifact access, LLM fact-checker (Gemini only).",
		Instruction: baseInstruction,
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

	return r, sessionSvc, memorySvc, primaryName, nil
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
