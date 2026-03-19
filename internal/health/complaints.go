package health

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image/color"
	"log"
	"net/http"
	"strings"
	"time"

	"cmon/internal/belt"
	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/summary"
)

type complaintDashboardPageData struct {
	DataURL string
}

type complaintDashboardPayload struct {
	GeneratedAt string                  `json:"generated_at"`
	TotalCount  int                     `json:"total_count"`
	GroupCount  int                     `json:"group_count"`
	Status      Status                  `json:"status"`
	Groups      []complaintGroupPayload `json:"groups"`
}

type complaintGroupPayload struct {
	Belt       string              `json:"belt"`
	Label      string              `json:"label"`
	Emoji      string              `json:"emoji"`
	Count      int                 `json:"count"`
	FillColor  string              `json:"fill_color"`
	TextColor  string              `json:"text_color"`
	Complaints []summary.Complaint `json:"complaints"`
}

var complaintsPageTemplate = template.Must(template.New("complaints-page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CMON — Complaint Monitor</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&family=DM+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg: #0c0e12;
      --surface: #14171e;
      --surface-raised: #1a1e27;
      --surface-bright: #222733;
      --border: rgba(255,255,255,0.06);
      --border-bright: rgba(255,255,255,0.1);
      --text: #e2e6ef;
      --text-dim: #7a839a;
      --text-faint: #4a5168;
      --accent: #5b9cf5;
      --accent-dim: rgba(91,156,245,0.15);
      --danger: #f55b5b;
      --danger-dim: rgba(245,91,91,0.12);
      --success: #4ade80;
      --success-dim: rgba(74,222,128,0.12);
      --warn: #fbbf24;
      --font-mono: 'JetBrains Mono', 'Fira Code', 'SF Mono', monospace;
      --font-sans: 'DM Sans', -apple-system, sans-serif;
    }

    *,*::before,*::after{box-sizing:border-box}
    html{scroll-behavior:smooth;-webkit-font-smoothing:antialiased}

    body {
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
      font-family: var(--font-sans);
      font-size: 14px;
      line-height: 1.5;
    }

    /* ── Noise overlay ── */
    body::before {
      content: "";
      position: fixed;
      inset: 0;
      z-index: 9999;
      pointer-events: none;
      opacity: 0.025;
      background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.9' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
      background-size: 180px;
    }

    /* ── Shell ── */
    .shell {
      max-width: 1480px;
      margin: 0 auto;
      padding: 20px 24px 60px;
    }

    /* ── Top bar ── */
    .topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding-bottom: 20px;
      border-bottom: 1px solid var(--border);
      margin-bottom: 20px;
    }
    .topbar-left {
      display: flex;
      align-items: center;
      gap: 14px;
      min-width: 0;
    }
    .logo {
      font-family: var(--font-mono);
      font-weight: 700;
      font-size: 15px;
      letter-spacing: -0.02em;
      color: var(--text);
      white-space: nowrap;
    }
    .logo span { color: var(--accent); }
    .status-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      border-radius: 4px;
      font-family: var(--font-mono);
      font-size: 11px;
      font-weight: 500;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      white-space: nowrap;
    }
    .status-chip.healthy {
      background: var(--success-dim);
      color: var(--success);
    }
    .status-chip.unhealthy {
      background: var(--danger-dim);
      color: var(--danger);
    }
    .status-chip.loading {
      background: var(--accent-dim);
      color: var(--accent);
    }
    .status-chip::before {
      content: "";
      width: 6px; height: 6px;
      border-radius: 50%;
      background: currentColor;
      animation: pulse-dot 2s ease-in-out infinite;
    }
    @keyframes pulse-dot {
      0%,100%{opacity:1}50%{opacity:0.3}
    }

    .topbar-right {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-shrink: 0;
    }
    .updated-ago {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--text-dim);
      white-space: nowrap;
    }

    /* ── Stats row ── */
    .stats-row {
      display: grid;
      grid-template-columns: repeat(4, 1fr);
      gap: 12px;
      margin-bottom: 16px;
    }
    .stat-card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 14px 16px;
      transition: border-color 0.2s;
    }
    .stat-card:hover { border-color: var(--border-bright); }
    .stat-label {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--text-dim);
      margin-bottom: 6px;
    }
    .stat-value {
      font-family: var(--font-mono);
      font-size: 22px;
      font-weight: 700;
      color: var(--text);
      line-height: 1.1;
    }
    .stat-value.accent { color: var(--accent); }
    .stat-sub {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--text-faint);
      margin-top: 4px;
    }

    /* ── Distribution bar ── */
    .dist-bar-wrap {
      margin-bottom: 16px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 14px 16px;
    }
    .dist-bar-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 10px;
    }
    .dist-bar-title {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--text-dim);
    }
    .dist-bar {
      display: flex;
      height: 28px;
      border-radius: 4px;
      overflow: hidden;
      gap: 2px;
    }
    .dist-seg {
      position: relative;
      min-width: 3px;
      transition: flex 0.5s cubic-bezier(0.22,1,0.36,1);
      cursor: default;
    }
    .dist-seg:first-child { border-radius: 4px 0 0 4px; }
    .dist-seg:last-child { border-radius: 0 4px 4px 0; }
    .dist-seg:only-child { border-radius: 4px; }
    .dist-legend {
      display: flex;
      flex-wrap: wrap;
      gap: 4px 14px;
      margin-top: 10px;
    }
    .dist-legend-item {
      display: flex;
      align-items: center;
      gap: 6px;
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--text-dim);
      white-space: nowrap;
    }
    .dist-legend-dot {
      width: 8px; height: 8px;
      border-radius: 2px;
      flex-shrink: 0;
    }
    .dist-legend-count {
      color: var(--text);
      font-weight: 600;
    }

    /* ── Toolbar ── */
    .toolbar {
      display: flex;
      align-items: center;
      gap: 10px;
      margin-bottom: 16px;
      flex-wrap: wrap;
    }
    .search-box {
      flex: 1 1 280px;
      position: relative;
    }
    .search-box input {
      width: 100%;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 6px;
      padding: 10px 12px 10px 36px;
      color: var(--text);
      font-family: var(--font-sans);
      font-size: 13px;
      outline: none;
      transition: border-color 0.15s, box-shadow 0.15s;
    }
    .search-box input:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 3px var(--accent-dim);
    }
    .search-box input::placeholder { color: var(--text-faint); }
    .search-icon {
      position: absolute;
      left: 11px;
      top: 50%;
      transform: translateY(-50%);
      color: var(--text-faint);
      pointer-events: none;
      width: 16px;
      height: 16px;
    }
    .search-kbd {
      position: absolute;
      right: 10px;
      top: 50%;
      transform: translateY(-50%);
      font-family: var(--font-mono);
      font-size: 10px;
      color: var(--text-faint);
      background: var(--surface-raised);
      border: 1px solid var(--border);
      border-radius: 3px;
      padding: 2px 5px;
      pointer-events: none;
      line-height: 1;
    }
    .search-count {
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--text-faint);
      white-space: nowrap;
      padding-left: 4px;
    }

    .tool-btn {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 9px 14px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 6px;
      color: var(--text-dim);
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 500;
      cursor: pointer;
      transition: background 0.15s, border-color 0.15s, color 0.15s;
      white-space: nowrap;
    }
    .tool-btn:hover {
      background: var(--surface-raised);
      border-color: var(--border-bright);
      color: var(--text);
    }
    .tool-btn.active {
      background: var(--accent-dim);
      border-color: rgba(91,156,245,0.25);
      color: var(--accent);
    }
    .tool-btn svg { width: 14px; height: 14px; flex-shrink: 0; }

    .refresh-btn {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 9px 16px;
      background: var(--accent);
      border: 1px solid transparent;
      border-radius: 6px;
      color: #0c0e12;
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 600;
      cursor: pointer;
      transition: opacity 0.15s, transform 0.1s;
      white-space: nowrap;
    }
    .refresh-btn:hover { opacity: 0.88; }
    .refresh-btn:active { transform: scale(0.97); }
    .refresh-btn:disabled {
      opacity: 0.5;
      cursor: not-allowed;
      transform: none;
    }
    .refresh-btn svg { width: 14px; height: 14px; flex-shrink: 0; }
    .refresh-btn.is-loading svg {
      animation: spin 0.8s linear infinite;
    }
    @keyframes spin { to { transform: rotate(360deg); } }

    /* ── Status banner ── */
    .banner {
      padding: 10px 14px;
      border-radius: 6px;
      font-size: 13px;
      margin-bottom: 16px;
      display: flex;
      align-items: center;
      gap: 8px;
      transition: opacity 0.3s;
    }
    .banner.info {
      background: var(--accent-dim);
      border: 1px solid rgba(91,156,245,0.15);
      color: var(--accent);
    }
    .banner.error {
      background: var(--danger-dim);
      border: 1px solid rgba(245,91,91,0.15);
      color: var(--danger);
    }
    .banner.success {
      background: var(--success-dim);
      border: 1px solid rgba(74,222,128,0.15);
      color: var(--success);
    }
    .banner svg { width: 16px; height: 16px; flex-shrink: 0; }
    .banner-text { flex: 1; }
    .banner strong { font-weight: 600; }

    /* ── Groups ── */
    .groups { display: grid; gap: 12px; }

    .group {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 6px;
      overflow: hidden;
      animation: fadeUp 0.3s ease both;
    }
    @keyframes fadeUp {
      from { opacity:0; transform:translateY(6px); }
      to { opacity:1; transform:translateY(0); }
    }

    .group-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      padding: 12px 16px;
      cursor: pointer;
      user-select: none;
      transition: background 0.15s;
    }
    .group-header:hover { background: rgba(255,255,255,0.02); }
    .group-header-left {
      display: flex;
      align-items: center;
      gap: 10px;
      min-width: 0;
    }
    .group-indicator {
      width: 10px; height: 10px;
      border-radius: 3px;
      flex-shrink: 0;
      box-shadow: 0 0 8px var(--glow-color, transparent);
    }
    .group-name {
      font-weight: 600;
      font-size: 14px;
      color: var(--text);
      white-space: nowrap;
    }
    .group-badge {
      font-family: var(--font-mono);
      font-size: 11px;
      font-weight: 600;
      padding: 2px 8px;
      border-radius: 3px;
      white-space: nowrap;
    }
    .group-chevron {
      width: 16px; height: 16px;
      color: var(--text-faint);
      transition: transform 0.2s ease;
      flex-shrink: 0;
    }
    .group.collapsed .group-chevron { transform: rotate(-90deg); }
    .group-body {
      overflow: hidden;
      transition: max-height 0.3s ease, opacity 0.2s ease;
      max-height: 4000px;
      opacity: 1;
    }
    .group.collapsed .group-body {
      max-height: 0;
      opacity: 0;
    }

    /* ── Table ── */
    .tbl-wrap {
      overflow-x: auto;
      -webkit-overflow-scrolling: touch;
    }
    table {
      width: 100%;
      border-collapse: collapse;
      min-width: 900px;
    }
    thead th {
      position: sticky;
      top: 0;
      z-index: 2;
      padding: 8px 12px;
      background: var(--surface-raised);
      color: var(--text-faint);
      font-size: 10px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      text-align: left;
      border-bottom: 1px solid var(--border);
      border-top: 1px solid var(--border);
      white-space: nowrap;
    }
    tbody td {
      padding: 9px 12px;
      border-bottom: 1px solid var(--border);
      color: var(--text);
      font-size: 13px;
      vertical-align: top;
    }
    tbody tr { transition: background 0.1s; }
    tbody tr:hover td { background: rgba(255,255,255,0.02); }
    td.mono {
      font-family: var(--font-mono);
      font-size: 12px;
      font-weight: 500;
    }
    td.desc-cell {
      max-width: 220px;
      white-space: nowrap;
      overflow: hidden;
      text-overflow: ellipsis;
    }
    td.desc-cell:hover {
      white-space: normal;
      overflow: visible;
    }

    /* debug columns */
    .debug-col { display: none; }
    body.show-debug .debug-col { display: table-cell; }

    /* ── Empty / Error ── */
    .empty-state {
      padding: 48px 24px;
      text-align: center;
      color: var(--text-faint);
      background: var(--surface);
      border: 1px dashed var(--border-bright);
      border-radius: 6px;
    }
    .empty-state strong {
      display: block;
      color: var(--text-dim);
      font-size: 14px;
      margin-bottom: 6px;
    }
    .error-box {
      padding: 16px;
      border-radius: 6px;
      background: var(--danger-dim);
      border: 1px solid rgba(245,91,91,0.15);
      color: var(--danger);
    }
    .error-box strong { display: block; margin-bottom: 4px; font-weight: 600; }

    /* ── Responsive ── */
    @media (max-width:960px) {
      .stats-row { grid-template-columns: repeat(2,1fr); }
      .topbar { flex-wrap: wrap; }
    }
    @media (max-width:680px) {
      .shell { padding: 12px 12px 40px; }
      .stats-row { grid-template-columns: 1fr 1fr; gap: 8px; }
      .stat-value { font-size: 18px; }
      .toolbar { gap: 8px; }

      table { min-width: 100%; }
      thead { display: none; }
      tbody, tbody tr, tbody td { display: block; width: 100%; }
      body.show-debug tbody td.debug-col { display: block; }
      tbody tr {
        padding: 10px 12px;
        margin-bottom: 6px;
        border: 1px solid var(--border);
        border-radius: 6px;
        background: var(--surface-raised);
      }
      tbody td {
        border: 0;
        padding: 3px 0;
        font-size: 13px;
      }
      tbody td::before {
        content: attr(data-label);
        display: inline-block;
        width: 90px;
        flex-shrink: 0;
        color: var(--text-faint);
        font-size: 10px;
        font-weight: 600;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        vertical-align: top;
        padding-top: 1px;
      }
      tbody td { display: flex; gap: 4px; }
      td.desc-cell { max-width: none; white-space: normal; }
    }
    @media (max-width:400px) {
      .stats-row { grid-template-columns: 1fr; }
    }
  </style>
