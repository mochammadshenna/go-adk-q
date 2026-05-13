# Tutorial: Build your first custom agent

**Goal:** Write a Go program that creates an ADK `LlmAgent`, gives it a custom
tool, and runs it against a real LLM provider.

**Prerequisites:**
- Completed [get-started.md](get-started.md)
- `GEMINI_API_KEY` or `GROQ_API_KEY` set in your environment

---

## Step 1 — Create a sample directory

```sh
mkdir -p sample/mytool && cd sample/mytool
```

## Step 2 — Write the tool

ADK tools are typed Go functions annotated with `json` and `jsonschema` tags.
Create `main.go`:

```go
package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"

    "google.golang.org/adk/agent/llmagent"
    "google.golang.org/adk/runner"
    "google.golang.org/adk/session"
    "google.golang.org/adk/tool"
    "google.golang.org/genai"

    "go-adk-q/model/groq"
)

// wordCountArgs is the input schema for the wordCount tool.
type wordCountArgs struct {
    Text string `json:"text" jsonschema:"description=The text to count words in"`
}

// wordCount counts words in the provided text.
func wordCount(_ context.Context, args wordCountArgs) (map[string]any, error) {
    words := len(strings.Fields(args.Text))
    return map[string]any{"word_count": words}, nil
}

func main() {
    ctx := context.Background()

    // Build a model using the provider Config pattern.
    model, err := groq.NewModel(ctx, groq.ConfigFromEnv())
    if err != nil {
        log.Fatalf("model: %v", err)
    }

    // Wrap the Go function as an ADK FunctionTool.
    wc, err := tool.NewFunctionTool(wordCount)
    if err != nil {
        log.Fatalf("tool: %v", err)
    }

    // Create the agent.
    ag, err := llmagent.New(llmagent.Config{
        Name:        "word-counter",
        Model:       model,
        Instruction: "You are a helpful assistant. Use the word_count tool when asked about word counts.",
        Tools:       []tool.Tool{wc},
    })
    if err != nil {
        log.Fatalf("agent: %v", err)
    }

    // Wire up ADK services and runner.
    r, err := runner.New(runner.Config{
        AppName:        "mytool",
        Agent:          ag,
        SessionService: session.NewInMemorySessionService(),
    })
    if err != nil {
        log.Fatalf("runner: %v", err)
    }

    // Create a session and send a message.
    sess, _ := r.SessionService().CreateSession(ctx, "mytool", "user1", nil)
    msg := genai.NewUserContent(genai.Text("How many words are in 'the quick brown fox'?"))

    for resp, err := range r.RunAsync(ctx, "user1", sess.ID, msg) {
        if err != nil {
            log.Fatalf("run: %v", err)
        }
        for _, part := range resp.Content.Parts {
            if t, ok := part.(genai.Text); ok {
                fmt.Print(string(t))
            }
        }
    }
    fmt.Println()
}
```

## Step 3 — Run it

```sh
cd ../..          # back to repo root
go run ./sample/mytool
```

You should see the agent call `wordCount` and report `4`.

---

## What you learned

| Concept | Where |
|---|---|
| `tool.NewFunctionTool` | Wraps any typed Go func as an ADK tool |
| `jsonschema` tags | Describe the tool's input schema to the LLM |
| `llmagent.Config` | Declarative agent construction |
| `runner.Config` | Wires agent, session, memory, artifact services |
| `r.RunAsync` | Returns an iterator over `LLMResponse` events |

---

## Patterns demonstrated in this repo

Look at `sample/` for more complete examples:

| Sample | What it shows |
|---|---|
| `sample/groq/` | Groq provider usage |
| `sample/openrouter/` | OpenRouter with model selection |
| `sample/opencodezen/` | Multi-agent collaboration |

And in `agents/`:

| Agent | Pattern |
|---|---|
| `agents/llmauditor.go` | Custom `Run` func (not `LlmAgent`) |
| `agents/collaboration/` | Multi-agent via `agenttool` |

---

## Next steps

- [Tutorial: Add a new LLM provider](add-provider.md)
- [Explanation: Architecture overview](../explanation/architecture.md)
- [Reference: Full API reference](../reference/api.md)
