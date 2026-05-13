# How-to: Add a provider (quick reference)

This is a condensed checklist version of the full tutorial at
[tutorials/add-provider.md](../tutorials/add-provider.md).

---

## Checklist

```
model/myprovider/
└── myprovider.go
```

### myprovider.go must export

- [ ] `Config` struct: `APIKey string`, `BaseURL string`, `Model string`
- [ ] `ConfigFromEnv() Config` — reads `MYPROVIDER_API_KEY`, `MYPROVIDER_MODEL`, `MYPROVIDER_BASE_URL`
- [ ] `NewModel(ctx context.Context, cfg Config) (model.LLM, error)`
- [ ] `KnownModels catalog.ProviderCatalog`

### cmd/tui/main.go

- [ ] `import "go-adk-q/model/myprovider"` added
- [ ] `catalog.Register(myprovider.KnownModels)` called in `init()`
- [ ] Provider added to `failover.New(...)` in `buildRunner()`

### Verification

```sh
go build ./...
go test ./model/myprovider/...
MYPROVIDER_API_KEY=x go run ./cmd/tui chat
# /model → verify provider appears
```

---

## oaibridge vs native LLM

| Use | When |
|---|---|
| `oaibridge.New(ctx, oaibridge.Config{...})` | Provider has OpenAI-compatible `/v1/chat/completions` |
| Implement `model.LLM` directly | Native or incompatible API |

See `model/echo/echo.go` for the minimal `model.LLM` implementation.

---

## Related

- [Tutorial: Add a new LLM provider](../tutorials/add-provider.md)
- [Reference: Provider reference](../reference/providers.md)
- [ADR-0005: Provider Config pattern](../adr/ADR-0005-provider-config-pattern.md)