</head>
<body>
  <main class="shell">
    <!-- Top bar -->
    <header class="topbar">
      <div class="topbar-left">
        <div class="logo">CMON<span>.</span></div>
        <div id="statusChip" class="status-chip loading">Connecting</div>
      </div>
      <div class="topbar-right">
        <span id="updatedAgo" class="updated-ago"></span>
      </div>
    </header>

    <!-- Stats -->
    <section class="stats-row">
      <div class="stat-card">
        <div class="stat-label">Total Pending</div>
        <div class="stat-value accent" id="totalCount">—</div>
        <div class="stat-sub" id="totalSub"></div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Belt Groups</div>
        <div class="stat-value" id="groupCount">—</div>
        <div class="stat-sub" id="groupSub"></div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Last Fetch</div>
        <div class="stat-value" id="lastFetchTime" style="font-size:15px">—</div>
        <div class="stat-sub" id="fetchSub"></div>
      </div>
      <div class="stat-card">
        <div class="stat-label">Uptime</div>
        <div class="stat-value" id="uptime" style="font-size:15px">—</div>
        <div class="stat-sub" id="uptimeSub"></div>
      </div>
    </section>

    <!-- Distribution bar -->
    <section class="dist-bar-wrap" id="distBarWrap" style="display:none">
      <div class="dist-bar-header">
        <div class="dist-bar-title">Complaint Distribution</div>
      </div>
      <div class="dist-bar" id="distBar"></div>
      <div class="dist-legend" id="distLegend"></div>
    </section>

    <!-- Toolbar -->
    <section class="toolbar">
      <div class="search-box">
        <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        <input id="searchInput" type="search" placeholder="Search complaints..." autocomplete="off" spellcheck="false">
        <span class="search-kbd" id="searchKbd">/</span>
      </div>
      <span class="search-count" id="searchCount"></span>

      <button id="debugToggle" class="tool-btn" type="button" title="Toggle debug columns (Telegram/WhatsApp IDs)">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 013 3L7 19l-4 1 1-4z"/></svg>
        Debug
      </button>

      <button id="refreshBtn" class="refresh-btn" type="button">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="23 4 23 10 17 10"/><path d="M20.49 15a9 9 0 11-2.12-9.36L23 10"/></svg>
        <span id="refreshLabel">Refresh</span>
      </button>

    </section>

    <!-- Banner -->
    <div id="banner" class="banner info">
      <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>
      <span class="banner-text" id="bannerText"><strong>Initializing...</strong> Loading complaint data.</span>
    </div>

    <!-- Content -->
    <section id="content" class="groups"></section>
  </main>

  <script>
    (() => {
      const DATA_URL = {{.DataURL}};

      // DOM refs
      const $ = (id) => document.getElementById(id);
      const searchInput = $("searchInput");
      const searchKbd = $("searchKbd");
      const searchCountEl = $("searchCount");
      const debugToggleBtn = $("debugToggle");
      const refreshBtn = $("refreshBtn");
      const refreshLabel = $("refreshLabel");

      const bannerEl = $("banner");
      const bannerText = $("bannerText");
      const contentEl = $("content");
      const statusChip = $("statusChip");
      const updatedAgoEl = $("updatedAgo");
      const distBarWrap = $("distBarWrap");
      const distBar = $("distBar");
      const distLegend = $("distLegend");

      let payload = null;
      let isLoading = false;
      let lastLoadTime = null;
      let agoTimer = null;
      let collapsedBelts = new Set();

      // Utils
      const esc = (v) => String(v ?? "").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;");

      function timeAgo(date) {
        const sec = Math.floor((Date.now() - date.getTime()) / 1000);
        if (sec < 5) return "just now";
        if (sec < 60) return sec + "s ago";
        const min = Math.floor(sec / 60);
        if (min < 60) return min + "m ago";
        const hr = Math.floor(min / 60);
        return hr + "h " + (min % 60) + "m ago";
      }

      function updateAgo() {
        if (!lastLoadTime) { updatedAgoEl.textContent = ""; return; }
        updatedAgoEl.textContent = "updated " + timeAgo(lastLoadTime);
      }

      // Status chip
      function setChip(status) {
        statusChip.className = "status-chip " + (status === "healthy" ? "healthy" : status === "loading" ? "loading" : "unhealthy");
        statusChip.textContent = status === "healthy" ? "Operational" : status === "loading" ? "Loading" : "Degraded";
      }

      // Banner
      function setBanner(kind, html) {
        bannerEl.className = "banner " + kind;
        const icons = {
          info: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="12" y1="16" x2="12" y2="12"/><line x1="12" y1="8" x2="12.01" y2="8"/></svg>',
          error: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="12" cy="12" r="10"/><line x1="15" y1="9" x2="9" y2="15"/><line x1="9" y1="9" x2="15" y2="15"/></svg>',
          success: '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M22 11.08V12a10 10 0 11-5.93-9.14"/><polyline points="22 4 12 14.01 9 11.01"/></svg>'
        };
        bannerEl.innerHTML = (icons[kind] || icons.info) + '<span class="banner-text">' + html + '</span>';
      }

      // Metrics
      function setMetric(id, val) { const e = $(id); if (e) e.textContent = val; }

      // Distribution bar
      function renderDistBar(groups) {
        if (!groups || groups.length === 0) { distBarWrap.style.display = "none"; return; }
        distBarWrap.style.display = "";
        const total = groups.reduce((s, g) => s + g.complaints.length, 0);
        if (total === 0) { distBarWrap.style.display = "none"; return; }

        distBar.innerHTML = groups.map((g) => {
          const pct = (g.complaints.length / total * 100);
          return '<div class="dist-seg" style="flex:' + pct + ';background:' + g.text_color + ';opacity:0.85" title="' + esc(g.label) + ': ' + g.complaints.length + '"></div>';
        }).join("");

        distLegend.innerHTML = groups.map((g) =>
          '<span class="dist-legend-item">' +
            '<span class="dist-legend-dot" style="background:' + g.text_color + '"></span>' +
            esc(g.label) + ' <span class="dist-legend-count">' + g.complaints.length + '</span>' +
          '</span>'
        ).join("");
      }

      // Search
      function matches(c, term) {
        if (!term) return true;
        return [c.complain_no, c.name, c.consumer_no, c.mobile_no, c.address, c.area, c.village, c.belt, c.description, c.complain_date, c.telegram_message_id, c.whatsapp_message_id].join(" ").toLowerCase().includes(term);
      }

      // Build table row
      function buildRow(c) {
        const tg = c.telegram_message_id || "—";
        const wa = c.whatsapp_message_id || "—";
        return '<tr>' +
          '<td data-label="Complaint" class="mono">' + esc(c.complain_no || "—") + '</td>' +
          '<td data-label="Name">' + esc(c.name || "—") + '</td>' +
          '<td data-label="Consumer" class="mono">' + esc(c.consumer_no || "—") + '</td>' +
          '<td data-label="Mobile" class="mono">' + esc(c.mobile_no || "—") + '</td>' +
          '<td data-label="Address">' + esc(c.address || "—") + '</td>' +
          '<td data-label="Area">' + esc(c.area || "—") + '</td>' +
          '<td data-label="Description" class="desc-cell">' + esc(c.description || "—") + '</td>' +
          '<td data-label="Date" class="mono">' + esc(c.complain_date || "—") + '</td>' +
          '<td data-label="Telegram" class="debug-col mono">' + esc(tg) + '</td>' +
          '<td data-label="WhatsApp" class="debug-col mono">' + esc(wa) + '</td>' +
        '</tr>';
      }

      // Build group
      function buildGroup(g, complaints) {
        const key = g.belt;
        const isCollapsed = collapsedBelts.has(key);
        const rows = complaints.map(buildRow).join("");
        return '<div class="group' + (isCollapsed ? ' collapsed' : '') + '" data-belt="' + esc(key) + '">' +
          '<div class="group-header" role="button" tabindex="0">' +
            '<div class="group-header-left">' +
              '<span class="group-indicator" style="background:' + g.text_color + ';--glow-color:' + g.text_color + '"></span>' +
              '<span class="group-name">' + esc(g.label) + ' Belt</span>' +
              '<span class="group-badge" style="background:' + g.fill_color + ';color:' + g.text_color + '">' + complaints.length + '</span>' +
            '</div>' +
            '<svg class="group-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>' +
          '</div>' +
          '<div class="group-body"><div class="tbl-wrap"><table>' +
            '<thead><tr>' +
              '<th>Complaint</th><th>Name</th><th>Consumer</th><th>Mobile</th>' +
              '<th>Address</th><th>Area</th><th>Description</th><th>Date</th>' +
              '<th class="debug-col">Telegram</th><th class="debug-col">WhatsApp</th>' +
            '</tr></thead>' +
            '<tbody>' + rows + '</tbody>' +
          '</table></div></div>' +
        '</div>';
      }

      // Main render
      function render() {
        if (!payload) { contentEl.innerHTML = ""; return; }

        const q = searchInput.value.trim().toLowerCase();
        const filtered = payload.groups
          .map((g) => ({ ...g, complaints: g.complaints.filter((c) => matches(c, q)) }))
          .filter((g) => g.complaints.length > 0);

        const visCount = filtered.reduce((s, g) => s + g.complaints.length, 0);

        // Stats
        setMetric("totalCount", q ? visCount : payload.total_count);
        $("totalSub").textContent = q ? "of " + payload.total_count + " total" : "complaints";
        setMetric("groupCount", q ? filtered.length : payload.group_count);
        $("groupSub").textContent = q ? "of " + payload.group_count + " total" : "active belts";
        setMetric("lastFetchTime", payload.status.last_fetch_time || "—");
        $("fetchSub").textContent = payload.status.last_fetch_status || "";
        setMetric("uptime", payload.status.uptime || "—");
        $("uptimeSub").textContent = payload.status.status === "healthy" ? "all systems go" : "needs attention";

        // Search count
        if (q) {
          searchCountEl.textContent = visCount + " result" + (visCount === 1 ? "" : "s");
          searchKbd.style.display = "none";
        } else {
          searchCountEl.textContent = "";
          searchKbd.style.display = searchInput === document.activeElement ? "none" : "";
        }

        // Distribution bar (unfiltered)
        renderDistBar(payload.groups);

        // Groups
        if (filtered.length === 0) {
          contentEl.innerHTML = '<div class="empty-state"><strong>' +
            esc(q ? "No complaints match your search" : "No pending complaints") +
            '</strong><span>' +
            esc(q ? "Try broadening your search term." : "The dashboard will update when complaints arrive.") +
            '</span></div>';
          return;
        }

        contentEl.innerHTML = filtered.map((g) => buildGroup(g, g.complaints)).join("");

        // Bind collapse toggles
        contentEl.querySelectorAll(".group-header").forEach((hdr) => {
          hdr.addEventListener("click", () => {
            const grp = hdr.closest(".group");
            const belt = grp.dataset.belt;
            grp.classList.toggle("collapsed");
            if (grp.classList.contains("collapsed")) collapsedBelts.add(belt);
            else collapsedBelts.delete(belt);
          });
          hdr.addEventListener("keydown", (e) => {
            if (e.key === "Enter" || e.key === " ") { e.preventDefault(); hdr.click(); }
          });
        });
      }

      // Fetch
      async function fetchData() {
        const r = await fetch(DATA_URL + "?ts=" + Date.now(), { headers: { Accept: "application/json" } });
        const d = await r.json().catch(() => ({}));
        if (!r.ok) throw new Error(d.error || "Status " + r.status);
        return d;
      }

      async function loadData(opts = {}) {
        if (isLoading) return;
        const silent = !!opts.silent;
        const scrape = !!opts.scrape;
        isLoading = true;
        refreshBtn.disabled = true;
        refreshBtn.classList.add("is-loading");

        try {
          if (scrape) {
            refreshLabel.textContent = "Scraping...";
            if (!silent) setBanner("info", "<strong>Scraping DGVCL portal...</strong> This may take a moment.");
            const sr = await fetch("/refresh", { method: "POST" });
            const sd = await sr.json().catch(() => ({}));
            if (!sr.ok) setBanner("error", "<strong>Scrape failed.</strong> " + esc(sd.error || "Unknown error"));
          }

          refreshLabel.textContent = "Loading...";
          if (!silent && !scrape) setBanner("info", "<strong>Loading...</strong> Fetching complaint data.");

          payload = await fetchData();
          lastLoadTime = new Date();
          updateAgo();
          render();

          const isHealthy = payload.status.status !== "unhealthy";
          setChip(isHealthy ? "healthy" : "unhealthy");
          if (!silent) {
            setBanner(
              isHealthy ? "success" : "error",
              isHealthy
                ? "<strong>Up to date.</strong> Last fetch: " + esc(payload.status.last_fetch_status || "ok") + "."
                : "<strong>Attention needed.</strong> " + esc(payload.status.last_fetch_status || "Check logs.")
            );
          } else {
            const isHealthy2 = payload.status.status !== "unhealthy";
            setBanner(
              isHealthy2 ? "success" : "error",
              isHealthy2
                ? "<strong>Dashboard ready.</strong> " + payload.total_count + " pending complaints loaded."
                : "<strong>Degraded.</strong> " + esc(payload.status.last_fetch_status || "Check logs.")
            );
          }
        } catch (err) {
          setChip("unhealthy");
          contentEl.innerHTML = '<div class="error-box"><strong>Failed to load dashboard</strong>' + esc(err.message) + '</div>';
          setBanner("error", "<strong>Load failed.</strong> " + esc(err.message));
          setMetric("totalCount", "—");
          setMetric("groupCount", "—");
        } finally {
          isLoading = false;
          refreshBtn.disabled = false;
          refreshBtn.classList.remove("is-loading");
          refreshLabel.textContent = "Refresh";
        }
      }

      // Ago ticker
      agoTimer = setInterval(updateAgo, 5000);

      // Events
      searchInput.addEventListener("input", render);
      searchInput.addEventListener("focus", () => { searchKbd.style.display = "none"; });
      searchInput.addEventListener("blur", () => {
        if (!searchInput.value) searchKbd.style.display = "";
      });

      debugToggleBtn.addEventListener("click", () => {
        document.body.classList.toggle("show-debug");
        debugToggleBtn.classList.toggle("active", document.body.classList.contains("show-debug"));
      });

      refreshBtn.addEventListener("click", () => loadData({ scrape: true }));

      // Keyboard shortcuts
      document.addEventListener("keydown", (e) => {
        if (e.key === "/" && document.activeElement !== searchInput && !e.ctrlKey && !e.metaKey) {
          e.preventDefault();
          searchInput.focus();
        }
        if (e.key === "Escape" && document.activeElement === searchInput) {
          searchInput.value = "";
          searchInput.blur();
          render();
        }
      });

      // Boot
      setChip("loading");
      loadData({ silent: true });
    })();
  </script>
