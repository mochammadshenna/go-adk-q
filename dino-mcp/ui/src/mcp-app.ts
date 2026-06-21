/**
 * Dino Dashboard — MCP App UI
 *
 * Standard @modelcontextprotocol/ext-apps SDK usage:
 *   1. new App({ name, version })
 *   2. Register handlers BEFORE connect (ontoolresult, ontoolinput, onhostcontextchanged,
 *      onteardown, onerror, ontoolcancelled)
 *   3. app.connect() — auto-detects PostMessageTransport when inside an iframe
 *   4. app.connect().then(() => app.getHostContext()) — apply initial host context
 *
 * Reference: https://github.com/modelcontextprotocol/ext-apps/tree/main/examples/basic-server-vanillajs
 *            https://apps.extensions.modelcontextprotocol.io/api/
 *
 * For development/testing without the build pipeline, see the standalone
 * dashboard_ui.html in the Go server package.
 */

import {
  App,
  applyDocumentTheme,
  applyHostFonts,
  applyHostStyleVariables,
} from "@modelcontextprotocol/ext-apps";

// --- Types ---

interface DinoSummary {
  name: string;
  period: string;
  diet: string;
  length: string;
  weight: string;
  funFact: string;
  imageStyle: string;
}

interface DashboardData {
  filter: string;
  dinosaurs: DinoSummary[];
  timestamp: string;
}

interface ToolInputParams {
  arguments?: Record<string, unknown>;
  structuredContent?: DashboardData;
}

// --- State ---

let allDinosaurs: DinoSummary[] = [];
let currentFilter = "";
let isFullscreen = false;

// --- DOM Setup ---

function buildDOM(): void {
  const root = document.getElementById("root");
  if (!root) return;

  root.innerHTML = `
    <div id="app">
      <!-- Loading -->
      <div id="loading-state" class="state" style="text-align:center;padding:48px 16px;">
        <div style="font-size:48px;margin-bottom:12px;">🦖</div>
        <h3>Loading Dino Dashboard...</h3>
        <p style="color:var(--color-text-secondary,#b8b2d0);margin-top:4px;">Preparing your dinosaur experience</p>
      </div>
      <!-- Error -->
      <div id="error-state" class="state hidden" style="text-align:center;padding:48px 16px;">
        <div style="font-size:48px;margin-bottom:12px;">⚠️</div>
        <h3>Could Not Load Dinos</h3>
        <p id="error-message" style="color:var(--color-text-secondary,#b8b2d0);margin-top:4px;"></p>
      </div>
      <!-- Dashboard -->
      <div id="dashboard-content" class="hidden">
        <header class="dash-header">
          <div class="dash-title">
            <span class="dino-icon">🦕</span>
            <h1>Dino Dashboard</h1>
          </div>
          <div style="display:flex;align-items:center;gap:8px;">
            <span id="dino-count" class="count-badge">0 dinosaurs</span>
            <button id="fullscreen-btn" class="btn-icon hidden">⛶</button>
          </div>
        </header>
        <div class="stats-row">
          <div class="stat-card"><div id="stat-total" class="stat-val">0</div><div class="stat-lbl">Species</div></div>
          <div class="stat-card"><div id="stat-carnivores" class="stat-val" style="color:var(--danger,#f87171)">0</div><div class="stat-lbl">Carnivores</div></div>
          <div class="stat-card"><div id="stat-herbivores" class="stat-val" style="color:var(--success,#34d399)">0</div><div class="stat-lbl">Herbivores</div></div>
          <div class="stat-card"><div id="stat-periods" class="stat-val">0</div><div class="stat-lbl">Periods</div></div>
        </div>
        <div class="filter-bar" id="filter-bar">
          <button class="filter-btn active" data-filter="">All</button>
          <button class="filter-btn" data-filter="Cretaceous">Cretaceous</button>
          <button class="filter-btn" data-filter="Jurassic">Jurassic</button>
          <button class="filter-btn" data-filter="Triassic">Triassic</button>
          <button class="filter-btn" data-filter="Carnivore">Carnivores</button>
          <button class="filter-btn" data-filter="Herbivore">Herbivores</button>
        </div>
        <div id="dino-grid" class="dino-grid"></div>
      </div>
    </div>
  `;

  // Filter listeners
  document.getElementById("filter-bar")?.addEventListener("click", (e) => {
    const btn = (e.target as HTMLElement).closest(".filter-btn");
    if (!btn) return;
    const filter = (btn as HTMLElement).getAttribute("data-filter") || "";
    applyFilter(filter);
  });

  document.getElementById("fullscreen-btn")?.addEventListener("click", () => {
    isFullscreen = !isFullscreen;
    window.parent.postMessage(
      {
        type: "ui-message-response",
        messageType: "request-display-mode",
        payload: { mode: isFullscreen ? "fullscreen" : "inline" },
      },
      "*",
    );
  });
}

