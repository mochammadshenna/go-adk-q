/**
 * Archipelago Hotels Dashboard — MCP App UI v2
 *
 * Premium hotel discovery interface for Claude Desktop.
 * Brand-themed gradient photography, live rate badges, room detail overlay.
 *
 * Protocol: @modelcontextprotocol/ext-apps
 */

import {
  App,
  applyDocumentTheme,
  applyHostFonts,
  applyHostStyleVariables,
} from "@modelcontextprotocol/ext-apps";

// ── Types ──────────────────────────────────────────────────────────────────

interface HotelSummary {
  id: string;
  name: string;
  brand: string;
  city: string;
  country: string;
  rating: number;
  stars: number;
  priceFrom: number;
  basePriceFrom?: number;
  currency: string;
  imageStyle: string;
  brandColor?: string;
  thumbnail?: string;
  tags?: string[];
  latitude?: number;
  longitude?: number;
}

interface RoomType {
  name: string;
  pricePerNight: number;
  baseRate?: number;
  currency: string;
  maxGuests: number;
  rateSource: string;
  roomImage?: string;
}

interface HotelDetail {
  id: string;
  name: string;
  brand: string;
  city: string;
  country: string;
  address: string;
  rating: number;
  stars: number;
  latitude: number;
  longitude: number;
  currency: string;
  imageStyle: string;
  brandColor?: string;
  thumbnail?: string;
  roomTypes?: RoomType[];
  startingPrice?: number;
  startingBasePrice?: number;
  bookingUrl?: string;
}

interface DashboardData {
  filter: string;
  hotels: HotelSummary[];
  total: number;
  match: number;
  message: string;
  sortBy?: string;
  city?: string;
  destination?: string;
}

interface ToolInputParams {
  arguments?: Record<string, unknown>;
  structuredContent?: DashboardData;
}

// ── State ───────────────────────────────────────────────────────────────────

interface State {
  allHotels: HotelSummary[];
  searchQuery: string;
  cityFilter: string;
  brandFilter: string;
  sortBy: string;
}

const state: State = {
  allHotels: [],
  searchQuery: "",
  cityFilter: "",
  brandFilter: "",
  sortBy: "",
};

let appRef: App;

// ── Brand theme ──────────────────────────────────────────────────────────────

type BrandTheme = {
  badge: string;
  badgeText: string;
  gradFrom: string;
  gradMid: string;
  gradTo: string;
  accent: string;
  highlight: string;
};

const BRAND_THEMES: Record<string, BrandTheme> = {
  Aston: {
    badge: "#fef3c7", badgeText: "#92400e",
    gradFrom: "#92400e", gradMid: "#78350f", gradTo: "#1c0a00",
    accent: "#f59e0b", highlight: "rgba(253,230,138,0.15)",
  },
  "Grand Aston": {
    badge: "#fef9c3", badgeText: "#713f12",
    gradFrom: "#a16207", gradMid: "#854d0e", gradTo: "#1c0d00",
    accent: "#eab308", highlight: "rgba(254,249,195,0.12)",
  },
  "Aston Collection": {
    badge: "#fef3c7", badgeText: "#7c2d12",
    gradFrom: "#b45309", gradMid: "#92400e", gradTo: "#1c0800",
    accent: "#f97316", highlight: "rgba(254,215,170,0.12)",
  },
  "The Alana": {
    badge: "#fce7f3", badgeText: "#831843",
    gradFrom: "#9d174d", gradMid: "#7f1d6e", gradTo: "#1a0822",
    accent: "#ec4899", highlight: "rgba(251,207,232,0.12)",
  },
  favehotels: {
    badge: "#ffedd5", badgeText: "#7c2d12",
    gradFrom: "#c2410c", gradMid: "#9a3412", gradTo: "#1a0800",
    accent: "#f97316", highlight: "rgba(253,186,116,0.12)",
  },
  "Hotel Neo": {
    badge: "#cffafe", badgeText: "#164e63",
    gradFrom: "#0e7490", gradMid: "#155e75", gradTo: "#042f3c",
    accent: "#22d3ee", highlight: "rgba(103,232,249,0.1)",
  },
  Neo: {
    badge: "#cffafe", badgeText: "#164e63",
    gradFrom: "#0e7490", gradMid: "#155e75", gradTo: "#042f3c",
    accent: "#22d3ee", highlight: "rgba(103,232,249,0.1)",
  },
  "Kamuela Villas": {
    badge: "#d1fae5", badgeText: "#064e3b",
    gradFrom: "#047857", gradMid: "#065f46", gradTo: "#022c1a",
    accent: "#10b981", highlight: "rgba(110,231,183,0.1)",
  },
  Kamuela: {
    badge: "#d1fae5", badgeText: "#064e3b",
    gradFrom: "#047857", gradMid: "#065f46", gradTo: "#022c1a",
    accent: "#10b981", highlight: "rgba(110,231,183,0.1)",
  },
  "Quest Hotels": {
    badge: "#e0e7ff", badgeText: "#1e1b4b",
    gradFrom: "#4338ca", gradMid: "#3730a3", gradTo: "#1e1b4b",
    accent: "#818cf8", highlight: "rgba(165,180,252,0.1)",
  },
  Quest: {
    badge: "#e0e7ff", badgeText: "#1e1b4b",
    gradFrom: "#4338ca", gradMid: "#3730a3", gradTo: "#1e1b4b",
    accent: "#818cf8", highlight: "rgba(165,180,252,0.1)",
  },
  Harper: {
    badge: "#ede9fe", badgeText: "#2e1065",
    gradFrom: "#6d28d9", gradMid: "#5b21b6", gradTo: "#1e0a3c",
    accent: "#a78bfa", highlight: "rgba(196,181,253,0.1)",
  },
  Nordic: {
    badge: "#dbeafe", badgeText: "#1e3a5f",
    gradFrom: "#1d4ed8", gradMid: "#1e40af", gradTo: "#0f172a",
    accent: "#60a5fa", highlight: "rgba(147,197,253,0.1)",
  },
  Huxley: {
    badge: "#e5e7eb", badgeText: "#111827",
    gradFrom: "#374151", gradMid: "#1f2937", gradTo: "#030712",
    accent: "#9ca3af", highlight: "rgba(209,213,219,0.08)",
  },
  Avanika: {
    badge: "#fdf2f8", badgeText: "#701a75",
    gradFrom: "#a21caf", gradMid: "#86198f", gradTo: "#1a0022",
    accent: "#e879f9", highlight: "rgba(240,171,252,0.1)",
  },
  "Four Corners": {
    badge: "#ccfbf1", badgeText: "#134e4a",
    gradFrom: "#0f766e", gradMid: "#0d9488", gradTo: "#022c22",
    accent: "#2dd4bf", highlight: "rgba(94,234,212,0.1)",
  },
  "Powered By Archi": {
    badge: "#f1f5f9", badgeText: "#0f172a",
    gradFrom: "#475569", gradMid: "#334155", gradTo: "#0f172a",
    accent: "#94a3b8", highlight: "rgba(148,163,184,0.08)",
  },
  Nomad: {
    badge: "#f5f5f4", badgeText: "#1c1917",
    gradFrom: "#78716c", gradMid: "#57534e", gradTo: "#1c1917",
    accent: "#a8a29e", highlight: "rgba(168,162,158,0.08)",
  },
};