</body>
</html>`))

func registerComplaintDashboard(mux *http.ServeMux, monitor *Monitor, sc *session.Client, stor *storage.Storage, refreshFn RefreshFunc) {
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_ = complaintsPageTemplate.Execute(w, complaintDashboardPageData{
			DataURL: "/data",
		})
	})

	mux.HandleFunc("/data", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		payload, err := buildComplaintDashboardPayload(monitor, sc, stor)
		if err != nil {
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(payload)
	})

	mux.HandleFunc("/refresh", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSONError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if refreshFn == nil {
			writeJSONError(w, http.StatusServiceUnavailable, "refresh not available")
			return
		}

		if err := refreshFn(); err != nil {
			log.Printf("⚠️  Dashboard-triggered scrape failed: %v", err)
			writeJSONError(w, http.StatusBadGateway, err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	mux.HandleFunc("/complaints", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusPermanentRedirect)
	})

	mux.HandleFunc("/complaints/", func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/complaints/", "/complaints":
			http.Redirect(w, r, "/", http.StatusPermanentRedirect)
		case "/complaints/data", "/complaints/data/":
			http.Redirect(w, r, "/data", http.StatusPermanentRedirect)
		default:
			http.NotFound(w, r)
		}
	})
}

func buildComplaintDashboardPayload(monitor *Monitor, sc *session.Client, stor *storage.Storage) (complaintDashboardPayload, error) {
	status := monitor.GetStatus()
	activeIDs := stor.GetAllSeenComplaints()
	if len(activeIDs) == 0 {
		return complaintDashboardPayload{
			GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
			TotalCount:  0,
			GroupCount:  0,
			Status:      status,
			Groups:      []complaintGroupPayload{},
		}, nil
	}

	complaints, err := summary.FetchAllPendingDetails(sc, stor)
	if err != nil {
		if strings.Contains(err.Error(), "no pending complaints found") || strings.Contains(err.Error(), "no complaints with valid API IDs") {
			return complaintDashboardPayload{
				GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
				TotalCount:  0,
				GroupCount:  0,
				Status:      status,
				Groups:      []complaintGroupPayload{},
			}, nil
		}
		return complaintDashboardPayload{}, fmt.Errorf("failed to fetch pending complaints: %w", err)
	}

	grouped := summary.GroupComplaints(complaints)
	groups := make([]complaintGroupPayload, 0, len(grouped))
	totalCount := 0
	for _, group := range grouped {
		style := belt.StyleFor(group.Belt)
		totalCount += len(group.Complaints)
		groups = append(groups, complaintGroupPayload{
			Belt:       belt.DisplayName(group.Belt),
			Label:      style.Label,
			Emoji:      style.Emoji,
			Count:      len(group.Complaints),
			FillColor:  colorToHex(style.Fill),
			TextColor:  colorToHex(style.Text),
			Complaints: group.Complaints,
		})
	}

	return complaintDashboardPayload{
		GeneratedAt: time.Now().Format("02 Jan 2006, 03:04 PM"),
		TotalCount:  totalCount,
		GroupCount:  len(groups),
		Status:      status,
		Groups:      groups,
	}, nil
}

func writeJSONError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(map[string]string{
		"error": message,
	})
}

func colorToHex(c color.Color) string {
	r, g, b, _ := c.RGBA()
	return fmt.Sprintf("#%02x%02x%02x", uint8(r>>8), uint8(g>>8), uint8(b>>8))
}