// --- Styles (injected) ---

const STYLES = `
  :root {
    --bg-p: #1e1b2e; --bg-s: #2d2a44; --bg-c: #3d3a54;
    --text-p: #f0eef8; --text-s: #b8b2d0;
    --accent: #a78bfa; --border: #4d4a64;
    --success: #34d399; --danger: #f87171; --warning: #fbbf24;
  }
  * { margin:0; padding:0; box-sizing:border-box; }
  body {
    font-family: var(--font-sans, system-ui, -apple-system, sans-serif);
    background: var(--color-background-primary, var(--bg-p));
    color: var(--color-text-primary, var(--text-p));
    padding: 8px; line-height: 1.5;
  }
  .hidden { display: none !important; }
  .dash-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 12px 16px;
    background: var(--color-background-secondary, var(--bg-s));
    border: 1px solid var(--color-border, var(--border));
    border-radius: var(--border-radius-lg, 16px);
    margin-bottom: 16px; flex-wrap: wrap; gap: 12px;
  }
  .dash-title { display: flex; align-items: center; gap: 10px; }
  .dash-title h1 {
    font-size: var(--font-heading-2-size, 20px);
    font-weight: 700; letter-spacing: -0.02em;
  }
  .dino-icon {
    width: 32px; height: 32px;
    background: var(--accent); border-radius: 8px;
    display: flex; align-items: center; justify-content: center;
    font-size: 18px;
  }
  .count-badge {
    font-size: var(--font-text-small-size, 13px);
    color: var(--color-text-secondary, var(--text-s));
    background: var(--color-background-primary, var(--bg-p));
    padding: 4px 12px; border-radius: 20px;
  }
  .btn-icon {
    background: none;
    border: 1px solid var(--color-border, var(--border));
    color: var(--color-text-secondary, var(--text-s));
    padding: 6px 12px; border-radius: var(--border-radius-sm, 6px);
    cursor: pointer; font-size: 14px;
    transition: border-color 0.2s, color 0.2s;
  }
  .btn-icon:hover { border-color: var(--accent); color: var(--accent); }
  .stats-row {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(100px, 1fr));
    gap: 10px; margin-bottom: 16px;
  }
  .stat-card {
    background: var(--color-background-secondary, var(--bg-s));
    border: 1px solid var(--color-border, var(--border));
    border-radius: var(--border-radius-md, 10px);
    padding: 12px; text-align: center;
  }
  .stat-val {
    font-size: var(--font-heading-1-size, 24px);
    font-weight: 700; color: var(--accent);
  }
  .stat-lbl {
    font-size: var(--font-text-small-size, 12px);
    color: var(--color-text-secondary, var(--text-s));
    margin-top: 2px;
  }
  .filter-bar { display: flex; gap: 8px; flex-wrap: wrap; margin-bottom: 16px; }
  .filter-btn {
    font-family: var(--font-sans, system-ui, sans-serif);
    font-size: var(--font-text-small-size, 13px);
    padding: 6px 14px;
    border: 1px solid var(--color-border, var(--border));
    border-radius: 20px; background: transparent;
    color: var(--color-text-secondary, var(--text-s));
    cursor: pointer; transition: all 0.2s;
  }
  .filter-btn:hover {
    background: var(--color-background-secondary, var(--bg-s));
    color: var(--color-text-primary, var(--text-p));
  }
  .filter-btn.active, .filter-btn[data-active="true"] {
    background: var(--accent); color: #fff; border-color: var(--accent);
  }
  .dino-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
    gap: 12px;
  }
  .dino-card {
    background: var(--color-background-secondary, var(--bg-s));
    border: 1px solid var(--color-border, var(--border));
    border-radius: var(--border-radius-md, 10px);
    overflow: hidden; transition: transform 0.2s, box-shadow 0.2s;
  }
  .dino-card:hover {
    transform: translateY(-2px);
    box-shadow: 0 8px 24px rgba(0,0,0,0.3);
  }
  .card-header {
    padding: 16px 16px 12px;
    display: flex; align-items: center; gap: 12px;
  }
  .dino-avatar {
    width: 48px; height: 48px; border-radius: var(--border-radius-md, 10px);
    display: flex; align-items: center; justify-content: center;
    font-size: 24px; color: white; flex-shrink: 0;
  }
  .dino-name { font-size: var(--font-heading-3-size, 16px); font-weight: 600; }
  .diet-badge { font-size: 11px; padding: 2px 8px; border-radius: 10px; font-weight: 500; }
  .diet-carnivore { background: rgba(248,113,113,0.2); color: var(--danger); }
  .diet-herbivore { background: rgba(52,211,153,0.2); color: var(--success); }
  .period-badge {
    display: inline-block; font-size: 11px;
    padding: 2px 8px; border-radius: 10px;
    background: var(--color-background-primary, var(--bg-p));
    color: var(--color-text-secondary, var(--text-s));
    font-weight: 500;
  }
  .card-body { padding: 0 16px 12px; }
  .detail-row {
    display: flex; justify-content: space-between;
    padding: 4px 0;
    font-size: var(--font-text-small-size, 13px);
  }
  .detail-lbl { color: var(--color-text-secondary, var(--text-s)); }
  .detail-val { font-weight: 500; }
  .fun-fact {
    margin: 8px 16px 12px;
    padding: 8px 12px;
    background: var(--color-background-primary, var(--bg-p));
    border-radius: var(--border-radius-sm, 6px);
    font-size: var(--font-text-small-size, 12px);
    font-style: italic;
    color: var(--color-text-secondary, var(--text-s));
    border-left: 3px solid var(--accent);
  }
`;

