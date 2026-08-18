# Manual Terminal Test Guide — go-adk-q

Comprehensive prompt-driven tests for every feature of the reference implementation.
Run each section top-to-bottom; earlier turns intentionally build state for later ones.

---

## Prerequisites

```bash
# At least one provider key is required
export GITHUB_PAT=github_pat_...        # recommended — gpt-4o, no extra config
export GOOGLE_API_KEY=<your-key>        # required for memory, artifacts, llm_auditor
export GROQ_API_KEY=<gsk_...>           # optional fallback
export NVIDIA_API_KEY=<nvapi-...>       # optional fallback
export OPENROUTER_API_KEY=<key>         # optional fallback
export HF_TOKEN=<hf_...>               # optional fallback

# Select a specific provider as primary (default: first configured)
export PROVIDER_SELECTED=openrouter     # or: github, gemini, groq, nvidia, huggingface

# Zero-key echo fallback (local testing without any real keys)
export ECHO_FALLBACK_ENABLED=1

# Confirm keys before starting
make env
```

### Test modes

| Command | What runs | When to use |
|---------|-----------|-------------|
| `make run` | Bubbletea TUI (`cmd/tui`) | memory, artifacts, LLM auditor, skills, tools |
| `make console` | ADK console (`main.go`) | multi-agent routing, notebook, code pipeline, state |
| `make test-failover-echo` | Broken Gemini → echo | failover without any key |
| `make test-failover` | Broken Gemini → real fallback | failover with a fallback key |
| `tail -f "$TMPDIR/go-adk-q-tui.log"` | TUI background log | see slog output while TUI runs |

---

## 1. Basic Tools (TUI)

**Setup:** `make run` — works with any configured provider

Tests `get_weather`, `get_current_time`, `calculate` FunctionTools.
Expected: each prompt triggers exactly one tool call; response cites tool output.

```
What is the weather in Tokyo?
```

> Expect: Sunny, 25°C, humidity 60%, wind 8 km/h E

```
What is the weather in London?
```

> Expect: Light rain, 12°C, humidity 85%

```
What time is it in New York right now?
```

> Expect: current UTC time formatted with city label

```
What time is it in San Francisco?
```

> Expect: current UTC time formatted with city label (different offset from NY)

```
What is 137 multiplied by 48?
```

> Expect: 137 × 48 = 6576

```
What is the square root of 1764?
```

> Expect: √1764 = 42

```
Calculate 2 to the power of 16 with 0 decimal places.
```

> Expect: 2 ^ 16 = 65536

```
Divide 355 by 113 and show me 7 decimal places.
```

> Expect: 355 ÷ 113 = 3.1415929 (approximation of π)

```
What's the weather in Paris and then calculate 15% of 89.90?
```

> Expect: two tool calls in sequence — weather report then 15% calculation

**Pass criteria:** every response includes tool-sourced data; no hallucinated city data.

---

## 2. Session Memory — Within-Session Recall (TUI)

**Setup:** `make run` — requires `GOOGLE_API_KEY`

After each turn, `AddSessionToMemory` saves the session. On subsequent turns,
`preloadmemorytool` automatically injects relevant past context.
`loadmemorytool` lets the model search explicitly.

Send these prompts **in order in the same TUI session**:

```
My favourite programming language is Go.
```

> Expect: acknowledgement. Memory saved in background.

```
My project is called Arch Dot and it's a CLI tool for managing dotfiles.
```

> Expect: acknowledgement. Memory saved.

```
I prefer tab indentation and 100-character line width.
```

> Expect: acknowledgement. Memory saved.

```
What is my favourite programming language?
```

> Expect: Rust — recalled from preloaded memory context

```
What is the name of my project?
```

> Expect: Ferris — recalled from memory

```
What coding style preferences have I mentioned?
```

> Expect: tab indentation, 100-character line width

```
Summarise everything you know about me and my project from our conversation.
```

> Expect: coherent summary citing all three facts above

