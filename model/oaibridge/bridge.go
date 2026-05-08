// Package oaibridge provides a reusable ADK model.LLM adapter for any
// provider that exposes an OpenAI-compatible chat completions API.
//
// # Design rationale
//
// Groq, NVIDIA NIM, OpenRouter, and HuggingFace all expose the same
// wire protocol as OpenAI's /v1/chat/completions endpoint. Genkit's
// compat_oai plugin already wraps the official openai-go client for
// exactly this purpose. This package sits one level above compat_oai and
// handles the ADK ↔ Genkit type translation that is identical regardless
// of which provider you choose.
//
// Each provider package (groq, nvidia, openrouter, huggingface) is a
// thin wrapper that supplies its own constants and any provider-specific
// behaviour (e.g. OpenRouter's HTTP-Referer header), then delegates
// everything else here.
//
// # Type mapping
//
//	ADK (google.golang.org/genai)          Genkit (github.com/firebase/genkit/go/ai)
//	────────────────────────────────────── ──────────────────────────────────────────
//	genai.Content{Role:"user"}             ai.Message{Role: ai.RoleUser}
//	genai.Content{Role:"user"} + FuncResp  ai.Message{Role: ai.RoleTool}
//	genai.Content{Role:"model"}            ai.Message{Role: ai.RoleModel}
//	Config.SystemInstruction               ai.Message{Role: ai.RoleSystem}  (prepended)
//	genai.Part{Text}                       ai.NewTextPart(...)
//	genai.Part{FunctionCall}               ai.NewToolRequestPart(...)
//	genai.Part{FunctionResponse}           ai.NewToolResponsePart(...)
//	FunctionDeclaration{ParametersJsonSchema any} → map[string]any via JSON round-trip
//
// # Genkit middleware hooks
//
// The optional Config.Hooks field lets callers compose Genkit model middleware
// (e.g. model/middleware.Fallback) without requiring a genkit.Genkit registry.
// Hooks are applied innermost-last: the first hook in the slice wraps the
// outermost call, the last hook wraps the raw ai.Model.Generate invocation.
//
//	groqModel, _ := oaibridge.NewModel(ctx, oaibridge.Config{...})   // ai.Model, not used here
//	nvidiaModel, _ := oaibridge.NewModel(ctx, oaibridge.Config{...}) // ai.Model, not used here
//
//	// Obtain the underlying Genkit models to use as fallback targets.
//	// In practice, create the Genkit models first, then wrap in the bridge.
//	fbHooks, _ := (&middleware.Fallback{
//	    Models: []middleware.FallbackModel{{Model: nvidiaGenkitModel}},
//	}).New(ctx)
//
//	bridgeWithFallback, _ := oaibridge.NewModel(ctx, oaibridge.Config{
//	    Provider:  "groq",
//	    BaseURL:   "https://api.groq.com/openai/v1",
//	    APIKey:    os.Getenv("GROQ_API_KEY"),
//	    ModelName: "llama-3.3-70b-versatile",
//	    Hooks:     []*ai.Hooks{fbHooks}, // nvidia tried if groq fails
//	})
//
// # Usage
//
//	import "go-adk-q/model/oaibridge"
//
//	m, err := oaibridge.NewModel(ctx, oaibridge.Config{
//	    Provider:  "acme",
//	    BaseURL:   "https://api.acme.ai/v1",
//	    APIKey:    os.Getenv("ACME_API_KEY"),
//	    ModelName: "acme-large",
//	})
package oaibridge

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"

	"github.com/firebase/genkit/go/ai"
	"github.com/firebase/genkit/go/core"
	"github.com/firebase/genkit/go/plugins/compat_oai"
	openai "github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// wrapOAIError converts openai-go HTTP errors and plain network errors into
// [core.GenkitError] so that status-aware Genkit middleware (fallback, retry)
// can reason about them by status code.
//
// The mapping follows the same pattern as googlegenai's wrapAPIError:
//   - [*openai.Error] (HTTP API error) → status from HTTP code via [core.StatusFromHTTPCode]
//   - plain network / connection error  → [core.UNAVAILABLE]
//   - already a [*core.GenkitError]     → returned unchanged
//
// This is applied in [buildModelNext]'s innermost lambda so that the Genkit
// fallback middleware sees properly typed errors from the primary model.
func wrapOAIError(err error) error {
	if err == nil {
		return nil
	}
	var ge *core.GenkitError
	if errors.As(err, &ge) {
		return err // already a GenkitError — pass through unchanged
	}
	var apiErr *openai.Error
	if errors.As(err, &apiErr) {
		// HTTP API error: map by status code to the appropriate StatusName.
		return core.NewError(core.StatusFromHTTPCode(apiErr.StatusCode), "%s", err)
	}
	// Plain network / connection / timeout error — treat as UNAVAILABLE so
	// fallback middleware can trigger and try the next provider.
	return core.NewError(core.UNAVAILABLE, "%s", err)
}

