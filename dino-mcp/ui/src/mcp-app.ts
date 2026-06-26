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
    width: 64px; height: 64px; border-radius: var(--border-radius-md, 10px);
    display: flex; align-items: center; justify-content: center;
    flex-shrink: 0; perspective: 400px; overflow: visible;
  }
  .dino-avatar svg {
    width: 56px; height: 56px;
    animation: dino3d 5s ease-in-out infinite;
    transform-origin: center center;
    filter: drop-shadow(2px 4px 6px rgba(0,0,0,0.4));
  }
  @keyframes dino3d {
    0%   { transform: rotateY(-30deg) rotateX(5deg) scale(0.95); }
    25%  { transform: rotateY(0deg)   rotateX(0deg) scale(1.05); }
    50%  { transform: rotateY(30deg)  rotateX(-5deg) scale(0.95); }
    75%  { transform: rotateY(0deg)   rotateX(0deg) scale(1.05); }
    100% { transform: rotateY(-30deg) rotateX(5deg) scale(0.95); }
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
    background: rgba(167,139,250,0.08);
    border-radius: var(--border-radius-sm, 6px);
    font-size: var(--font-text-small-size, 12px);
    font-style: italic;
    color: var(--color-text-secondary, var(--text-s));
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

// --- Dinosaur 3D SVG silhouettes with gradient shading + spin animation ---

function dinoSVG(name: string, color: string): string {
  const gid = `g${Math.abs(name.split("").reduce((h,c)=>((h<<5)-h)+c.charCodeAt(0)|0,0))}`;
  const grad = `<defs><radialGradient id="${gid}" cx="30%" cy="25%" r="70%"><stop offset="0%" stop-color="rgba(255,255,255,0.5)"/><stop offset="50%" stop-color="rgba(255,255,255,0)"/><stop offset="100%" stop-color="rgba(0,0,0,0.45)"/></radialGradient></defs>`;
  const shapes: Record<string, string> = {
    // T-Rex: right-facing bipedal — massive jaw-head, tiny arm nub, two thick legs, long tail
    "Tyrannosaurus Rex":
      `<path d="M5,42C10,34 18,28 27,25C34,21 43,18 50,16C55,13 61,10 66,10C71,9 76,11 78,18L78,28C75,33 70,31 65,29C61,28 56,28 51,30L47,34C49,38 52,42 53,44L50,46C45,48 39,48 34,47C27,47 22,50 20,57C18,64 19,72 22,72L30,72 31,66 33,72 42,72C44,67 41,61 39,55C37,51 34,49 36,49C42,51 48,51 52,48C44,48 30,47 17,45C11,43 5,42 5,42Z"/>`,
    // Triceratops: right-facing quadruped — frill + 2 long brow-horns + 1 nose-horn, four legs
    "Triceratops":
      `<path d="M4,54C8,48 13,46 19,46C25,45 32,45 40,44C47,43 53,42 58,40C62,37 63,32 60,28C57,24 52,22 54,20C56,17 60,16 64,18C68,20 68,26 65,30L62,32C67,29 73,27 70,33L65,35C68,33 71,33 70,38C67,42 62,40 58,40C58,43 60,47 62,51C58,53 53,50 51,47C46,47 40,49 34,53L30,66 22,66 22,54 18,66 10,66 10,54C8,54 6,54 4,54Z"/>
       <path d="M64,18L78,6 74,14 66,18ZM58,22L72,12 68,20 60,24ZM63,30L76,24 72,32 64,34Z"/>`,
    // Stegosaurus: right-facing quadruped — arched body, distinctive triangular back-plates, spiked tail
    "Stegosaurus":
      `<path d="M4,60C8,54 13,52 18,52C22,52 26,52 30,50C34,48 38,46 43,44C48,42 53,42 58,44C62,46 64,50 64,55C64,59 62,62 58,62L54,66 46,66 46,62 42,66 34,66 34,62 30,66 22,66 22,60C16,60 10,60 4,60Z"/>
       <path d="M19,51L13,30 25,49ZM28,49L23,25 34,47ZM37,47L33,22 44,45ZM46,45L43,26 54,43ZM57,45L56,32 64,45Z"/>
       <path d="M4,58L10,64 4,66ZM2,56L8,60 6,66Z"/>`,
    // Velociraptor: right-facing running — lean horizontal body, stiff tail, raised sickle claw
    "Velociraptor":
      `<path d="M4,36C8,30 14,26 20,24C23,23 26,23 28,22L33,14C35,12 38,13 37,17L32,22C36,20 40,19 40,23C39,27 36,29 33,31L30,33C32,37 35,42 34,46L30,50C27,52 23,52 20,52C17,50 15,47 16,43C13,45 9,46 6,45Z"/>
       <path d="M4,36C9,33 16,30 22,28L28,26 30,33 22,24Z"/>
       <path d="M20,52L22,64 26,70C29,70 29,66 26,64L26,52Z"/>
       <path d="M28,50L30,62 34,68C37,68 37,64 34,62L34,50Z"/>
       <path d="M34,46L48,42 44,36Z"/>`,
    // Brachiosaurus: tall neck reaching upper-right, small head, large body, four legs
    "Brachiosaurus":
      `<path d="M4,60C8,54 13,52 18,52C21,52 24,52 27,51C29,47 32,41 35,35C38,29 42,22 46,16C48,12 51,9 55,9C58,9 60,11 59,15C58,19 55,22 52,23L49,24L52,24C50,26 47,28 45,31C43,34 41,38 39,43C37,48 35,53 34,57L40,59 46,63 46,67 40,67 40,63 34,63 32,67 24,67 24,61 20,67 12,67 12,61C9,61 6,61 4,60Z"/>`,
    // Ankylosaurus: low armored quadruped — wide flat body, small head, bony spikes on back, club tail
    "Ankylosaurus":
      `<path d="M6,62C2,58 2,52 6,50C8,48 12,46 16,46C14,44 14,40 18,38C22,36 28,36 32,38C36,36 42,34 48,34C54,34 60,36 64,40C68,40 72,42 74,46C78,46 80,50 78,54C76,58 72,60 68,62L60,66 52,66 52,62 48,66 40,66 40,62 36,66 28,66 28,62 24,66 16,66 16,62C12,62 8,62 6,62Z"/>
       <path d="M16,46L12,34 22,44ZM25,42L22,30 32,40ZM34,40L32,27 42,38ZM43,40L42,28 52,38ZM54,40L54,28 62,40Z"/>
       <path d="M74,50C78,46 82,48 82,54C82,58 78,62 74,60Z"/>`,
    // Diplodocus: extremely long neck (upper-right) AND long whip-tail (left), four legs
    "Diplodocus":
      `<path d="M2,40C5,36 9,34 14,34C17,34 20,35 21,37C23,33 26,28 30,23C33,18 37,13 42,10C45,8 48,9 48,12C47,16 44,19 40,21L36,22L40,22C37,24 34,27 32,30C29,33 27,37 26,40L34,42 42,46 42,50 36,50 36,46 30,46 28,50 20,50 20,44 16,50 8,50 8,44C5,44 3,42 2,40Z"/>
       <path d="M2,40C7,37 13,34 19,33L26,32 28,38 20,34Z"/>`,
    // Spinosaurus: right-facing bipedal like T-Rex but with tall neural spine sail on back
    "Spinosaurus":
      `<path d="M5,46C10,38 18,32 27,29C33,26 39,24 44,22C47,20 50,18 52,20L50,28C53,22 57,18 58,22L56,30C59,24 63,20 64,24L62,32C65,26 68,22 68,26L65,34 60,36 54,34 50,36L46,38C48,42 51,46 52,48L49,50C44,52 38,52 33,51C26,51 21,54 19,61C17,68 18,75 21,75L29,75 30,69 32,75 41,75C43,70 40,64 38,58C36,54 33,52 35,52C41,54 47,54 51,51C43,51 29,50 16,48C10,47 5,46 5,46Z"/>`,
    // Parasaurolophus: right-facing bipedal hadrosaur — duck bill, distinctive backward tube crest
    "Parasaurolophus":
      `<path d="M4,52C8,46 13,44 19,44C25,43 32,43 39,42C45,41 51,41 56,42C60,40 63,36 65,34C67,30 68,24 66,20C64,16 60,14 57,16C54,18 53,22 55,26C52,24 50,22 48,26C50,30 52,36 54,40L50,44C48,42 46,42 44,44L40,58 32,58 32,50 28,60 20,60 20,52 16,62 8,62 8,54C6,54 5,53 4,52Z"/>
       <path d="M66,20C68,16 72,11 76,9C79,8 80,11 78,15C76,19 71,24 66,26Z"/>`,
    // Pachycephalosaurus: right-facing bipedal — very prominent bone dome on top of skull
    "Pachycephalosaurus":
      `<path d="M4,52C8,46 13,44 19,44C25,43 31,43 37,44L43,46C45,42 47,37 48,32C49,27 49,22 47,18C45,14 41,12 38,14C35,16 34,20 35,26C32,22 29,20 28,24C28,28 30,32 33,36L37,44C39,48 40,52 39,56L35,64 27,64 27,56 24,64 16,64 16,56 12,64 4,64 8,56C7,54 5,53 4,52Z"/>
       <path d="M35,26C35,20 37,14 41,12C46,10 50,14 49,20C48,26 44,30 38,32Z"/>`,
    // Raptor (Deinonychus): right-facing bipedal — larger than Velociraptor, prominent sickle claw raised
    "Raptor":
      `<path d="M4,40C8,33 15,28 22,26C25,25 28,25 30,23L35,15C37,13 40,14 39,18L34,23C38,21 42,20 42,24C41,28 38,30 35,32L32,34C34,38 37,43 36,47L31,52C28,54 24,54 20,54C17,52 15,49 16,45C13,47 9,48 6,47Z"/>
       <path d="M4,40C9,37 16,33 22,31L30,29 32,36 24,28Z"/>
       <path d="M21,54L23,66 27,72C30,72 30,68 27,66L27,54Z"/>
       <path d="M29,52L31,64 35,70C38,70 38,66 35,64L35,52Z"/>
       <path d="M36,47L50,43 46,37Z"/>`,
    // Maiasaura: right-facing hadrosaur — large duck-billed head, broad body, no crest (distinguishes from Parasaurolophus)
    "Maiasaura":
      `<path d="M4,54C8,48 13,46 19,46C25,45 32,45 40,44C46,43 52,43 57,44C61,44 64,42 66,40C68,38 70,34 68,30C66,26 62,24 58,26C55,28 54,32 57,36C53,34 50,32 50,36C50,40 53,44 57,46L52,48C48,46 44,46 40,48L36,62 28,62 28,54 24,64 16,64 16,56 12,64 4,64 8,56C7,54 5,54 4,54Z"/>
       <path d="M66,40C68,36 72,32 76,32C78,32 79,35 77,38C75,41 71,44 68,44Z"/>`,
  };
  const body = shapes[name] ?? `<ellipse cx="40" cy="50" rx="28" ry="18"/>`;
  return `<svg viewBox="0 0 80 80" xmlns="http://www.w3.org/2000/svg">${grad}<g fill="${color}">${body}</g><g fill="url(#${gid})">${body}</g></svg>`;
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
        <div class="dino-avatar" style="background:${avatarColor(d.name)}">${dinoSVG(d.name, avatarColor(d.name))}</div>
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
