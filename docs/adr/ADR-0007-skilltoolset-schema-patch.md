# ADR-0007: Patch SkillToolset Schemas for OpenAI-Compatible Providers

**Status**: Accepted
**Date**: 2025-06-01
**Deciders**: go-adk-q maintainers

---

## Context

After adopting the SkillToolset (ADR-0006), a schema validation failure was
discovered when using OpenAI-compatible providers (Groq, NVIDIA NIM,
OpenRouter, HuggingFace, OpenCode):

```
POST /v1/chat/completions → 400 Bad Request
"invalid tool schema: object schema must have 'properties'"
```

The root cause: the upstream `google.golang.org/adk/tool/skilltoolset` package
infers JSON Schemas for tool arguments using Go reflection. For `list_skills`,
which takes no arguments (empty struct), the inferred schema is:

```json
{"type": "object"}
```

The OpenAI API specification requires that object schemas include a
`"properties"` field, even when empty:

```json
{"type": "object", "properties": {}}
```

Without this field, every OpenAI-compatible provider rejects the tool
registration with HTTP 400, making `list_skills` (and by extension all skills)
unavailable to non-Gemini providers.

A secondary issue was identified with `load_skill`: the auto-generated schema
for the `name` argument lacked a clear description, causing small models
(7B-13B parameter range) to misread the parameter and call `load_skill` with
incorrect JSON.

---

## Decision

Create a local wrapper package `go-adk-q/tool/skilltoolset` that:

1. Replaces `list_skills` with a version that has `"properties": {}` explicitly
   set in the input schema (using `jsonschema.Schema{Properties: map...}`).
2. Replaces `load_skill` with a version that has an explicit, unambiguous input
   schema describing the required `name` field.
3. Delegates `ProcessRequest` to the upstream toolset unchanged, preserving
   the skill system-instruction injection behaviour.

All callers in the codebase must use the local package, never the upstream
directly.

```go
// CORRECT
import localskilltoolset "go-adk-q/tool/skilltoolset"
skillset, err := localskilltoolset.New(ctx, localskilltoolset.Config{...})

// WRONG — upstream has broken schemas for OAI-compat providers
import "google.golang.org/adk/tool/skilltoolset"
```

---

## Alternatives Considered

### Upstream fix (contribute to ADK)

Considered, but the fix needs to land in this codebase immediately. A
contribution upstream cannot be relied upon for the current release timeline.
The local wrapper is a bridge solution that can be removed once the fix is
merged upstream and the ADK version is bumped.

### Manual schema override via reflection

Considered, but the upstream `SkillToolset` type uses unexported fields and
an internal `tools` slice. There is no stable public API to replace individual
tools without re-implementing the constructor.

### Disable SkillToolset for OpenAI-compat providers

Rejected because skills are a core feature of the root agent. Disabling them
for the most commonly-used providers would make the skills system effectively
unusable in multi-provider setups.

---

## Consequences

### Positive

- `list_skills` and `load_skill` work correctly with all tested providers:
  Gemini, GitHub Models, Groq, NVIDIA NIM, OpenRouter, HuggingFace, OpenCode.
- No changes required in calling code beyond using the local import path.
- `ProcessRequest` delegation means skill system-instruction injection still works.
- The patch is isolated to one 150-line file — easy to remove once ADK fixes upstream.

### Negative / Trade-offs

- Duplicates tool name constants and XML-serialisation logic from the upstream
  ADK package (which uses `internal` packages, blocking direct import).
- The local package must be kept in sync if the upstream changes `list_skills`
  or `load_skill` behaviour.
- Adds a maintenance obligation until the upstream fix is available.

---

## Implementation

`tool/skilltoolset/skilltoolset.go` wraps the upstream `SkillToolset`:

```go
// Patch 1: list_skills with explicit empty properties schema
emptyObjectSchema := &jsonschema.Schema{
    Type:       "object",
    Properties: map[string]*jsonschema.Schema{},  // non-nil → serialised as "properties":{}
}

// Patch 2: load_skill with unambiguous name field schema
inputSchema := &jsonschema.Schema{
    Type: "object",
    Properties: map[string]*jsonschema.Schema{
        "name": {
            Type:        "string",
            Description: `The exact skill name as returned by list_skills (e.g. "go-expert").`,
        },
    },
    Required: []string{"name"},
}
```

The `SkillToolset.Tools()` method returns the patched versions in place of
the upstream originals. `ProcessRequest` delegates unchanged.
