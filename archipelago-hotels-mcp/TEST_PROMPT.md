# 🧪 Archipelago Hotels MCP — Test Prompts

> Copy-paste these into **Claude Desktop** or **Pi Agent** to verify every brand, tool, and rate fallback path.

---

## 1. Full Brand + Room Rate Audit (the one)

Tests: `search_hotels` + `get_hotel_detail` (every brand, every DB connection, room rate extraction)

> Search all Archipelago hotels. Group them by brand and count how many hotels each brand has. Then for each brand, pick one hotel and show me its full details including all room types with prices. List every brand, the hotel name, and every room type name + price.

**Expect:** 15–25 turns, all 13+ brands listed with hotel counts, each brand has at least one hotel with room types and non-zero prices. No "hotel not found" or "database error" responses.

---

## 2. Problematic Brand Verification

Tests: `favehotel` DB name override → `db_favewebsite`, `pba` routing → `db_pba`

> Search for hotels in the **favehotels** brand and show me 2 hotels with full details including room rates and prices.

**Expect:** favehotels hotels return with room types and prices. If this works, the `db_prefix_name` → actual DB mapping is fixed.

> Same but for **Powered By Archi** — show 2 hotels with room types and prices.

**Expect:** PBA hotels return (they use `tb_hroom` table and UUID primary keys). Different DB (`db_pba`), different table name.

---

## 3. Multi-City Coverage

Tests: all city search paths, international coverage, zero-result case

> Find hotels in **Bali**, **Yogyakarta**, **Kuala Lumpur**, and **Tokyo**. Show me how many in each city, then full details for the first hotel in each.

**Expect:** Bali (15+) and Yogyakarta (8+) have hotels. Kuala Lumpur may have 0–few. **Tokyo returns 0 → must show empty results gracefully, not an error.** Details include room types with prices.

---

## 4. Rate Fallback Chain

Tests: `recommend_hotel` + `get_hotel_detail` — verifies `BatchMinRates` → `trySB` → `tryStored` → `StartingPrice`

> I need **luxury** hotels in **Jakarta** for a **business** trip. Show me the top 5 recommendations with full details including room rates.

**Expect:** 5 hotels with prices. Prices may come from `tb_hrooms.room_rate` (stored) or `StartingPrice` (SB returns empty for most hotels). No "live" price source unless an SB credential pair works.

---

## 5. Price Spread Test

Tests: rate min calculation, `room_rate = 0` exclusion, currency display

> Show me all hotels in **Bali** sorted by price from cheapest to most expensive, with the first hotel's full room breakdown.

**Expect:** Prices sorted ascending. No `$0` or null prices. First hotel has all rooms listed with individual prices. Prices in IDR (Indonesian Rupiah).

---

## 6. Dashboard & Recommend

Tests: `hotel_dashboard` tool, `recommend_hotel` with different vibes

> Recommend hotels in **Yogyakarta** for a **cultural** trip. What's your top pick and why? Also show me the dashboard for Yogyakarta.

**Expect:** Top pick has a reason (matches "culture" vibe). Dashboard returns hotels in Yogyakarta with a "Showing X hotels" message.

---

## 7. Edge Case: Empty City

Tests: zero-result handling (must not error)

> Find hotels in **London**.

**Expect:** `"No Archipelago hotels found in 'London'. Try Jakarta, Bali, Yogyakarta, Kuala Lumpur, or Tokyo."` or similar empty-state message. **Not** an error/exception.

---

## 8. Edge Case: No Params

Tests: default search (no city filter)

> Show me all hotels with no filters, and tell me how many total hotels there are.

**Expect:** 200+ hotels returned (default limit is 200). Message says "Showing 200 hotels" or similar.

---

## 9. Performance Cache Test

Tests: cache is working (second call is ~250× faster)

> Search hotels in **Bali** twice in a row. Compare the response times.

**Expect:** First call ~1–3s (cold: brand DB connections + rate fetch). Second call ~10–50ms (all rates cached). If both are same speed → cache isn't working.

---

## 10. Stdio Transport Cleanliness

Tests: no stdout noise (must pass for Claude Desktop / Pi Agent)

> *(not a prompt — run this in terminal)*
> ```
> echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}' | ./bin/archipelago-hotels-mcp stdio | head -1
> ```

**Expect:** Output is a JSON `{"jsonrpc":"2.0","id":1,"result":{...}}`. **Not** `resources: dashboard_ui.html embedded (...)`.

---

## Expected Output Matrix

| Prompt | Tool(s) | Success Criterion | Failure Mode |
|--------|---------|-------------------|--------------|
| #1 | search + detail | All brands + rooms priced | Missing brand → brand DB down |
| #2 (fave) | search + detail | Hotels with rooms | DB name mapping broken |
| #2 (PBA) | search + detail | Hotels with rooms (tb_hroom) | PBA routing broken |
| #3 (Tokyo) | search | `"No hotels found"` not error | Zero-result crashes |
| #4 | recommend + detail | 5 hotels with prices | Rate fallback chain broken |
| #5 | search + detail | Sorted prices, no zeros | Room rate min calculation wrong |
| #6 | recommend + dashboard | Pick has reason | Scoring algorithm wrong |
| #7 (London) | search | Empty message, not error | Handler panics on empty |
| #8 | search | 200+ hotels | Default limit wrong |
| #9 | search × 2 | Second call >> first | Cache broken |
| #10 | stdio | JSON output, not text | Stdout noise present |

## Quickest Validation

Run **#1** (full audit) — it's the hardest test. If all brands return hotels with room types and prices, everything is working.