```
Search your memory for anything I said about indentation.
```

> Expect: load_memory tool fires with query "indentation"; returns tab preference

```
I changed my mind — I now prefer 2-space indentation. Update accordingly.
```

> Expect: acknowledges the change; new memory entry added

```
What indentation do I prefer now?
```

> Expect: 2-space (the updated preference, not the original tab)

**Pass criteria:** facts stated early in the session are retrievable later; updates override earlier values.

---

## 3. Artifacts via Notebook Agent (Console)

**Setup:** `make console` — requires `GOOGLE_API_KEY`

The `notebook_agent` has `save_artifact`, `load_artifact`, `save_to_state`, `read_from_state`.
Route explicitly with "ask notebook agent" to ensure delegation.

```
Ask the notebook agent to save a file called notes.txt with this content: "Meeting on Friday at 3pm. Action items: review PR #42, update CHANGELOG."
```

> Expect: artifact saved, version 1 returned

```
Ask the notebook agent to load notes.txt.
```

> Expect: full file content returned, version shown

```
Ask the notebook agent to save an updated version of notes.txt with this content: "Meeting on Friday at 3pm. Action items: review PR #42, update CHANGELOG, write unit tests."
```

> Expect: version 2 returned

```
Ask the notebook agent to load version 1 of notes.txt.
```

> Expect: original content without "write unit tests" — version pinning works

```
Ask the notebook agent to save a file called config.json with this content: {"theme": "dark", "fontSize": 14} and mime type application/json.
```

> Expect: JSON artifact saved as version 1 with correct mime type

```
Ask the notebook agent to load config.json.
```

> Expect: JSON content returned

```
Ask the notebook agent to save a markdown file called SUMMARY.md with a short description of what we stored today.
```

> Expect: model composes a description and saves it as artifact

```
Ask the notebook agent to list what files it has saved — load each one and summarise their contents.
```

> Expect: loads notes.txt (latest), config.json, SUMMARY.md and gives a summary

**Pass criteria:** version numbers increment; version pinning retrieves historical content; mime types are respected.

---

## 4. Session State (Console)

**Setup:** `make console` — requires `GOOGLE_API_KEY`

Tests `save_to_state`, `read_from_state`, and the `temp:` ephemeral prefix.

```
Ask the notebook agent to store the value "production" under the key "environment".
```

> Expect: key written, written=true

```
Ask the notebook agent to read the value of the "environment" key.
```

> Expect: "production"

```
Ask the notebook agent to store "v2.1.3" under "release_version".
```

> Expect: key written

```
Ask the notebook agent to read "release_version".
```

> Expect: "v2.1.3"

```
Ask the notebook agent to store "do-not-persist" under "temp:scratch_pad".
```

> Expect: written with temp: prefix; note it won't survive session end

```
Ask the notebook agent to read "temp:scratch_pad".
```

> Expect: "do-not-persist" (still readable within the session)

```
Ask the notebook agent to read "nonexistent_key".
```

> Expect: found=false, empty value — graceful not-found path

```
Ask the notebook agent to calculate 8 to the power of 3, then store the result under "cube_result".
```

> Expect: 8^3 = 512 calculated, then stored in state under "cube_result"

```
Ask the notebook agent to read "cube_result".
```

> Expect: "512"

**Pass criteria:** state round-trips correctly; temp: keys behave identically within session; missing keys return found=false not an error.

---

## 5. Long-Running Tasks (Console)

**Setup:** `make console` — requires `GOOGLE_API_KEY`

Tests `start_report` (IsLongRunning tool) and `check_report` polling pattern.

```
Ask the notebook agent to start a report on "Go concurrency patterns".
```

> Expect: status=PENDING, task_id=rpt-XXXXXX returned immediately

```
Ask the notebook agent to check the status of the report (use the task_id from the previous response).
```

> Expect: status=IN_PROGRESS, progress ~40%

```
Check the report status again.
```

