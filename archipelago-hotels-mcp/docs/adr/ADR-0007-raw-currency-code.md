# ADR-0007: Raw hotel_currency DB Column as Display Prefix; No Hardcoded Symbol Map

**File(s):** `ui/src/mcp-app.ts`, `internal/repository/hotel.go`
**Decision date:** 2026-06-22

---

## Decision

The currency display in the UI uses the raw ISO currency code from the `hotel_currency` column in `tb_hotels` directly as the price prefix. No server-side or client-side symbol mapping (e.g., `IDR → "Rp"`) is applied. `fmtPrice()` formats numbers using the appropriate locale for the currency code and prepends the code itself: `"IDR 406.000"`, `"USD 28.00"`, `"SGD 45.00"`.

### Implementation

```typescript
// ui/src/mcp-app.ts — fmtPrice (no symbol lookup)
function fmtPrice(v: number, currency: string): string {
  if (v <= 0 || !currency) return "";
  const locale = currency.toUpperCase() === "IDR" ? "id-ID" : "en-US";
  return currency + " " + Math.round(v).toLocaleString(locale);
}

// ponytail: fmtPriceShort/fmtPriceFull kept as aliases — all call sites use full locale format
const fmtPriceShort = fmtPrice;
const fmtPriceFull  = fmtPrice;

// Usage — currency always comes from hotel data, never hardcoded
fmtPrice(h.priceFrom, h.currency)        // hotel card: "IDR 850.000"
fmtPrice(room.pricePerNight, room.currency) // room rate: "IDR 1.200.000"
fmtPrice(detail.startingPrice, detail.currency) // overlay header
```

```go
// hotel.go — SQL query reads hotel_currency from the DB column
selectCols := `SELECT
    ...
    COALESCE(h.hotel_currency, 'IDR'),  // fallback only if column is NULL
    ...`

// HotelRow — currency threaded through all layers to the UI
type HotelRow struct {
    ...
    Currency string
    ...
}
```

```go
// Tool handlers — currency flows from DB row to structuredContent JSON
summaries = append(summaries, hotelSummary{
    Currency: h.Currency,  // raw DB value, e.g. "IDR", "USD", "SGD"
    ...
})
```

### Key Details

| Aspect | Implementation | File/Line |
|--------|---------------|-----------|
| DB column | `tb_hotels.hotel_currency` — ISO 4217 code (e.g., `"IDR"`, `"USD"`) | `hotel.go:SearchHotels` |
| NULL fallback | `COALESCE(h.hotel_currency, 'IDR')` — only when column IS NULL | `hotel.go:selectCols` |
| Client display | `currency + " " + toLocaleString(locale)` | `mcp-app.ts:fmtPrice` |
| IDR locale | `"id-ID"` — period as thousands separator: `406.000` | `mcp-app.ts:fmtPrice` |
| Other locales | `"en-US"` — comma as thousands separator: `1,200.00` | `mcp-app.ts:fmtPrice` |
| Previous approach | `currencySymbol()` map: `IDR → "Rp"`, `USD → "$"` — removed | git history |

### Alternatives Considered

| Option | Rejected because |
|--------|-----------------|
| Symbol map (`IDR → "Rp"`) | Requires maintenance as Archipelago expands to new countries; user requirement is explicit: currency must come from the DB column |
| Always show `"Rp"` | Hardcoded assumption that all hotels are in Indonesia; breaks for international properties |
| No currency prefix (number only) | Ambiguous — `850.000` without context looks like European thousands-formatted USD |
| Server-side symbol resolution | Moves business logic into the API layer; client already has the currency code |

### History

The original implementation hardcoded `"Rp"` as the price prefix throughout the UI template strings. This was replaced with a `currencySymbol()` lookup map. The map was subsequently removed entirely on user direction — the raw DB column value is the authoritative display string.