// buildModelNext wraps an ai.Model.Generate call in a ModelNext function and
// optionally layers any number of WrapModel hooks around it. Hooks are applied
// outermost-first: hooks[0] is the outermost wrapper, hooks[len-1] is closest
// to the raw Generate call.
//
// This lets oaibridge callers inject Genkit model middleware (e.g.
// model/middleware.Fallback) without requiring a genkit.Genkit registry or
// calling genkit.Generate().
func buildModelNext(m ai.Model, hooks []*ai.Hooks) ai.ModelNext {
	// Start with the innermost call: the raw model, with HTTP errors wrapped
	// as GenkitError so status-aware middleware can filter by status code.
	next := ai.ModelNext(func(ctx context.Context, p *ai.ModelParams) (*ai.ModelResponse, error) {
		resp, err := m.Generate(ctx, p.Request, p.Callback)
		return resp, wrapOAIError(err)
	})
	// Layer hooks from innermost to outermost (iterate in reverse so the first
	// hook in the slice ends up as the outermost wrapper).
	for i := len(hooks) - 1; i >= 0; i-- {
		if hooks[i] == nil || hooks[i].WrapModel == nil {
			continue
		}
		wm := hooks[i].WrapModel
		inner := next
		next = func(ctx context.Context, p *ai.ModelParams) (*ai.ModelResponse, error) {
			return wm(ctx, p, inner)
		}
	}
	return next
}

// Config holds everything needed to create a model.LLM backed by an
// OpenAI-compatible provider.
type Config struct {
	// Provider is a short, lowercase identifier used as the Genkit action
	// namespace (e.g. "groq", "nvidia", "openrouter", "huggingface").
	// Must be unique per binary if you plan to use Genkit's dev server;
	// for plain ADK usage uniqueness is not required but is good practice.
	Provider string

	// BaseURL is the provider's OpenAI-compatible REST endpoint.
	// Examples:
	//   Groq:        "https://api.groq.com/openai/v1"
	//   NVIDIA NIM:  "https://integrate.api.nvidia.com/v1"
	//   OpenRouter:  "https://openrouter.ai/api/v1"
	//   HuggingFace: "https://api-inference.huggingface.co/v1"
	BaseURL string

	// APIKey is the bearer token for authentication.
	APIKey string

	// ModelName is the model identifier the provider accepts.
	// Provider-specific format, e.g.:
	//   Groq:        "llama-3.3-70b-versatile"
	//   NVIDIA NIM:  "nvidia/llama-3.1-nemotron-70b-instruct"
	//   OpenRouter:  "meta-llama/llama-3.3-70b-instruct"
	//   HuggingFace: "mistralai/Mistral-7B-Instruct-v0.3"
	ModelName string

	// Label is a human-readable name displayed in Genkit tooling.
	// Defaults to "<Provider> / <ModelName>" when empty.
	Label string

	// ExtraHeaders are provider-specific HTTP headers added to every
	// request. The map is converted to option.WithHeader calls internally
	// so callers do not need to import github.com/openai/openai-go/option.
	//
	// Example (OpenRouter requires these for usage attribution):
	//   ExtraHeaders: map[string]string{
	//       "HTTP-Referer": "https://example.com",
	//       "X-Title":      "My ADK App",
	//   }
	ExtraHeaders map[string]string

	// Hooks is an optional ordered list of Genkit model hooks applied around
	// every ai.Model.Generate call. Use this to compose model/middleware.Fallback
	// or any other ai.Hooks without a Genkit registry.
	//
	// Hooks are applied outermost-first: Hooks[0] wraps the outermost call,
	// the last hook wraps the raw ai.Model.Generate invocation.
	//
	// When nil or empty, the raw ai.Model.Generate is called directly.
	Hooks []*ai.Hooks
}

