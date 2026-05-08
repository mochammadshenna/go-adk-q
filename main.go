// Package main is a minimal but complete reference implementation of the
// Google Agent Development Kit (ADK) for Go — google.golang.org/adk v1.2.0.
//
// # What is demonstrated
//
// This single file covers every core ADK Go pattern from the official docs:
//
//  1. FunctionTool    – typed Go function → ADK tool (struct args + json/jsonschema tags)
//  2. LlmAgent        – the "thinking" agent; non-deterministic, LLM-driven
//  3. SequentialAgent – deterministic pipeline; sub-agents run in strict order
//  4. LoopAgent       – iterative refinement; loops until MaxIterations or exit
//  5. ParallelAgent   – concurrent execution; independent sub-agents run at once
//  6. Custom agent    – direct Run func implementation; arbitrary control flow
//  7. Multi-agent     – root LlmAgent delegates to sub-agents via agenttool.New
//  8. Launcher        – full.NewLauncher() exposes console / web webui api / web api
//  9. State           – active state read/write from tool context (ctx.State().Set/Get)
//
// 10. Artifacts       – versioned file storage from tool context (ctx.Artifacts().Save/Load)
// 11. Trace           – loggingSpanExporter wired into launcher TelemetryOptions
// 12. Custom model    – groq/nvidia/openrouter/huggingface packages bridge Genkit compat_oai → ADK model.LLM
//
// # Running
//
//	# Highest priority — GitHub Models (single PAT, many models)
//	export GITHUB_PAT=github_pat_...          # required for GitHub Models
//	export GITHUB_MODEL=gpt-4o                # optional; default: gpt-4o
//
//	# Required (if GITHUB_PAT not set)
//	export GOOGLE_API_KEY=<your-key>
//	export GOOGLE_MODEL=gemini-2.0-flash  # optional; default model (upgrade to gemini-2.5-flash if your key allows)
//
//	# Optional — each unlocks a provider agent when set
//	export GROQ_API_KEY=<gsk_...>              # Groq LPU; see https://console.groq.com/keys
//	export GROQ_MODEL=llama-3.1-8b-instant     # override; default: llama-3.3-70b-versatile
//
//	export NVIDIA_API_KEY=<nvapi-...>          # NVIDIA NIM; see https://build.nvidia.com
//	export NVIDIA_MODEL=meta/llama-3.1-70b-instruct  # override; default: minimaxai/minimax-m1
//	export NVIDIA_BASE_URL=http://nim:8000/v1  # on-premises NIM override
//
//	export OPENROUTER_API_KEY=<key>            # OpenRouter; see https://openrouter.ai/keys
//	export OPENROUTER_MODEL=openai/gpt-4o      # override; default: meta-llama/llama-3.3-70b-instruct
//	export OPENROUTER_SITE_URL=https://myapp.example.com  # attribution header
//	export OPENROUTER_APP_NAME="My App"        # attribution header
//
//	export HF_TOKEN=<hf_...>                  # HuggingFace; see https://hf.co/settings/tokens
//	export HF_MODEL=NousResearch/Hermes-2-Pro-Llama-3-8B  # override; default: mistralai/Mistral-7B-Instruct-v0.3
//	export HF_ENDPOINT_URL=https://xyz.endpoints.huggingface.cloud  # dedicated endpoint
//
//	# Local testing — adds a zero-credential echo stub as last fallback
//	export ECHO_FALLBACK_ENABLED=1  # combine with GOOGLE_MODEL=gemini-broken for failover demo
//
//	go run . console          # interactive terminal
//	go run . web webui api    # browser dev-UI + REST API at http://localhost:8080
//	go run . web api          # REST API only at http://localhost:8080
//
// # Architecture overview
//
//	root_agent (LlmAgent)
//	├── weather_time_agent  (LlmAgent + get_weather + get_current_time tools)
//	├── code_pipeline       (SequentialAgent: Writer → Reviewer → Refactorer)
//	├── doc_refinement_loop (LoopAgent: Drafter → QualityChecker, max 3 iter)
//	├── parallel_analysis   (ParallelAgent: ResearchAgent ∥ SummaryAgent)
//	├── router_agent        (Custom agent: Run func with conditional logic)
//	├── notebook_agent      (LlmAgent: state r/w + artifact save/load + calculator)
//	├── groq_agent          (LlmAgent: LLaMA 3.3 70B on Groq LPU  [GROQ_API_KEY])
//	├── nvidia_agent        (LlmAgent: Nemotron 70B on NVIDIA NIM  [NVIDIA_API_KEY])
//	├── openrouter_agent    (LlmAgent: any model via OpenRouter    [OPENROUTER_API_KEY])
//	└── huggingface_agent   (LlmAgent: Mistral 7B via HF serverless [HF_TOKEN])
package main

