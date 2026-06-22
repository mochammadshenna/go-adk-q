# ADR-0007: Use Raw hotel_currency DB Column Value in UI; No Hardcoded Symbol Map

**File(s):** `ui/src/mcp-app.ts`, `internal/repository/hotel.go`
**Decision date:** 2026-06-22

---

## Context

The `tb_hotels` table has a `hotel_currency` column containing ISO 4217 currency codes (e.g., `"IDR"`, `"USD"`, `"SGD"`). The UI must display prices with a currency indicator.

## Rejected Approach

A hardcoded symbol map (`IDR → "Rp"`, `USD → "$"`, etc.) was considered and rejected because:

- It creates a maintenance burden as Archipelago expands to new countries and currencies.
- It encodes assumptions in application code that the database already captures.
- Any new currency code requires a code change and redeployment.

## Decision

Pass the raw currency code from `hotel_currency` through the API to the UI unchanged. The `fmtPrice(v, currency)` function uses the code directly as the display prefix:

```
IDR 406.000
USD 28.00
SGD 45.00
```

`toLocaleString("id-ID")` is used for IDR (period as thousands separator per Indonesian convention). `toLocaleString("en-US")` is used for all other currencies.

```typescript
// ui/src/mcp-app.ts
function fmtPrice(v: number, currency: string): string {
  if (v <= 0 || !currency) return "";
  const locale = currency.toUpperCase() === "IDR" ? "id-ID" : "en-US";
  return currency + " " + Math.round(v).toLocaleString(locale);
}
```

```go
// internal/repository/hotel.go — COALESCE only guards against NULL, not a symbol override
COALESCE(h.hotel_currency, 'IDR')
```

## Consequence

Future non-IDR hotels automatically show the correct currency code without any code changes. The DB column is the single source of truth for currency display. Symbol formatting is driven entirely by the ISO code and locale rules, not by application-maintained lookup tables.
