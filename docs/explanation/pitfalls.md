# Explanation: Pitfalls

> Ten documented gotchas from this codebase. Each entry: exact symptom →
> root cause → fix → prevention. Read before adding a new agent, tool, or
> provider.

---

## P1 — Missing `jsonschema:` tag makes tools invisible to the LLM

**Symptom**: A FunctionTool is registered and the server starts cleanly, but
the LLM never calls the tool — or calls it with empty, nil, or garbage
arguments that cause downstream panics.

**Root cause**: The JSON Schema sent to the model for each tool parameter
includes a `description` field. This field is populated from the `jsonschema:`
struct tag. When the tag is absent, `description` is omitted from the schema,
and many models either skip the tool entirely or cannot construct valid arguments.

```go
// WRONG — LLM cannot understand the `city` parameter
type getWeatherArgs struct {
    City string `json:"city"`
}

// CORRECT — LLM receives: {"city": {"type":"string","description":"The city name."}}
type getWeatherArgs struct {
    City string `json:"city" jsonschema:"The name of the city to get weather for."`
}
```

**Fix**: Add `jsonschema:"Description text."` to every field in every args struct.

**Prevention**: Copy the canonical FunctionTool skeleton from `AGENTS.md §5`.
Both tags are in the template — don't remove either.

---

## P2 — OpenAI-compatible API rejects `list_skills` with HTTP 400

**Symptom**: When using Groq, NVIDIA NIM, OpenRouter, or any strict
OpenAI-compatible endpoint, calling `list_skills` returns an HTTP 400 error
with a message like "invalid schema: properties required" or
"tool schema validation failed".

**Root cause**: The upstream `google.golang.org/adk/tool/skilltoolset`
generates a JSON Schema of `{"type":"object"}` with no `"properties"` key for
`list_skills` (which takes no arguments). OpenAI's strict schema validation
requires that object schemas include a `"properties"` field, even if empty.

**Fix**: Always use the local `tool/skilltoolset` wrapper:

```go
import localskilltoolset "go-adk-q/tool/skilltoolset"

skillset, err := localskilltoolset.New(ctx, localskilltoolset.Config{...})
```

The local wrapper replaces `list_skills` with a patched version whose schema
includes `"properties": {}`.

**Prevention**: The local package is the only export path for SkillToolset in
this repo. Never import `google.golang.org/adk/tool/skilltoolset` directly.

---

## P3 — Groq / NVIDIA multi-tool prompt → `tool_use_failed`

**Symptom**: An agent using `groqLLM` or `nvidiaLLM` returns an error
response containing `tool_use_failed`, or the model output is garbled JSON
that cannot be parsed as a function call.

**Root cause**: Groq's LLaMA-based models and NVIDIA NIM's MiniMax M1 model
mishandle requests where multiple tool declarations are present in the same
system prompt. The model attempts to generate a tool-call JSON blob but
produces invalid output.

```go
// WRONG — adding tools to a Groq agent
groqAgent, _ := llmagent.New(llmagent.Config{
    Model:    groqLLM,
    Tools:    []tool.Tool{tools.NewWeatherTool()},  // causes tool_use_failed
    Toolsets: agentToolsets,                         // causes tool_use_failed
})
```

**Fix**: Remove all `Tools` and `Toolsets` from Groq and NVIDIA agent configs.
These agents are comparison/routing agents only — they don't need tools.

**Prevention**: Rule R4. Provider-specific agents are defined without tools.
The root agent (using `m`, the failover chain) handles all tool use.

---

## P4 — LoopAgent runs MaxIterations unconditionally

**Symptom**: A doc-refinement or code-review loop always runs the full
`MaxIterations` cycles even when the LLM output clearly says "APPROVED" or
equivalent. The loop never exits early.

**Root cause**: The ADK `LoopAgent` terminates early only when a sub-agent
calls the `exit_loop` FunctionTool. If no sub-agent has `exitlooptool.New()`
in its `Tools` list, the LLM has no mechanism to signal termination, so the
loop runs to completion every time.

```go
// WRONG — no ExitLoopTool; loop always runs MaxIterations
reviewer, _ := llmagent.New(llmagent.Config{
    Instruction: "If quality is good, respond APPROVED. Otherwise suggest improvement.",
})

// CORRECT — ExitLoopTool gives the LLM a way to terminate the loop
reviewer, _ := llmagent.New(llmagent.Config{
    Tools: []tool.Tool{exitlooptool.New()},
    Instruction: "If quality is APPROVED, call exit_loop. Otherwise suggest ONE improvement.",
})
```

**Fix**: Add `exitlooptool.New()` to the `Tools` of the checker/reviewer agent
inside every `LoopAgent`.

**Prevention**: Rule R5. Treat ExitLoopTool as a required component of any
LoopAgent design, the same way a loop in code needs a break condition.

---

## P5 — Importing `firebase/genkit` outside `model/oaibridge`

**Symptom**: Compile-time error "duplicate type registration" or
"conflicting plugin init"; or subtle runtime issues where telemetry spans
appear duplicated, or the Genkit reflection server starts when it shouldn't.

