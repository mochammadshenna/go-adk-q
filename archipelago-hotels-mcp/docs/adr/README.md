# Architecture Decision Records

> ADRs document significant technical decisions: the context that prompted them,
> the options considered, and the chosen outcome. Once accepted, an ADR is
> immutable — superseded decisions get a new ADR rather than an edit.
>
> Format follows [adr.github.io](https://adr.github.io/) conventions.

---

## Index

| # | File | Status | Date | Summary |
|---|------|--------|------|---------|
| 0001 | [ADR-0001-go-mcp-sdk.md](ADR-0001-go-mcp-sdk.md) | Accepted | 2026-06-22 | Use Go + `modelcontextprotocol/go-sdk` v1.6.1 for a single-binary, type-safe MCP server |
| 0002 | [ADR-0002-multi-db-pool-lazy-connect.md](ADR-0002-multi-db-pool-lazy-connect.md) | Accepted | 2026-06-22 | `repository.Pool` connects to the central DB eagerly and brand DBs on first use |
| 0003 | [ADR-0003-rate-fallback-chain.md](ADR-0003-rate-fallback-chain.md) | Accepted | 2026-06-22 | Three-level price fallback: SimpleBooking live API → stored `room_rate` → `hotel_starting_price` |
| 0004 | [ADR-0004-gin-http-transport.md](ADR-0004-gin-http-transport.md) | Accepted | 2026-06-22 | Wrap MCP Streamable HTTP and REST convenience endpoints with Gin v1.11 |
| 0005 | [ADR-0005-mcp-apps-embedded-ui.md](ADR-0005-mcp-apps-embedded-ui.md) | Accepted | 2026-06-22 | Embed Vite-built TypeScript UI in the binary as an MCP App resource (`ui://hotel-dashboard`) |
| 0006 | [ADR-0006-resize-image-url-csp.md](ADR-0006-resize-image-url-csp.md) | Accepted | 2026-06-22 | Rewrite thumbnail URLs through the CDN proxy to satisfy ext-app iframe CSP with a single `img-src` entry |
| 0007 | [ADR-0007-raw-currency-code.md](ADR-0007-raw-currency-code.md) | Accepted | 2026-06-22 | Return the raw `hotel_currency` ISO code from the DB; let clients format it via `Intl.NumberFormat` |

---

## Adding a new ADR

1. Copy `ADR-template.md` (or use the template below) to `docs/adr/ADR-NNNN-short-title.md`.
2. Fill in **Status**, **Context**, **Decision**, and **Consequences**.
3. Add a row to the index table above.
4. Set status to `Proposed`; update to `Accepted` once reviewed.

### Template

```markdown
# ADR-NNNN: Title

**File(s):** `path/to/relevant/file.go`
**Decision date:** YYYY-MM-DD

---

## Decision

One paragraph describing what was decided.

## Context

What situation or constraint prompted this decision? What alternatives were considered?

## Consequences

What are the positive and negative results? What is now easier or harder?
```

### Status vocabulary

| Status | Meaning |
|--------|---------|
| `Proposed` | Under discussion, not yet binding |
| `Accepted` | In effect; code reflects this decision |
| `Deprecated` | No longer recommended but not yet removed |
| `Superseded by ADR-NNNN` | Replaced by a later decision |
