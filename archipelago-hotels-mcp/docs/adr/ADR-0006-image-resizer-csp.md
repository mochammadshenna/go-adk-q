# ADR-0006: resizeImageURL — CSP-Safe Thumbnail URL Rewriting

**Canonical file:** [`ADR-0006-resize-image-url-csp.md`](ADR-0006-resize-image-url-csp.md)
**File(s):** `internal/repository/hotel.go`
**Decision date:** 2026-06-22

---

## Context

Hotel thumbnail URLs (`thumbnail_desktop`) are stored in per-brand MySQL databases and point to arbitrary CDN origins (e.g. `storage.astonwebsite.com`, `storage.favrhotelwebsite.com`). When the MCP Apps UI renders inside Claude Desktop's sandboxed iframe, its Content Security Policy (CSP) blocks image requests to any origin not explicitly declared in `resourceDomains`. Declaring every brand CDN origin individually is fragile and requires a server update each time a brand adds or changes a CDN.

## Decision

All thumbnail URLs are rewritten at query time by a pure string-transformation function (`resizeImageURL`) into URLs served by the single Archipelago image CDN: `images.archipelagohotels.com`. Only that one origin is declared in `resourceDomains`. No HTTP requests are made by the MCP server during the transformation.

### How the Rewrite Works

```
Input:  https://storage.astonwebsite.com/images/hotel/thumb.jpg
                   ^       ^^^^^^^^^
                   sub     domain = bucket name
                   
Bucket: "astonwebsite"
Output: https://images.archipelagohotels.com/astonwebsite/images/hotel/thumb.jpg
```

The function (`resizeImageURL`, ported from the Sentec platform `ResizeImage` helper):

1. Extracts the second DNS label of the origin hostname as the CDN bucket name.
2. Special-cases `sentineltech.*` → bucket `sentineltech-publicwebsite`.
3. Strips the `sub.domain.com/` prefix from the original URL to obtain the path.
4. Prepends `{url_image_resizer}{bucket}/` (env-configurable, default `https://images.archipelagohotels.com/`).
5. Optionally appends `?d=WxH` or `?s=W` resize/crop params when called with non-zero dimensions.

### CSP Allowlist

`resourceDomains: ["images.archipelagohotels.com"]` is set on both the MCP Resource registration and on every Tool's `_meta.ui` object, so the single rewritten domain is allowed regardless of which tool triggered the UI render.

### Environment Variable

| Variable | Default | Purpose |
|----------|---------|---------|
| `url_image_resizer` | `https://images.archipelagohotels.com/` | Base URL for the image CDN proxy; trailing slash required |

## Rejected Alternatives

| Option | Rejected because |
|--------|-----------------|
| **Base64-inline proxy** (`fetchAsDataURI`) | Makes outbound HTTP requests to arbitrary DB-sourced URLs — SSRF risk with no allowlist, redirect-following, no private-IP rejection. Also ~33 % size overhead: 200 KB × 50 hotels = 13 MB tool response, exceeds practical MCP message limits. |
| **Server-side `/img?url=` proxy endpoint** | HTTP latency per thumbnail; requires SSRF allowlist; unavailable in stdio transport (no HTTP server). |
| **Embed images in binary** | Brand thumbnails are dynamic content; thousands of hotel images cannot be compiled in. |
| **Declare all brand CDN origins in `resourceDomains`** | Requires a server release per new brand or CDN change; breaks for self-hosted brand sites with unknown origins. |
| **Disable / override CSP** | Claude Desktop enforces CSP at the host process level; MCP servers cannot opt out. |
| **CORS headers on brand CDNs** | Third-party operators; we do not control their CORS configuration. |

## Consequences

- **Security**: Zero SSRF exposure — no outbound HTTP at thumbnail-fetch time. The rewrite function is a pure deterministic transformation operating on the input string only.
- **Simplicity**: One CDN domain in `resourceDomains`; no per-brand maintenance.
- **Portability**: Works identically in stdio and HTTP transport modes because no HTTP client is involved.
- **Configurability**: `url_image_resizer` lets staging/dev environments point at a different CDN proxy without code changes.
- **Limitation**: The CDN proxy (`images.archipelagohotels.com`) must itself be configured to forward requests by bucket/path to the original brand CDN origins. That is an infrastructure concern outside this codebase.
