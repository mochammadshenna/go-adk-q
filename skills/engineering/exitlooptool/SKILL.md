---
name: exitlooptool
description: Detect and break out of infinite loops, retry loops, and stuck iteration patterns in agent workflows and Go code.
compatibility: Designed for go-adk-q — a Google ADK Go reference implementation with Bubbletea TUI.
---
# ExitLoopTool

Use when an agent loop, retry loop, or iterative process is stuck and needs a clean exit.

## What "stuck" looks like

- An agent tool call returns the same error repeatedly and the agent keeps retrying
- A `for` loop in Go has a termination condition that's never reached
- A streaming response never sends the done signal
- An ADK iterator runs but no events are produced
- A goroutine is blocked on a channel that will never receive

## Detection

### In the TUI (runtime)

Symptoms:
- Spinner showing "Thinking…" for more than 30 seconds
- `streamingText` not updating despite the model responding
- `ctrl+c` not quitting (goroutine leak blocking the exit signal)

Check the log:
```bash
tail -50 "$TMPDIR/go-adk-q-tui.log"
```

Look for: repeated identical log lines, missing `event_type: turn_complete`, error messages from the provider.

### In Go code (static)

Look for loops without a guaranteed exit:

```go
// Dangerous: termination depends on external state that may never change
for {
    resp, err := stream.Next()
    if err == io.EOF { break }
    // what if err is never io.EOF?
}

// Safer: add a context deadline
ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
defer cancel()
for {
    resp, err := stream.Next()
    if err == io.EOF || ctx.Err() != nil { break }
}
```

### In ADK agent loops

The ADK runner's `Run()` iterates over events. A stuck iterator means either:
1. The provider is not sending a `turn_complete` event
2. The context is not being cancelled
3. A tool call is blocking without returning

Check the event stream:
```go
for event := range runner.Run(ctx, ...) {
    log.Printf("event: %+v", event)  // add this temporarily
}
```

## Exit strategies

### Immediate exit (user-triggered)
`ctrl+c` in the TUI sends `tea.Quit`. If this doesn't work, the goroutine is not listening to the context:

```go
// Wrong: goroutine has no exit
go func() {
    for event := range ch { ... }
}()

// Right: goroutine respects context cancellation
go func() {
    for {
        select {
        case event, ok := <-ch:
            if !ok { return }
            ...
        case <-ctx.Done():
            return
        }
    }
}()
```

### Timeout exit
For any operation that could block indefinitely:

```go
ctx, cancel := context.WithTimeout(parentCtx, 30*time.Second)
defer cancel()
```

In `cmd/tui/main.go`, the agent runner context is the program context — it gets cancelled on `ctrl+c`. Make sure all goroutines respect it.

### Agent retry loop exit
If an agent is retrying a failing tool call, add a max-retry guard:

```go
const maxRetries = 3
for attempt := 0; attempt < maxRetries; attempt++ {
    if err := callTool(); err == nil { break }
    if attempt == maxRetries-1 {
        return fmt.Errorf("tool failed after %d attempts: %w", maxRetries, err)
    }
}
```

## For go-adk-q specifically

The stream goroutine in `chat.go:startAgentStream()` uses a buffered channel (64). If the channel fills up without the UI consuming events, the goroutine blocks. This shouldn't happen in practice because the UI consumes on every tick — but if it does, the symptom is a frozen UI with the goroutine stuck on `ch <- msg`.

The fix: always drain the channel on `ctrl+c`:
```go
case <-ctx.Done():
    // drain remaining events so goroutine can exit
    for len(ch) > 0 { <-ch }
    return
```
