package tools

// state.go demonstrates direct SESSION STATE read/write from inside a FunctionTool.
//
// ADK provides two mechanisms for state:
//
//  1. OutputKey (passive) — LlmAgent saves its final LLM reply to a named state key
//     automatically after the turn completes (used in main.go's pipeline agents).
//
//  2. ctx.State().Set / ctx.State().Get (active) — a tool directly reads or writes
//     arbitrary values to session state at any point during tool execution.
//     This is shown here.
//
// State survives for the lifetime of the session. All agents in the same session
// share the same state namespace. Use the "temp:" prefix for values that should
// NOT be persisted across sessions (ADK strips them on session save).

import (
	"fmt"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ── save_to_state ─────────────────────────────────────────────────────────────

type saveStateArgs struct {
	Key   string `json:"key"   jsonschema:"The state key to write. Use 'temp:' prefix for ephemeral values."`
	Value string `json:"value" jsonschema:"The string value to store at that key."`
}

type saveStateResult struct {
	Key     string `json:"key"`
	Written bool   `json:"written"`
	Message string `json:"message"`
}

// saveToState writes a key/value pair into the current session state.
// This value is then readable by any subsequent agent or tool in the same session.
func saveToState(ctx tool.Context, args saveStateArgs) (saveStateResult, error) {
	// ctx.State() returns session.State, which is the live mutable state for this session.
	// Set persists the value immediately — the next agent in the pipeline can read it.
	if err := ctx.State().Set(args.Key, args.Value); err != nil {
		return saveStateResult{}, fmt.Errorf("state.Set(%q): %w", args.Key, err)
	}
	return saveStateResult{
		Key:     args.Key,
		Written: true,
		Message: fmt.Sprintf("Stored %q = %q in session state.", args.Key, args.Value),
	}, nil
}

// NewSaveToStateTool creates the save_to_state FunctionTool.
func NewSaveToStateTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "save_to_state",
		Description: "Writes a key/value string pair to session state. The value persists for the entire session and is readable by any subsequent agent or tool. Use 'temp:' key prefix for values that should not be persisted across sessions.",
	}, saveToState)
	if err != nil {
		panic(fmt.Sprintf("NewSaveToStateTool: %v", err))
	}
	return t
}

// ── read_from_state ───────────────────────────────────────────────────────────

type readStateArgs struct {
	Key string `json:"key" jsonschema:"The state key to read."`
}

type readStateResult struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Found   bool   `json:"found"`
	Message string `json:"message"`
}

// readFromState retrieves a value from session state by key.
// If the key is absent, Found is false and Value is empty.
func readFromState(ctx tool.Context, args readStateArgs) (readStateResult, error) {
	// ctx.State().Get returns (any, error).
	// A missing key returns an error; a nil value is also possible.
	raw, err := ctx.State().Get(args.Key)
	if err != nil || raw == nil {
		return readStateResult{
			Key:     args.Key,
			Found:   false,
			Message: fmt.Sprintf("Key %q not found in session state.", args.Key),
		}, nil
	}
	// State values are stored as any; assert to string for text values.
	// For richer types, assert to the original concrete type.
	s, ok := raw.(string)
	if !ok {
		s = fmt.Sprintf("%v", raw) // graceful fallback for non-string values
	}
	return readStateResult{
		Key:     args.Key,
		Value:   s,
		Found:   true,
		Message: fmt.Sprintf("Read %q = %q from session state.", args.Key, s),
	}, nil
}

// NewReadFromStateTool creates the read_from_state FunctionTool.
func NewReadFromStateTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "read_from_state",
		Description: "Reads a value from session state by key. Returns the stored string value and whether the key was found. Use this to retrieve values that were previously saved with save_to_state or written via an agent's OutputKey.",
	}, readFromState)
	if err != nil {
		panic(fmt.Sprintf("NewReadFromStateTool: %v", err))
	}
	return t
}