// llmBridge implements model.LLM. It is safe for concurrent use once created.
type llmBridge struct {
	name    string       // "provider/modelName" — used by Name() and failover display
	aiModel ai.Model     // Genkit model action; created once in NewModel
	next    ai.ModelNext // pre-built hook chain (may wrap aiModel with middleware)
}

// NewModel creates a model.LLM backed by the provider described in cfg.
//
// It initialises the compat_oai plugin (openai-go client) and wraps it in
// an ADK model.LLM adapter. The returned model is safe for concurrent use
// and can be shared across multiple LlmAgents.
func NewModel(ctx context.Context, cfg Config) (model.LLM, error) {
	switch {
	case cfg.Provider == "":
		return nil, fmt.Errorf("oaibridge.NewModel: Provider is required")
	case cfg.BaseURL == "":
		return nil, fmt.Errorf("oaibridge.NewModel: BaseURL is required")
	case cfg.APIKey == "":
		return nil, fmt.Errorf("oaibridge.NewModel: APIKey is required")
	case cfg.ModelName == "":
		return nil, fmt.Errorf("oaibridge.NewModel: ModelName is required")
	}

	// Convert ExtraHeaders to openai-go request options. Keeping this
	// conversion internal means provider packages only depend on oaibridge,
	// not on github.com/openai/openai-go/option directly.
	extraOpts := make([]option.RequestOption, 0, len(cfg.ExtraHeaders))
	for k, v := range cfg.ExtraHeaders {
		extraOpts = append(extraOpts, option.WithHeader(k, v))
	}

	// OpenAICompatible.Init builds the openai-go client from APIKey, BaseURL,
	// and Opts. We pass ExtraHeaders via Opts so they are included in the
	// client construction. Init must be called exactly once per plugin instance.
	plugin := &compat_oai.OpenAICompatible{
		Provider: cfg.Provider,
		APIKey:   cfg.APIKey,
		BaseURL:  cfg.BaseURL,
		Opts:     extraOpts,
	}
	plugin.Init(ctx)

	label := cfg.Label
	if label == "" {
		label = fmt.Sprintf("%s / %s", cfg.Provider, cfg.ModelName)
	}

	// DefineModel calls ai.NewModel (not ai.DefineModel) — it does NOT
	// register to any global Genkit registry. Safe to call without genkit.Init.
	aiModel := plugin.DefineModel(cfg.Provider, cfg.ModelName, ai.ModelOptions{
		Label: label,
		Stage: ai.ModelStageStable,
		// BasicText: Multiturn, Tools, SystemRole all true; Media false.
		// All four target providers support these capabilities.
		Supports: &compat_oai.BasicText,
	})

	return &llmBridge{
		name:    cfg.Provider + "/" + cfg.ModelName,
		aiModel: aiModel,
		next:    buildModelNext(aiModel, cfg.Hooks),
	}, nil
}

// Name satisfies model.LLM.
func (b *llmBridge) Name() string { return b.name }

// GenerateContent satisfies model.LLM.
//
// The stream parameter is accepted but currently yields a single complete
// response regardless of its value. Incremental token streaming would
// require forwarding the ai.ModelStreamCallback to the ADK event iterator,
// which is straightforward to add when needed.
func (b *llmBridge) GenerateContent(
	ctx context.Context,
	req *model.LLMRequest,
	_ bool, // stream — future incremental streaming
) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		aiReq, err := toAIRequest(req)
		if err != nil {
			yield(nil, fmt.Errorf("oaibridge: build request: %w", err))
			return
		}

		// Call the pre-built hook chain. When no Hooks were configured this is
		// equivalent to b.aiModel.Generate(ctx, aiReq, nil). When hooks are
		// present (e.g. model/middleware.Fallback) they wrap this call and may
		// retry with a different Genkit model on failure.
		resp, err := b.next(ctx, &ai.ModelParams{Request: aiReq})
		if err != nil {
			yield(nil, fmt.Errorf("oaibridge: generate [%s]: %w", b.name, err))
			return
		}

		yield(fromAIResponse(resp), nil)
	}
}

// ── Request conversion: ADK (genai) → Genkit (ai) ────────────────────────────