**Root cause**: `github.com/firebase/genkit/go` registers global state (plugin
registry, telemetry hooks, reflection API) in `init()`. Importing it from more
than one package in the same binary causes double registration.

**Fix**: Remove the genkit import from the offending package. Use
`oaibridge.NewModel(oaibridge.Config{...})` to create any OpenAI-compatible
provider model — this delegates to Genkit internally without exposing the import.

**Prevention**: ADR-0002. The import boundary is explicit:

```
model/oaibridge   ← only file with: import "github.com/firebase/genkit/go/..."
model/{groq,...}  ← import "go-adk-q/model/oaibridge" (not genkit)
all other code    ← no genkit imports permitted
```

---

## P6 — Duplicate `OutputKey` in SequentialAgent silently loses data

**Symptom**: A SequentialAgent pipeline runs without error, but the output of
an early stage is mysteriously missing when later stages try to reference it
via `{key}` interpolation. The later stage's output overwrites the earlier one.

**Root cause**: Two agents in the same `SubAgents` list share the same
`OutputKey` string. ADK stores each agent's final text response in
`session.State[OutputKey]`. The second agent overwrites the first silently.

```go
// WRONG — "result" is overwritten by stage2
stage1, _ := llmagent.New(llmagent.Config{OutputKey: "result", ...})
stage2, _ := llmagent.New(llmagent.Config{OutputKey: "result", ...})

// CORRECT — distinct keys
stage1, _ := llmagent.New(llmagent.Config{OutputKey: "stage1_draft", ...})
stage2, _ := llmagent.New(llmagent.Config{OutputKey: "stage2_review", ...})
```

**Fix**: Rename keys to be unique and semantically descriptive.

**Prevention**: Rule R8. Always prefix keys with the agent or stage name.

---

## P7 — Failover first-token latency is the full round-trip time

**Symptom**: Compared to a single-provider streaming setup, the first token
from the agent takes noticeably longer, even when the primary provider is
healthy and fast.

**Root cause**: `failover.Model.GenerateContent` always buffers the complete
response from the upstream provider before forwarding it to the caller
(ADR-0003). This is necessary so that a partially-delivered response can be
cleanly retried on a different provider if an error occurs mid-stream.

The trade-off: first-token latency = full response time (not streaming latency).

**Fix**: This is intentional design, not a bug. For use-cases where streaming
latency is critical and you can accept reduced reliability, use a single
provider model directly and skip the failover wrapper.

**Prevention**: Set realistic latency expectations in product design.
Document this behaviour in any user-facing SLO.

---

## P8 — Echo model deployed in production

**Symptom**: Every agent response is a variation of:
`"I am the echo model and I received: [your message]"`.
Users are confused. Logs show `provider=echo`.

**Root cause**: `ECHO_FALLBACK_ENABLED=1` was set in a non-development
environment (staging, production).

The echo model (`model/echo/echo.go`) never calls any LLM API. It reflects
its configured static message back. Its sole purpose is to verify the failover
chain works end-to-end in CI without requiring real credentials.

**Fix**: Remove `ECHO_FALLBACK_ENABLED` from the deployment config.
Restart the service.

**Prevention**: Add `ECHO_FALLBACK_ENABLED` to the explicit deny-list in your
CI/CD production environment variable policy.

---

## P9 — All providers nil → failover chain empty at startup

**Symptom**: The binary starts, but the first agent invocation panics with
`"no providers in failover chain"` or `"model chain is empty"`.
`make env` shows all API keys as `(not set)`.

**Root cause**: `failover.New` accepts nil entries safely (they are filtered
out). But if every entry is nil — i.e. not a single API key is configured —
the resulting chain has zero providers and will fail on the first
`GenerateContent` call.

**Fix**: Set at least one provider API key. The minimum viable setup:

```sh
export GOOGLE_API_KEY=<key>    # Gemini — free tier at aistudio.google.com
```

**Prevention**: Add a startup assertion after `failover.New`:

```go
m, err := failover.New(ctx, ...)
mustOK(err, "create failover model")
if m == nil {
    log.Fatal("no LLM providers configured — set at least one API key")
}
```

---

## P10 — Skills not found at runtime (wrong working directory)

**Symptom**: `list_skills` returns an empty XML list (`<available_skills></available_skills>`).
The TUI starts normally but skill commands produce no results.
Logs may show `stat ./skills: no such file or directory`.

**Root cause**: `main.go` checks for the skills directory with
`os.Stat("./skills")`. This is a relative path from the current working
directory at runtime. If the binary is launched from a directory other than
the repo root (e.g. `./bin/tui` from a deploy directory), `./skills` is not
found and the SkillToolset is never initialized.

**Fix**: Always run from the repo root:

```sh
# Correct — from repo root
make run
go run ./cmd/tui chat

# Wrong — skills directory not found
cd /tmp && /path/to/tui chat
```

For deployed binaries, set an absolute path:

```sh
export SKILLS_DIR=/opt/go-adk-q/skills
# Then update main.go to read from SKILLS_DIR if set
```

**Prevention**: Use `make run` for local development. For production deployments,
pass the skills directory as an environment variable or command-line flag.