> Expect: status=IN_PROGRESS or DONE, progress ~80%

```
Check one more time.
```

> Expect: status=DONE, progress=100, summary field populated

```
Ask the notebook agent to start another report on "Go concurrency patterns" again.
```

> Expect: status=ALREADY_RUNNING if the first task is still in state, OR a new task_id if it completed (idempotency guard active)

```
Ask the notebook agent to start a deep report on "Kubernetes networking".
```

> Expect: new task_id, depth=deep

```
Immediately ask the notebook agent to start the same "Kubernetes networking" report.
```

> Expect: ALREADY_RUNNING with the existing task_id — no duplicate task

**Pass criteria:** IsLongRunning prevents the LLM from re-calling immediately; polling advances 40% per check; ALREADY_RUNNING prevents duplicates.

---

## 6. LLM Auditor — Fact-Checking (TUI)

**Setup:** `make run` — requires `GOOGLE_API_KEY`

The LLM Auditor is wired as a sub-agent tool. The root agent delegates when asked
to "fact-check" or "verify". The critic grounds claims via Google Search; the reviser
applies minimal fixes. Expect 2 LLM round-trips (critic → reviser) per audit.

```
Use the LLM auditor to fact-check this answer: "The Eiffel Tower is 330 metres tall and was built in 1889 for the 1900 World's Fair."
```

