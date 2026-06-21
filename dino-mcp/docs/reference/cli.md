# Reference: CLI Reference

> **Complete reference for the `dino-mcp` CLI.**
> Entry point: [`cmd/dino-mcp/main.go`](https://github.com/msw/dino-mcp/blob/main/cmd/dino-mcp/main.go)

**Official references:**
- [MCP Spec — Transports](https://spec.modelcontextprotocol.io/specification/2025-03-26/basic/transports/)
- [MCP Go SDK — Transports](https://pkg.go.dev/github.com/modelcontextprotocol/go-sdk/mcp#Transport)

---

## Usage

```bash
dino-mcp <subcommand> [flags]
```

---

## Subcommands

| Subcommand | Alias | Description | Use Case | Source (main.go) |
|------------|-------|-------------|----------|-------------------|
| `stdio` | — | Run server over stdin/stdout | Claude Desktop, Copilot, Cursor | `runStdio()` (line 57) |
| `http` | `streamable-http` | Run as Streamable HTTP server | MCP Inspector, curl, browser, tunnel | `runHTTP()` (line 65) |
| `help` | `--help`, `-h` | Print usage and exit | Discovering commands | `printUsage()` (line 74) |

### stdio Subcommand

Runs the MCP server over stdin/stdout using JSON-RPC. The calling process (Claude Desktop, etc.) writes JSON-RPC requests to stdin and reads responses from stdout. Stderr is reserved for logging.

**Source** (from `main.go` lines 38-45):
```go
case "stdio":
    fs.Parse(args)
    if *version { fmt.Printf("dino-mcp %s\n", server.Version); return }
    runStdio(*verbose)
```

**Transport implementation** (from `server.go` line 73):
```go
func RunStdio(ctx context.Context, s *mcp.Server, logger *slog.Logger) error {
    logger.Info("starting dino-mcp on stdio transport")
    return s.Run(ctx, &mcp.StdioTransport{})
}
```

The `mcp.StdioTransport` reads from `os.Stdin`, writes to `os.Stdout`, and is a blocking call — the process stays alive until the context is cancelled (SIGINT/SIGTERM).

### http Subcommand

Runs the MCP server over Streamable HTTP using Gin on the configured address.

**Source** (from `main.go` lines 47-54):
```go
case "http", "streamable-http":
    fs.Parse(args)
    if *version { ... }
    runHTTP(*addr, *verbose)
```

**Implementation** — `RunStreamableHTTP()` in `server.go` lines 77-130:
```go
func RunStreamableHTTP(ctx context.Context, s *mcp.Server, addr string, logger *slog.Logger) error {
    gin.SetMode(gin.ReleaseMode)
    r := gin.New()
    r.Use(gin.Recovery())
    r.Use(ginLogger(logger))
    r.Use(corsMiddleware())
    
    mcpHandler := mcp.NewStreamableHTTPHandler(
        func(r *http.Request) *mcp.Server { return s },
        &mcp.StreamableHTTPOptions{
            Logger: logger,
            DisableLocalhostProtection: true,  // for Cloudflare Tunnel
        },
    )
    r.Any("/mcp", gin.WrapH(mcpHandler))
    r.Any("/mcp/*any", gin.WrapH(mcpHandler))
    // ... dashboard, api, health routes ...
}
```

---

## Flags

| Flag | Type | Default | Applies to | Description | Usage |
|------|------|---------|------------|-------------|-------|
| `-addr` | string | `:9010` | `http` | HTTP listen address (with port) | `dino-mcp http -addr :8080` |
| `-verbose` | bool | `false` | all | Enable DEBUG-level logging to stderr | `dino-mcp http -verbose` |
| `-version` | bool | `false` | `stdio`, `http` | Print version and exit | `dino-mcp stdio -version` |

**Flag parsing** (from `main.go` lines 29-33):
```go
fs := flag.NewFlagSet("dino-mcp", flag.ExitOnError)
addr := fs.String("addr", ":9010", "HTTP listen address (for http mode)")
verbose := fs.Bool("verbose", false, "Enable verbose debug logging")
version := fs.Bool("version", false, "Print version and exit")
```

---

## Environment Variables

| Variable | Default | Description | Set by |
|----------|---------|-------------|--------|
| `GIN_MODE` | `release` (or `debug` with `-verbose`) | Controls Gin's log output | User |
| `STREAMABLE_HTTP_ACCEPT` | — | Optional: override Accept header validation | MCP SDK |
| `NO_COLOR` | — | Disable colored log output | User / CI |

---

## Route Table (http mode)

| Method | Path | Handler | Purpose | Source (server.go) |
|--------|------|---------|---------|---------------------|
| ANY | `/mcp` | `gin.WrapH(mcpHandler)` | MCP protocol JSON-RPC | line 93 |
| ANY | `/mcp/*any` | `gin.WrapH(mcpHandler)` | MCP subpaths | line 94 |
| GET | `/dashboard` | serve embedded HTML | Standalone dashboard UI | line 97 |
| GET | `/dashboard/*any` | serve embedded HTML | Dashboard subpaths | line 104 |
| GET | `/api/dinosaurs` | filtered JSON list | REST API for dashboard | line 112 |
| GET | `/api/dinosaurs?filter=X` | filtered JSON list | Filtered REST API | line 113 |
| GET | `/health` | JSON status | Health check endpoint | line 123 |

---

## Examples

### Development mode (verbose logging)

```bash
dino-mcp http -addr :9010 -verbose
```

### Production mode (release, quiet)

```bash
GIN_MODE=release dino-mcp http -addr :8080
```

### Claude Desktop (stdio)

```bash
dino-mcp stdio
```

### Checking version (for Claude Desktop config debugging)

```bash
dino-mcp stdio -version
# Output: dino-mcp dev  (or git hash if built with tags)
```

### Remote tunnel (via cloudflared)

```bash
dino-mcp http -addr :9010 &
cloudflared tunnel --url http://localhost:9010
```
Or use `make run-tunnel`.

---

## Exit Codes

| Code | Meaning | When |
|------|---------|------|
| 0 | Success | Clean exit (SIGINT/SIGTERM) or `-version` |
| 1 | Runtime error | Port in use, socket error, or unknown subcommand |

**Error handling** (from `main.go` lines 80-85 and 52-53):
```go
default:
    fmt.Fprintf(os.Stderr, "unknown subcommand: %s\n\n", subcommand)
    printUsage()
    os.Exit(1)
```
