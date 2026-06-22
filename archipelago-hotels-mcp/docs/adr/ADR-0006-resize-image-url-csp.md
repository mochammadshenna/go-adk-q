# ADR-0006: Pure URL Rewriting via resizeImageURL() for CSP-Safe Thumbnails

**File(s):** `internal/repository/hotel.go`
**Decision date:** 2026-06-22

---

## Decision

Hotel thumbnail URLs stored in brand databases are rewritten at query time into URLs served by the Archipelago image CDN (`images.archipelagohotels.com`), which is declared in `resourceDomains`. The transformation is a deterministic string operation — no HTTP requests are made by the server. This solves the MCP Apps iframe Content Security Policy restriction without base64 inlining or a proxy endpoint.

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

    // Assemble — optional resize/crop query params
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

// GetThumbnails Phase 2 — called with width=0, height=0, no resize params
result[cid] = resizeImageURL(thumbURL, 0, 0, "center")
```

```go
// resourceDomains allowlists the CDN in the iframe CSP
// Set on BOTH the Resource registration and all Tool registrations
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
| HTTP calls | Zero — pure string transformation | `hotel.go:resizeImageURL` |
| CSP allowlist | `resourceDomains` on resource and tool `_meta.ui` | `resources/dashboard.go`, tool files |
| Thumbnail column guard | `HasColumn(prefix, "tb_hotels", "thumbnail_desktop")` before query | `hotel.go:GetThumbnails` |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Base64-inline images | ~33% size overhead; 200 KB thumbnail → 267 KB per image; 50 hotels = 13 MB tool response; exceeds practical MCP message limits |
| Server-side proxy endpoint (`/img?url=...`) | Adds HTTP latency per thumbnail; requires SSRF allowlist; complicates stdio deployment (no HTTP server in stdio mode) |
| Serve images from embedded binary | Brand thumbnails are dynamic content — thousands of hotel images cannot be embedded at compile time |
| Ignore CSP, rely on host override | Not portable; Claude Desktop CSP is enforced by the host process and cannot be disabled by the MCP server |
| CORS headers on brand CDN | Brand CDN origins are operated by third parties; we do not control their CORS config |

### Security Notes

The original Go implementation included a base64 proxy (`fetchAsDataURI`) that made outbound HTTP requests to arbitrary URLs from the brand DB. This was identified as an SSRF vector (no allowlist, followed redirects, no private-IP rejection). The `resizeImageURL` approach eliminates all outbound requests from the server at thumbnail-fetch time entirely.