> Expect: critic identifies the "1900 World's Fair" claim as inaccurate (it was the 1889 World's Fair / Exposition Universelle); reviser corrects to 1889

```
Use the LLM auditor to verify this: "Python was created by Guido van Rossum and first released in 1991."
```

> Expect: both claims Accurate; revised answer identical to original

```
Use the LLM auditor to check this: "The Great Wall of China is visible from the Moon with the naked eye."
```

> Expect: critic marks this as Inaccurate (NASA/multiple sources refute it); reviser softens or corrects the claim

```
Use the LLM auditor to review: "Go 1.18 introduced generics and was released in March 2022."
```

> Expect: both claims Accurate; minimal or no revision

```
Use the LLM auditor to fact-check: "The speed of light in a vacuum is approximately 300,000 km/s, which Einstein proved in 1905."
```

> Expect: speed of light Accurate; "Einstein proved" claim Disputed or Inaccurate (it was measured before Einstein); reviser rephrases to "described" or similar

```
Use the LLM auditor on this: "Redis is an in-memory key-value store written in C, created by Salvatore Sanfilippo in 2009."
```

> Expect: all claims Accurate; no revision

```
Use the LLM auditor to verify: "The first iPhone was released in 2007 by Steve Jobs. It ran iOS 1.0 and supported 5G connectivity."
```

> Expect: 2007 and Steve Jobs Accurate; 5G on first iPhone Inaccurate (no 5G until iPhone 12 in 2020); reviser removes 5G claim

```
Fact-check without the LLM auditor: Is the Eiffel Tower 500 metres tall?
```

> Expect: root agent answers directly from knowledge — no auditor delegation; no critic/reviser pass. Confirms correct height ~330m.

**Pass criteria:** inaccurate claims are corrected; accurate answers are unchanged; the EndMark sentinel never appears in final output.

---

## 7. Skills (TUI)

**Setup:** `make run` — works with any configured provider; requires `./skills/` directory present

The skills toolset is loaded from `./skills/` at startup; `list_skills`, `load_skill`,
and `load_skill_resource` are injected into the agent's tool schema. The LLM calls
`load_skill` when the user's request matches a skill's description.

### 7a. go-expert skill

```
Load the go-expert skill and explain the best way to handle errors in Go when wrapping them for callers.
```

> Expect: skill loaded; response follows go-expert guidelines: errors.Is/As, %w wrapping

```
Using the go-expert skill, write a Go function that reads a file and returns its lines as a slice. Show the complete runnable snippet.
```

> Expect: complete package + import block; context.Context first param; errors wrapped with %w

```
With the go-expert skill active, show me how to write a table-driven test for a function that parses integers.
```

> Expect: t.Run subtests; table struct with input/expected/wantErr fields; no string-matched errors

```
Go expert: how do I share data safely between goroutines — when should I use channels vs mutexes?
```

> Expect: explanation following skill guideline — channels when ownership is clear, mutex for struct field protection

```
Go expert: I have a package called utils that contains helper functions. Is this a good design?
```

> Expect: skill flags utils/helpers as design smell; suggests splitting by purpose

```
Go expert: write a context-cancellable HTTP client wrapper with timeout and retries.
```

> Expect: context.Context first param; uses context.WithTimeout; defer resp.Body.Close

### 7b. code-reviewer skill

```
Load the code-reviewer skill and review this Go code:
func divide(a, b int) int {
    return a / b
}
```

> Expect: structured review with ## Critical issues (missing b==0 check), ## Suggestions, ## Praise

```
Code reviewer: review this error handling:
func getUser(id string) User {
    user, _ := db.Find(id)
    return user
}
```

> Expect: Critical issue: error ignored with _, nil returned on failure; suggest returning (User, error)

```
Code reviewer: check this for security issues:
query := "SELECT * FROM users WHERE name = '" + name + "'"
db.Query(query)
```

> Expect: Critical: SQL injection vulnerability; fix: use parameterised queries

```
Code reviewer: evaluate this concurrency pattern:
var counter int
go func() { counter++ }()
go func() { counter++ }()
```

> Expect: Critical: data race on counter; fix: sync/atomic or sync.Mutex

```
Code reviewer: review this function for performance:
func containsDuplicate(nums []int) bool {
    for i := 0; i < len(nums); i++ {
        for j := i+1; j < len(nums); j++ {
            if nums[i] == nums[j] { return true }
        }
    }
    return false
}
```

> Expect: O(n²) flagged; fix: use map[int]struct{} for O(n)

```
Code reviewer: what is good about this code?
func (s *Server) Shutdown(ctx context.Context) error {
    s.mu.Lock()
    s.done = true
    s.mu.Unlock()
    return s.httpServer.Shutdown(ctx)
}
```

> Expect: ## Praise section notes mutex protection, context usage, clean shutdown pattern

### 7c. terminal-assistant skill

```
Load the terminal-assistant skill. How do I safely delete all .log files in the current directory and its subdirectories?
```

> Expect: find with -print0 + xargs -0; warns rm is destructive; POSIX sh

```
Terminal assistant: write a bash script that backs up a directory to a timestamped tarball.
```

> Expect: set -euo pipefail; quoted variables; $(date +%Y%m%d_%H%M%S)

```
Terminal assistant: how do I extract the value of the "version" field from a package.json using jq?
```

> Expect: jq -r '.version' package.json; -r for raw string output

```
Terminal assistant: I need to find all Go files modified in the last 24 hours. What command do I run on macOS?
```

> Expect: find . -name "*.go" -mtime -1; notes macOS find behaviour

```
Terminal assistant: how do I see which port a process is listening on, on macOS?
```

> Expect: lsof -i :<port> or lsof -iTCP -sTCP:LISTEN; or netstat -anp tcp

```
Terminal assistant: write a one-liner to count the number of TODO comments across all .go files in this repo.
```

> Expect: grep -r "TODO" --include="*.go" | wc -l or similar; notes -r flag

### 7d. Verifying a skill was actually called

To confirm `load_skill` fired (not just that the response looks skill-like), grep the log
while the TUI is running:

```bash
# In a separate terminal pane
tail -f "$TMPDIR/go-adk-q-tui.log" | grep -i "load_skill\|list_skills\|skill"
```

Expected log entries when a skill loads:

| Event | What to look for |
|-------|-----------------|
| `list_skills` called | `span=function_tool.execute tool=list_skills` |
| `load_skill` called | `span=function_tool.execute tool=load_skill name=go-expert` (or other skill name) |
| `load_skill_resource` called | `span=function_tool.execute tool=load_skill_resource` |
| Skills toolset init | `INFO skills toolset enabled path=./skills` (at startup) |

To do a one-shot grep after a session:

```bash
grep "load_skill\|list_skills" "$TMPDIR/go-adk-q-tui.log"
```

If `load_skill` does not appear in the log, the model answered from training knowledge
without loading the skill — the response may look correct but is not skill-guided.

**Pass criteria:** each skill response visibly follows the skill's format and guidelines; non-skill questions do not unnecessarily trigger skill loading.

---

## 8. Multi-Agent Routing (Console)

**Setup:** `make console` — requires `GOOGLE_API_KEY`

Tests that the root agent correctly delegates to the right sub-agent based on description matching.

### 8a. weather_time_agent

```
What is the weather in Sydney right now?
```

> Expect: delegated to weather_time_agent; Partly cloudy, 22°C

```
What time is it in Tokyo?
```

> Expect: weather_time_agent; current UTC time with offset

```
I'm flying from Paris to New York. What's the weather at both cities?
```

> Expect: weather_time_agent calls get_weather twice

### 8b. code_pipeline (SequentialAgent)

```
Write, review, and refactor a Go function that reverses a string.
```

> Expect: 3-stage pipeline — CodeWriter generates code, CodeReviewer lists improvements, CodeRefactorer applies them; final output is refined code

```
Use the code pipeline to create a Go function that checks if a number is prime.
```

> Expect: pipeline runs; reviewer flags edge cases (0, 1, negatives); refactorer applies fixes

```
Ask the code pipeline to write a thread-safe counter in Go.
```

> Expect: Writer produces initial version; Reviewer finds sync issues if any; Refactorer improves

### 8c. doc_refinement_loop (LoopAgent)

```
Draft and refine a technical document explaining what a goroutine is.
```

> Expect: 1-3 iteration cycles visible; QualityChecker eventually outputs APPROVED or max 3 iterations reached

```
Use the doc refinement loop to write a short explanation of Go interfaces for beginners.
```

> Expect: iterative improvement visible; final version cleaner than initial draft

### 8d. parallel_analysis (ParallelAgent)

```
Analyse Go's generics feature from both technical and business perspectives.
```

> Expect: TechResearcher and BizAnalyst run concurrently; both results returned

```
Do a parallel analysis of WebAssembly.
```

> Expect: parallel execution; technical + business angles both present in output

### 8e. router_agent (Custom agent)

```
Show me the router agent demo.
```

> Expect: custom Run function output — route, state count, routing explanation text

```
Trigger the custom routing logic.
```

> Expect: [RouterAgent] prefix in output; route=default unless state key set

### 8f. Optional provider agents

```
Ask the Groq agent: explain Go channels in one sentence.
```

> Expect: groq_agent responds (if GROQ_API_KEY set); otherwise "not available"

```
Ask the NVIDIA agent: what is the difference between a process and a goroutine?
```

> Expect: nvidia_agent responds (if NVIDIA_API_KEY set)

```
Compare the Groq and Gemini answers to: what is dependency injection?
```

> Expect: both agents invoked; response shows two answers for comparison

**Pass criteria:** root agent routes to the correct sub-agent in every case; incorrect delegations are a failure.

---

## 9. Failover Chain

### 9a. Echo fallback (no API keys needed)

```bash
make test-failover-echo
```

Then in the console session:

```
hello
```

> Expect slog output: `WARN failover: provider error, trying next provider=gemini-intentionally-broken` then `INFO failover: recovered via fallback provider=echo`

```
what is the capital of France?
```

> Expect: echo stub mirrors the input (not a real answer) — confirms echo is active

```
calculate 2 + 2
```

> Expect: echo mirrors; no tool call — echo does not execute tools

```
hello again
```

> Expect: same echo behaviour; confirms persistent failover

### 9b. Live failover (requires a real fallback key)

```bash
make test-failover-live
# Requires at least one of GROQ/NVIDIA/OPENROUTER/HF set
```

```
hello
```

> Expect: slog WARN on Gemini failure; INFO on fallback recovery; real answer from backup provider

```
what is 5 + 5?
```

> Expect: answered by the fallback provider using the `calculate` tool (basic tools are enabled for all providers with function calling)

```
tell me a fun fact about Go
```

> Expect: fallback provider answers from training knowledge

**Pass criteria:** failover fires automatically; no crash; answer is delivered from the fallback provider.

---

## 10. TUI Artifacts via load_artifacts Tool

**Setup:** `make run` (TUI) — requires `GOOGLE_API_KEY`

The TUI agent has `load_artifacts` to inspect saved files. Artifacts must first be saved
via the web API or console notebook_agent (different process — in-memory services are
separate). Within the same TUI session, no artifacts are pre-populated.

```
List all artifacts you have access to.
```

> Expect: "no artifacts available" or empty list — load_artifacts returns nothing on a fresh session; the model should state this clearly rather than hallucinating files

```
Do I have any files saved?
```

> Expect: same — empty artifact list; model confirms nothing is stored

```
Load an artifact called notes.txt.
```

> Expect: error response or "artifact not found" — graceful handling of missing artifact

```
What artifacts would you be able to load if I had saved some?
```

> Expect: model explains the load_artifacts tool capability; lists supported operations

**Pass criteria:** model does not hallucinate artifact contents; missing artifact is handled gracefully.

---

## 11. Tool Edge Cases and Error Handling

**Setup:** `make run` — requires `GOOGLE_API_KEY`

```
What is the weather in Atlantis?
```

> Expect: `get_weather` fires; returns "Weather data for Atlantis is not available in the simulation"; model relays this honestly

```
What is the weather in an unnamed city?
```

> Expect: model prompts for a city name OR passes empty string and gets not-available response

```
Calculate 100 divided by 0.
```

> Expect: `calculate` returns division-by-zero error; model relays the error message

```
Calculate the square root of negative 4.
```

> Expect: `calculate` returns "cannot take square root of a negative number"; model reports the error

```
Calculate 2 to the power of 1000 with 5 decimal places.
```

> Expect: result (very large number or +Inf); no crash; model formats it

```
What is the time in a city that does not exist?
```

> Expect: `get_current_time` returns not-available; model handles gracefully

```
Add the word "hello" and the number 5.
```

> Expect: model recognises type mismatch; either prompts for clarification or declines with explanation

**Pass criteria:** tool errors are relayed clearly; agent does not crash; no silent failures.

---

## 12. Conversational Bypass (No Tool Delegation)

**Setup:** `make run`

The agent's instruction says to answer greetings and simple questions directly.
These should NOT trigger any tool call.

```
Hello!
```

> Expect: direct greeting; no tool call

```
What is your name?
```

> Expect: "layar-cli" or a description of itself; no tool call

```
How are you today?
```

> Expect: direct conversational answer; no tool call

```
Tell me a joke about programming.
```

> Expect: a joke; no tool call; no delegation

```
What is 2 + 2?
```

> Expect: direct answer "4" without invoking the calculate tool (trivial arithmetic)

```
Who wrote Go?
```

> Expect: Robert Griesemer, Rob Pike, Ken Thompson; no tool call; no LLM Auditor invocation

**Pass criteria:** no spurious tool calls on conversational input.

---

## 13. LLM Auditor — Edge Cases (TUI)

**Setup:** `make run` — requires `GOOGLE_API_KEY`

```
Fact-check this: "Water boils at 100°C at standard atmospheric pressure."
```

> Expect: Accurate verdict; revised answer identical to input; EndMark never visible in output

```
Use the LLM auditor on: "The answer is 42."
```

> Expect: Not Applicable verdict (no factual claim to verify); no revision

```
Ask the LLM auditor to verify an empty statement.
```

> Expect: graceful handling; either "nothing to verify" or a minimal Accurate/NA verdict

```
Use the auditor to fact-check a 3-part statement: "Go was created at Google. It was first released in 2009. The mascot is a gopher drawn by Rob Pike's wife Renée French."
```

> Expect: first two Accurate; third Accurate (Renée French did design the gopher)

```
Verify: "Rust's ownership model was invented by Mozilla. Firefox is written entirely in Rust."
```

> Expect: "invented by Mozilla" Disputed/Inaccurate (ownership concept predates Rust); "entirely in Rust" Inaccurate (partly C++/JS); reviser softens both claims

```
Fact-check using the LLM auditor and then save the result as an artifact called audit_result.txt.
Statement to check: "The Linux kernel was written by Linus Torvalds and first released in 1991."
```

> Expect: auditor runs (both claims Accurate); but TUI has no save_artifact — model should explain it cannot save in this mode; or use load_artifacts which also won't help. Tests graceful degradation when a requested tool is absent.

**Pass criteria:** EndMark sentinel NEVER appears in user-visible output; accurate inputs are unchanged; inaccurate inputs are meaningfully corrected.

---

## 14. Load Log Output While Testing (TUI)

Run this in a separate terminal pane while the TUI is active:

```bash
tail -f "$TMPDIR/go-adk-q-tui.log"
```

Expected slog entries to watch for:

| Event | Log line |
|-------|----------|
| Startup | `INFO model chain providers=...` |
| Skills loaded | `INFO skills toolset enabled path=./skills` |
| Failover event | `WARN failover: provider error, trying next` |
| Failover success | `INFO failover: recovered via fallback` |
| Memory save | *(no log by default — add slog.Debug in AddSessionToMemory call if needed)* |
| Tool spans | `INFO trace span=function_tool.execute ...` |
| LLM spans | `INFO trace span=llm_agent.run ...` |

```bash
# Grep for errors only
grep -i "error\|fatal\|warn" "$TMPDIR/go-adk-q-tui.log"

# Watch tool call spans
grep "function_tool" "$TMPDIR/go-adk-q-tui.log"

# Watch LLM auditor spans
grep "critic_agent\|reviser_agent\|llm_auditor" "$TMPDIR/go-adk-q-tui.log"

# Token usage across all turns
grep "duration_ms\|attributes" "$TMPDIR/go-adk-q-tui.log"
```

---

## 15. Full Session Integration Run (TUI)

A single coherent session exercising every major feature in sequence.
Run `make run` with `GOOGLE_API_KEY` set.

```
Hi, I'm Alice. I'm a Go developer working on a CLI tool called "depot".
```

```
My depot project uses Go 1.22 and targets Linux and macOS.
```

```
What's the weather in the city where Go was invented? (Google's HQ is in Mountain View, California)
```

```
Calculate how many days are in 3 years and 47 days (ignore leap years).
```

```
Load the go-expert skill and show me how to write a context-aware HTTP request function for depot.
```

```
Use the code-reviewer skill to review this: func fetchData(url string) []byte { resp, _ := http.Get(url); body, _ := ioutil.ReadAll(resp.Body); return body }
```

```
What is my name and what am I working on?
```

```
Search your memory for anything I've told you about the "depot" project.
```

```
Use the LLM auditor to fact-check: "Go 1.22 introduced range-over-integers and was the first Go version to ship slices.Concat."
```

```
What tools and skills have been used in this conversation so far?
```

**Expected arc:**

1. Memory: Alice + depot + Go 1.22 stored across all turns
2. Weather: Mountain View not in mock data → "not available" response
3. Calculator: 3 × 365 + 47 = 1142 days
4. Skills: go-expert and code-reviewer both load; reviews follow their structured formats
5. Memory recall: "Alice", "depot", "Go 1.22" all retrievable
6. LLM Auditor: range-over-integers Accurate for Go 1.22; slices.Concat claim checked
7. Final question: model summarises tools used — demonstrates working memory

---

## Quick Reference — Expected Tool Triggers

| Prompt keyword | Tool/agent invoked |
|----------------|--------------------|
| "weather in [city]" | `get_weather` |
| "time in [city]" | `get_current_time` |
| "calculate / multiply / divide / sqrt / power" | `calculate` |
| "remember / recall / search memory" | `load_memory` |
| *(any turn with prior context)* | `preload_memory` auto-injects |
| "list / load artifacts / files" | `load_artifacts` |
| "fact-check / verify / use LLM auditor" | `llm_auditor` sub-agent |
| "go expert / code reviewer / terminal assistant" | `load_skill` |
| "what skills are available / list skills" | `list_skills` |
| "weather + time questions" (console) | `weather_time_agent` |
| "write / review / refactor code" (console) | `code_pipeline` |
| "draft / improve / refine document" (console) | `doc_refinement_loop` |
| "analyse from multiple angles" (console) | `parallel_analysis` |
| "save / read state / calculate / report" (console) | `notebook_agent` |

---

## Known Limitations

- **In-memory only.** `session.InMemoryService()`, `memory.InMemoryService()`, and `artifact.InMemoryService()` do not persist across process restarts. Every `make run` starts fresh.
- **TUI has no `save_artifact`.** The TUI agent has `load_artifacts` but not `save_artifact`. Console `notebook_agent` has both. In-memory services are per-process so artifacts saved in console are not visible in TUI.
- **Non-Gemini providers have partial tool support.** Basic tools (`weather`, `time`, `calculator`, `list_skills`, `load_skill`, `load_skill_resource`) are enabled for all providers with function calling. Advanced tools (`preload_memory`, `load_memory`, `load_artifacts`, `llm_auditor`) require `GOOGLE_API_KEY` — these involve multi-step tool chains that non-Gemini models handle less reliably.
- **LLM Auditor requires ~2 LLM calls.** Critic + Reviser each make one round-trip. Expect 5-15s latency per audit vs 1-3s for a direct answer.
- **Google Search grounding in critic.** `geminitool.GoogleSearch{}` is wired to the critic agent. If your API key tier does not support grounding, the critic uses training knowledge only and `GroundingMetadata` will be nil — `afterCritic` handles this gracefully (no reference section appended).
- **Failover echo is testing-only.** `ECHO_FALLBACK_ENABLED=1` returns the user's input verbatim. Never use in production.

---

## 16. TUI Mouse Scroll and Text Copy

**Setup:** `make run` — no API key needed for UI behaviour tests

### 16a. Touchpad / mouse wheel scroll

Scroll up and down through chat history using the touchpad or mouse wheel.

> Expect: viewport scrolls smoothly up and down through message history

Keyboard scroll also works at any time:

| Key | Action |
|-----|--------|
| `↑` / `↓` | Scroll one line |
| `pgup` / `pgdn` | Scroll one page |
| `ctrl+u` / `ctrl+d` | Scroll half page |

### 16b. Copying text (ctrl+t toggle)

Mouse mode is active by default so touchpad scroll works. When mouse mode is on, the terminal cannot intercept click-drag for native text selection. Use `ctrl+t` to toggle:

1. Press **`ctrl+t`** — status bar shows `Copy mode ON — select text freely  (ctrl+t for scroll)`
   - Mouse/touchpad scroll stops
   - Click and drag to select any text, code block, or response → copy with ⌘C (macOS) or your terminal's copy shortcut
2. Press **`ctrl+t`** again — status bar shows `Scroll mode ON — touchpad scrolls`
   - Mouse/touchpad scroll resumes
   - Text selection via click-drag is blocked again

> The footer always shows `ctrl+t: copy mode` as a reminder.

**Pass criteria:**

- Touchpad scroll works without pressing any key (default state)
- `ctrl+t` disables mouse mode — text can be selected and copied freely
- `ctrl+t` again re-enables scroll — touchpad works again
- No other functionality is affected by toggling