// toAIRequest converts an ADK LLMRequest to a Genkit ModelRequest.
//
// Field-by-field mapping:
//
//	Config.SystemInstruction → RoleSystem message (prepended to Messages)
//	Contents                 → Messages (role-mapped, parts converted)
//	Config.Tools             → []*ai.ToolDefinition (flattened from genai.Tool groups)
//	Config.{Temperature,…}  → Config map[string]any (OpenAI field names)
func toAIRequest(req *model.LLMRequest) (*ai.ModelRequest, error) {
	aiReq := &ai.ModelRequest{}

	// System instruction lives in Config.SystemInstruction in ADK.
	// Prepend as a RoleSystem message so compat_oai emits role:"system" to
	// OpenAI. Without this Groq / NVIDIA would not apply the system prompt.
	if req.Config != nil && req.Config.SystemInstruction != nil {
		sysMsg, err := contentToMessage(req.Config.SystemInstruction, ai.RoleSystem)
		if err != nil {
			return nil, fmt.Errorf("system instruction: %w", err)
		}
		aiReq.Messages = append(aiReq.Messages, sysMsg)
	}

	// Conversation history.
	for i, c := range req.Contents {
		msg, err := contentToMessage(c, "")
		if err != nil {
			return nil, fmt.Errorf("contents[%d]: %w", i, err)
		}
		aiReq.Messages = append(aiReq.Messages, msg)
	}

	if req.Config != nil {
		// Tool definitions — each genai.Tool groups N FunctionDeclarations;
		// flatten to the flat []*ai.ToolDefinition slice Genkit expects.
		for _, gt := range req.Config.Tools {
			for _, decl := range gt.FunctionDeclarations {
				td, err := declToToolDef(decl)
				if err != nil {
					return nil, fmt.Errorf("tool %q: %w", decl.Name, err)
				}
				aiReq.Tools = append(aiReq.Tools, td)
			}
		}

		// Generation parameters. compat_oai's WithConfig accepts map[string]any
		// and unmarshals it into openai.ChatCompletionNewParams. We use the
		// OpenAI field names (snake_case) here.
		if cfg := genaiConfigToMap(req.Config); cfg != nil {
			aiReq.Config = cfg
		}
	}

	return aiReq, nil
}

// contentToMessage converts a *genai.Content to an *ai.Message.
//
// When role is non-empty it overrides the content's own Role field —
// used when converting Config.SystemInstruction which carries no role.
//
// Role mapping:
//   - "user" + any FunctionResponse part → ai.RoleTool  (OpenAI "tool" role)
//   - "user"                              → ai.RoleUser
//   - "model"                             → ai.RoleModel
func contentToMessage(c *genai.Content, role ai.Role) (*ai.Message, error) {
	r := role
	if r == "" {
		switch c.Role {
		case "user":
			// ADK/Gemini encode tool results as "user" messages with
			// FunctionResponse parts. OpenAI (and every OAI-compat provider)
			// requires those to be "tool" role messages instead.
			for _, p := range c.Parts {
				if p.FunctionResponse != nil {
					r = ai.RoleTool
					break
				}
			}
			if r == "" {
				r = ai.RoleUser
			}
		case "model":
			r = ai.RoleModel
		default:
			r = ai.Role(c.Role)
		}
	}

	msg := &ai.Message{Role: r}
	for _, p := range c.Parts {
		ap, err := genaiPartToAI(p)
		if err != nil {
			return nil, err
		}
		if ap != nil {
			msg.Content = append(msg.Content, ap)
		}
	}
	return msg, nil
}

// genaiPartToAI converts a *genai.Part to an *ai.Part.
//
// Supported conversions (matching what OAI-compatible providers handle):
//   - Text           → ai.NewTextPart
//   - FunctionCall   → ai.NewToolRequestPart  (model → client: call this tool)
//   - FunctionResponse → ai.NewToolResponsePart (client → model: here is the result)
//
// Unrecognised part types (inline data, video, etc.) are silently dropped.
// Groq, NVIDIA NIM, OpenRouter, and HuggingFace serverless inference are all
// text-only; passing binary data would cause an API error anyway.
func genaiPartToAI(p *genai.Part) (*ai.Part, error) {
	switch {
	case p.Text != "":
		return ai.NewTextPart(p.Text), nil

	case p.FunctionCall != nil:
		fc := p.FunctionCall
		return ai.NewToolRequestPart(&ai.ToolRequest{
			Name:  fc.Name,
			Input: fc.Args, // map[string]any — already the right type
			Ref:   fc.ID,   // tool_call_id used by the OpenAI protocol
		}), nil

	case p.FunctionResponse != nil:
		fr := p.FunctionResponse
		return ai.NewToolResponsePart(&ai.ToolResponse{
			Name:   fr.Name,
			Output: fr.Response, // map[string]any
			Ref:    fr.ID,       // must match the originating FunctionCall.ID
		}), nil

	default:
		return nil, nil
	}
}

