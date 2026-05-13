# Tutorial: Get started with go-adk-q

**Goal:** Run the interactive TUI chat in under 5 minutes.

**Prerequisites:**
- Go 1.22+ installed (`go version`)
- At least one LLM provider API key

---

## Step 1 — Clone the repository

```sh
git clone https://github.com/your-org/go-adk-q
cd go-adk-q
```

## Step 2 — Set an API key

The TUI tries providers in priority order; the first one with a valid key wins.
Set whichever you have:

```sh
# Google Gemini (recommended for first run)
export GEMINI_API_KEY=your_key_here

# OR GitHub Models (free with a GitHub account)
export GITHUB_TOKEN=your_token_here

# OR Groq (free tier available)
export GROQ_API_KEY=your_key_here
```

## Step 3 — Run the TUI

```sh
go run ./cmd/tui chat
```

The TUI starts in the default Catppuccin dark theme.

## Step 4 — Send your first message

Type a message and press **Enter** to send. The agent replies using your
configured provider, with full Markdown rendering.

## Step 5 — Try a slash command

Type `/` to see the autocomplete menu:

| Command | Action |
|---|---|
| `/theme` | Cycle through 8 colour themes |
| `/model` | Open the model/provider picker |
| `/settings` | Open settings (theme, character limit) |
| `/skills` | List available agent skills |
| `/help` | Toggle the help overlay |
| `/clear` | Clear conversation history |

## Step 6 — Try one-shot mode

You can also send a single message and get the response printed to stdout:

```sh
go run ./cmd/tui run "Explain the ADK SequentialAgent pattern in one paragraph"
```

---

## What just happened?

When you type a message:

1. The TUI passes it to an ADK `LlmAgent` via an ADK `Runner`.
2. The `Runner` uses a `failover.Model` that tries providers left-to-right.
3. The first provider that responds successfully is used; failures are logged.
4. The response is streamed back and rendered as Markdown in the viewport.

Session history is persisted across restarts via the ADK `InMemorySessionService`
(or the file-backed variant if configured).

---

## Next steps

- [Tutorial: Build your first custom agent](first-agent.md)
- [Tutorial: Add a new LLM provider](add-provider.md)
- [Reference: All config environment variables](../reference/config.md)
