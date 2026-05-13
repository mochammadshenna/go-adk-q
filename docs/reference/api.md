# Reference: API

Go package-level reference for the key public APIs in this module.

Module path: `go-adk-q`

---

## model/catalog

```go
import "go-adk-q/model/catalog"
```

### ModelEntry

```go
type ModelEntry struct {
    ID      string   // exact model ID passed to the API
    Label   string   // display name; falls back to ID if empty
    Tags    []string // e.g. "fast", "long-ctx"
    Default bool     // pre-selected in the /model picker
}

func (e ModelEntry) DisplayName() string
```

### ProviderCatalog

```go
type ProviderCatalog struct {
    Provider string        // lowercase identifier, e.g. "groq"
    Label    string        // display name, e.g. "Groq"
    Models   []ModelEntry
}
```

### Functions

```go
func Register(c ProviderCatalog)
func All() []ProviderCatalog
func ForProvider(name string) (ProviderCatalog, bool)
```

---

## model/failover

```go
import "go-adk-q/model/failover"
```

```go
// New creates a failover chain. primary must not be nil.
// Nil fallbacks are silently dropped.
func New(primary model.LLM, fallbacks ...model.LLM) *Model

func (m *Model) Name() string
func (m *Model) GenerateContent(
    ctx context.Context,
    req *model.LLMRequest,
    stream bool,
) iter.Seq2[*model.LLMResponse, error]
```

---

## model/oaibridge

```go
import "go-adk-q/model/oaibridge"
```

```go
type Config struct {
    APIKey    string
    BaseURL   string
    ModelName string
}

func New(ctx context.Context, cfg Config) (model.LLM, error)
```

---

## model/echo

```go
import "go-adk-q/model/echo"
```

```go
// New returns a model.LLM that echoes the user's last message.
// No API key required. Useful for testing.
func New() model.LLM
```

---

## tool/skilltoolset

```go
import localskilltoolset "go-adk-q/tool/skilltoolset"
```

```go
type Config struct {
    SkillsDir string // path to skills/ directory
}

func New(cfg Config) (tool.Toolset, error)
```

Discovers all `SKILL.md` files under `SkillsDir` and exposes each as an
ADK `tool.Tool` that returns the skill's Markdown content.

---

## Skills

<a name="skills"></a>

Skills are Markdown files in `skills/`. They are loaded by `SkillToolset`
and returned verbatim to the calling agent. Each file follows the structure:

```markdown
# skill-name

## Purpose
## Trigger conditions
## Inputs
## Process
## Output format
## Checklist
```

The agent uses the Checklist section to determine whether the skill completed.

---

## agents

```go
import "go-adk-q/agents"
```

### LlmAuditor

```go
// LlmAuditor is a custom agent that audits LLM outputs for quality,
// accuracy, and instruction-following. Implements agent.Agent directly
// (not LlmAgent) so it can inspect raw request/response pairs.
func NewLlmAuditor(model model.LLM) (agent.Agent, error)
```

### Collaboration agents

Located in `agents/collaboration/`. Multi-agent workflows using ADK's
`agenttool` pattern for agent-to-agent delegation.
