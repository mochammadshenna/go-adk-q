package tools

// artifacts.go demonstrates ARTIFACT storage from inside a FunctionTool.
//
// Artifacts vs State:
//
//	session.State  — key/value store for structured data (strings, numbers, maps).
//	                 Values live in memory; scoped to one session.
//	                 Write with ctx.State().Set / read with ctx.State().Get.
//
//	Artifacts      — binary/text file store for named, versioned content.
//	                 Backed by artifact.Service (in-memory, GCS, or custom).
//	                 Scoped to app + user + session + filename.
//	                 Write with ctx.Artifacts().Save / read with ctx.Artifacts().Load.
//
// The artifact service must be wired in at startup via launcher.Config.ArtifactService.
// This demo uses artifact.InMemoryService() (see main.go).
//
// Artifacts support versioning: each Save returns a revision number;
// Load retrieves the latest version; LoadVersion retrieves a specific one.

import (
	"context"
	"fmt"

	"google.golang.org/genai"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

// ── save_artifact ─────────────────────────────────────────────────────────────

type saveArtifactArgs struct {
	// Filename is required — the logical name for the artifact (e.g. "report.txt").
	Filename string `json:"filename" jsonschema:"Logical file name for the artifact (e.g. 'report.txt', 'analysis.md'). Used later to load it."`
	// Content is required — the text to store.
	Content string `json:"content" jsonschema:"Text content to store as the artifact."`
	// MimeType is optional — defaults to plain text.
	MimeType string `json:"mime_type,omitempty" jsonschema:"MIME type of the content (default: text/plain). Use text/markdown for markdown, application/json for JSON, etc."`
}

type saveArtifactResult struct {
	Filename string `json:"filename"`
	Version  int64  `json:"version"`
	Message  string `json:"message"`
}

// saveArtifact stores content as a named, versioned artifact in the artifact service.
// Each call creates a new version; the previous versions remain accessible via LoadVersion.
func saveArtifact(ctx tool.Context, args saveArtifactArgs) (saveArtifactResult, error) {
	mimeType := args.MimeType
	if mimeType == "" {
		mimeType = "text/plain"
	}

	// Build the genai.Part that wraps the artifact content.
	// For text: use NewPartFromText (sets Part.Text).
	// For binary: use genai.NewPartFromBytes(data, mimeType) (sets Part.InlineData).
	var part *genai.Part
	if mimeType == "text/plain" || mimeType == "text/markdown" {
		part = genai.NewPartFromText(args.Content)
	} else {
		part = genai.NewPartFromBytes([]byte(args.Content), mimeType)
	}

	// ctx.Artifacts() returns agent.Artifacts — a thin session-scoped wrapper
	// around the artifact.Service that automatically fills in app name, user ID,
	// and session ID from the current invocation context.
	resp, err := ctx.Artifacts().Save(context.Background(), args.Filename, part)
	if err != nil {
		return saveArtifactResult{}, fmt.Errorf("artifacts.Save(%q): %w", args.Filename, err)
	}

	return saveArtifactResult{
		Filename: args.Filename,
		Version:  resp.Version,
		Message:  fmt.Sprintf("Saved %q as artifact version %d.", args.Filename, resp.Version),
	}, nil
}

// NewSaveArtifactTool creates the save_artifact FunctionTool.
func NewSaveArtifactTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "save_artifact",
		Description: "Stores text content as a named, versioned artifact. Each save creates a new version. Returns the filename and version number. Load it later with load_artifact.",
	}, saveArtifact)
	if err != nil {
		panic(fmt.Sprintf("NewSaveArtifactTool: %v", err))
	}
	return t
}

// ── load_artifact ─────────────────────────────────────────────────────────────

type loadArtifactArgs struct {
	// Filename is required.
	Filename string `json:"filename" jsonschema:"The artifact filename to load (e.g. 'report.txt')."`
	// Version is optional — when 0 or omitted, loads the latest version.
	Version int `json:"version,omitempty" jsonschema:"Specific version to load (0 = latest)."`
}

type loadArtifactResult struct {
	Filename string `json:"filename"`
	Version  int64  `json:"version"`
	Content  string `json:"content"`
	MimeType string `json:"mime_type"`
	Found    bool   `json:"found"`
}

// loadArtifact retrieves a previously saved artifact by filename.
// If Version is 0, the latest version is returned.
func loadArtifact(ctx tool.Context, args loadArtifactArgs) (loadArtifactResult, error) {
	artifacts := ctx.Artifacts()

	var (
		part    *genai.Part
		version int64
	)

	if args.Version > 0 {
		// LoadVersion fetches a specific historical revision.
		// artifact.LoadResponse contains only Part; the version is echoed from the input.
		resp, err := artifacts.LoadVersion(context.Background(), args.Filename, args.Version)
		if err != nil {
			return loadArtifactResult{Found: false}, fmt.Errorf("artifacts.LoadVersion(%q, %d): %w", args.Filename, args.Version, err)
		}
		part = resp.Part
		version = int64(args.Version)
	} else {
		// Load fetches the latest revision.
		// artifact.LoadResponse has no Version field — version 0 means "latest".
		resp, err := artifacts.Load(context.Background(), args.Filename)
		if err != nil {
			return loadArtifactResult{Found: false}, fmt.Errorf("artifacts.Load(%q): %w", args.Filename, err)
		}
		part = resp.Part
		version = 0 // 0 = latest; use load_artifact with explicit version to get a specific one
	}

	if part == nil {
		return loadArtifactResult{Found: false}, nil
	}

	// Extract the text content from the Part.
	// For text/plain artifacts saved with NewPartFromText, Part.Text is set.
	// For binary artifacts, Part.InlineData.Data holds the raw bytes.
	content := part.Text
	mimeType := "text/plain"
	if part.InlineData != nil {
		content = string(part.InlineData.Data)
		mimeType = part.InlineData.MIMEType
	}

	return loadArtifactResult{
		Filename: args.Filename,
		Version:  version,
		Content:  content,
		MimeType: mimeType,
		Found:    true,
	}, nil
}

// NewLoadArtifactTool creates the load_artifact FunctionTool.
func NewLoadArtifactTool() tool.Tool {
	t, err := functiontool.New(functiontool.Config{
		Name:        "load_artifact",
		Description: "Loads a previously saved artifact by filename. Returns its text content, mime type, and version. Set version=0 (or omit) to load the latest version. Returns found=false if the artifact does not exist.",
	}, loadArtifact)
	if err != nil {
		panic(fmt.Sprintf("NewLoadArtifactTool: %v", err))
	}
	return t
}