// declToToolDef converts a genai.FunctionDeclaration to an ai.ToolDefinition.
//
// FunctionDeclaration.ParametersJsonSchema is typed as `any` in genai:
// at runtime it is either a *jsonschema.Schema (from functiontool.New) or
// already a map[string]any. We JSON-round-trip it to guarantee map[string]any
// because that is what ai.ToolDefinition.InputSchema requires.
func declToToolDef(decl *genai.FunctionDeclaration) (*ai.ToolDefinition, error) {
	schema, err := anyToMap(decl.ParametersJsonSchema)
	if err != nil {
		return nil, fmt.Errorf("parameters schema: %w", err)
	}
	return &ai.ToolDefinition{
		Name:        decl.Name,
		Description: decl.Description,
		InputSchema: schema,
	}, nil
}

// genaiConfigToMap converts a genai.GenerateContentConfig to a map[string]any
// using OpenAI field names. compat_oai's WithConfig accepts this format and
// unmarshals it into openai.ChatCompletionNewParams.
//
// Only non-zero / non-nil fields are emitted so that provider-side defaults
// apply for anything the caller did not explicitly set.
func genaiConfigToMap(c *genai.GenerateContentConfig) map[string]any {
	if c == nil {
		return nil
	}
	m := make(map[string]any, 4)
	if c.Temperature != nil {
		m["temperature"] = float64(*c.Temperature)
	}
	if c.MaxOutputTokens != 0 {
		m["max_tokens"] = int(c.MaxOutputTokens)
	}
	if c.TopP != nil {
		m["top_p"] = float64(*c.TopP)
	}
	if len(c.StopSequences) > 0 {
		m["stop"] = c.StopSequences
	}
	if len(m) == 0 {
		return nil
	}
	return m
}

// ── Response conversion: Genkit (ai) → ADK (genai) ───────────────────────────

// fromAIResponse converts a Genkit ModelResponse to an ADK LLMResponse.
//
// TurnComplete is always true: the Generate call above is non-streaming and
// returns only when the model has finished producing its response.
func fromAIResponse(resp *ai.ModelResponse) *model.LLMResponse {
	if resp == nil || resp.Message == nil {
		return &model.LLMResponse{TurnComplete: true}
	}

	content := &genai.Content{Role: "model"}
	for _, p := range resp.Message.Content {
		gp := aiPartToGenai(p)
		if gp != nil {
			content.Parts = append(content.Parts, gp)
		}
	}

	return &model.LLMResponse{
		Content:      content,
		TurnComplete: true,
	}
}

// aiPartToGenai converts an *ai.Part back to a *genai.Part.
//
// Only PartText and PartToolRequest appear in model responses (the model
// either replies with text or asks to call a tool). Other part kinds are
// skipped.
func aiPartToGenai(p *ai.Part) *genai.Part {
	switch p.Kind {
	case ai.PartText:
		return &genai.Part{Text: p.Text}

	case ai.PartToolRequest:
		if p.ToolRequest == nil {
			return nil
		}
		args, _ := anyToMap(p.ToolRequest.Input)
		return &genai.Part{
			FunctionCall: &genai.FunctionCall{
				ID:   p.ToolRequest.Ref,
				Name: p.ToolRequest.Name,
				Args: args,
			},
		}

	default:
		return nil
	}
}

// ── Helpers ───────────────────────────────────────────────────────────────────

// anyToMap converts any value to map[string]any via a JSON round-trip.
//
// Fast path: if v is already map[string]any, return it directly with no
// allocation. Slow path: marshal to JSON then unmarshal, handling typed
// structs like *jsonschema.Schema from functiontool.
func anyToMap(v any) (map[string]any, error) {
	if v == nil {
		return nil, nil
	}
	if m, ok := v.(map[string]any); ok {
		return m, nil
	}
	b, err := json.Marshal(v)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("unmarshal: %w", err)
	}
	return m, nil
}
