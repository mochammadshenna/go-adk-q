// Package skilltoolset wraps google.golang.org/adk/tool/skilltoolset with two
// schema patches required for OpenAI-compatible providers.
//
// Patch 1 — list_skills empty-properties:
//   OpenAI-compatible APIs reject {"type":"object"} with no "properties" key.
//   ListSkillsArgs is an empty Go struct, so its inferred schema has no
//   "properties".  We replace list_skills with a version whose InputSchema
//   carries Properties: map[string]*jsonschema.Schema{} so the serialised JSON
//   always emits "properties": {}.
//
// Patch 2 — load_skill schema clarity:
//   Some small models (Groq llama-3.1-8b) emit XML-style tool calls or use the
//   wrong argument key ("skill_name" instead of "name").  We replace load_skill
//   with a version that has a clearer description and an explicit InputSchema
//   that names the required "name" field unambiguously.
//
// The upstream ProcessRequest method (which injects skill system instructions)
// is preserved by delegating to the inner toolset.
package skilltoolset

import (
	"context"
	"fmt"
	"html"
	"strings"

	"github.com/google/jsonschema-go/jsonschema"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/model"
	adktool "google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
	upstreamts "google.golang.org/adk/tool/skilltoolset"
	"google.golang.org/adk/tool/skilltoolset/skill"
)

// Config mirrors adkskilltoolset.Config so callers don't need to import both.
type Config = upstreamts.Config

// SkillToolset wraps the upstream SkillToolset and replaces list_skills with
// a version whose schema satisfies OpenAI-compatible strict validation.
type SkillToolset struct {
	inner  *upstreamts.SkillToolset
	tools  []adktool.Tool
}

// New creates a SkillToolset with the list_skills schema patched.
func New(ctx context.Context, cfg Config) (*SkillToolset, error) {
	inner, err := upstreamts.New(ctx, cfg)
	if err != nil {
		return nil, err
	}

	// Build replacement list_skills with explicit empty-properties schema.
	patchedList, err := newPatchedListSkills(cfg.Source)
	if err != nil {
		return nil, fmt.Errorf("patched list_skills: %w", err)
	}

	// Build replacement load_skill with clearer schema for weak models.
	patchedLoad, err := newPatchedLoadSkill(cfg.Source)
	if err != nil {
		return nil, fmt.Errorf("patched load_skill: %w", err)
	}

	// Retrieve the original tools list; swap patched versions.
	origTools, err := inner.Tools(nil)
	if err != nil {
		return nil, fmt.Errorf("inner.Tools: %w", err)
	}
	tools := make([]adktool.Tool, 0, len(origTools))
	for _, t := range origTools {
		switch t.Name() {
		case "list_skills":
			tools = append(tools, patchedList)
		case "load_skill":
			tools = append(tools, patchedLoad)
		default:
			tools = append(tools, t)
		}
	}

	return &SkillToolset{inner: inner, tools: tools}, nil
}

// Name implements tool.Toolset.
func (ts *SkillToolset) Name() string { return ts.inner.Name() }

// Tools implements tool.Toolset and returns tools with list_skills patched.
func (ts *SkillToolset) Tools(_ agent.ReadonlyContext) ([]adktool.Tool, error) {
	return ts.tools, nil
}

// ProcessRequest delegates to the upstream toolset so that the skill system
// instruction is still injected into each LLM request.
func (ts *SkillToolset) ProcessRequest(ctx adktool.Context, req *model.LLMRequest) error {
	return ts.inner.ProcessRequest(ctx, req)
}