function injectStyles(): void {
  const styleEl = document.createElement("style");
  styleEl.textContent = STYLES;
  document.head.appendChild(styleEl);
}

// --- Avatar colors ---

const COLORS = [
  "#a78bfa", "#7c3aed", "#6366f1", "#3b82f6", "#06b6d4",
  "#34d399", "#10b981", "#f59e0b", "#f97316", "#ef4444",
  "#ec4899", "#8b5cf6",
];

function avatarColor(name: string): string {
  let hash = 0;
  for (let i = 0; i < name.length; i++)
    hash = ((hash << 5) - hash) + name.charCodeAt(i), hash |= 0;
  return COLORS[Math.abs(hash) % COLORS.length];
}

function esc(str: string): string {
  const d = document.createElement("div");
  d.textContent = str;
  return d.innerHTML;
}

// --- Rendering ---

function renderDinos(dinos: DinoSummary[]): void {
  const grid = document.getElementById("dino-grid");
  const count = document.getElementById("dino-count");
  if (!grid || !count) return;

  if (dinos.length === 0) {
    grid.innerHTML = `<div style="grid-column:1/-1;text-align:center;padding:48px 16px;color:var(--color-text-secondary,var(--text-s))">
      <div style="font-size:48px;margin-bottom:8px;">🔍</div>
      <h3 style="font-size:16px;margin-bottom:4px;color:var(--color-text-primary,var(--text-p))">No Dinosaurs Found</h3>
      <p>Try a different filter</p></div>`;
    count.textContent = "0 dinosaurs";
    renderStats(dinos);
    return;
  }

  grid.innerHTML = dinos
    .map(
      (d) => `
    <div class="dino-card" role="article" aria-label="${esc(d.name)}">
      <div class="card-header">
        <div class="dino-avatar" style="background:${avatarColor(d.name)}">${esc(d.name.charAt(0))}</div>
        <div>
          <div class="dino-name">${esc(d.name)}</div>
          <span class="period-badge">${esc(d.period || "Unknown")}</span>
        </div>
      </div>
      <div class="card-body">
        <div class="detail-row">
          <span class="detail-lbl">Diet</span>
          <span><span class="diet-badge ${(d.diet || "").toLowerCase() === "carnivore" ? "diet-carnivore" : "diet-herbivore"}">${esc(d.diet || "Unknown")}</span></span>
        </div>
        <div class="detail-row"><span class="detail-lbl">Length</span><span class="detail-val">${esc(d.length || "Unknown")}</span></div>
        <div class="detail-row"><span class="detail-lbl">Weight</span><span class="detail-val">${esc(d.weight || "Unknown")}</span></div>
      </div>
      ${d.funFact ? `<div class="fun-fact">💡 ${esc(d.funFact)}</div>` : ""}
    </div>`,
    )
    .join("");

  count.textContent = `${dinos.length} dinosaur${dinos.length !== 1 ? "s" : ""}`;
  renderStats(dinos);
}

