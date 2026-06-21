# How-to: Add a Dinosaur Species

> **Add a new dinosaur to the dashboard.** All data lives in `internal/tools/dashboard.go` as a Go slice of `DinoSummary` structs. No database, no config file, no API call.

**Source file:** [`internal/tools/dashboard.go`](https://github.com/msw/dino-mcp/blob/main/internal/tools/dashboard.go)
**Data type:** `DinoSummary` (defined in `server.go` lines 176-185)

---

## The Data Model

Every dinosaur is a `DinoSummary` struct (from `server.go` lines 176-185):

```go
type DinoSummary struct {
    Name       string `json:"name" jsonschema:"Dinosaur name."`
    Period     string `json:"period" jsonschema:"Geological period."`
    Diet       string `json:"diet" jsonschema:"Diet type."`
    Length     string `json:"length" jsonschema:"Length description."`
    Weight     string `json:"weight" jsonschema:"Weight description."`
    FunFact    string `json:"funFact" jsonschema:"A fun fact."`
    ImageStyle string `json:"imageStyle" jsonschema:"CSS style hint for image rendering."`
}
```

| Field | Type | Example | Notes |
|-------|------|---------|-------|
| `Name` | string | `"Iguanodon"` | Displayed on the card header |
| `Period` | string | `"Cretaceous"` | Used for period filtering (must match: Triassic, Jurassic, or Cretaceous) |
| `Diet` | string | `"Herbivore"` | Used for diet filtering (must be Carnivore or Herbivore) |
| `Length` | string | `"33 ft (10 m)"` | Displayed in card body |
| `Weight` | string | `"5 tons (4,500 kg)"` | Displayed in card body |
| `FunFact` | string | `"Iguanodon had a spike on its thumb..."` | Displayed in italic block on card |
| `ImageStyle` | string | `"bg-yellow-800"` | CSS class hint (only used if the UI supports avatar backgrounds) |

---

## Steps

### 1. Edit `internal/tools/dashboard.go`

Find the `dinoData` slice (line ~100):

```go
var dinoData = []DinoSummary{
    {
        Name: "Tyrannosaurus Rex", Period: "Cretaceous", Diet: "Carnivore",
        Length: "40 ft (12 m)", Weight: "9 tons (8,000 kg)",
        FunFact:    "T-Rex had the strongest bite force of any land animal ever — over 12,000 pounds!",
        ImageStyle: "bg-red-900",
    },
    // ... 11 more entries ...
    {
        Name: "Maiasaura", Period: "Cretaceous", Diet: "Herbivore",
        Length: "30 ft (9 m)", Weight: "3 tons (2,700 kg)",
        FunFact:    "Maiasaura means 'good mother lizard' — evidence shows they cared for their young in nesting colonies.",
        ImageStyle: "bg-lime-800",
    },
}
```

### 2. Add a new entry

Add your dinosaur at the end of the slice (before the closing `}`):

```go
{
    Name:       "Iguanodon",
    Period:     "Cretaceous",
    Diet:       "Herbivore",
    Length:     "33 ft (10 m)",
    Weight:     "5 tons (4,500 kg)",
    FunFact:    "Iguanodon had a spike on its thumb that could be used for defense against predators.",
    ImageStyle: "bg-yellow-800",
},
```

### 3. Rebuild and test

```bash
make build-fast
make test
```

### 4. Verify

| Check | How | Expected result |
|-------|-----|-----------------|
| Standalone dashboard | http://localhost:9010/dashboard | New species card visible |
| Filter by diet | http://localhost:9010/api/dinosaurs?filter=Herbivore | Iguanodon included |
| Filter by period | http://localhost:9010/api/dinosaurs?filter=Cretaceous | Iguanodon included |
| MCP Inspector | `make test-inspector` → call `dino_dashboard` | New species in `structuredContent.dinosaurs` |
| Claude Desktop | "Show me herbivore dinosaurs" | Iguanodon appears in dashboard |

---

## How Filtering Works

The `tools.FilteredDinosaurs()` function (line ~195 in `dashboard.go`) matches your filter against three fields:

```go
func FilteredDinosaurs(filter string) []DinoSummary {
    if filter == "" { return dinoData }  // no filter → all
    fl := toLower(filter)
    var result []DinoSummary
    for _, d := range dinoData {
        if contains(toLower(d.Diet), fl) ||     // match diet (carnivore, herbivore)
            contains(toLower(d.Period), fl) ||   // match period (triassic, jurassic, cretaceous)
            contains(toLower(d.Name), fl) {      // match name (partial)
            result = append(result, d)
        }
    }
    return result
}
```

So "Carnivore" matches all carnivores, "Cretaceous" matches all Cretaceous species, and "T-Rex" matches T-Rex only.

---

## Updating the UI

If you want to make visual changes to the card layout (not just add data), edit `ui/src/mcp-app.ts`. The card HTML template is in the `renderDinos()` function (~line 240):

```typescript
grid.innerHTML = dinos.map(d => `
    <div class="dino-card">
      <div class="card-header">
        <div class="dino-avatar" style="background:${avatarColor(d.name)}">${esc(d.name.charAt(0))}</div>
        <div>
          <div class="dino-name">${esc(d.name)}</div>
          <span class="period-badge">${esc(d.period || "Unknown")}</span>
        </div>
      </div>
      ...
    </div>
`).join("");
```

After editing the TypeScript:

```bash
make build-ui   # rebuilds the Vite bundle → updates dashboard_ui.html
make build-go   # recompiles the binary with the new HTML
```