import (
	"context"
	"fmt"
	"iter"
	"log"
	"log/slog"
	"os"
	"strings"

	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"google.golang.org/genai"

	"google.golang.org/adk/agent"
	"google.golang.org/adk/agent/llmagent"
	"google.golang.org/adk/agent/workflowagents/loopagent"
	"google.golang.org/adk/agent/workflowagents/parallelagent"
	"google.golang.org/adk/agent/workflowagents/sequentialagent"
	"google.golang.org/adk/artifact"
	"google.golang.org/adk/cmd/launcher"
	"google.golang.org/adk/cmd/launcher/full"
	"google.golang.org/adk/model"
	"google.golang.org/adk/model/gemini"
	"google.golang.org/adk/session"
	"google.golang.org/adk/telemetry"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/agenttool"
	"google.golang.org/adk/tool/skilltoolset/skill"

	localskilltoolset "go-adk-q/tool/skilltoolset"

	"go-adk-q/model/echo"
	"go-adk-q/model/failover"
	"go-adk-q/model/githubmodels"
	"go-adk-q/model/groq"
	"go-adk-q/model/huggingface"
	"go-adk-q/model/nvidia"
	"go-adk-q/model/openrouter"
	"go-adk-q/tools"
)

func main() {
	ctx := context.Background()

	// ── 1. Models ─────────────────────────────────────────────────────────────
	// All provider models are created here, before the agent graph, so they
	// serve two purposes:
	//
	//   (a) Form the failover chain for the primary model m — if Gemini returns
	//       an error (rate-limit, outage, quota) the next configured provider
	//       is tried automatically, transparent to every agent that uses m.
	//
	//   (b) Power dedicated provider comparison sub-agents (groq_agent, etc.)
	//       so users can explicitly target a specific provider.
	//
	// failover.New filters out nil entries, so it is always safe to pass all
	// four provider LLMs regardless of which API keys are configured.

	// ── Provider setup ────────────────────────────────────────────────────────
	// Every provider is optional. The first configured provider becomes primary;
	// the rest form the automatic fallback chain in priority order:
	//   GitHub Models → Gemini → Groq → NVIDIA → OpenRouter → HuggingFace → echo (test only)
	//
	// If GOOGLE_API_KEY is not set, Gemini is skipped entirely — no wasted
	// round-trips and no WARN noise.  Set at least one provider key or the
	// binary exits with a clear error.

	// candidateLLMs collects every configured LLM in priority order.
	// nil entries are skipped so unconfigured providers cost nothing.
	var candidateLLMs []model.LLM
	var err error

	// GitHub Models — highest priority; enabled when GITHUB_PAT is set.
	// Supports GPT-4o, LLaMA 4, Claude, Gemini, DeepSeek, Mistral and more
	// via a single GitHub PAT with the "models" permission.
	// Set GITHUB_MODEL to override the default (gpt-4o).
	var githubLLM model.LLM
	if ghCfg := githubmodels.ConfigFromEnv(); ghCfg.PAT != "" {
		githubLLM, err = githubmodels.NewModel(ctx, ghCfg)
		mustOK(err, "create githubmodels model")
		candidateLLMs = append(candidateLLMs, githubLLM)
	} else {
		slog.Info("GITHUB_PAT not set — GitHub Models skipped; set it at https://github.com/settings/tokens")
	}

	// Gemini — optional; enabled when GOOGLE_API_KEY is set.
	// GOOGLE_MODEL overrides the model name (e.g. set to a bad value to force
	// failover during testing: GOOGLE_MODEL=gemini-intentionally-broken).
	if googleKey := os.Getenv("GOOGLE_API_KEY"); googleKey != "" {
		geminiModelName := os.Getenv("GOOGLE_MODEL")
		if geminiModelName == "" {
			// gemini-2.0-flash is stable and available on all AI Studio key tiers.
			// Upgrade to gemini-2.5-flash via GOOGLE_MODEL if your key supports it.
			geminiModelName = "gemini-2.0-flash"
		}
		geminiModel, geminiErr := gemini.NewModel(ctx, geminiModelName, &genai.ClientConfig{
			APIKey: googleKey,
		})
		mustOK(geminiErr, "create gemini model")
		candidateLLMs = append(candidateLLMs, geminiModel)
	} else {
		slog.Info("GOOGLE_API_KEY not set — Gemini skipped; set it at https://aistudio.google.com/apikey")
	}

	// Groq — optional; enabled when GROQ_API_KEY is set.
	var groqLLM model.LLM
	if groqCfg := groq.ConfigFromEnv(); groqCfg.APIKey != "" {
		groqLLM, err = groq.NewModel(ctx, groqCfg)
		mustOK(err, "create groq model")
		candidateLLMs = append(candidateLLMs, groqLLM)
	}

	// NVIDIA NIM — optional; enabled when NVIDIA_API_KEY is set.
	var nvidiaLLM model.LLM
	if nvidiaCfg := nvidia.ConfigFromEnv(); nvidiaCfg.APIKey != "" {
		nvidiaLLM, err = nvidia.NewModel(ctx, nvidiaCfg)
		mustOK(err, "create nvidia model")
		candidateLLMs = append(candidateLLMs, nvidiaLLM)
	}

	// OpenRouter — optional; enabled when OPENROUTER_API_KEY is set.
	var openrouterLLM model.LLM
	if orCfg := openrouter.ConfigFromEnv(); orCfg.APIKey != "" {
		if orCfg.SiteURL == "" {
			orCfg.SiteURL = "https://github.com/example/go-adk-q"
		}
		if orCfg.AppName == "" {
			orCfg.AppName = "go-adk-q ADK reference"
		}
		openrouterLLM, err = openrouter.NewModel(ctx, orCfg)
		mustOK(err, "create openrouter model")
		candidateLLMs = append(candidateLLMs, openrouterLLM)
	}

	// HuggingFace — optional; enabled when HF_TOKEN is set.
	var huggingfaceLLM model.LLM
	if hfCfg := huggingface.ConfigFromEnv(); hfCfg.Token != "" {
		huggingfaceLLM, err = huggingface.NewModel(ctx, hfCfg)
		mustOK(err, "create huggingface model")
		candidateLLMs = append(candidateLLMs, huggingfaceLLM)
	}

	// Echo fallback — zero-credential stub, always succeeds.
	// Activated by ECHO_FALLBACK_ENABLED=1; appended last so it only fires
	// when every real provider has failed. Never use in production.
	var echoLLM model.LLM
	if echo.Enabled() {
		echoLLM = echo.Default()
		candidateLLMs = append(candidateLLMs, echoLLM)
		slog.Warn("echo fallback enabled — for local testing only; not for production")
	}

	// Require at least one provider.
	if len(candidateLLMs) == 0 {
		log.Fatal("no model providers configured — set at least one of: " +
			"GITHUB_PAT, GOOGLE_API_KEY, GROQ_API_KEY, NVIDIA_API_KEY, OPENROUTER_API_KEY, HF_TOKEN")
	}

	// PROVIDER_SELECTED moves the named provider to the front of the chain.
	// Valid values: github, gemini, groq, nvidia, openrouter, huggingface, echo.
	// Example: export PROVIDER_SELECTED=openrouter
	candidateLLMs = applyProviderSelected(candidateLLMs)

	// Build the failover chain: first provider is primary, rest are fallbacks.
	m := failover.New(candidateLLMs[0], candidateLLMs[1:]...)
	slog.Info("model chain", "providers", m.Name())

	// Skills toolset — enabled for any provider when ./skills/ dir exists.
	// skilltoolset uses plain functiontool.New internally with no Gemini
	// dependency; any model with function calling support can use it.
	var agentToolsets []tool.Toolset
	if _, statErr := os.Stat("./skills"); statErr == nil {
		rawSource := skill.NewFileSystemSource(os.DirFS("./skills"))
		preloaded, _, preloadErr := skill.WithCompletePreloadSource(ctx, rawSource)
		if preloadErr != nil {
			slog.Warn("skills preload failed — running without skills", "error", preloadErr)
		} else {
			st, stErr := localskilltoolset.New(ctx, localskilltoolset.Config{Source: preloaded})
			if stErr != nil {
				slog.Warn("skills toolset init failed — running without skills", "error", stErr)
			} else {
				agentToolsets = append(agentToolsets, st)
				slog.Info("skills toolset enabled", "path", "./skills")
			}
		}
	}

	// ── 2. FunctionTools ─────────────────────────────────────────────────────
	// tools.NewWeatherTool / tools.NewTimeTool are in tools/tools.go.
	// They use functiontool.New with typed args structs and json/jsonschema tags.
	// The ADK framework auto-generates JSON schema from struct field types + tags
	// so the LLM knows exactly what arguments to supply when calling each tool.
	weatherTool := tools.NewWeatherTool()
	timeTool := tools.NewTimeTool()

	// ── 3. LlmAgent (multi-tool) ─────────────────────────────────────────────
	// LlmAgent is the core "thinking" agent. It is non-deterministic: the LLM
	// decides dynamically which tools to call, in what order, and what to reply.
	//
	// Key Config fields:
	//   Name        – unique identifier used in multi-agent routing
	//   Description – used by parent agents to decide when to delegate here
	//   Instruction – system prompt; supports {state_key} template substitution
	//   OutputKey   – saves the agent's final reply to session state[OutputKey]
	//   Tools       – slice of tool.Tool (FunctionTool, GeminiTool, AgentTool…)
	weatherTimeAgent, err := llmagent.New(llmagent.Config{
		Name:  "weather_time_agent",
		Model: m,
		Description: "Answers questions about the current weather and local time in any city. " +
			"Use for weather or time queries only.",
		Instruction: "You answer weather and time questions for cities. " +
			"Use get_weather for weather and get_current_time for time. " +
			"Always state the city clearly in your response.",
		Tools:    []tool.Tool{weatherTool, timeTool},
		Toolsets: agentToolsets,
	})
	mustOK(err, "create weather_time_agent")

	// ── 4. SequentialAgent ───────────────────────────────────────────────────
	// SequentialAgent executes its SubAgents one after another in strict order.
	// It is deterministic — no LLM drives the orchestration, only the order matters.
	//
	// State passing between stages:
	//   - Stage N sets OutputKey: "key"  → stored in session.State["key"]
	//   - Stage N+1 references {key}     → ADK interpolates from session.State
	//
	// This pattern is ideal for pipelines where each step depends on the previous.
	codeWriter, err := llmagent.New(llmagent.Config{
		Name:  "CodeWriter",
		Model: m,
		Instruction: "You are a Go code generator. " +
			"Write a complete, runnable Go function based on the user's specification. " +
			"Output ONLY the code block enclosed in ```Go ... ```.",
		OutputKey: "generated_code", // → session.State["generated_code"]
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create CodeWriter")

	codeReviewer, err := llmagent.New(llmagent.Config{
		Name:  "CodeReviewer",
		Model: m,
		Instruction: "You are an expert Go code reviewer. " +
			"Review this code:\n```Go\n{generated_code}\n```\n" +
			"List up to 3 concise, actionable improvements. " +
			"If the code is already excellent, write 'No major issues found.'",
		OutputKey: "temp:review_comments", // temp: prefix = not persisted across sessions
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create CodeReviewer")

	codeRefactorer, err := llmagent.New(llmagent.Config{
		Name:  "CodeRefactorer",
		Model: m,
		Instruction: "You are a Go refactoring specialist. " +
			"Refactor this code:\n```Go\n{generated_code}\n```\n" +
			"Apply these review comments: {temp:review_comments}\n" +
			"Output ONLY the final, improved code block.",
		OutputKey: "refactored_code",
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create CodeRefactorer")

	codePipeline, err := sequentialagent.New(sequentialagent.Config{
		AgentConfig: agent.Config{
			Name:        "code_pipeline",
			Description: "Writes, reviews, and refactors Go code in a strict sequential pipeline.",
			SubAgents:   []agent.Agent{codeWriter, codeReviewer, codeRefactorer},
		},
	})
	mustOK(err, "create code_pipeline")

	// ── 5. LoopAgent ─────────────────────────────────────────────────────────
	// LoopAgent repeatedly runs its SubAgents until either:
	//   (a) MaxIterations is reached, or
	//   (b) a sub-agent calls exitlooptool (signals early termination).
	//
	// Use for iterative refinement tasks: doc drafting, image generation retries,
	// consensus building, etc.
	docDrafter, err := llmagent.New(llmagent.Config{
		Name:  "DocDrafter",
		Model: m,
		Instruction: "You are a technical writer. " +
			"Write or improve a concise, clear technical document based on the user's topic. " +
			"If there is existing draft content in {document_draft}, improve it. " +
			"Otherwise create a fresh draft. Output the full document text only.",
		OutputKey: "document_draft",
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create DocDrafter")

	qualityChecker, err := llmagent.New(llmagent.Config{
		Name:  "QualityChecker",
		Model: m,
		Instruction: "You are a technical editor. " +
			"Evaluate this document:\n{document_draft}\n\n" +
			"If the quality is publication-ready, respond EXACTLY with: APPROVED\n" +
			"Otherwise, give ONE specific, actionable improvement suggestion.",
		OutputKey: "quality_verdict",
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create QualityChecker")

	docRefinementLoop, err := loopagent.New(loopagent.Config{
		MaxIterations: 3, // Terminates after 3 draft-review cycles at most.
		AgentConfig: agent.Config{
			Name: "doc_refinement_loop",
			Description: "Iteratively drafts and quality-checks a technical document. " +
				"Runs up to 3 refinement cycles.",
			SubAgents: []agent.Agent{docDrafter, qualityChecker},
		},
	})
	mustOK(err, "create doc_refinement_loop")

	// ── 6. ParallelAgent ─────────────────────────────────────────────────────
	// ParallelAgent runs all SubAgents concurrently in independent branches.
	// Sub-agents do NOT share conversation history during parallel execution.
	// Results are collected after all branches complete.
	//
	// Use when tasks are independent and latency matters (e.g., multi-source
	// data gathering, running multiple LLM analyses simultaneously).
	techResearcher, err := llmagent.New(llmagent.Config{
		Name:        "TechResearcher",
		Model:       m,
		Description: "Researches the technical background and implementation details of a topic.",
		Instruction: "Research the technical aspects of the given topic. " +
			"Provide key technical facts, how it works, and relevant technical context. Be concise.",
		OutputKey: "tech_research",
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create TechResearcher")

	bizAnalyst, err := llmagent.New(llmagent.Config{
		Name:        "BizAnalyst",
		Model:       m,
		Description: "Analyzes the business impact and market implications of a topic.",
		Instruction: "Analyze the business and economic impact of the given topic. " +
			"Cover market size, adoption trends, and strategic implications. Be concise.",
		OutputKey: "biz_analysis",
		Toolsets:  agentToolsets,
	})
	mustOK(err, "create BizAnalyst")

	parallelAnalysis, err := parallelagent.New(parallelagent.Config{
		AgentConfig: agent.Config{
			Name: "parallel_analysis",
			Description: "Performs technical research and business analysis concurrently " +
				"for comprehensive topic coverage.",
			SubAgents: []agent.Agent{techResearcher, bizAnalyst},
		},
	})
	mustOK(err, "create parallel_analysis")

	// ── 7. Custom Agent ───────────────────────────────────────────────────────
	// agent.New with a Run func gives complete control over execution flow.
	// The Run function signature: func(agent.InvocationContext) iter.Seq2[*session.Event, error]
	//
	// Use custom agents for:
	//   - Conditional routing based on runtime state
	//   - External API calls within orchestration logic
	//   - Unique patterns not covered by Sequential/Loop/Parallel
	//   - Dynamic sub-agent selection
	//
	// iter.Seq2 is Go's push-iterator pattern (range-over-func, Go 1.23+).
	// yield each *session.Event as it is produced; return false to stop.
	routerAgent, err := agent.New(agent.Config{
		Name: "router_agent",
		Description: "A custom agent demonstrating direct Run func implementation with " +
			"conditional routing logic based on session state.",
		Run: conditionalRouter,
	})
	mustOK(err, "create router_agent")

	// ── 8. Notebook Agent — State + Artifacts ────────────────────────────────
	// This agent demonstrates the two explicit data-persistence mechanisms in ADK:
	//
	// STATE (session.State via ctx.State()):
	//   - Mutable key/value store, alive for the session lifetime.
	//   - Any tool or agent in the same session can read and write state.
	//   - Write: ctx.State().Set(key, value)
	//   - Read:  ctx.State().Get(key) → (any, error)
	//   - The "temp:" key prefix marks values as ephemeral (not persisted across sessions).
	//   - OutputKey on LlmAgent is the passive/automatic way to write state;
	//     ctx.State().Set is the active/programmatic way.
	//
	// ARTIFACTS (agent.Artifacts via ctx.Artifacts()):
	//   - Named, versioned binary/text files backed by artifact.Service.
	//   - The service is injected at startup via launcher.Config.ArtifactService.
	//   - This demo uses artifact.InMemoryService() (in-process, no external deps).
	//   - For production: artifact.GCSService(...) (Google Cloud Storage).
	//   - Write: ctx.Artifacts().Save(ctx, filename, *genai.Part) → version
	//   - Read:  ctx.Artifacts().Load(ctx, filename)              → *genai.Part
	//   - Read historical: ctx.Artifacts().LoadVersion(ctx, filename, version)
	//
	// Difference:
	//   State  → structured values (strings, maps); ephemeral; no versioning.
	//   Artifacts → file-like content (text, bytes); versioned; persists via service.
	notebookAgent, err := llmagent.New(llmagent.Config{
		Name:  "notebook_agent",
		Model: m,
		Description: "Manages a persistent notebook: saves/reads session state values, " +
			"stores/retrieves versioned text artifacts, and performs arithmetic calculations. " +
			"Use for: 'remember X', 'recall X', 'save a file', 'load a file', or math.",
		Instruction: `You are a notebook agent that manages memory and files for the user.

STATE operations (key/value, in-memory, session-scoped):
  - save_to_state   key value  → writes value to session state under key
  - read_from_state key        → reads value from session state by key

ARTIFACT operations (versioned files, backed by artifact service):
  - save_artifact   filename content  → stores content as a named versioned artifact
  - load_artifact   filename          → retrieves the latest version of a named artifact
  - load_artifact   filename version  → retrieves a specific historical version

CALCULATION:
  - calculate  operation a b  → arithmetic: add/subtract/multiply/divide/power/sqrt

LONG-RUNNING operations (returns immediately; poll for completion):
  - start_report  topic       → starts async report generation; returns task_id
  - check_report  task_id     → polls status; returns progress and final summary

Always confirm what was stored or retrieved, and include the version number for artifacts.`,
		Tools: []tool.Tool{
			// State read/write (tools/state.go)
			tools.NewSaveToStateTool(),
			tools.NewReadFromStateTool(),
			// Artifact save/load (tools/artifacts.go)
			tools.NewSaveArtifactTool(),
			tools.NewLoadArtifactTool(),
			// Optional params pattern (tools/calculator.go)
			tools.NewCalculatorTool(),
			// Long-running tool pattern (tools/longtask.go)
			tools.NewStartReportTool(),
			tools.NewCheckReportTool(),
		},
		Toolsets: agentToolsets,
	})
	mustOK(err, "create notebook_agent")

	// ── 10. Custom Model Provider Agents ─────────────────────────────────────
	//
	// The provider LLMs were created in section 1 (model chain). Here they are
	// wrapped into dedicated comparison agents so users can explicitly invoke a
	// specific provider. Each agent is created only when its LLM is non-nil
	// (i.e., the corresponding API key env var is set).
	//
	// These agents complement the failover chain on the root model m:
	//   - m           → transparent automatic failover (Gemini → configured fallbacks)
	//   - groq_agent  → intentional Groq-only inference (speed/latency comparison)
	//   - nvidia_agent → intentional NVIDIA NIM-only inference
	//   - openrouter_agent → intentional OpenRouter routing
	//   - huggingface_agent → intentional HuggingFace serverless/endpoint inference

	var groqAgent agent.Agent
	if groqLLM != nil {
		groqAgent, err = llmagent.New(llmagent.Config{
			Name:  "groq_agent",
			Model: groqLLM,
			Description: "Ultra-low-latency LLaMA 3.3 70B agent on Groq LPU hardware. " +
				"Use for speed-sensitive tasks, quick Q&A, or comparing LLaMA vs Gemini outputs.",
			Instruction: "You are a helpful assistant running on Groq's LPU hardware, " +
				"powered by Meta's LLaMA 3.3 70B model. Be direct and concise.",
			// No Tools/Toolsets: LLaMA on Groq returns 400 "tool_use_failed" for
			// multi-tool prompts and garbles structured tool-call JSON.
		})
		mustOK(err, "create groq_agent")
	}

	var nvidiaAgent agent.Agent
	if nvidiaLLM != nil {
		nvidiaAgent, err = llmagent.New(llmagent.Config{
			Name:  "nvidia_agent",
			Model: nvidiaLLM,
			Description: "MiniMax M1 agent on NVIDIA NIM. Use for tasks requiring " +
				"strong reasoning, long-context understanding, or to compare NVIDIA-optimised " +
				"models against other providers.",
			Instruction: "You are a helpful assistant powered by MiniMax M1, " +
				"running on NVIDIA NIM infrastructure. Be precise and thorough.",
			// No Tools/Toolsets: NVIDIA NIM returns 500 "only supports single
			// tool-calls at once" when multiple tools are advertised.
		})
		mustOK(err, "create nvidia_agent")
	}

	var openrouterAgent agent.Agent
	if openrouterLLM != nil {
		openrouterAgent, err = llmagent.New(llmagent.Config{
			Name:  "openrouter_agent",
			Model: openrouterLLM,
			Description: "LLaMA 3.3 70B agent routed through OpenRouter. OpenRouter provides " +
				"automatic failover across providers and access to 200+ models with one API key. " +
				"Use to compare results or when a specific provider is unavailable.",
			Instruction: "You are a helpful assistant. Your requests are routed through " +
				"OpenRouter to Meta's LLaMA 3.3 70B Instruct model. Be helpful and clear.",
			// No Tools/Toolsets: LLaMA models via OpenRouter mangle structured
			// tool-call JSON — use text-only mode.
		})
		mustOK(err, "create openrouter_agent")
	}

	var huggingfaceAgent agent.Agent
	if huggingfaceLLM != nil {
		huggingfaceAgent, err = llmagent.New(llmagent.Config{
			Name:  "huggingface_agent",
			Model: huggingfaceLLM,
			Description: "Mistral 7B Instruct agent running on HuggingFace's serverless " +
				"inference API. Use for open-source model comparisons or tasks that benefit " +
				"from Mistral's instruction-following style.",
			Instruction: "You are a helpful assistant powered by Mistral 7B Instruct v0.3, " +
				"running on HuggingFace's serverless inference API. Be helpful and concise.",
			// No Tools/Toolsets: HuggingFace serverless models silently mangle
			// tool-call JSON — use text-only mode.
		})
		mustOK(err, "create huggingface_agent")
	}

	// ── 11. Multi-Agent System (root) ─────────────────────────────────────────
	// The root LlmAgent coordinates all sub-agents. Sub-agents are wrapped with
	// agenttool.New so the LLM can invoke each by name based on Descriptions.
	//
	// The four external-provider agents (Groq, NVIDIA, OpenRouter, HuggingFace)
	// are conditionally included based on env vars. This pattern keeps the binary
	// functional with any subset of API keys configured.
	rootTools := []tool.Tool{
		agenttool.New(weatherTimeAgent, nil),
		agenttool.New(codePipeline, nil),
		agenttool.New(docRefinementLoop, nil),
		agenttool.New(parallelAnalysis, nil),
		agenttool.New(routerAgent, nil),
		agenttool.New(notebookAgent, nil),
	}

	// Build a dynamic routing hint and tool list for whichever providers are live.
	type optAgent struct {
		a    agent.Agent
		hint string // line appended to root_agent instruction when active
	}
	for _, oa := range []optAgent{
		{groqAgent, "- Fast/low-latency inference or LLaMA vs Gemini comparison → groq_agent"},
		{nvidiaAgent, "- Reasoning-heavy tasks or NVIDIA Nemotron model comparison → nvidia_agent"},
		{openrouterAgent, "- Access to 200+ models or provider failover comparison → openrouter_agent"},
		{huggingfaceAgent, "- Open-source Mistral model or HuggingFace model comparison → huggingface_agent"},
	} {
		if oa.a != nil {
			rootTools = append(rootTools, agenttool.New(oa.a, nil))
		}
	}

	// Collect active provider routing hints for the instruction prompt.
	providerHints := ""
	for _, oa := range []optAgent{
		{groqAgent, "\n- Fast/low-latency or LLaMA: groq_agent"},
		{nvidiaAgent, "\n- Reasoning / NVIDIA Nemotron: nvidia_agent"},
		{openrouterAgent, "\n- Any model / OpenRouter routing: openrouter_agent"},
		{huggingfaceAgent, "\n- Open-source Mistral / HuggingFace: huggingface_agent"},
	} {
		if oa.a != nil {
			providerHints += oa.hint
		}
	}

	rootAgent, err := llmagent.New(llmagent.Config{
		Name:  "root_agent",
		Model: m,
		Description: "Central AI coordinator with access to weather/time, code pipeline, " +
			"document refinement, parallel analysis, custom routing, a notebook agent, " +
			"and optionally Groq, NVIDIA NIM, OpenRouter, and HuggingFace model agents.",
		Instruction: `You are a helpful AI assistant. Answer conversational messages, greetings, and simple questions DIRECTLY without invoking any sub-agent.

Only delegate to a sub-agent when the user explicitly requests that type of work:
- Weather or time for a specific city     → weather_time_agent
- Write, review, or refactor Go code      → code_pipeline
- Draft or improve a technical document   → doc_refinement_loop
- Analyze a topic from multiple angles    → parallel_analysis
- Custom routing or conditional logic demo → router_agent
- Store/recall values, save/load files, or arithmetic → notebook_agent` + providerHints + `

IMPORTANT: For greetings ("hi", "hello"), general questions, or anything not in the list above, respond directly in plain text. Do NOT call any sub-agent for conversational input.`,
		Tools:    rootTools,
		Toolsets: agentToolsets,
	})
	mustOK(err, "create root_agent")

	// ── 12. Launch ────────────────────────────────────────────────────────────
	// full.NewLauncher() supports these run modes (ADK v1.2.0 syntax):
	//   go run . console          → interactive terminal session
	//   go run . web webui api    → browser dev-UI + REST API (http://localhost:8080)
	//   go run . web api          → REST API only (http://localhost:8080)
	//   go run . web webui        → web UI only (proxies to an existing API server)
	//
	// The first token is the primary subcommand; additional tokens are
	// sublaunchers that start co-located sub-servers within the same process.
	//
	// launcher.Config wires in three infrastructure services:
	//
	// ArtifactService:
	//   artifact.InMemoryService() — in-process versioned file storage.
	//   Required for notebook_agent's save_artifact / load_artifact tools.
	//   For production replace with a GCS-backed service.
	//
	// TelemetryOptions:
	//   ADK instruments every agent turn, tool call, and LLM request with
	//   OpenTelemetry traces automatically. You don't need to add spans manually.
	//
	//   telemetry.WithGenAICaptureMessageContent(true):
	//     Includes full prompt/response text in span attributes.
	//     Disabled by default to avoid logging sensitive content in production.
	//     Also controllable via OTEL_INSTRUMENTATION_GENAI_CAPTURE_MESSAGE_CONTENT=true.
	//
	//   telemetry.WithSpanProcessors(processor):
	//     Registers additional span processors alongside the default OTLP exporter.
	//     Here we register loggingSpanExporter which prints every span to stderr.
	//     In production: use OTLP to Cloud Trace, Jaeger, Honeycomb, etc.
	//     For Cloud Trace: telemetry.WithOtelToCloud(true) + telemetry.WithGcpResourceProject("my-project")
	config := &launcher.Config{
		AgentLoader:     agent.NewSingleLoader(rootAgent),
		ArtifactService: artifact.InMemoryService(),
		TelemetryOptions: []telemetry.Option{
			telemetry.WithGenAICaptureMessageContent(true),
			telemetry.WithSpanProcessors(
				sdktrace.NewSimpleSpanProcessor(&loggingSpanExporter{}),
			),
		},
	}
	l := full.NewLauncher()
	if err = l.Execute(ctx, config, os.Args[1:]); err != nil {
		log.Fatalf("Run failed: %v\n\n%s", err, l.CommandLineSyntax())
	}
}

// ── Custom Agent: conditionalRouter ─────────────────────────────────────────

// conditionalRouter is the Run function for router_agent.
// It demonstrates direct implementation of the agent.Agent interface.
//
// In Go, any function with signature func(agent.InvocationContext) iter.Seq2[*session.Event, error]
// can power an agent. This is the idiomatic Go approach: no embedding,
// no interface boilerplate — just a function.
//
// Key patterns shown:
//   - Reading session state: ctx.Session().State()
//   - Producing events via the push-iterator yield pattern
//   - Conditional branching at the orchestration level (not inside an LLM)
func conditionalRouter(ctx agent.InvocationContext) iter.Seq2[*session.Event, error] {
	return func(yield func(*session.Event, error) bool) {
		// Read a routing key from session state — set by a previous agent via OutputKey.
		// State uses a Get/Set/All interface (not a plain map).
		// This is the standard way workflow agents communicate: one writes a state key,
		// the next reads it. No direct function calls between agents.
		state := ctx.Session().State()
		route := "default"
		if v, err := state.Get("route"); err == nil {
			if s, ok := v.(string); ok && s != "" {
				route = s
			}
		}

		// Count state entries using the All() iterator.
		stateCount := 0
		for range state.All() {
			stateCount++
		}

		// In a real custom agent you would call sub-agents conditionally here:
		//   for event, err := range someSubAgent.Run(ctx) { yield(event, err) }
		// or call external APIs, query databases, apply business rules, etc.
		text := fmt.Sprintf(
			"[RouterAgent] Conditional routing active — route=%q.\n"+
				"Session state keys present: %d.\n"+
				"In a production agent this node would call different sub-agents,\n"+
				"external APIs, or databases based on runtime conditions.",
			route, stateCount,
		)

		yield(&session.Event{
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{
					Parts: []*genai.Part{{Text: text}},
				},
			},
		}, nil)
	}
}

// ── Trace: loggingSpanExporter ────────────────────────────────────────────────
//
// loggingSpanExporter implements sdktrace.SpanExporter.
// It writes one slog.Info line per completed span to stderr, showing:
//   - span name (e.g. "llm_agent.run", "function_tool.execute")
//   - trace ID and span ID for correlation
//   - duration in milliseconds
//   - number of OTEL attributes captured
//
// ADK automatically creates spans for every agent turn, LLM request, and tool
// call — you do not need to instrument your own code unless you want custom spans.
//
// To send traces to a backend instead of logging them, replace this exporter with:
//   - Cloud Trace: telemetry.WithOtelToCloud(true) in TelemetryOptions
//   - OTLP endpoint: use otlptracegrpc.New(...) or otlptracehttp.New(...)
//   - Jaeger / Zipkin: their respective OTEL exporters
type loggingSpanExporter struct{}

// ExportSpans is called by the SimpleSpanProcessor when a span ends.
func (e *loggingSpanExporter) ExportSpans(_ context.Context, spans []sdktrace.ReadOnlySpan) error {
	for _, s := range spans {
		slog.Info("trace",
			"span", s.Name(),
			"trace_id", s.SpanContext().TraceID().String(),
			"span_id", s.SpanContext().SpanID().String(),
			"duration_ms", s.EndTime().Sub(s.StartTime()).Milliseconds(),
			"status", s.Status().Code.String(),
			"attributes", len(s.Attributes()),
		)
	}
	return nil
}

// Shutdown is called when the tracer provider shuts down.
func (e *loggingSpanExporter) Shutdown(_ context.Context) error { return nil }

// mustOK terminates the program if err is non-nil.
// Used during startup where any construction failure is unrecoverable.
func mustOK(err error, what string) {
	if err != nil {
		log.Fatalf("failed to %s: %v", what, err)
	}
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