// newPatchedListSkills returns a list_skills FunctionTool whose input schema
// has an explicit "properties": {} so OpenAI-compatible APIs accept it.
func newPatchedListSkills(source skill.Source) (adktool.Tool, error) {
	emptyObjectSchema := &jsonschema.Schema{
		Type:       "object",
		Properties: map[string]*jsonschema.Schema{}, // non-nil → serialised as "properties":{}
	}
	type listSkillsArgs struct{}
	type listSkillsResult struct {
		SkillsXML string `json:"skills"`
	}
	return functiontool.New(
		functiontool.Config{
			Name:        "list_skills",
			Description: "Lists all available skills with their names and descriptions.",
			InputSchema: emptyObjectSchema,
		},
		func(ctx adktool.Context, _ listSkillsArgs) (*listSkillsResult, error) {
			frontmatters, err := source.ListFrontmatters(ctx)
			if err != nil {
				return nil, err
			}
			return &listSkillsResult{SkillsXML: skillsToXML(frontmatters)}, nil
		},
	)
}

// skillsToXML mirrors the upstream SkillsToXML helper (which lives in an
// internal package and cannot be imported from outside the ADK module).
func skillsToXML(frontmatters []*skill.Frontmatter) string {
	var sb strings.Builder
	sb.WriteString("<available_skills>\n")
	for _, fm := range frontmatters {
		sb.WriteString("<skill>\n<name>\n")
		sb.WriteString(html.EscapeString(fm.Name))
		sb.WriteString("\n</name>\n<description>\n")
		sb.WriteString(html.EscapeString(fm.Description))
		sb.WriteString("\n</description>\n</skill>\n")
	}
	sb.WriteString("</available_skills>")
	return sb.String()
}

// newPatchedLoadSkill returns a load_skill FunctionTool with an explicit
// InputSchema that names the required "name" field unambiguously, reducing
// errors from small models that misread the auto-generated schema.
func newPatchedLoadSkill(source skill.Source) (adktool.Tool, error) {
	nameSchema := &jsonschema.Schema{
		Type:        "string",
		Description: "The exact skill name as returned by list_skills (e.g. \"go-expert\").",
	}
	inputSchema := &jsonschema.Schema{
		Type:        "object",
		Description: "Arguments for load_skill.",
		Properties: map[string]*jsonschema.Schema{
			"name": nameSchema,
		},
		Required: []string{"name"},
	}
	type loadSkillArgs struct {
		Name string `json:"name"`
	}
	type frontmatterJSON struct {
		Name          string            `json:"name"`
		Description   string            `json:"description"`
		License       string            `json:"license,omitempty"`
		Compatibility string            `json:"compatibility,omitempty"`
		Metadata      map[string]string `json:"metadata,omitempty"`
		AllowedTools  []string          `json:"allowed-tools,omitempty"`
	}
	type loadSkillResult struct {
		SkillName    string           `json:"skill_name,omitempty"`
		Instructions string           `json:"instructions,omitempty"`
		Frontmatter  *frontmatterJSON `json:"frontmatter,omitempty"`
	}
	return functiontool.New(
		functiontool.Config{
			Name: "load_skill",
			Description: `Loads the SKILL.md instructions for a named skill.
Call with JSON: {"name": "<skill_name>"}
The skill_name must match exactly the "name" field returned by list_skills.`,
			InputSchema: inputSchema,
		},
		func(ctx adktool.Context, args loadSkillArgs) (*loadSkillResult, error) {
			if args.Name == "" {
				return nil, fmt.Errorf("skill name is required to load a skill")
			}
			fm, err := source.LoadFrontmatter(ctx, args.Name)
			if err != nil {
				return nil, fmt.Errorf("load frontmatter for skill %q: %w", args.Name, err)
			}
			instructions, err := source.LoadInstructions(ctx, args.Name)
			if err != nil {
				return nil, fmt.Errorf("load instructions for skill %q: %w", args.Name, err)
			}
			return &loadSkillResult{
				SkillName:    args.Name,
				Instructions: instructions,
				Frontmatter: &frontmatterJSON{
					Name:          fm.Name,
					Description:   fm.Description,
					License:       fm.License,
					Compatibility: fm.Compatibility,
					Metadata:      fm.Metadata,
					AllowedTools:  fm.AllowedTools,
				},
			}, nil
		},
	)
}