const DEFAULT_THEME: BrandTheme = {
  badge: "#e0e7ff", badgeText: "#1e1b4b",
  gradFrom: "#4f46e5", gradMid: "#4338ca", gradTo: "#1e1b4b",
  accent: "#818cf8", highlight: "rgba(165,180,252,0.1)",
};

function hexToTheme(hex: string): BrandTheme {
  const r = parseInt(hex.slice(1, 3), 16) || 0;
  const g = parseInt(hex.slice(3, 5), 16) || 0;
  const b = parseInt(hex.slice(5, 7), 16) || 0;
  // Gradient: brand-tinted-light → brand color → near-black (top-to-bottom photo style)
  const mix = (c: number) => Math.round(c * 0.45 + 255 * 0.55);
  const gradFrom = `rgb(${mix(r)},${mix(g)},${mix(b)})`;
  const gradTo   = `rgb(${Math.round(r*0.08)},${Math.round(g*0.08)},${Math.round(b*0.08)})`;
  const pastel = (c: number) => Math.round(c * 0.15 + 255 * 0.85);
  return {
    badge: `rgb(${pastel(r)},${pastel(g)},${pastel(b)})`,
    badgeText: hex,
    gradFrom,
    gradMid: hex,
    gradTo,
    accent: hex,
    highlight: `rgba(${r},${g},${b},0.1)`,
  };
}