function renderStats(dinos: DinoSummary[]): void {
  const total = dinos.length;
  const carn = dinos.filter((d) => d.diet?.toLowerCase() === "carnivore").length;
  const herb = dinos.filter((d) => d.diet?.toLowerCase() === "herbivore").length;
  const periods = new Set(dinos.map((d) => d.period).filter(Boolean)).size;
  setText("stat-total", total);
  setText("stat-carnivores", carn);
  setText("stat-herbivores", herb);
  setText("stat-periods", periods);
}

function setText(id: string, val: string | number): void {
  const el = document.getElementById(id);
  if (el) el.textContent = String(val);
}

function applyFilter(filter: string): void {
  currentFilter = filter;
  document.querySelectorAll(".filter-btn").forEach((btn) => {
    const isActive = (btn as HTMLElement).getAttribute("data-filter") === filter;
    btn.classList.toggle("active", isActive);
    btn.setAttribute("data-active", isActive ? "true" : "false");
  });
  if (!filter) {
    renderDinos(allDinosaurs);
  } else {
    const f = filter.toLowerCase();
    renderDinos(
      allDinosaurs.filter(
        (d) =>
          d.diet?.toLowerCase() === f ||
          d.period?.toLowerCase() === f ||
          d.name?.toLowerCase().includes(f),
      ),
    );
  }
}

function showDashboard(data: DashboardData): void {
  allDinosaurs = data.dinosaurs || [];
  hide("loading-state");
  hide("error-state");
  show("dashboard-content");
  applyFilter(currentFilter || data.filter || "");
}

function showError(msg: string): void {
  hide("loading-state");
  show("error-state");
  hide("dashboard-content");
  const el = document.getElementById("error-message");
  if (el) el.textContent = msg;
}

function hide(id: string): void {
  document.getElementById(id)?.classList.add("hidden");
}
function show(id: string): void {
  document.getElementById(id)?.classList.remove("hidden");
}

// --- App Lifecycle ---

async function main(): Promise<void> {
  injectStyles();
  buildDOM();

  const app = new App({ name: "Dino Dashboard", version: "0.1.0" });

  // Called when the tool provides input data
  app.ontoolinput = (params: ToolInputParams): void => {
    const data = params.structuredContent as DashboardData | undefined;
    if (data?.dinosaurs && data.dinosaurs.length > 0) {
      showDashboard(data);
    } else if (params.arguments) {
      // Partial arguments from the LLM — show loading with filter hint
      if ((params.arguments.filter as string) && allDinosaurs.length > 0) {
        applyFilter(params.arguments.filter as string);
      }
    }
  };

  // Called for streaming partial input (while LLM generates)
  app.ontoolinputpartial = (params: ToolInputParams): void => {
    // Show preview if we have enough data
    if (params.structuredContent?.dinosaurs) {
      showDashboard(params.structuredContent);
    }
  };

  // Called when tool returns result
  app.ontoolresult = (result): void => {
    if (result.isError) {
      const firstText = result.content?.find((c) => c.type === "text");
      showError(firstText && "text" in firstText ? firstText.text : "Tool returned an error.");
      return;
    }
    const data = result.structuredContent as DashboardData | undefined;
    if (data?.dinosaurs && data.dinosaurs.length > 0) {
      showDashboard(data);
    }
  };

  // Host theme/context changes
  app.onhostcontextchanged = (ctx: any): void => {
    if (ctx.theme) applyDocumentTheme(ctx.theme);
    if (ctx.styles?.variables) applyHostStyleVariables(ctx.styles.variables);
    if (ctx.styles?.css?.fonts) applyHostFonts(ctx.styles.css.fonts);
    if (ctx.safeAreaInsets) {
      const { top = 0, right = 0, bottom = 0, left = 0 } = ctx.safeAreaInsets;
      document.body.style.padding = `${top}px ${right}px ${bottom}px ${left}px`;
    }
    if (ctx.availableDisplayModes?.includes("fullscreen")) {
      const btn = document.getElementById("fullscreen-btn");
      if (btn) btn.classList.remove("hidden");
    }
    if (ctx.displayMode) {
      isFullscreen = ctx.displayMode === "fullscreen";
      const btn = document.getElementById("fullscreen-btn");
      if (btn) btn.textContent = isFullscreen ? "✕" : "⛶";
    }
  };

  // Cleanup
  app.onteardown = async (): Promise<Record<string, unknown>> => {
    allDinosaurs = [];
    return {};
  };

  // Connect to the host (auto-detects PostMessageTransport inside iframe)
  await app.connect();
}

main().catch((err) => {
  console.error("Dino Dashboard failed:", err);
  const el = document.getElementById("error-message");
  if (el) el.textContent = `Initialization error: ${err instanceof Error ? err.message : String(err)}`;
  show("error-state");
});
