# Reference: CLI

The TUI binary exposes two Cobra subcommands.

---

## `tui chat`

Start the interactive Bubbletea chat UI.

```sh
go run ./cmd/tui chat [flags]
```

### Flags

| Flag | Default | Description |
|---|---|---|
| `--model` | (first available) | Override the starting model ID |
| `--theme` | `0` (Catppuccin) | Starting theme index (0–7) |
| `--log` | `` | Path to write structured JSON logs |

### Keyboard shortcuts (in chat mode)

| Key | Action |
|---|---|
| `Enter` | Send message |
| `Shift+Enter` | Insert newline |
| `↑` / `↓` | Scroll conversation history |
| `PgUp` / `PgDn` | Scroll by page |
| `Ctrl+C` or `Esc` | Quit |

### Slash commands

Type `/` to open the autocomplete menu:

| Command | Description |
|---|---|
| `/settings` | Open settings overlay (theme, character limit) |
| `/model` | Open model/provider picker |
| `/theme` | Cycle to next colour theme |
| `/help` | Toggle help overlay |
| `/clear` | Clear conversation history |
| `/skills` | List available agent skills |

---

## `tui run`

One-shot mode: send a single message and print the response to stdout.

```sh
go run ./cmd/tui run "your message here"
```

Exit code is 0 on success, non-zero on error.

Useful for scripting:

```sh
go run ./cmd/tui run "Summarise this file" < myfile.txt
```

---

## Build and install

```sh
# Build binary to ./tui
go build -o tui ./cmd/tui

# Install to $GOPATH/bin
go install ./cmd/tui
```