function brandTheme(brand: string, brandColor?: string): BrandTheme {
  if (brandColor && /^#[0-9a-fA-F]{6}$/.test(brandColor)) return hexToTheme(brandColor);
  return BRAND_THEMES[brand] ?? DEFAULT_THEME;
}

// ── Helpers ─────────────────────────────────────────────────────────────────

function esc(s: string): string {
  return String(s)
    .replace(/&/g, "&amp;")
    .replace(/</g, "&lt;")
    .replace(/>/g, "&gt;")
    .replace(/"/g, "&quot;")
    .replace(/'/g, "&#x27;");
}

function fmtPrice(v: number, currency: string): string {
  if (v <= 0 || !currency) return "";
  const locale = currency.toUpperCase() === "IDR" ? "id-ID" : "en-US";
  return currency + " " + Math.round(v).toLocaleString(locale);
}

// ponytail: fmtPriceShort kept as alias — all call sites use full locale format now
const fmtPriceShort = fmtPrice;
const fmtPriceFull  = fmtPrice;

function deriveStars(rating: number, stars: number): number {
  if (stars > 0) return stars;
  if (rating >= 9.0) return 5;
  if (rating >= 8.0) return 4;
  if (rating >= 7.0) return 3;
  if (rating >= 6.0) return 2;
  return rating > 0 ? 1 : 0;
}

function starsHtml(n: number): string {
  if (n <= 0) return "";
  const filled = Math.min(5, Math.round(n));
  const empty = 5 - filled;
  return (
    '<span class="stars-filled">' + "★".repeat(filled) + "</span>" +
    '<span class="stars-empty">' + "★".repeat(empty) + "</span>"
  );
}

function ratingBg(r: number): string {
  if (r >= 8.5) return "#059669";
  if (r >= 7.5) return "#0891b2";
  if (r >= 6.0) return "#d97706";
  return "#dc2626";
}

function deriveBedType(name: string): string {
  const n = name.toLowerCase();
  if (n.includes("twin")) return "Twin Beds";
  if (n.includes("king")) return "King Bed";
  if (n.includes("queen")) return "Queen Bed";
  if (n.includes("double")) return "Double Bed";
  if (n.includes("suite") || n.includes("villa") || n.includes("pent")) return "King Bed";
  if (n.includes("family") || n.includes("connect")) return "Twin Beds";
  if (n.includes("single")) return "Single Bed";
  return "Queen Bed";
}

function deriveAmenities(roomName: string, brand: string): string[] {
  const n = roomName.toLowerCase();
  const b = brand.toLowerCase();
  const amenities = ["Free WiFi", "AC"];
  if (n.includes("suite") || n.includes("villa") || n.includes("premium") || n.includes("grand") || n.includes("deluxe")) {
    amenities.push("Bathtub");
  }
  if (n.includes("pool") || b.includes("kamuela") || b.includes("alana")) {
    amenities.push("Pool Access");
  }
  if (n.includes("sea") || n.includes("ocean") || n.includes("bay") || n.includes("view")) {
    amenities.push("Sea View");
  } else if (n.includes("city") || n.includes("garden")) {
    amenities.push("City View");
  }
  if (n.includes("exec") || n.includes("business") || b.includes("aston") || b.includes("quest")) {
    amenities.push("Work Desk");
  }
  amenities.push("Breakfast Option");
  return amenities.slice(0, 4);
}

function rateSourceBadge(src: string): string {
  const s = src.toLowerCase();
  if (s === "simplebooking" || s === "live") {
    return '<span class="rate-badge rate-live">● Live</span>';
  }
  if (s === "stored") {
    return '<span class="rate-badge rate-stored">● Stored</span>';
  }
  return '<span class="rate-badge rate-fallback">◐ Estimate</span>';
}

function unique<T>(a: T[]): T[] {
  return [...new Set(a)];
}

function setText(id: string, v: string | number): void {
  const el = document.getElementById(id);
  if (el) el.textContent = String(v);
}

function hide(id: string): void {
  document.getElementById(id)?.classList.add("hidden");
}
function show(id: string): void {
  document.getElementById(id)?.classList.remove("hidden");
}

// ── Photo gradient art ────────────────────────────────────────────────────────

function buildPhotoGradient(theme: BrandTheme): string {
  const { gradFrom, gradMid, gradTo, highlight } = theme;
  return [
    `radial-gradient(ellipse 80% 60% at 15% -5%, ${highlight} 0%, transparent 55%)`,
    `radial-gradient(ellipse 50% 70% at 90% 110%, rgba(0,0,0,0.5) 0%, transparent 55%)`,
    `radial-gradient(ellipse 40% 40% at 75% 20%, rgba(255,255,255,0.04) 0%, transparent 50%)`,
    `linear-gradient(145deg, ${gradFrom} 0%, ${gradMid} 45%, ${gradTo} 100%)`,
  ].join(", ");
}

// ── CSS ───────────────────────────────────────────────────────────────────────

const STYLES = `
  *, *::before, *::after { margin: 0; padding: 0; box-sizing: border-box; }

  :root {
    --bg-0: #f8fafc;
    --bg-1: #ffffff;
    --bg-2: #f1f5f9;
    --bg-3: #e2e8f0;
    --text-1: #0f172a;
    --text-2: #475569;
    --text-3: #94a3b8;
    --border: rgba(0,0,0,0.08);
    --border-h: rgba(0,0,0,0.16);
    --gold: #00215B;
    --gold-dim: rgba(0,33,91,0.08);
    --gold-border: rgba(0,33,91,0.2);
    --r-sm: 8px;
    --r-md: 12px;
    --r-lg: 16px;
    --shadow: 0 1px 8px rgba(0,0,0,0.08);
    --shadow-h: 0 6px 24px rgba(0,0,0,0.14);
    --t: 200ms ease;
  }

  body {
    font-family: 'Zen Kaku Gothic New', var(--font-sans, system-ui, sans-serif);
    background: var(--color-background-primary, var(--bg-0));
    color: var(--color-text-primary, var(--text-1));
    line-height: 1.5;
    font-size: 14px;
    -webkit-font-smoothing: antialiased;
  }

  #app { padding: 10px; max-width: 1400px; margin: 0 auto; }

  @keyframes fadeUp {
    from { opacity: 0; transform: translateY(16px); }
    to   { opacity: 1; transform: translateY(0); }
  }
  @keyframes pulse { 0%,100% { opacity:1; } 50% { opacity:0.4; } }
  @keyframes shimmer {
    0%   { background-position: -400px 0; }
    100% { background-position:  400px 0; }
  }
  @keyframes slideInRight {
    from { transform: translateX(100%); opacity: 0; }
    to   { transform: translateX(0);   opacity: 1; }
  }
  @keyframes scaleIn {
    from { transform: scale(0.94) translateY(12px); opacity: 0; }
    to   { transform: scale(1)    translateY(0);    opacity: 1; }
  }
  @keyframes backdropIn { from { opacity:0; } to { opacity:1; } }

  .hidden { display: none !important; }

  /* ── Loading ── */
  #loading-state {
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    min-height: 60vh; gap: 16px; text-align: center; padding: 32px;
  }
  .loading-emoji { font-size: 56px; animation: pulse 2s ease-in-out infinite; }
  .loading-title { font-size: 18px; font-weight: 700; letter-spacing: -0.02em; }
  .loading-sub { font-size: 13px; color: var(--text-2); }
  .skeleton-grid {
    display: grid; grid-template-columns: repeat(3,1fr); gap: 8px;
    width: 100%; max-width: 900px; margin-top: 20px;
  }
  .skeleton-card { background: var(--bg-1); border: 1px solid var(--border); border-radius: var(--r-md); overflow: hidden; }
  .skeleton-photo {
    height: 180px;
    background: linear-gradient(90deg, var(--bg-2) 25%, var(--bg-3) 50%, var(--bg-2) 75%);
    background-size: 400px 100%;
    animation: shimmer 1.5s infinite;
  }
  .skeleton-body { padding: 12px; display: flex; flex-direction: column; gap: 8px; }
  .skeleton-line {
    border-radius: 4px; height: 12px;
    background: linear-gradient(90deg, var(--bg-2) 25%, var(--bg-3) 50%, var(--bg-2) 75%);
    background-size: 400px 100%;
    animation: shimmer 1.5s infinite;
  }

  /* ── Error ── */
  #error-state {
    display: flex; flex-direction: column; align-items: center; justify-content: center;
    min-height: 60vh; gap: 12px; text-align: center; padding: 32px;
  }

  /* ── Header ── */
  .app-header {
    display: flex; align-items: center; justify-content: space-between;
    padding: 14px 18px; margin-bottom: 8px;
    background: var(--bg-1); border: 1px solid var(--border);
    border-radius: var(--r-lg); flex-wrap: wrap; gap: 12px;
  }
  .header-brand { display: flex; align-items: center; gap: 12px; }
  .header-icon {
    width: 40px; height: 40px; border-radius: 10px; flex-shrink: 0;
    object-fit: cover; display: block;
  }
  .header-name { font-size: 17px; font-weight: 700; letter-spacing: -0.02em; line-height: 1.2; }
  .header-sub { font-size: 12px; color: var(--text-2); margin-top: 1px; }
  .header-count {
    font-size: 13px; font-weight: 600; color: var(--gold);
    background: var(--gold-dim); border: 1px solid var(--gold-border);
    padding: 5px 14px; border-radius: 20px;
  }

  /* ── Stats ── */
  .stats-row {
    display: grid; grid-template-columns: repeat(5, 1fr); gap: 6px; margin-bottom: 10px;
  }
  .stat-card {
    background: var(--bg-1); border: 1px solid var(--border);
    border-radius: var(--r-sm); padding: 10px 8px; text-align: center;
  }
  .stat-val { display: block; font-size: 17px; font-weight: 700; color: var(--gold); letter-spacing: -0.02em; }
  .stat-lbl { display: block; font-size: 9px; color: var(--text-3); text-transform: uppercase; letter-spacing: 0.06em; margin-top: 2px; }

  /* ── Filter bar ── */
  .filter-bar { display: flex; gap: 6px; flex-wrap: wrap; margin-bottom: 12px; align-items: center; }
  .search-wrap { flex: 2; min-width: 160px; position: relative; }
  .search-icon {
    position: absolute; left: 10px; top: 50%; transform: translateY(-50%);
    width: 14px; height: 14px; color: var(--text-3); pointer-events: none;
  }
  .search-input {
    width: 100%; padding: 8px 12px 8px 32px;
    font-family: inherit; font-size: 13px;
    background: var(--bg-2); color: var(--text-1);
    border: 1px solid var(--border); border-radius: var(--r-sm);
    outline: none; transition: border-color var(--t);
  }
  .search-input::placeholder { color: var(--text-3); }
  .search-input:focus { border-color: var(--gold); }
  .filter-sel {
    flex: 1; min-width: 110px; padding: 8px 28px 8px 10px;
    font-family: inherit; font-size: 12px;
    background: var(--bg-2); color: var(--text-1);
    border: 1px solid var(--border); border-radius: var(--r-sm);
    cursor: pointer; outline: none; transition: border-color var(--t);
    appearance: none;
    background-image: url("data:image/svg+xml,%3Csvg xmlns='http://www.w3.org/2000/svg' width='12' height='12' viewBox='0 0 24 24' fill='none' stroke='%236e6b8a' stroke-width='2'%3E%3Cpath d='M6 9l6 6 6-6'/%3E%3C/svg%3E");
    background-repeat: no-repeat; background-position: right 8px center;
  }
  .filter-sel:focus { border-color: var(--gold); }

  /* ── Hotel grid ── */
  .hotel-grid { display: grid; grid-template-columns: repeat(auto-fill, minmax(290px, 1fr)); gap: 10px; }

  /* ── Hotel card ── */
  .hotel-card {
    background: var(--bg-1); border: 1px solid var(--border);
    border-radius: var(--r-md); overflow: hidden; cursor: pointer;
    transition: transform var(--t), box-shadow var(--t), border-color var(--t);
    box-shadow: var(--shadow);
    animation: fadeUp 0.35s ease both;
    display: flex; flex-direction: column;
  }
  .hotel-card:hover { transform: translateY(-4px); box-shadow: var(--shadow-h); border-color: var(--border-h); }
  .hotel-card:active { transform: translateY(-2px); }

  /* ── Card photo ── */
  .card-photo { position: relative; height: 180px; overflow: hidden; flex-shrink: 0; }
  .card-photo-bg {
    position: absolute; inset: 0;
    transition: transform 0.5s ease;
  }
  .hotel-card:hover .card-photo-bg { transform: scale(1.04); }
  /* Real photo thumbnail overlay */
  .card-photo-thumb {
    position: absolute; inset: 0; width: 100%; height: 100%;
    object-fit: cover; object-position: center top; z-index: 0;
    transition: transform 0.5s ease;
  }
  .hotel-card:hover .card-photo-thumb { transform: scale(1.04); }
  /* Suppress light-shaft effects when real photo present */
  .card-photo-bg:has(.card-photo-thumb)::before,
  .card-photo-bg:has(.card-photo-thumb)::after { display: none; }
  /* Window/light-shaft layers */
  .card-photo-bg::before {
    content: '';
    position: absolute; top: 10%; right: 18%; width: 22%; height: 55%;
    background: linear-gradient(180deg, rgba(255,255,255,0.06) 0%, rgba(255,255,255,0.02) 60%, transparent 100%);
    border-left: 1px solid rgba(255,255,255,0.05);
    border-right: 1px solid rgba(255,255,255,0.05);
    border-top: 1px solid rgba(255,255,255,0.07);
  }
  .card-photo-bg::after {
    content: '';
    position: absolute; top: 12%; right: 42%; width: 14%; height: 38%;
    background: linear-gradient(180deg, rgba(255,255,255,0.04) 0%, transparent 100%);
    border-left: 1px solid rgba(255,255,255,0.04);
    border-top: 1px solid rgba(255,255,255,0.05);
  }
  /* Text readability gradient */
  .card-photo::after {
    content: ''; position: absolute; inset: 0;
    background: linear-gradient(180deg, transparent 0%, rgba(0,0,0,0.15) 40%, rgba(0,0,0,0.65) 100%);
    pointer-events: none;
  }
  .card-photo-content {
    position: absolute; inset: 0; z-index: 2;
    display: flex; flex-direction: column; justify-content: space-between;
    padding: 10px 12px;
  }
  .card-photo-top { display: flex; justify-content: space-between; align-items: flex-start; }
  .card-photo-bottom { display: flex; flex-direction: column; }

  .brand-badge {
    font-size: 10px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 0.06em; padding: 3px 9px; border-radius: 6px;
    white-space: nowrap; backdrop-filter: blur(4px);
  }
  .rating-chip {
    display: flex; flex-direction: column; align-items: center;
    padding: 4px 8px; border-radius: 8px; min-width: 44px;
    backdrop-filter: blur(8px);
    background: rgba(0,0,0,0.4); border: 1px solid rgba(255,255,255,0.12);
  }
  .rating-num { font-size: 15px; font-weight: 800; line-height: 1; color: #fff; }
  .rating-label { font-size: 8px; text-transform: uppercase; letter-spacing: 0.05em; opacity: 0.7; }

  .card-photo-name {
    font-size: 16px; font-weight: 700; color: #fff; line-height: 1.2;
    text-shadow: 0 1px 6px rgba(0,0,0,0.5);
    text-wrap: balance;
  }
  .card-photo-stars { margin-top: 3px; font-size: 12px; line-height: 1; }
  .stars-filled { color: #fbbf24; }
  .stars-empty  { color: rgba(255,255,255,0.25); }

  /* ── Header icon ── */
  .header-icon-a {
    background: #00215B; color: #fff; width: 40px; height: 40px; border-radius: 10px;
    display: flex; align-items: center; justify-content: center;
    font-size: 20px; font-weight: 900; flex-shrink: 0;
  }

  /* ── Card body ── */
  .card-body { padding: 10px 12px; display: flex; flex-direction: column; gap: 6px; }
  .card-location { display: flex; align-items: flex-start; justify-content: flex-end; gap: 5px; font-size: 12px; color: var(--text-2); }
  .card-location svg { width: 12px; height: 12px; flex-shrink: 0; margin-top: 1px; }
  .card-location-text { line-height: 1.4; text-align: right; }

  /* ── Card footer ── */
  .card-footer {
    display: flex; align-items: center; justify-content: space-between;
    padding: 10px 12px; gap: 8px;
  }
  .price-block { display: flex; flex-direction: column; gap: 2px; }
  .price-from { font-size: 11px; color: var(--text-2); text-transform: uppercase; letter-spacing: 0.06em; font-weight: 600; }
  .price-row { display: flex; align-items: baseline; gap: 3px; }
  .price-val { font-size: 24px; font-weight: 800; letter-spacing: -0.02em; line-height: 1; }
  .price-night { font-size: 11px; color: var(--text-3); }
  .price-na { font-size: 11px; color: var(--text-3); font-style: italic; }

  .btn-rooms {
    font-family: inherit; font-size: 12px; font-weight: 600;
    padding: 8px 16px; border: none; border-radius: var(--r-sm);
    color: #fff; cursor: pointer; white-space: nowrap;
    transition: opacity var(--t), transform var(--t);
    flex-shrink: 0;
  }
  .btn-rooms:hover { opacity: 0.88; transform: scale(1.03); }
  .btn-rooms:active { transform: scale(0.98); }

  /* ── Empty state ── */
  .empty-grid {
    grid-column: 1 / -1;
    display: flex; flex-direction: column; align-items: center;
    padding: 64px 16px; gap: 10px; text-align: center;
  }
  .empty-icon { font-size: 48px; }
  .empty-title { font-size: 15px; font-weight: 600; }
  .empty-sub { font-size: 13px; color: var(--text-2); }

  /* ── Overlay ── */
  #room-overlay {
    position: fixed; inset: 0; z-index: 100;
  }
  .overlay-backdrop {
    position: absolute; inset: 0; background: rgba(0,0,0,0.6);
    animation: backdropIn 0.2s ease; cursor: pointer;
  }
  .overlay-sheet {
    position: absolute; z-index: 1; width: min(520px, calc(100vw - 32px));
    max-height: min(88vh, 860px);
    top: 50%; left: 50%; transform: translate(-50%, -50%);
    background: var(--bg-1); border-radius: 18px; border: 1px solid var(--border);
    box-shadow: 0 32px 80px rgba(0,0,0,0.22);
    overflow-y: auto; overflow-x: hidden;
    animation: scaleIn 0.28s cubic-bezier(0.16, 1, 0.3, 1);
    display: flex; flex-direction: column;
  }
  .overlay-close {
    position: absolute; top: 14px; right: 14px; z-index: 10;
    width: 32px; height: 32px; border-radius: 50%;
    background: var(--bg-2); border: 1px solid var(--border);
    color: var(--text-2); font-size: 16px; cursor: pointer;
    display: flex; align-items: center; justify-content: center;
    transition: background var(--t), color var(--t);
  }
  .overlay-close:hover { background: var(--bg-3); color: var(--text-1); }

  /* Overlay hero */
  .overlay-hero { position: relative; height: 200px; flex-shrink: 0; overflow: hidden; }
  .overlay-hero-bg { position: absolute; inset: 0; }
  .overlay-hero-thumb { position: absolute; inset: 0; width: 100%; height: 100%; object-fit: cover; object-position: center top; }
  .overlay-hero::after {
    content: ''; position: absolute; inset: 0;
    background: linear-gradient(180deg, rgba(0,0,0,0.1) 0%, rgba(0,0,0,0.7) 100%);
    pointer-events: none;
  }
  .overlay-hero-top { position: absolute; top: 12px; left: 14px; z-index: 2; }
  .overlay-hero-content { position: absolute; bottom: 16px; left: 16px; right: 16px; z-index: 2; }
  .overlay-hotel-name {
    font-size: 20px; font-weight: 800; color: #fff; line-height: 1.2;
    text-shadow: 0 2px 8px rgba(0,0,0,0.4); text-wrap: balance;
  }
  .overlay-hotel-meta { display: flex; align-items: center; gap: 8px; margin-top: 4px; flex-wrap: wrap; }
  .book-now-btn { display: inline-block; background: #00215b; color: #fff; padding: 12px 32px; border-radius: 6px; font-weight: 700; font-size: 15px; text-decoration: none; white-space: nowrap; text-transform: uppercase; letter-spacing: 0.08em; cursor: pointer; }
  .book-now-btn:hover { background: #003080; }
  .book-now-wrap { padding: 16px 20px 0; }
  .overlay-rating-badge { font-size: 13px; font-weight: 700; color: #fff; padding: 2px 8px; border-radius: 6px; }
  .overlay-stars { font-size: 12px; }

  /* Overlay body */
  .overlay-body { padding: 16px; flex: 1; }
  .overlay-address {
    display: flex; align-items: flex-start; gap: 6px;
    font-size: 12px; color: var(--text-2); margin-bottom: 14px; line-height: 1.4;
  }
  .overlay-address svg { width: 12px; height: 12px; flex-shrink: 0; margin-top: 1px; }

  .section-title {
    font-size: 11px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 0.08em; color: var(--text-3); margin-bottom: 10px;
  }

  /* ── Room cards ── */
  .room-card {
    background: var(--bg-2); border: 1px solid var(--border);
    border-radius: var(--r-sm); padding: 12px;
    margin-bottom: 8px; transition: border-color var(--t);
  }
  .room-card:hover { border-color: var(--border-h); }
  .room-card:last-child { margin-bottom: 0; }
  .room-card-img { width: 100%; height: 120px; object-fit: cover; border-radius: 4px; margin-bottom: 8px; display: block; }

  .room-card-header { display: flex; align-items: flex-start; justify-content: space-between; gap: 8px; }
  .room-name { font-size: 14px; font-weight: 700; line-height: 1.2; }
  .room-price-block { text-align: right; flex-shrink: 0; display: flex; flex-direction: column; align-items: flex-end; gap: 2px; }
  .room-price-row { display: flex; align-items: baseline; gap: 4px; }
  .room-price { font-size: 16px; font-weight: 800; letter-spacing: -0.02em; }
  .room-price-original { font-size: 12px; color: var(--text-3); text-decoration: line-through; text-decoration-color: #e53e3e; }
  .price-from-original { font-size: 11px; color: var(--text-3); text-decoration: line-through; text-decoration-color: #e53e3e; line-height: 1.2; }
  .starting-price-original { font-size: 13px; color: var(--text-3); text-decoration: line-through; text-decoration-color: #e53e3e; line-height: 1.3; }
  .room-price-night { font-size: 10px; color: var(--text-3); }

  .room-details { display: flex; align-items: center; gap: 6px; margin-top: 8px; flex-wrap: wrap; }
  .room-detail-item { display: flex; align-items: center; gap: 4px; font-size: 11px; color: var(--text-2); }

  .room-amenities { display: flex; gap: 4px; flex-wrap: wrap; margin-top: 8px; }
  .amenity-chip {
    font-size: 10px; padding: 2px 7px; border-radius: 20px;
    background: var(--bg-3); color: var(--text-2); border: 1px solid var(--border);
  }

  /* Rate source badges */
  .rate-badge {
    font-size: 9px; font-weight: 700; text-transform: uppercase;
    letter-spacing: 0.06em; padding: 2px 7px; border-radius: 4px;
    margin-top: 4px; display: inline-block;
  }
  .rate-live     { background: rgba(16,185,129,0.15); color: #10b981; }
  .rate-stored   { background: rgba(14,165,233,0.15); color: #0ea5e9; }
  .rate-fallback { background: rgba(245,158,11,0.15); color: #f59e0b; }

  /* Starting price callout */
  .starting-callout {
    display: flex; align-items: center; justify-content: space-between;
    margin-bottom: 14px; gap: 12px;
  }
  .starting-callout-left { display: flex; flex-direction: column; gap: 1px; }
  .starting-callout-label { font-size: 14px; color: var(--text-2); }
  .starting-callout-price { font-size: 26px; font-weight: 800; color: #2BC14B; }
  .book-now-wrap { padding: 16px 20px 0; }

  /* No rooms */
  .no-rooms {
    display: flex; flex-direction: column; align-items: center;
    padding: 32px 16px; gap: 8px; text-align: center;
    color: var(--text-2); font-size: 13px;
  }
  .no-rooms-icon { font-size: 32px; margin-bottom: 4px; }

  /* ── Responsive ── */
  @media (max-width: 500px) {
    .stats-row { grid-template-columns: repeat(3, 1fr); }
    .stats-row .stat-card:nth-child(4),
    .stats-row .stat-card:nth-child(5) { display: none; }
    .hotel-grid { grid-template-columns: 1fr; }
    .overlay-sheet {
      top: auto !important; left: 0 !important; right: 0 !important; bottom: 0 !important;
      transform: none !important; width: 100% !important; max-height: 90vh;
      border-radius: 18px 18px 0 0; border-bottom: none;
      animation: none;
    }
  }

  /* ── Reduced motion ── */
  @media (prefers-reduced-motion: reduce) {
    .hotel-card { animation: none; }
    .hotel-card:hover { transform: none; }
    .overlay-sheet { animation: none; }
    .overlay-backdrop { animation: none; }
    .skeleton-photo, .skeleton-line { animation: none; }
    .card-photo-bg { transition: none; }
  }
`;

function injectStyles(): void {
  const link = document.createElement("link");
  link.rel = "stylesheet";
  link.href = "https://fonts.googleapis.com/css2?family=Zen+Kaku+Gothic+New:wght@400;500;700;900&display=swap";
  document.head.appendChild(link);
  const s = document.createElement("style");
  s.textContent = STYLES;
  document.head.appendChild(s);
}

// ── DOM builder ──────────────────────────────────────────────────────────────

function buildDOM(): void {
  const root = document.getElementById("root");
  if (!root) return;

  root.innerHTML = `
  <div id="app">

    <div id="loading-state">
      <div class="loading-emoji"><svg width="56" height="56" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect width="16" height="20" x="4" y="2" rx="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01"/><path d="M16 6h.01"/><path d="M12 6h.01"/><path d="M12 10h.01"/><path d="M16 10h.01"/><path d="M8 10h.01"/><path d="M12 14h.01"/><path d="M16 14h.01"/><path d="M8 14h.01"/></svg></div>
      <div class="loading-title">Archipelago Hotels</div>
      <div class="loading-sub">Discovering properties across Indonesia…</div>
      <div class="skeleton-grid">
        <div class="skeleton-card"><div class="skeleton-photo"></div><div class="skeleton-body"><div class="skeleton-line" style="width:75%"></div><div class="skeleton-line" style="width:40%"></div></div></div>
        <div class="skeleton-card"><div class="skeleton-photo"></div><div class="skeleton-body"><div class="skeleton-line" style="width:65%"></div><div class="skeleton-line" style="width:35%"></div></div></div>
        <div class="skeleton-card"><div class="skeleton-photo"></div><div class="skeleton-body"><div class="skeleton-line" style="width:80%"></div><div class="skeleton-line" style="width:45%"></div></div></div>
      </div>
    </div>

    <div id="error-state" class="hidden">
      <div style="font-size:48px">⚠️</div>
      <div style="font-size:15px;font-weight:600">Could Not Load Hotels</div>
      <div id="error-message" style="font-size:13px;color:var(--text-2);max-width:360px"></div>
    </div>

    <div id="dashboard-content" class="hidden">
      <div class="app-header">
        <div class="header-brand">
          <div class="header-icon-a">A</div>
          <div>
            <div class="header-name">Archipelago Hotels</div>
            <div class="header-sub" id="header-sub">Searching…</div>
          </div>
        </div>
        <div class="header-count" id="header-count">0</div>
      </div>

      <div class="stats-row">
        <div class="stat-card"><span class="stat-val" id="s-hotels">—</span><span class="stat-lbl">Hotels</span></div>
        <div class="stat-card"><span class="stat-val" id="s-brands">—</span><span class="stat-lbl">Brands</span></div>
        <div class="stat-card"><span class="stat-val" id="s-cities">—</span><span class="stat-lbl">Cities</span></div>
        <div class="stat-card"><span class="stat-val" id="s-rating">—</span><span class="stat-lbl">Avg Rating</span></div>
        <div class="stat-card"><span class="stat-val" id="s-price">—</span><span class="stat-lbl">From/Night</span></div>
      </div>

      <div class="filter-bar">
        <div class="search-wrap">
          <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <circle cx="11" cy="11" r="8"/><path d="m21 21-4.35-4.35"/>
          </svg>
          <input id="search-input" type="text" class="search-input" placeholder="Search hotels, cities…" autocomplete="off"/>
        </div>
        <select id="city-filter" class="filter-sel"><option value="">All Cities</option></select>
        <select id="brand-filter" class="filter-sel"><option value="">All Brands</option></select>
        <select id="sort-filter" class="filter-sel">
          <option value="">Recommended</option>
          <option value="price-asc">Price: Low → High</option>
          <option value="price-desc">Price: High → Low</option>
          <option value="rating">Highest Rated</option>
          <option value="stars">Star Rating</option>
        </select>
      </div>

      <div id="hotel-grid" class="hotel-grid"></div>
    </div>

    <div id="room-overlay" style="display:none">
      <div class="overlay-backdrop" id="overlay-backdrop"></div>
      <div class="overlay-sheet" id="overlay-sheet">
        <button class="overlay-close" id="overlay-close" aria-label="Close">✕</button>
        <div id="overlay-content">
          <div style="padding:80px 24px;text-align:center;color:var(--text-2)">
            <div style="margin-bottom:10px;animation:pulse 1.5s infinite"><svg width="32" height="32" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect width="16" height="20" x="4" y="2" rx="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01"/><path d="M16 6h.01"/><path d="M12 6h.01"/><path d="M12 10h.01"/><path d="M16 10h.01"/><path d="M8 10h.01"/><path d="M12 14h.01"/><path d="M16 14h.01"/><path d="M8 14h.01"/></svg></div>
            <div>Loading room details…</div>
          </div>
        </div>
      </div>
    </div>

  </div>`;

  document.getElementById("search-input")?.addEventListener("input", (e) => {
    state.searchQuery = (e.target as HTMLInputElement).value;
    applyFilters();
  });
  document.getElementById("city-filter")?.addEventListener("change", (e) => {
    state.cityFilter = (e.target as HTMLSelectElement).value;
    applyFilters();
  });
  document.getElementById("brand-filter")?.addEventListener("change", (e) => {
    state.brandFilter = (e.target as HTMLSelectElement).value;
    applyFilters();
  });
  document.getElementById("sort-filter")?.addEventListener("change", (e) => {
    state.sortBy = (e.target as HTMLSelectElement).value;
    applyFilters();
  });
  document.getElementById("overlay-backdrop")?.addEventListener("click", closeOverlay);
  document.getElementById("overlay-close")?.addEventListener("click", closeOverlay);
  document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeOverlay(); });
  document.addEventListener("click", (e) => {
    const btn = (e.target as Element).closest<HTMLAnchorElement>(".book-now-btn");
    if (btn?.href) {
      e.preventDefault();
      e.stopPropagation();
      // MCP server (outside sandbox) opens URL via exec.Command("open", url)
      appRef?.callServerTool({ name: "open_booking_url", arguments: { url: btn.href } })
        .catch(() => {
          // fallback: let Claude Desktop handle via postMessage
          window.parent.postMessage({ type: "OPEN_BOOKING_URL", url: btn.href }, "*");
        });
    }
  });
}

// ── Stats / filters ──────────────────────────────────────────────────────────

function renderStats(hotels: HotelSummary[]): void {
  const brands = unique(hotels.map(h => h.brand)).length;
  const cities = unique(hotels.map(h => h.city)).length;
  const withRating = hotels.filter(h => h.rating > 0);
  const avg = withRating.length > 0
    ? withRating.reduce((s, h) => s + h.rating, 0) / withRating.length
    : 0;
  const minHotel = hotels.reduce<HotelSummary | null>((m, h) => {
    if (h.priceFrom <= 0) return m;
    return !m || h.priceFrom < m.priceFrom ? h : m;
  }, null);

  setText("s-hotels", hotels.length);
  setText("s-brands", brands);
  setText("s-cities", cities);
  setText("s-rating", avg > 0 ? avg.toFixed(1) : "—");
  setText("s-price", minHotel ? fmtPriceShort(minHotel.priceFrom, minHotel.currency) : "—");
  setText("header-count", String(hotels.length));
  setText("header-sub",
    `${hotels.length} hotel${hotels.length !== 1 ? "s" : ""} · ${brands} brand${brands !== 1 ? "s" : ""} · ${cities} cit${cities !== 1 ? "ies" : "y"}`
  );
}

function populateFilters(hotels: HotelSummary[]): void {
  const cities = unique(hotels.map(h => h.city).filter(Boolean)).sort();
  const brands = unique(hotels.map(h => h.brand).filter(Boolean)).sort();
  const citySel = document.getElementById("city-filter") as HTMLSelectElement | null;
  const brandSel = document.getElementById("brand-filter") as HTMLSelectElement | null;
  if (citySel) {
    citySel.innerHTML = '<option value="">All Cities</option>' +
      cities.map(c => `<option value="${esc(c)}">${esc(c)}</option>`).join("");
  }
  if (brandSel) {
    brandSel.innerHTML = '<option value="">All Brands</option>' +
      brands.map(b => `<option value="${esc(b)}">${esc(b)}</option>`).join("");
  }
}

function applyFilters(): void {
  let hotels = state.allHotels;

  const q = state.searchQuery.toLowerCase().trim();
  if (q) {
    hotels = hotels.filter(h =>
      h.name.toLowerCase().includes(q) ||
      h.city.toLowerCase().includes(q) ||
      h.brand.toLowerCase().includes(q)
    );
  }
  if (state.cityFilter)  hotels = hotels.filter(h => h.city  === state.cityFilter);
  if (state.brandFilter) hotels = hotels.filter(h => h.brand === state.brandFilter);

  hotels = [...hotels];
  switch (state.sortBy) {
    case "price-asc":  hotels.sort((a, b) => (a.priceFrom || 99_999_999) - (b.priceFrom || 99_999_999)); break;
    case "price-desc": hotels.sort((a, b) => (b.priceFrom || 0) - (a.priceFrom || 0)); break;
    case "rating":     hotels.sort((a, b) => b.rating - a.rating); break;
    case "stars":      hotels.sort((a, b) => deriveStars(b.rating, b.stars) - deriveStars(a.rating, a.stars)); break;
  }

  renderHotels(hotels);
}

// ── Hotel grid ───────────────────────────────────────────────────────────────

function renderHotels(hotels: HotelSummary[]): void {
  const grid = document.getElementById("hotel-grid");
  if (!grid) return;

  renderStats(hotels);

  if (hotels.length === 0) {
    grid.innerHTML = `
      <div class="empty-grid">
        <div class="empty-icon">🔍</div>
        <div class="empty-title">No Hotels Found</div>
        <div class="empty-sub">Try adjusting your search or filters</div>
      </div>`;
    return;
  }

  grid.innerHTML = hotels.map((h, i) => {
    const theme = brandTheme(h.brand, h.brandColor);
    const stars = deriveStars(h.rating, h.stars);
    const gradient = buildPhotoGradient(theme);
    const delay = `${Math.min(i, 24) * 0.035}s`;

    return `
    <article class="hotel-card" style="animation-delay:${delay}" data-id="${esc(h.id)}" data-name="${esc(h.name)}">
      <div class="card-photo">
        <div class="card-photo-bg" style="background:${gradient}">${h.thumbnail ? `<img class="card-photo-thumb" src="${esc(h.thumbnail)}" alt="" loading="lazy" onerror="console.warn('[hotels-mcp] img failed:',this.src);this.remove()">` : ""}</div>
        <div class="card-photo-content">
          <div class="card-photo-top">
            <span class="brand-badge" style="background:${theme.badge};color:${theme.badgeText}">${esc(h.brand)}</span>
            ${h.rating > 0 ? `
            <div class="rating-chip">
              <span class="rating-num" style="color:${ratingBg(h.rating)}">${h.rating.toFixed(1)}</span>
              <span class="rating-label">/ 10</span>
            </div>` : ""}
          </div>
          <div class="card-photo-bottom">
            <div class="card-photo-name">${esc(h.name)}</div>
            ${stars > 0 ? `<div class="card-photo-stars">${starsHtml(stars)}</div>` : ""}
          </div>
        </div>
      </div>
      <div class="card-body">
        <div class="card-location">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
            <path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/>
          </svg>
          <div class="card-location-text">${h.city ? `<strong>${esc(h.city)}</strong>${h.country ? ", " + esc(h.country) : ""}` : esc(h.country ?? "")}</div>
        </div>
      </div>
      <div class="card-footer">
        <div class="price-block">
          ${h.priceFrom > 0
            ? `<span class="price-from">Start from</span>${(h.basePriceFrom ?? 0) > h.priceFrom ? `<div class="price-from-original">${fmtPriceShort(h.basePriceFrom!, h.currency)}</div>` : ""}<div class="price-row"><span class="price-val" style="color:${theme.accent}">${fmtPriceShort(h.priceFrom, h.currency)}</span><span class="price-night">/night</span></div>`
            : `<span class="price-na">Rate not Available</span>`
          }
        </div>
        <button class="btn-rooms" style="background:linear-gradient(135deg,${theme.gradFrom},${theme.gradMid})">View Rooms</button>
      </div>
    </article>`;
  }).join("");

  grid.querySelectorAll<HTMLElement>(".hotel-card").forEach(card => {
    const id   = card.dataset.id   ?? "";
    const name = card.dataset.name ?? "Hotel";
    const btn  = card.querySelector(".btn-rooms");
    btn?.addEventListener("click", (e) => { e.stopPropagation(); openOverlay(id, name, card); });
    card.addEventListener("click", () => openOverlay(id, name, card));
  });
}

// ── Overlay ───────────────────────────────────────────────────────────────────

function openOverlay(hotelId: string, hotelName: string, anchor?: HTMLElement): void {
  const overlay = document.getElementById("room-overlay");
  if (overlay) overlay.style.display = "block";
  const sheet = overlay?.querySelector<HTMLElement>(".overlay-sheet");
  if (sheet) {
    sheet.scrollTop = 0;
    // Reset inline styles so CSS defaults (centered) apply
    sheet.style.top = ""; sheet.style.left = ""; sheet.style.transform = "";

    if (anchor && window.innerWidth > 600) {
      const rect = anchor.getBoundingClientRect();
      const sw = Math.min(520, window.innerWidth - 32);
      const sh = Math.min(860, window.innerHeight * 0.88);
      // Prefer right of card; fall back to left; fall back to center
      let left = rect.right + 12;
      if (left + sw > window.innerWidth - 16) left = rect.left - sw - 12;
      if (left < 16) left = (window.innerWidth - sw) / 2;
      const top = Math.max(16, Math.min(rect.top, window.innerHeight - sh - 16));
      sheet.style.top = top + "px";
      sheet.style.left = left + "px";
      sheet.style.transform = "none";
    }
  }

  const content = document.getElementById("overlay-content");
  if (content) {
    content.innerHTML = `
      <div style="padding:80px 24px;text-align:center;color:var(--text-2)">
        <div style="width:28px;height:28px;margin:0 auto 10px;animation:pulse 1.5s infinite;color:var(--text-2)"><svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" stroke-linecap="round" stroke-linejoin="round"><rect width="16" height="20" x="4" y="2" rx="2"/><path d="M9 22v-4h6v4"/><path d="M8 6h.01"/><path d="M16 6h.01"/><path d="M12 6h.01"/><path d="M12 10h.01"/><path d="M16 10h.01"/><path d="M8 10h.01"/><path d="M12 14h.01"/><path d="M16 14h.01"/><path d="M8 14h.01"/></svg></div>
        <div>Loading <strong style="color:var(--text-1)">${esc(hotelName)}</strong>…</div>
      </div>`;
  }

  appRef?.callServerTool({ name: "get_hotel_detail", arguments: { hotelId } })
    .then((result: any) => {
      const detail: HotelDetail = result?.structuredContent ?? result;
      if (content) {
        content.innerHTML = detail?.id
          ? renderRoomDetail(detail)
          : `<div class="no-rooms"><div class="no-rooms-icon">🔍</div><div>No data for ${esc(hotelName)}</div></div>`;
      }
    })
    .catch((err: any) => {
      if (content) {
        content.innerHTML = `<div class="no-rooms">
          <div class="no-rooms-icon">⚠️</div>
          <div>Could not load rooms</div>
          <div style="font-size:11px;margin-top:4px">${esc(String(err))}</div>
        </div>`;
      }
    });
}

function closeOverlay(): void {
  const overlay = document.getElementById("room-overlay");
  if (overlay) overlay.style.display = "none";
}

// ── Room detail ───────────────────────────────────────────────────────────────

function renderRoomDetail(detail: HotelDetail): string {
  const theme = brandTheme(detail.brand, detail.brandColor);
  const gradient = buildPhotoGradient(theme);
  const stars = deriveStars(detail.rating, detail.stars);
  const rooms = detail.roomTypes ?? [];

  const _pad = (n: number) => String(n).padStart(2, "0");
  const _fmt = (d: Date) => `${d.getFullYear()}-${_pad(d.getMonth()+1)}-${_pad(d.getDate())}`;
  const _now = new Date();
  const _tom = new Date(_now); _tom.setDate(_tom.getDate() + 1);
  let bookingHref = "";
  if (detail.bookingUrl) {
    try {
      const _u = new URL(detail.bookingUrl);
      if (_u.protocol === "https:" || _u.protocol === "http:") {
        _u.searchParams.set("in", _fmt(_now));
        _u.searchParams.set("out", _fmt(_tom));
        _u.searchParams.set("guests", "A,A");
        _u.searchParams.set("cur", detail.currency);
        bookingHref = _u.toString();
      }
    } catch { /* ignore invalid or non-http URLs */ }
  }

  const heroThumb = detail.thumbnail
    ? `<img class="overlay-hero-thumb" src="${esc(detail.thumbnail)}" alt="" loading="lazy" onerror="console.warn('[hotels-mcp] img failed:',this.src);this.remove()">`
    : "";

  return `
    <div class="overlay-hero">
      <div class="overlay-hero-bg" style="background:${gradient}">${heroThumb}</div>
      <div class="overlay-hero-top">
        <span class="brand-badge" style="background:${theme.badge};color:${theme.badgeText}">${esc(detail.brand)}</span>
      </div>
      <div class="overlay-hero-content">
        <div class="overlay-hotel-name">${esc(detail.name)}</div>
        <div class="overlay-hotel-meta">
          ${detail.rating > 0 ? `<span class="overlay-rating-badge" style="background:${ratingBg(detail.rating)}">${detail.rating.toFixed(1)}/10</span>` : ""}
          ${stars > 0 ? `<span class="overlay-stars">${starsHtml(stars)}</span>` : ""}
        </div>
      </div>
    </div>

    <div class="overlay-body">
      ${detail.address?.replace(/[\s.]+/g, "").length > 0 ? `
      <div class="overlay-address">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 10c0 7-9 13-9 13s-9-6-9-13a9 9 0 0 1 18 0z"/><circle cx="12" cy="10" r="3"/>
        </svg>
        <span>${esc(detail.address)}</span>
      </div>` : ""}

      ${((detail.startingPrice ?? 0) > 0 || bookingHref) ? `
      <div class="starting-callout">
        <div class="starting-callout-left">
          <span class="starting-callout-label">Starting from</span>
          ${(detail.startingBasePrice ?? 0) > (detail.startingPrice ?? 0) ? `<span class="starting-price-original">${fmtPriceFull(detail.startingBasePrice!, detail.currency)}</span>` : ""}
          ${(detail.startingPrice ?? 0) > 0 ? `<span class="starting-callout-price">${fmtPriceFull(detail.startingPrice!, detail.currency)}</span>` : ""}
        </div>
        ${bookingHref ? `<a href="${esc(bookingHref)}" class="book-now-btn">BOOK NOW</a>` : ""}
      </div>` : ""}

      <div class="section-title">Rooms &amp; Suites</div>
      ${rooms.length === 0
        ? `<div class="no-rooms"><div class="no-rooms-icon">🔍</div><div>No room data for this property</div></div>`
        : rooms.map(r => renderRoomCard(r, theme, detail.brand)).join("")
      }
    </div>`;
}

function renderRoomCard(room: RoomType, theme: BrandTheme, _brand: string): string {
  const bedType = deriveBedType(room.name);

  const rawImg = room.roomImage ?? "";
  const imgUrl = (rawImg.startsWith("https://") || rawImg.startsWith("/")) ? rawImg : "";

  return `
  <div class="room-card">
    ${imgUrl ? `<img class="room-card-img" src="${esc(imgUrl)}" alt="${esc(room.name)}" loading="lazy">` : ""}
    <div class="room-card-header">
      <div style="flex:1;min-width:0">
        <div class="room-name">${esc(room.name)}</div>
        <div class="room-details">
          <span class="room-detail-item">${esc(bedType)}</span>
          ${room.maxGuests > 0 ? `<span class="room-detail-item">· <svg width="12" height="12" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"><path d="M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2"/><circle cx="12" cy="7" r="4"/></svg> ${room.maxGuests} guest${room.maxGuests !== 1 ? "s" : ""}</span>` : ""}
        </div>
      </div>
      <div class="room-price-block">
        ${room.pricePerNight > 0
          ? `${ (room.baseRate ?? 0) > room.pricePerNight ? `<div class="room-price-original">${fmtPriceShort(room.baseRate, room.currency)}</div>` : ""}
             <div class="room-price-row"><div class="room-price" style="color:#00215b">${fmtPriceShort(room.pricePerNight, room.currency)}</div><div class="room-price-night">/night</div></div>`
          : `<div class="room-price-night">Rate not Available</div>`
        }
      </div>
    </div>
  </div>`;
}

// ── Show dashboard / error ────────────────────────────────────────────────────

function showDashboard(data: DashboardData): void {
  state.allHotels = data.hotels ?? [];
  hide("loading-state");
  hide("error-state");
  show("dashboard-content");
  if (state.allHotels.length > 0) populateFilters(state.allHotels);
  const autoCity = data.destination || data.city || "";
  if (autoCity) {
    state.cityFilter = autoCity;
    const citySel = document.getElementById("city-filter") as HTMLSelectElement | null;
    if (citySel) citySel.value = autoCity;
  }
  if (data.sortBy) {
    state.sortBy = data.sortBy;
    const sel = document.getElementById("sort-filter") as HTMLSelectElement | null;
    if (sel) sel.value = data.sortBy;
  }
  applyFilters();
}

function showError(msg: string): void {
  hide("loading-state");
  hide("dashboard-content");
  show("error-state");
  setText("error-message", msg);
}

// ── MCP App lifecycle ─────────────────────────────────────────────────────────

async function main(): Promise<void> {
  injectStyles();
  buildDOM();

  const app = new App({ name: "Archipelago Hotels Dashboard", version: "2.0.0" });
  appRef = app;

  app.ontoolinput = (params: ToolInputParams): void => {
    const data = params.structuredContent as DashboardData | undefined;
    if (data?.hotels?.length) showDashboard(data);
  };

  app.ontoolinputpartial = (params: ToolInputParams): void => {
    if (params.structuredContent?.hotels?.length) showDashboard(params.structuredContent);
  };

  app.ontoolresult = (result: any): void => {
    if (result?.isError) {
      const t = result.content?.find((c: any) => c.type === "text");
      showError(t && "text" in t ? t.text : "Tool returned an error.");
      return;
    }
    const data = result?.structuredContent as DashboardData | undefined;
    if (data?.hotels?.length) showDashboard(data);
  };

  app.onhostcontextchanged = (ctx: any): void => {
    if (ctx?.theme) applyDocumentTheme(ctx.theme);
    if (ctx?.styles?.variables) applyHostStyleVariables(ctx.styles.variables);
    if (ctx?.styles?.css?.fonts) applyHostFonts(ctx.styles.css.fonts);
    if (ctx?.safeAreaInsets) {
      const { top = 0, right = 0, bottom = 0, left = 0 } = ctx.safeAreaInsets;
      document.body.style.padding = `${top + 10}px ${right + 10}px ${bottom + 10}px ${left + 10}px`;
    }
  };

  app.onteardown = async (): Promise<Record<string, unknown>> => {
    state.allHotels = [];
    return {};
  };

  await app.connect();
}

main().catch((err) => {
  console.error("[archipelago-hotels-mcp] init error:", err);
  const el = document.getElementById("error-message");
  if (el) el.textContent = `Init error: ${err instanceof Error ? err.message : String(err)}`;
  show("error-state");
  hide("loading-state");
});
