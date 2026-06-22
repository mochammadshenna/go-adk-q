# ADR-0004: Image URL Rewriting via resizeImageURL (not HTTP Proxy)

**File(s):** `internal/repository/hotel.go`
**Status:** Accepted
**Decision date:** 2025-06

---

## Context

Hotel `thumbnail_desktop` URLs are stored in brand databases and hosted on brand CDN domains (e.g. `*.sentineltech.com`, `*.astonimc.com`). These origins are blocked by Claude Desktop's iframe Content Security Policy and cannot be loaded directly by the MCP App UI.

An initial implementation fetched images server-side and returned them as base64 data URIs. This approach was rejected due to three problems:

1. **SSRF risk**: the server made outbound HTTP requests to arbitrary URLs read from the database, with no allowlist, redirect following, and no private-IP rejection.
2. **Memory pressure**: base64 encoding adds ~33% overhead; 50 hotels at 200 KB each produces ~13 MB per tool response.
3. **stdio incompatibility**: a proxy HTTP endpoint does not exist in stdio mode (no HTTP server running when launched by Claude Desktop).

## Decision

Replace all server-side image fetching with a pure string transformation. `resizeImageURL()` rewrites brand CDN URLs into equivalent URLs on `images.archipelagohotels.com` — Archipelago's own image resizer service. That domain is declared in `resourceDomains` on both the MCP resource and all tool registrations, so the iframe CSP allows it.

No HTTP requests are made by the server at thumbnail-fetch time.

### URL Transformation Pattern

```
https://cdn.astonimc.com/media/thumb.jpg
  → https://images.archipelagohotels.com/astonimc/media/thumb.jpg

https://sentineltech.com/img/property.jpg
  → https://images.archipelagohotels.com/sentineltech-publicwebsite/img/property.jpg
```

### Implementation

```go
// hotel.go — resizeImageURL (ported from Sentec platform ResizeImage)
func resizeImageURL(img string, width, height int, location string) string {
    if img == "" { return "" }

    urlImage := os.Getenv("url_image_resizer")
    if urlImage == "" { urlImage = "https://images.archipelagohotels.com/" }

    // Extract hostname and derive CDN bucket name
    r := regexp.MustCompile(`^(?:https?://)?(?:www\.)?([^/]+)`)
    matches := r.FindStringSubmatch(img)
    bucketName := ""
    if len(matches) >= 2 {
        urls := strings.Split(matches[1], ".")
        if urls[0] == "sentineltech" {
            bucketName = "sentineltech-publicwebsite"  // special case
        } else if len(urls) >= 2 {
            bucketName = urls[1]  // e.g. "astonwebsite" from "storage.astonwebsite.com"
        }
    }
    baseURL := urlImage + bucketName + "/"

    // Strip "subdomain.domain.com/" prefix from the original URL
    cdn := strings.Split(img, ".")
    trim := strings.Replace(img, cdn[0]+"."+cdn[1]+"."+"com/", "", 1)

    // Assemble final URL — optional resize/crop query params
    switch {
    case width == 0 && height == 0:
        return baseURL + trim
    case width != 0 && height == 0:
        return baseURL + trim + "?s=" + fmt.Sprint(width) + "&location=" + location
    case width == 0 || height == 0:
        return baseURL + trim + "?location=" + location
    default:
        return baseURL + trim + "?d=" + fmt.Sprintf("%dx%d", width, height) + "&location=" + location
    }
}

// GetThumbnails — rewrite at query time, no resize params needed for dashboard cards
result[cid] = resizeImageURL(thumbURL, 0, 0, "center")
```

```go
// resourceDomains allowlists the CDN in the iframe CSP.
// Must appear on BOTH the Resource registration and all Tool registrations.
Meta: mcp.Meta{
    "ui": map[string]any{
        "resourceDomains": []string{"images.archipelagohotels.com"},
    },
},
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| Input URL pattern | `https://{sub}.{domain}.com/{path}` (brand CDN origins) | `hotel.go:GetThumbnails` |
| Output URL pattern | `https://images.archipelagohotels.com/{bucket}/{path}` | `hotel.go:resizeImageURL` |
| Sentineltech special case | `sentineltech.*` → bucket `sentineltech-publicwebsite` | `hotel.go:resizeImageURL` |
| Default base URL | `https://images.archipelagohotels.com/` — overridable via `url_image_resizer` env | `hotel.go:resizeImageURL` |
| HTTP calls at serve time | Zero — pure string transformation | `hotel.go:resizeImageURL` |
| CSP allowlist | `resourceDomains` on resource and tool `_meta.ui` | `resources/dashboard.go`, tool files |
| Thumbnail column guard | `HasColumn(prefix, "tb_hotels", "thumbnail_desktop")` before query | `hotel.go:GetThumbnails` |

## Consequences

- No outbound HTTP requests from the server: eliminates the SSRF vector entirely.
- Relies on `images.archipelagohotels.com` being reachable and correctly mirroring brand CDN content; if that service is down, thumbnails will fail to load in the UI (tool results and text responses are unaffected).
- The `sentineltech` bucket name is a hardcoded special case; new brand CDN patterns may require a corresponding rule.
- Base URL is overridable via the `url_image_resizer` environment variable for staging and local development.

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Base64-inline images | ~33% size overhead; 50 hotels at 200 KB each = ~13 MB tool response; exceeds practical MCP message limits |
| Server-side proxy endpoint (`/img?url=...`) | Adds HTTP latency per thumbnail; SSRF risk; unavailable in stdio mode (no HTTP server) |
| Serve images from embedded binary | Brand thumbnails are dynamic — thousands of hotel images cannot be embedded at compile time |
| Ignore CSP, rely on host override | Claude Desktop's iframe CSP is enforced by the host process and cannot be disabled by the MCP server |
| CORS headers on brand CDN | Brand CDN origins are operated by third parties (Sentec platform); we do not control their CORS configuration |

### Security Notes

The original implementation (`fetchAsDataURI`) made outbound HTTP requests to arbitrary URLs sourced from the brand database: no allowlist, redirect following enabled, no private-IP rejection. `resizeImageURL` replaces this with a deterministic string rewrite — the server never initiates an outbound connection at thumbnail-fetch time.
