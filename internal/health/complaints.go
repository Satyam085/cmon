// This file owns the dashboard page template + the shared payload types.
// Route registration lives in dashboard_routes.go, JSON shaping in
// dashboard_payload.go, and CSV/JSON export flattening in dashboard_export.go.

package health

import (
	"html/template"

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
      /* Surfaces — refined warm-paper */
      --bg: #f1ede4;
      --surface: #ffffff;
      --surface-raised: #fbf8f2;
      --surface-bright: #ece6d8;
      --surface-deep: #e3ddcd;

      /* Borders */
      --border: rgba(40,32,20,0.09);
      --border-bright: rgba(40,32,20,0.16);
      --border-strong: rgba(40,32,20,0.28);

      /* Text */
      --text: #14181f;
      --text-2: #2f3742;
      --text-dim: #5b6472;
      --text-faint: #8b95a5;

      /* Accents */
      --accent: #1f5fe8;
      --accent-hover: #1648c9;
      --accent-dim: rgba(31,95,232,0.10);
      --accent-soft: rgba(31,95,232,0.05);

      --danger: #dc2626;
      --danger-dim: rgba(220,38,38,0.09);
      --success: #15803d;
      --success-dim: rgba(21,128,61,0.10);
      --warn: #b45309;

      /* Shadows */
      --shadow-sm: 0 1px 2px rgba(20,16,8,0.04);
      --shadow-md: 0 1px 3px rgba(20,16,8,0.06), 0 1px 2px rgba(20,16,8,0.03);
      --shadow-lg: 0 6px 24px rgba(20,16,8,0.07), 0 1px 3px rgba(20,16,8,0.04);

      /* Type */
      --font-mono: 'JetBrains Mono', 'SF Mono', 'Roboto Mono', monospace;
      --font-sans: 'DM Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;

      /* Radii */
      --r-xs: 4px;
      --r-sm: 6px;
      --r-md: 8px;
      --r-lg: 12px;
    }

    *,*::before,*::after { box-sizing: border-box; }
    html { scroll-behavior: smooth; -webkit-font-smoothing: antialiased; -moz-osx-font-smoothing: grayscale; }

    body {
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
      font-family: var(--font-sans);
      font-size: 14px;
      line-height: 1.5;
    }

    /* ── Subtle paper texture ── */
    body::before {
      content: "";
      position: fixed;
      inset: 0;
      z-index: 9999;
      pointer-events: none;
      opacity: 0.035;
      background-image: url("data:image/svg+xml,%3Csvg viewBox='0 0 256 256' xmlns='http://www.w3.org/2000/svg'%3E%3Cfilter id='n'%3E%3CfeTurbulence type='fractalNoise' baseFrequency='0.75' numOctaves='4' stitchTiles='stitch'/%3E%3C/filter%3E%3Crect width='100%25' height='100%25' filter='url(%23n)'/%3E%3C/svg%3E");
      background-size: 200px;
    }

    /* ── Shell ── */
    .shell {
      max-width: 1400px;
      margin: 0 auto;
      padding: 24px 28px 64px;
    }

    /* ── Top bar ── */
    .topbar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 4px 0 20px;
      border-bottom: 1px solid var(--border);
      margin-bottom: 24px;
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
      font-size: 16px;
      letter-spacing: -0.02em;
      color: var(--text);
      white-space: nowrap;
    }
    .logo span { color: var(--accent); }

    .status-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 5px 11px;
      border-radius: 999px;
      font-family: var(--font-mono);
      font-size: 10.5px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      white-space: nowrap;
      border: 1px solid transparent;
    }
    .status-chip.healthy {
      background: var(--success-dim);
      color: var(--success);
      border-color: rgba(21,128,61,0.18);
    }
    .status-chip.unhealthy {
      background: var(--danger-dim);
      color: var(--danger);
      border-color: rgba(220,38,38,0.18);
    }
    .status-chip.loading {
      background: var(--accent-dim);
      color: var(--accent);
      border-color: rgba(31,95,232,0.18);
    }
    .status-chip::before {
      content: "";
      width: 6px; height: 6px;
      border-radius: 50%;
      background: currentColor;
      animation: pulse-dot 2s ease-in-out infinite;
    }
    @keyframes pulse-dot { 0%,100%{opacity:1} 50%{opacity:0.35} }

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
    .ws-status {
      font-family: var(--font-mono);
      font-size: 10.5px;
      font-weight: 500;
      padding: 4px 10px;
      border-radius: 999px;
      border: 1px solid transparent;
    }
    .ws-status.connected {
      color: var(--success);
      background: var(--success-dim);
      border-color: rgba(21,128,61,0.18);
    }
    .ws-status.disconnected {
      color: var(--text-faint);
      background: var(--surface-bright);
      border-color: var(--border-bright);
    }

    /* ── Stats ── */
    .stats-row {
      display: grid;
      grid-template-columns: repeat(2, 1fr);
      gap: 12px;
      margin-bottom: 16px;
    }
    .stat-card {
      position: relative;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      padding: 16px 18px;
      transition: border-color 0.2s, box-shadow 0.2s;
      box-shadow: var(--shadow-sm);
      overflow: hidden;
    }
    .stat-card:hover {
      border-color: var(--border-bright);
      box-shadow: var(--shadow-md);
    }
    .stat-label {
      font-size: 10.5px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.1em;
      color: var(--text-dim);
      margin-bottom: 8px;
    }
    .stat-value {
      font-family: var(--font-mono);
      font-size: 32px;
      font-weight: 600;
      color: var(--text);
      line-height: 1;
      letter-spacing: -0.02em;
    }
    .stat-value.accent { color: var(--accent); }
    .stat-sub {
      font-size: 11.5px;
      color: var(--text-faint);
      margin-top: 6px;
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
      position: relative;
      flex: 1 1 280px;
      min-width: 200px;
    }
    .search-box input {
      width: 100%;
      padding: 9px 12px 9px 38px;
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      background: var(--surface);
      font-family: var(--font-sans);
      font-size: 14px;
      color: var(--text);
      outline: none;
      transition: border-color 0.15s, box-shadow 0.15s;
      box-shadow: var(--shadow-sm);
    }
    .search-box input:hover { border-color: var(--border-bright); }
    .search-box input:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 3px var(--accent-dim);
    }
    .search-box input::placeholder { color: var(--text-faint); }
    .search-icon {
      position: absolute;
      left: 12px;
      top: 50%;
      transform: translateY(-50%);
      width: 16px;
      height: 16px;
      color: var(--text-faint);
      pointer-events: none;
    }
    .search-kbd {
      position: absolute;
      right: 10px;
      top: 50%;
      transform: translateY(-50%);
      font-family: var(--font-mono);
      font-size: 11px;
      font-weight: 500;
      color: var(--text-dim);
      background: var(--surface-bright);
      border: 1px solid var(--border-bright);
      border-radius: var(--r-xs);
      padding: 1px 6px;
      line-height: 1.4;
    }
    .search-count {
      font-family: var(--font-mono);
      font-size: 12px;
      color: var(--text-dim);
      white-space: nowrap;
    }
    .tool-btn {
      display: inline-flex;
      align-items: center;
      gap: 7px;
      padding: 8px 13px;
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      background: var(--surface);
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 500;
      color: var(--text-2);
      cursor: pointer;
      transition: all 0.15s;
      white-space: nowrap;
      box-shadow: var(--shadow-sm);
    }
    .tool-btn:hover {
      border-color: var(--border-bright);
      color: var(--text);
      background: var(--surface-raised);
    }
    .tool-btn.active {
      background: var(--accent-dim);
      border-color: rgba(31,95,232,0.3);
      color: var(--accent);
    }
    .tool-btn svg { width: 14px; height: 14px; flex-shrink: 0; }
    .refresh-btn {
      display: inline-flex;
      align-items: center;
      gap: 7px;
      padding: 8px 16px;
      border: 1px solid var(--accent);
      border-radius: var(--r-md);
      background: var(--accent);
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 600;
      color: #fff;
      cursor: pointer;
      transition: all 0.15s;
      white-space: nowrap;
      margin-left: auto;
      box-shadow: 0 1px 3px rgba(31,95,232,0.25);
    }
    .refresh-btn:hover { background: var(--accent-hover); border-color: var(--accent-hover); }
    .refresh-btn:disabled { opacity: 0.6; cursor: not-allowed; }
    .refresh-btn svg { width: 14px; height: 14px; flex-shrink: 0; }
    .refresh-btn.is-loading svg { animation: spin 1s linear infinite; }
    @keyframes spin { to { transform: rotate(360deg); } }

    /* ── Banner ── */
    .banner {
      display: flex;
      align-items: center;
      gap: 10px;
      padding: 11px 14px;
      border-radius: var(--r-md);
      margin-bottom: 16px;
      font-size: 13.5px;
      border: 1px solid;
      box-shadow: var(--shadow-sm);
    }
    .banner svg { width: 18px; height: 18px; flex-shrink: 0; }
    .banner.info { background: var(--accent-soft); border-color: rgba(31,95,232,0.15); color: var(--accent); }
    .banner.error { background: var(--danger-dim); border-color: rgba(220,38,38,0.18); color: var(--danger); }
    .banner.success { background: var(--success-dim); border-color: rgba(21,128,61,0.18); color: var(--success); }
    .banner-text { color: var(--text-2); flex: 1; }
    .banner-text strong { font-weight: 600; color: var(--text); }

    /* ── Distribution bar ── */
    .dist-bar-wrap {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      padding: 14px 18px;
      margin-bottom: 16px;
      box-shadow: var(--shadow-sm);
    }
    .dist-bar-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      margin-bottom: 12px;
    }
    .dist-bar-title {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.1em;
      color: var(--text-dim);
    }
    .dist-bar {
      display: flex;
      height: 8px;
      border-radius: var(--r-xs);
      overflow: hidden;
      gap: 2px;
      background: var(--surface-bright);
    }
    .dist-seg {
      border-radius: 2px;
      min-width: 4px;
      transition: flex 0.4s ease;
    }
    .dist-legend {
      display: flex;
      flex-wrap: wrap;
      gap: 12px 18px;
      margin-top: 12px;
    }
    .dist-legend-item {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      font-size: 12.5px;
      color: var(--text-dim);
    }
    .dist-legend-dot {
      width: 8px;
      height: 8px;
      border-radius: 50%;
      flex-shrink: 0;
    }
    .dist-legend-count {
      font-family: var(--font-mono);
      font-weight: 600;
      color: var(--text);
    }

    /* ── Date range filter ── */
    .date-filter {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 8px;
      font-size: 12.5px;
      color: var(--text-muted);
    }
    .date-filter input[type="date"] {
      border: none;
      background: transparent;
      padding: 2px 4px;
      font-size: 12.5px;
      color: var(--text);
      font-family: inherit;
    }
    .date-filter input[type="date"]:focus {
      outline: none;
      background: var(--accent-soft);
      border-radius: 4px;
    }
    .date-filter-label {
      font-size: 11px;
      font-weight: 600;
      letter-spacing: 0.04em;
      text-transform: uppercase;
      color: var(--text-faint);
    }
    .date-filter-sep { color: var(--text-faint); }
    .date-filter.active {
      border-color: var(--accent);
      background: var(--accent-soft);
    }
    .date-filter-clear {
      border: none;
      background: transparent;
      color: var(--text-muted);
      cursor: pointer;
      font-size: 16px;
      line-height: 1;
      padding: 0 4px;
      border-radius: 4px;
    }
    .date-filter-clear:hover { color: var(--text); background: var(--surface-bright); }

    /* ── Belt filter pills ── */
    .belt-tabs {
      display: flex;
      flex-wrap: wrap;
      gap: 8px;
      margin-bottom: 14px;
    }
    .belt-tab {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 6px 12px;
      font-size: 13px;
      font-weight: 500;
      color: var(--text-muted);
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: 999px;
      cursor: pointer;
      transition: background 0.12s, border-color 0.12s, color 0.12s;
    }
    .belt-tab:hover {
      background: var(--surface-bright);
      color: var(--text);
    }
    .belt-tab.active {
      background: var(--accent);
      border-color: var(--accent);
      color: #fff;
    }
    .belt-tab .belt-tab-count {
      font-family: var(--font-mono);
      font-size: 12px;
      opacity: 0.85;
    }

    /* ── Groups ── */
    .groups { display: flex; flex-direction: column; gap: 14px; }
    .group {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      overflow: hidden;
      box-shadow: var(--shadow-sm);
      transition: border-color 0.15s, box-shadow 0.15s;
    }
    .group:hover { border-color: var(--border-bright); box-shadow: var(--shadow-md); }
    .group-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      padding: 14px 18px;
      cursor: pointer;
      user-select: none;
      transition: background 0.15s;
    }
    .group-header:hover { background: var(--surface-raised); }
    .group-header-left {
      display: flex;
      align-items: center;
      gap: 12px;
    }
    .group-indicator {
      width: 10px;
      height: 10px;
      border-radius: 50%;
      flex-shrink: 0;
      box-shadow: 0 0 0 3px var(--surface), 0 0 8px var(--glow-color, transparent);
    }
    .group-name {
      font-weight: 600;
      font-size: 14.5px;
      color: var(--text);
      letter-spacing: -0.005em;
    }
    .group-badge {
      font-family: var(--font-mono);
      font-size: 12px;
      font-weight: 600;
      padding: 3px 9px;
      border-radius: 999px;
      letter-spacing: 0.02em;
    }
    .group-chevron {
      width: 16px;
      height: 16px;
      color: var(--text-faint);
      transition: transform 0.2s;
      flex-shrink: 0;
    }
    .group.collapsed .group-chevron { transform: rotate(-90deg); }
    .group-body { border-top: 1px solid var(--border); }
    .group.collapsed .group-body { display: none; }

    /* ── Villages drill-down ── */
    .villages-btn {
      display: inline-flex;
      align-items: center;
      gap: 4px;
      padding: 3px 8px;
      font-size: 11.5px;
      font-weight: 500;
      color: var(--text-muted);
      background: transparent;
      border: 1px solid var(--border);
      border-radius: 6px;
      cursor: pointer;
      transition: background 0.12s, border-color 0.12s, color 0.12s;
    }
    .villages-btn svg { width: 12px; height: 12px; }
    .villages-btn:hover {
      background: var(--surface-bright);
      color: var(--text);
      border-color: var(--accent);
    }
    .villages-popover {
      position: absolute;
      z-index: 10;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      box-shadow: var(--shadow-lg);
      padding: 10px 12px;
      min-width: 200px;
      max-height: 320px;
      overflow-y: auto;
      font-size: 13px;
    }
    .villages-popover-title {
      font-size: 11px;
      font-weight: 600;
      color: var(--text-muted);
      text-transform: uppercase;
      letter-spacing: 0.05em;
      margin-bottom: 6px;
    }
    .villages-popover-row {
      display: flex;
      justify-content: space-between;
      gap: 12px;
      padding: 4px 0;
    }
    .villages-popover-row .v-name { color: var(--text); }
    .villages-popover-row .v-count {
      font-family: var(--font-mono);
      color: var(--text-muted);
      font-weight: 600;
    }
    .villages-popover-empty {
      color: var(--text-faint);
      font-style: italic;
      padding: 6px 0;
    }
    .villages-popover-row.clickable {
      cursor: pointer;
      border-radius: var(--r-xs);
      padding: 4px 6px;
      margin: 0 -6px;
      transition: background 0.1s;
    }
    .villages-popover-row.clickable:hover {
      background: var(--accent-soft);
    }
    .villages-popover-row.active {
      background: var(--accent-dim);
      font-weight: 600;
    }
    .villages-popover-row.active .v-name {
      color: var(--accent);
    }

    /* ── Village filter chip (toolbar) ── */
    .village-filter-chip {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 5px 10px 5px 12px;
      font-size: 12.5px;
      font-weight: 600;
      color: var(--accent);
      background: var(--accent-dim);
      border: 1px solid rgba(31,95,232,0.25);
      border-radius: 999px;
      white-space: nowrap;
      animation: chip-in 0.2s ease-out;
    }
    @keyframes chip-in {
      from { opacity: 0; transform: scale(0.92); }
      to   { opacity: 1; transform: scale(1); }
    }
    .village-filter-chip svg { width: 12px; height: 12px; flex-shrink: 0; }
    .village-filter-chip .vf-label {
      font-size: 10px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.06em;
      color: var(--text-dim);
    }
    .village-filter-clear {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      width: 18px;
      height: 18px;
      border: none;
      background: transparent;
      color: var(--accent);
      cursor: pointer;
      border-radius: 50%;
      font-size: 14px;
      line-height: 1;
      padding: 0;
      transition: background 0.12s;
    }
    .village-filter-clear:hover {
      background: rgba(31,95,232,0.15);
    }

    /* ── Table ── */
    .tbl-wrap { overflow-x: auto; }
    table {
      width: 100%;
      min-width: 1080px;
      border-collapse: collapse;
      table-layout: fixed;
    }
    body.show-debug table { min-width: 1260px; }
    th, td {
      padding: 11px 14px;
      text-align: left;
      border-bottom: 1px solid var(--border);
      overflow-wrap: anywhere;
      word-break: break-word;
    }
    th {
      font-size: 10.5px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--text-dim);
      background: var(--surface-raised);
      position: sticky;
      top: 0;
      white-space: nowrap;
    }
    td {
      color: var(--text-2);
      vertical-align: top;
      font-size: 13px;
      line-height: 1.45;
    }
    tbody tr:last-child td { border-bottom: none; }
    .mono { font-family: var(--font-mono); font-size: 12px; color: var(--text); }
    .desc-cell { word-break: break-word; }
    tr:hover td { background: var(--surface-raised); }

    /* ── Debug columns — hidden by default ── */
    .debug-col { display: none; }
    body.show-debug .debug-col { display: table-cell; }
    body.show-debug col.debug-col { display: table-column; }

    /* ── Resolve button ── */
    .resolve-btn {
      display: inline-flex;
      align-items: center;
      gap: 5px;
      padding: 5px 11px;
      border: 1px solid rgba(21,128,61,0.3);
      border-radius: var(--r-sm);
      background: var(--success-dim);
      font-family: var(--font-sans);
      font-size: 12px;
      font-weight: 600;
      color: var(--success);
      cursor: pointer;
      transition: all 0.15s;
      white-space: nowrap;
    }
    .resolve-btn:hover {
      background: var(--success);
      color: #fff;
      border-color: var(--success);
    }
    .resolve-btn:disabled { opacity: 0.5; cursor: not-allowed; }
    .resolve-btn svg { width: 13px; height: 13px; flex-shrink: 0; }

    /* Resolved row — dimmed + non-interactive */
    tr.row-resolved {
      opacity: 0.35;
      pointer-events: none;
      transition: opacity 0.3s ease;
    }

    /* ── Modal ── */
    .modal-backdrop {
      display: none;
      position: fixed;
      inset: 0;
      z-index: 10000;
      background: rgba(20,20,30,0.4);
      backdrop-filter: blur(6px);
      align-items: center;
      justify-content: center;
      padding: 16px;
    }
    .modal-backdrop.open { display: flex; }
    .modal {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      padding: 24px;
      width: 100%;
      max-width: 440px;
      box-shadow: 0 16px 48px rgba(20,16,8,0.18);
      animation: modal-in 0.2s ease-out;
    }
    @keyframes modal-in {
      from { opacity: 0; transform: scale(0.96) translateY(8px); }
      to { opacity: 1; transform: scale(1) translateY(0); }
    }
    .modal-title {
      font-size: 17px;
      font-weight: 700;
      color: var(--text);
      margin-bottom: 4px;
      letter-spacing: -0.01em;
    }
    .modal-sub {
      font-size: 13px;
      color: var(--text-dim);
      margin-bottom: 18px;
    }
    .modal-label {
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      color: var(--text-dim);
      margin-bottom: 6px;
    }
    .modal-textarea {
      width: 100%;
      padding: 10px 12px;
      border: 1px solid var(--border);
      border-radius: var(--r-sm);
      background: var(--bg);
      font-family: var(--font-sans);
      font-size: 13px;
      color: var(--text);
      resize: vertical;
      outline: none;
      transition: border-color 0.15s, box-shadow 0.15s;
    }
    .modal-textarea:focus {
      border-color: var(--accent);
      box-shadow: 0 0 0 3px var(--accent-dim);
    }
    .modal-actions {
      display: flex;
      justify-content: flex-end;
      gap: 8px;
      margin-top: 18px;
    }
    .modal-cancel {
      padding: 8px 14px;
      border: 1px solid var(--border);
      border-radius: var(--r-sm);
      background: var(--surface);
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 500;
      color: var(--text-2);
      cursor: pointer;
      transition: all 0.15s;
    }
    .modal-cancel:hover { background: var(--surface-raised); color: var(--text); }
    .modal-confirm {
      padding: 8px 14px;
      border: 1px solid var(--success);
      border-radius: var(--r-sm);
      background: var(--success);
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 600;
      color: #fff;
      cursor: pointer;
      transition: all 0.15s;
      display: inline-flex;
      align-items: center;
      gap: 6px;
    }
    .modal-confirm:hover { opacity: 0.9; }
    .modal-confirm:disabled { opacity: 0.5; cursor: not-allowed; }
    .modal-spinner {
      display: inline-block;
      width: 14px;
      height: 14px;
      border: 2px solid rgba(255,255,255,0.3);
      border-top-color: #fff;
      border-radius: 50%;
      animation: spin 0.6s linear infinite;
    }

    /* ── Site footer ── */
    .site-footer {
      text-align: center;
      padding: 32px 0 18px;
      font-family: var(--font-mono);
      font-size: 11px;
      color: var(--text-faint);
      letter-spacing: 0.04em;
    }

    /* ── Empty / error states ── */
    .empty-state {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 8px;
      padding: 56px 24px;
      text-align: center;
      color: var(--text-dim);
      font-size: 14px;
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      box-shadow: var(--shadow-sm);
    }
    .empty-state strong { color: var(--text); font-size: 15px; font-weight: 600; }
    .error-box {
      padding: 16px 18px;
      border-radius: var(--r-md);
      background: var(--danger-dim);
      border: 1px solid rgba(220,38,38,0.2);
      color: var(--danger);
      font-size: 13px;
    }

    /* ── Print-only header (hidden on screen) ── */
    .print-only-header { display: none; }

    /* ── Mobile responsive ── */
    @media (max-width: 768px) {
      html, body { overflow-x: hidden; }
      body { font-size: 13px; }
      .shell { padding: 12px 12px 32px; max-width: 100%; }

      .topbar {
        flex-wrap: wrap;
        gap: 8px;
        padding-bottom: 12px;
        margin-bottom: 14px;
      }
      .logo { font-size: 14px; }
      .status-chip { font-size: 9.5px; padding: 4px 9px; letter-spacing: 0.06em; }
      .topbar-left { flex: 1 1 auto; min-width: 0; }
      .topbar-right {
        width: 100%;
        justify-content: flex-end;
        gap: 6px;
      }
      .updated-ago, .ws-status { font-size: 10px; padding: 3px 8px; }

      .stats-row {
        grid-template-columns: 1fr 1fr;
        gap: 8px;
        margin-bottom: 12px;
      }
      .stat-card { padding: 11px 12px; }
      .stat-value { font-size: 22px; }
      .stat-label { font-size: 9px; letter-spacing: 0.06em; margin-bottom: 6px; }
      .stat-sub { font-size: 10px; margin-top: 4px; }

      .toolbar { gap: 8px; }
      .search-box { flex: 1 1 100%; min-width: 0; }
      .date-filter { width: 100%; box-sizing: border-box; }
      .search-box input {
        font-size: 16px;       /* prevents iOS auto-zoom on focus */
        padding: 9px 12px 9px 36px;
      }
      .search-count { width: 100%; order: 5; font-size: 11px; }
      .tool-btn, .refresh-btn {
        flex: 1 1 0;
        justify-content: center;
        margin-left: 0;
        padding: 9px 10px;
        font-size: 12.5px;
      }

      .banner { font-size: 12.5px; padding: 10px 12px; }
      .dist-bar-wrap { padding: 11px 12px; }
      .dist-bar-title { font-size: 10px; letter-spacing: 0.08em; }
      .dist-legend { gap: 6px 12px; font-size: 11.5px; }

      .group-header { padding: 11px 12px; }
      .group-name { font-size: 13px; }
      .group-badge { font-size: 11px; padding: 2px 8px; }

      /* Card-based table layout — break out of fixed table layout */
      .tbl-wrap { overflow: visible; padding: 4px 8px 8px; }
      table {
        display: block;
        min-width: 0 !important;
        width: 100% !important;
        table-layout: auto !important;
      }
      thead, tbody, tr, th, td { display: block; }
      thead { display: none; }
      colgroup { display: none; }

      tbody tr {
        background: var(--surface);
        border: 1px solid var(--border);
        border-radius: var(--r-sm);
        margin: 8px 0;
        padding: 8px 10px 10px;
        box-shadow: var(--shadow-sm);
      }
      tbody tr:last-child td { border-bottom: 1px solid var(--border); }
      tr:hover td { background: transparent; }

      td {
        display: grid;
        grid-template-columns: 76px minmax(0, 1fr);
        column-gap: 10px;
        align-items: baseline;
        padding: 5px 0;
        border-bottom: 1px solid var(--border);
        text-align: left;
        font-size: 12.5px;
        line-height: 1.45;
        min-width: 0;
        word-break: break-word;
        overflow-wrap: anywhere;
      }
      td:last-child { border-bottom: none; }
      td::before {
        content: attr(data-label);
        font-weight: 600;
        font-size: 9.5px;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        color: var(--text-dim);
        line-height: 1.6;
      }
      .mono { font-size: 11.5px; }
      .desc-cell, td[data-label="Address"] {
        grid-template-columns: minmax(0, 1fr);
        row-gap: 2px;
      }
      .action-col {
        margin-top: 4px;
        padding-top: 9px !important;
        border-top: 1px dashed var(--border);
        border-bottom: none !important;
        display: block !important;
      }
      .action-col::before { display: none; }
      .resolve-btn {
        width: 100%;
        justify-content: center;
        padding: 8px 14px;
        font-size: 13px;
      }
      .modal { padding: 20px; border-radius: var(--r-md); }
      .modal-title { font-size: 16px; }
    }

    @media (max-width: 420px) {
      .shell { padding: 10px 10px 28px; }
      .stats-row { grid-template-columns: 1fr; }
      .tool-btn, .refresh-btn { flex: 1 1 100%; }
      .stat-value { font-size: 20px; }
      td { grid-template-columns: 70px minmax(0, 1fr); column-gap: 8px; }
    }

    /* ── Print styles ── */
    @page { margin: 12mm 10mm 14mm; size: A4; }

    @media print {
      * { transition: none !important; animation: none !important; }
      html, body {
        background: #fff !important;
        color: #000 !important;
        font-family: 'Georgia', 'Times New Roman', serif;
        font-size: 10pt;
        line-height: 1.4;
      }
      body::before { display: none !important; }

      .shell { max-width: 100%; padding: 0; margin: 0; }

      /* Hide UI chrome */
      .topbar, .stats-row, .toolbar, .banner, .dist-bar-wrap, .ws-status, .site-footer,
      #resolveModal, .empty-state, .search-count, .search-kbd,
      .debug-col, .complaint-col, .action-col,
      .resolve-btn, .group-chevron { display: none !important; }

      /* Print header */
      .print-only-header {
        display: block !important;
        border-bottom: 2px solid #000;
        padding-bottom: 6pt;
        margin-bottom: 10pt;
      }
      .print-brand {
        font-family: 'Georgia', serif;
        font-size: 16pt;
        font-weight: 700;
        letter-spacing: -0.01em;
        color: #000;
      }
      .print-sub {
        font-size: 8.5pt;
        color: #555;
        font-style: italic;
        margin-top: 1pt;
      }
      .print-meta {
        display: flex;
        flex-wrap: wrap;
        gap: 16pt;
        font-size: 9pt;
        color: #333;
        margin-top: 6pt;
      }
      .print-meta strong { color: #000; font-weight: 700; }
      .print-meta-label {
        text-transform: uppercase;
        letter-spacing: 0.06em;
        font-size: 7.5pt;
        color: #777;
        font-family: 'Helvetica', sans-serif;
        margin-right: 2pt;
      }

      /* Groups */
      .groups { display: block; }
      .group {
        margin-bottom: 14pt;
        border: none !important;
        border-radius: 0 !important;
        overflow: visible !important;
        box-shadow: none !important;
        background: transparent !important;
        /* Each belt starts on its own page so the printed report can be
           handed out belt-by-belt. break-inside: avoid keeps a single belt
           together when its rows fit on one page; tall belts will still
           split across pages naturally. */
        break-before: page;
        page-break-before: always;
        break-inside: auto;
        page-break-inside: auto;
      }
      /* Don't force a page break before the very first group — that would
         leave the print header alone on page 1. */
      .group:first-of-type {
        break-before: auto;
        page-break-before: auto;
      }
      .group:last-child { margin-bottom: 0; }
      .group-header {
        display: flex !important;
        background: #f0eee9 !important;
        padding: 5pt 8pt !important;
        border: 1px solid #000 !important;
        border-bottom: none !important;
        border-radius: 0 !important;
        cursor: default !important;
        print-color-adjust: exact;
        -webkit-print-color-adjust: exact;
      }
      .group-header-left { gap: 6pt; display: flex; align-items: center; }
      .group-indicator {
        width: 9pt; height: 9pt;
        border-radius: 50%;
        display: inline-block;
        box-shadow: none !important;
        border: 1px solid #000;
        print-color-adjust: exact;
        -webkit-print-color-adjust: exact;
      }
      .group-name {
        font-family: 'Georgia', serif;
        font-weight: 700;
        font-size: 11pt;
        color: #000;
      }
      .group-badge {
        font-family: 'Helvetica', sans-serif;
        font-size: 8.5pt;
        font-weight: 700;
        padding: 1pt 6pt;
        border-radius: 8pt;
        border: 1px solid #000;
        background: #fff !important;
        color: #000 !important;
      }
      .group-body { display: block !important; border-top: none !important; }
      .group.collapsed .group-body { display: block !important; }

      /* Table — restore table layout (override mobile cards) */
      .tbl-wrap { overflow: visible !important; padding: 0 !important; }
      table {
        display: table !important;
        width: 100% !important;
        min-width: 0 !important;
        border-collapse: collapse;
        table-layout: fixed !important;
        font-size: 8.5pt;
        border: 1px solid #000;
        border-top: none;
      }
      colgroup { display: table-column-group !important; }

      /* Print column widths (visible: Name, Consumer, Mobile, Address, Area, Description, Date) */
      colgroup col:nth-child(1) { width: 0 !important; }   /* Complaint — hidden */
      colgroup col:nth-child(2) { width: 13% !important; } /* Name */
      colgroup col:nth-child(3) { width: 11% !important; } /* Consumer */
      colgroup col:nth-child(4) { width: 11% !important; } /* Mobile */
      colgroup col:nth-child(5) { width: 21% !important; } /* Address */
      colgroup col:nth-child(6) { width: 13% !important; } /* Area */
      colgroup col:nth-child(7) { width: 19% !important; } /* Description */
      colgroup col:nth-child(8) { width: 12% !important; } /* Date */
      colgroup col:nth-child(9),
      colgroup col:nth-child(10),
      colgroup col:nth-child(11) { width: 0 !important; }  /* Telegram, WhatsApp, Action — hidden */

      thead { display: table-header-group !important; }
      tbody { display: table-row-group !important; }
      tr { display: table-row !important; box-shadow: none !important; background: transparent !important; border: none !important; margin: 0 !important; padding: 0 !important; border-radius: 0 !important; }
      th, td {
        display: table-cell !important;
        padding: 3pt 5pt;
        border: 1px solid #999;
        text-align: left;
        word-wrap: break-word;
        white-space: normal;
        color: #000;
        vertical-align: top;
        background: transparent !important;
        font-size: 8.5pt;
        line-height: 1.35;
        grid-template-columns: none !important;
      }
      th {
        background: #ececec !important;
        font-family: 'Helvetica', sans-serif;
        font-weight: 700;
        font-size: 7.5pt;
        text-transform: uppercase;
        letter-spacing: 0.06em;
        position: static;
        print-color-adjust: exact;
        -webkit-print-color-adjust: exact;
      }
      td::before { display: none !important; }
      tbody tr { page-break-inside: avoid; }
      tr:hover td { background: transparent !important; }
      .mono { font-family: 'Courier New', monospace; font-size: 8pt; }
    }

  </style>
</head>
<body>
  <main class="shell">
    <!-- Print-only header (hidden on screen) -->
    <header class="print-only-header" aria-hidden="true">
      <div class="print-brand">CMON — Complaint Monitor</div>
      <div class="print-sub">DGVCL Pending Complaints Report</div>
      <div class="print-meta">
        <span><span class="print-meta-label">Printed</span><strong id="printDate">—</strong></span>
        <span><span class="print-meta-label">Pending</span><strong id="printTotal">—</strong></span>
        <span><span class="print-meta-label">Belts</span><strong id="printGroups">—</strong></span>
        <span id="printFiltersWrap" hidden><span class="print-meta-label">Filters</span><strong id="printFilters">—</strong></span>
      </div>
    </header>

    <!-- Top bar -->
    <header class="topbar">
      <div class="topbar-left">
        <div class="logo">CMON<span>.</span></div>
        <div id="statusChip" class="status-chip loading">Connecting</div>
      </div>
      <div class="topbar-right">
        <span id="wsStatus" class="ws-status" style="display:none"></span>
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
    </section>

    <!-- Distribution bar -->
    <section class="dist-bar-wrap" id="distBarWrap" style="display:none">
      <div class="dist-bar-header">
        <div class="dist-bar-title">Complaint Distribution</div>
      </div>
      <div class="dist-bar" id="distBar"></div>
      <div class="dist-legend" id="distLegend"></div>
    </section>

    <!-- Belt filter pills -->
    <nav class="belt-tabs" id="beltTabs" role="tablist" aria-label="Filter by belt"></nav>

    <!-- Toolbar -->
    <section class="toolbar">
      <div class="search-box">
        <svg class="search-icon" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><circle cx="11" cy="11" r="8"/><line x1="21" y1="21" x2="16.65" y2="16.65"/></svg>
        <input id="searchInput" type="search" placeholder="Search complaints..." autocomplete="off" spellcheck="false">
        <span class="search-kbd" id="searchKbd">/</span>
      </div>
      <span class="search-count" id="searchCount"></span>
      <span id="villageFilterChip" style="display:none"></span>

      <div class="date-filter" title="Filter complaints by complain date (inclusive)">
        <span class="date-filter-label">Date</span>
        <input id="fromDate" type="date" aria-label="From date">
        <span class="date-filter-sep">→</span>
        <input id="toDate" type="date" aria-label="To date">
        <button id="dateClearBtn" class="date-filter-clear" type="button" title="Clear date filter" hidden>&times;</button>
      </div>

      <button id="debugToggle" class="tool-btn" type="button" title="Toggle debug columns (Telegram/WhatsApp IDs)">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M12 20h9"/><path d="M16.5 3.5a2.12 2.12 0 013 3L7 19l-4 1 1-4z"/></svg>
        Debug
      </button>

      <button id="printBtn" class="tool-btn" type="button" title="Print complaints">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 6 2 18 2 18 9"/><path d="M6 18H4a2 2 0 01-2-2v-5a2 2 0 012-2h16a2 2 0 012 2v5a2 2 0 01-2 2h-2"/><rect x="6" y="14" width="12" height="8"/></svg>
        Print
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

  <footer class="site-footer">Made by Satyam</footer>

  <!-- Resolve Modal -->
  <div id="resolveModal" class="modal-backdrop" role="dialog" aria-modal="true" aria-labelledby="modalTitle">
    <div class="modal">
      <div class="modal-title" id="modalTitle">✅ Mark as Resolved</div>
      <div class="modal-sub" id="modalSub">Complaint <strong id="modalComplaintNo"></strong></div>
      <div class="modal-label">Remark (optional)</div>
      <textarea id="modalRemark" class="modal-textarea" placeholder="Enter resolution note..." rows="3"></textarea>
      <div class="modal-actions">
        <button id="modalCancelBtn" class="modal-cancel" type="button">Cancel</button>
        <button id="modalConfirmBtn" class="modal-confirm" type="button">Mark Resolved</button>
      </div>
    </div>
  </div>

  <script>
    (() => {
      const DATA_URL = {{.DataURL}};
      const WS_URL = (location.protocol === "https:" ? "wss://" : "ws://") + location.host + "/ws";

      // WebSocket connection
      let ws = null;
      let wsReconnectTimer = null;
      let wsConnected = false;

      function connectWS() {
        if (ws && ws.readyState === WebSocket.OPEN) return;

        try {
          ws = new WebSocket(WS_URL);
        } catch (e) {
          console.error("WebSocket creation failed:", e);
          scheduleReconnect();
          return;
        }

        ws.onopen = () => {
          console.log("📡 WebSocket connected");
          wsConnected = true;
          updateWSStatus(true);
          if (wsReconnectTimer) { clearTimeout(wsReconnectTimer); wsReconnectTimer = null; }
        };

        ws.onmessage = (event) => {
          try {
            const msg = JSON.parse(event.data);
            console.log("📥 WebSocket message:", msg);

            if (msg.type === "refresh" || msg.type === "resolved") {
              loadData({ silent: true });
            }
          } catch (e) {
            console.error("Failed to parse WebSocket message:", e);
          }
        };

        ws.onclose = () => {
          console.log("📡 WebSocket disconnected");
          wsConnected = false;
          updateWSStatus(false);
          scheduleReconnect();
        };

        ws.onerror = (err) => {
          console.error("WebSocket error:", err);
        };
      }

      function scheduleReconnect() {
        if (wsReconnectTimer) return;
        const delay = Math.min(30000, 1000 * Math.pow(2, (connectWS.reconnectCount || 0)));
        connectWS.reconnectCount = (connectWS.reconnectCount || 0) + 1;
        console.log("📡 Scheduling reconnect in " + delay + "ms");
        wsReconnectTimer = setTimeout(() => {
          wsReconnectTimer = null;
          connectWS();
        }, delay);
      }

      function updateWSStatus(connected) {
        const el = document.getElementById("wsStatus");
        if (!el) return;
        el.style.display = "";
        el.className = "ws-status " + (connected ? "connected" : "disconnected");
        el.textContent = connected ? "● Live" : "○ Reconnecting...";
      }

      connectWS();

      // DOM refs
      const $ = (id) => document.getElementById(id);
      const searchInput = $("searchInput");
      const searchKbd = $("searchKbd");
      const searchCountEl = $("searchCount");
      const debugToggleBtn = $("debugToggle");
      const printBtn = $("printBtn");
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
      const beltTabsEl = $("beltTabs");
      const fromDateEl = $("fromDate");
      const toDateEl = $("toDate");
      const dateClearBtn = $("dateClearBtn");
      const dateFilterEl = document.querySelector(".date-filter");

      let payload = null;
      let isLoading = false;
      let lastLoadTime = null;
      let agoTimer = null;

      // activeBelt is "" for "all belts" or a canonical belt key (the same key
      // the server uses in groups[].belt). Initialised from the ?belt= query
      // string so a deep-link or refresh preserves the filter.
      let activeBelt = (new URLSearchParams(location.search).get("belt") || "").trim();

      // Date range filter (inclusive). Strings in YYYY-MM-DD form because that
      // is what <input type="date"> emits and consumes. "" means unset on
      // either side. Initialised from ?from= / ?to= URL params for deep-links.
      let activeFromDate = (new URLSearchParams(location.search).get("from") || "").trim();
      let activeToDate = (new URLSearchParams(location.search).get("to") || "").trim();
      let collapsedBelts = new Set();

      // Village filter — scoped to the active belt. Cleared when the belt
      // changes because village names are belt-specific.
      let activeVillage = (new URLSearchParams(location.search).get("village") || "").trim();

      // Utils
      const esc = (v) => String(v ?? "").replace(/&/g,"&amp;").replace(/</g,"&lt;").replace(/>/g,"&gt;").replace(/"/g,"&quot;").replace(/'/g,"&#39;");

      // parseComplainDate mirrors the Go parseComplaintDate in summary/image.go.
      // Accepts the DGVCL date formats and returns a YYYY-MM-DD string suitable
      // for direct comparison against <input type="date"> values, or "" if
      // unparseable. Date-only comparison is what the date-range filter wants;
      // we don't need the time component.
      function parseComplainDateYMD(raw) {
        if (!raw) return "";
        const s = String(raw).trim();
        if (!s) return "";
        // ISO: 2026-03-04 [time]
        let m = s.match(/^(\d{4})-(\d{2})-(\d{2})/);
        if (m) return m[1] + "-" + m[2] + "-" + m[3];
        // DD-MM-YYYY [time]
        m = s.match(/^(\d{2})-(\d{2})-(\d{4})/);
        if (m) return m[3] + "-" + m[2] + "-" + m[1];
        // DD/MM/YYYY [time]
        m = s.match(/^(\d{2})\/(\d{2})\/(\d{4})/);
        if (m) return m[3] + "-" + m[2] + "-" + m[1];
        return "";
      }

      // dateInRange returns true when a complaint's complain_date sits within
      // [from, to] inclusive. Empty bounds are treated as open-ended. A
      // complaint with an unparseable date is kept when no filter is active
      // and dropped as soon as either bound is set — otherwise the filter
      // would silently let bad rows through.
      function dateInRange(complainDate, from, to) {
        if (!from && !to) return true;
        const ymd = parseComplainDateYMD(complainDate);
        if (!ymd) return false;
        if (from && ymd < from) return false;
        if (to && ymd > to) return false;
        return true;
      }

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

      // Village filter match
      function villageMatches(c, village) {
        if (!village) return true;
        const cv = (c.village || "").trim().toLowerCase();
        return cv === village.toLowerCase();
      }

      // Sync village filter URL param
      function syncVillageFilterURL() {
        const u = new URL(location.href);
        if (activeVillage) u.searchParams.set("village", activeVillage);
        else u.searchParams.delete("village");
        history.replaceState(null, "", u.toString());
      }

      // Render village filter chip in toolbar
      function renderVillageFilterChip() {
        const chip = $("villageFilterChip");
        if (!chip) return;
        if (!activeVillage) {
          chip.style.display = "none";
          chip.innerHTML = "";
          return;
        }
        chip.style.display = "";
        chip.innerHTML =
          '<span class="village-filter-chip">' +
            '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 21h18"/><path d="M5 21V8l7-5 7 5v13"/><path d="M9 21v-6h6v6"/></svg>' +
            '<span class="vf-label">Village</span> ' +
            esc(activeVillage) +
            '<button class="village-filter-clear" type="button" title="Clear village filter">&times;</button>' +
          '</span>';
        chip.querySelector(".village-filter-clear").addEventListener("click", () => {
          activeVillage = "";
          syncVillageFilterURL();
          renderVillageFilterChip();
          render();
        });
      }

      // Build table row
      function buildRow(c) {
        const tg = c.telegram_message_id || "—";
        const wa = c.whatsapp_message_id || "—";
        const apiID = c.api_id || "";
        const resolveBtn = apiID
          ? '<button class="resolve-btn" data-api-id="' + esc(apiID) + '" data-complaint-no="' + esc(c.complain_no || "") + '" title="Mark complaint as resolved on the DGVCL portal">' +
              '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg>' +
              'Resolve' +
            '</button>'
          : '<span style="color:var(--text-faint);font-size:11px">—</span>';
        return '<tr>' +
          '<td data-label="Complaint" class="complaint-col mono">' + esc(c.complain_no || "—") + '</td>' +
          '<td data-label="Name">' + esc(c.name || "—") + '</td>' +
          '<td data-label="Consumer" class="mono">' + esc(c.consumer_no || "—") + '</td>' +
          '<td data-label="Mobile" class="mono">' + esc(c.mobile_no || "—") + '</td>' +
          '<td data-label="Address">' + esc(c.address || "—") + '</td>' +
          '<td data-label="Area">' + esc(c.area || "—") + '</td>' +
          '<td data-label="Description" class="desc-cell">' + esc(c.description || "—") + '</td>' +
          '<td data-label="Date" class="mono">' + esc(c.complain_date || "—") + '</td>' +
          '<td data-label="Telegram" class="debug-col mono">' + esc(tg) + '</td>' +
          '<td data-label="WhatsApp" class="debug-col mono">' + esc(wa) + '</td>' +
          '<td data-label="Action" class="action-col">' + resolveBtn + '</td>' +
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
              '<button class="villages-btn" type="button" data-belt-key="' + esc(key) + '" title="Show village breakdown">' +
                '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><path d="M3 21h18"/><path d="M5 21V8l7-5 7 5v13"/><path d="M9 21v-6h6v6"/></svg>' +
                'Villages' +
              '</button>' +
            '</div>' +
            '<svg class="group-chevron" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2"><polyline points="6 9 12 15 18 9"/></svg>' +
          '</div>' +
          '<div class="group-body"><div class="tbl-wrap"><table>' +
            '<colgroup>' +
              '<col class="complaint-col" style="width:11%">' +
              '<col style="width:12%">' +
              '<col style="width:9%">' +
              '<col style="width:9%">' +
              '<col style="width:15%">' +
              '<col style="width:7%">' +
              '<col style="width:19%">' +
              '<col style="width:9%">' +
              '<col class="debug-col" style="width:7%">' +
              '<col class="debug-col" style="width:7%">' +
              '<col class="action-col" style="width:9%">' +
            '</colgroup>' +
            '<thead><tr>' +
              '<th class="complaint-col">Complaint</th><th>Name</th><th>Consumer</th><th>Mobile</th>' +
              '<th>Address</th><th>Area</th><th>Description</th><th>Date</th>' +
              '<th class="debug-col">Telegram</th><th class="debug-col">WhatsApp</th>' +
              '<th class="action-col">Action</th>' +
            '</tr></thead>' +
            '<tbody>' + rows + '</tbody>' +
          '</table></div></div>' +
        '</div>';
      }

      // Main render
      // renderBeltTabs paints the pill row above the groups. Belts are pulled
      // from the unfiltered payload so the row stays stable as the user types
      // in the search box. activeBelt keys off the same g.belt field that
      // the group cards use, so canonicalisation is the server's job.
      function renderBeltTabs(groups) {
        if (!groups || groups.length === 0) { beltTabsEl.innerHTML = ""; return; }
        const total = groups.reduce((s, g) => s + g.complaints.length, 0);

        // "All belts" pill first, then one per belt in payload order.
        const pills = [
          '<button type="button" class="belt-tab' + (activeBelt === "" ? " active" : "") +
            '" data-belt-key="" role="tab" aria-selected="' + (activeBelt === "" ? "true" : "false") + '">' +
            'All <span class="belt-tab-count">' + total + '</span>' +
          '</button>',
        ].concat(groups.map((g) => {
          const isActive = activeBelt === g.belt;
          return '<button type="button" class="belt-tab' + (isActive ? " active" : "") +
            '" data-belt-key="' + esc(g.belt) + '" role="tab" aria-selected="' + (isActive ? "true" : "false") + '">' +
            esc(g.label) + ' <span class="belt-tab-count">' + g.complaints.length + '</span>' +
          '</button>';
        })).join("");

        beltTabsEl.innerHTML = pills;
        beltTabsEl.querySelectorAll(".belt-tab").forEach((btn) => {
          btn.addEventListener("click", () => {
            const key = btn.dataset.beltKey || "";
            if (key === activeBelt) return;
            activeBelt = key;
            // Clear village filter when switching belts — villages are
            // belt-specific so keeping a stale village filter would hide
            // every complaint in the new belt.
            activeVillage = "";
            syncVillageFilterURL();
            renderVillageFilterChip();
            // Sync the URL so refresh / share preserves the filter without
            // adding a navigation entry per click.
            const u = new URL(location.href);
            if (activeBelt) u.searchParams.set("belt", activeBelt);
            else u.searchParams.delete("belt");
            history.replaceState(null, "", u.toString());
            render();
          });
        });
      }

      function render() {
        if (!payload) { contentEl.innerHTML = ""; return; }

        // Drop the active-belt filter silently if a refresh removed that belt
        // (e.g. last complaint resolved). Keeps the UI from getting stuck on
        // an empty tab.
        if (activeBelt && !payload.groups.some((g) => g.belt === activeBelt)) {
          activeBelt = "";
          const u = new URL(location.href);
          u.searchParams.delete("belt");
          history.replaceState(null, "", u.toString());
        }

        const q = searchInput.value.trim().toLowerCase();
        const filtered = payload.groups
          .filter((g) => activeBelt === "" || g.belt === activeBelt)
          .map((g) => ({
            ...g,
            complaints: g.complaints.filter((c) =>
              matches(c, q) &&
              dateInRange(c.complain_date, activeFromDate, activeToDate) &&
              villageMatches(c, activeVillage)
            ),
          }))
          .filter((g) => g.complaints.length > 0);

        const visCount = filtered.reduce((s, g) => s + g.complaints.length, 0);
        const anyFilter = !!(q || activeBelt || activeFromDate || activeToDate || activeVillage);

        // Stats
        setMetric("totalCount", anyFilter ? visCount : payload.total_count);
        $("totalSub").textContent = anyFilter ? "of " + payload.total_count + " total" : "complaints";
        setMetric("groupCount", anyFilter ? filtered.length : payload.group_count);
        $("groupSub").textContent = anyFilter ? "of " + payload.group_count + " total" : "active belts";

        // Print header sync — reflect what's actually on screen so the printed
        // report's totals match its rows. Also render an active-filter summary
        // so the printed page documents the filter that produced it.
        setMetric("printTotal", anyFilter ? visCount : payload.total_count);
        setMetric("printGroups", anyFilter ? filtered.length : payload.group_count);
        const filterParts = [];
        if (activeBelt) filterParts.push("Belt: " + activeBelt);
        if (activeVillage) filterParts.push("Village: " + activeVillage);
        if (activeFromDate || activeToDate) {
          filterParts.push("Date: " + (activeFromDate || "—") + " → " + (activeToDate || "—"));
        }
        if (q) filterParts.push("Search: " + q);
        const printFilters = $("printFilters");
        const printFiltersWrap = $("printFiltersWrap");
        if (filterParts.length === 0) {
          if (printFiltersWrap) printFiltersWrap.hidden = true;
          if (printFilters) printFilters.textContent = "—";
        } else {
          if (printFilters) printFilters.textContent = filterParts.join(" · ");
          if (printFiltersWrap) printFiltersWrap.hidden = false;
        }

        // Update the search-count chip to reflect any-filter visibility, not
        // just the search box. Previously this only triggered for q != ""; with
        // the date + belt filters that gave misleading "no count" output when
        // a date-only filter was active.
        if (q) {
          searchCountEl.textContent = visCount + " result" + (visCount === 1 ? "" : "s");
          searchKbd.style.display = "none";
        } else {
          searchCountEl.textContent = "";
          searchKbd.style.display = searchInput === document.activeElement ? "none" : "";
        }

        // Distribution bar + belt-filter tabs (unfiltered — they reflect the
        // full payload, independent of search and active belt).
        renderDistBar(payload.groups);
        renderBeltTabs(payload.groups);
        renderVillageFilterChip();

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

        // Bind resolve buttons
        contentEl.querySelectorAll(".resolve-btn").forEach((btn) => {
          btn.addEventListener("click", (e) => {
            e.stopPropagation();
            openResolveModal(btn.dataset.apiId, btn.dataset.complaintNo, btn);
          });
        });

        // Bind village drill-down buttons
        contentEl.querySelectorAll(".villages-btn").forEach((btn) => {
          btn.addEventListener("click", (e) => {
            e.stopPropagation(); // don't trigger group collapse
            openVillagesPopover(btn.dataset.beltKey, btn);
          });
        });
      }

      // ── Village drill-down popover ──
      // Single shared popover element so we never have two open at once;
      // re-positioned and re-populated each time a button is clicked.
      let villagesPopover = null;

      function closeVillagesPopover() {
        if (villagesPopover) {
          villagesPopover.remove();
          villagesPopover = null;
        }
      }

      async function openVillagesPopover(beltKey, anchor) {
        // Toggle off if the popover already belongs to this anchor.
        if (villagesPopover && villagesPopover.dataset.anchorBelt === beltKey) {
          closeVillagesPopover();
          return;
        }
        closeVillagesPopover();

        // Create immediately with a loading state so the click is responsive
        // even on a slow upstream.
        const pop = document.createElement("div");
        pop.className = "villages-popover";
        pop.dataset.anchorBelt = beltKey;
        pop.innerHTML = '<div class="villages-popover-title">Villages</div>' +
          '<div class="villages-popover-empty">Loading…</div>';
        document.body.appendChild(pop);
        positionPopover(pop, anchor);
        villagesPopover = pop;

        try {
          const resp = await fetch("/villages?belt=" + encodeURIComponent(beltKey));
          if (!resp.ok) throw new Error("HTTP " + resp.status);
          const data = await resp.json();
          if (villagesPopover !== pop) return; // user clicked another button mid-flight
          renderVillagesPopover(pop, data);
          positionPopover(pop, anchor); // re-position now that content size changed
        } catch (err) {
          if (villagesPopover !== pop) return;
          pop.innerHTML = '<div class="villages-popover-title">Villages</div>' +
            '<div class="villages-popover-empty">Failed to load: ' + esc(String(err.message || err)) + '</div>';
        }
      }

      function renderVillagesPopover(pop, data) {
        const villages = (data && data.villages) || [];
        const title = '<div class="villages-popover-title">' +
          esc((data && data.belt) || "Villages") +
          ' <span style="color:var(--text-faint);font-weight:500;text-transform:none;letter-spacing:0">(' + (data.total || 0) + ' total)</span>' +
          '</div>';
        if (villages.length === 0) {
          pop.innerHTML = title + '<div class="villages-popover-empty">No villages on file.</div>';
          return;
        }
        // "All" row to clear the village filter
        const allActive = !activeVillage;
        let allRow = '<div class="villages-popover-row clickable' + (allActive ? ' active' : '') + '" data-village="">' +
          '<span class="v-name">All Villages</span>' +
          '<span class="v-count">' + (data.total || 0) + '</span></div>';

        const rows = villages.map((v) => {
          const isActive = activeVillage && activeVillage.toLowerCase() === v.name.toLowerCase();
          return '<div class="villages-popover-row clickable' + (isActive ? ' active' : '') + '" data-village="' + esc(v.name) + '">' +
            '<span class="v-name">' + esc(v.name) + '</span>' +
            '<span class="v-count">' + v.count + '</span></div>';
        }).join("");
        pop.innerHTML = title + allRow + rows;

        // Bind click handlers for village filtering
        pop.querySelectorAll(".villages-popover-row.clickable").forEach((row) => {
          row.addEventListener("click", () => {
            const village = row.dataset.village || "";
            activeVillage = village;
            // Auto-select the belt this popover belongs to if "All belts" is
            // currently shown, so the village filter is meaningful.
            if (village && !activeBelt && pop.dataset.anchorBelt) {
              activeBelt = pop.dataset.anchorBelt;
              const u = new URL(location.href);
              u.searchParams.set("belt", activeBelt);
              history.replaceState(null, "", u.toString());
            }
            syncVillageFilterURL();
            renderVillageFilterChip();
            closeVillagesPopover();
            render();
          });
        });
      }

      // positionPopover anchors the popover below the button, keeping it in the
      // viewport horizontally so it doesn't get clipped on narrow screens.
      function positionPopover(pop, anchor) {
        const rect = anchor.getBoundingClientRect();
        const popRect = pop.getBoundingClientRect();
        const margin = 8;
        let left = rect.left + window.scrollX;
        if (left + popRect.width + margin > window.innerWidth) {
          left = window.innerWidth - popRect.width - margin;
        }
        if (left < margin) left = margin;
        pop.style.top = (rect.bottom + window.scrollY + 4) + "px";
        pop.style.left = left + "px";
      }

      // Click-outside / Escape closes the popover.
      document.addEventListener("click", (e) => {
        if (!villagesPopover) return;
        if (villagesPopover.contains(e.target)) return;
        if (e.target.closest(".villages-btn")) return;
        closeVillagesPopover();
      });

      // ── Resolve modal ──
      const resolveModal    = $("resolveModal");
      const modalComplaintNo = $("modalComplaintNo");
      const modalRemark     = $("modalRemark");
      const modalCancelBtn  = $("modalCancelBtn");
      const modalConfirmBtn = $("modalConfirmBtn");

      let _pendingAPIID = null;
      let _pendingComplaintNo = null;
      let _pendingBtn = null;

      function openResolveModal(apiID, complaintNo, btn) {
        _pendingAPIID = apiID;
        _pendingComplaintNo = complaintNo;
        _pendingBtn = btn;
        modalComplaintNo.textContent = complaintNo || apiID;
        modalRemark.value = "Restored";
        modalConfirmBtn.disabled = false;
        modalConfirmBtn.innerHTML = "Mark Resolved";
        resolveModal.classList.add("open");
        setTimeout(() => modalRemark.focus(), 150);
      }

      function closeResolveModal() {
        resolveModal.classList.remove("open");
        _pendingAPIID = null;
        _pendingComplaintNo = null;
        _pendingBtn = null;
      }

      modalCancelBtn.addEventListener("click", closeResolveModal);
      resolveModal.addEventListener("click", (e) => { if (e.target === resolveModal) closeResolveModal(); });

      modalConfirmBtn.addEventListener("click", async () => {
        if (!_pendingAPIID) return;
        const savedAPIID = _pendingAPIID;
        const savedComplaintNo = _pendingComplaintNo;
        const savedBtn = _pendingBtn;
        modalConfirmBtn.disabled = true;
        modalConfirmBtn.innerHTML = '<span class="modal-spinner"></span>Resolving...';
        modalCancelBtn.disabled = true;

        try {
          const resp = await fetch("/resolve", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ complaint_id: savedAPIID, remark: modalRemark.value.trim() })
          });
          const data = await resp.json().catch(() => ({}));
          if (!resp.ok) throw new Error(data.error || "Status " + resp.status);

          // Dim the row and disable the button so it looks resolved
          // immediately. No DOM removal — keeps things stable during
          // rapid-fire resolves.
          if (savedBtn) {
            const row = savedBtn.closest("tr");
            if (row) row.classList.add("row-resolved");
            savedBtn.disabled = true;
            savedBtn.innerHTML = '<svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5"><polyline points="20 6 9 17 4 12"/></svg> Resolved';
          }

          // Silently purge from in-memory payload so the complaint
          // won't reappear on the next render() / loadData() cycle.
          if (payload && payload.groups) {
            for (const g of payload.groups) {
              const idx = g.complaints.findIndex(c => (c.api_id || "") === savedAPIID);
              if (idx !== -1) {
                g.complaints.splice(idx, 1);
                g.count = g.complaints.length;
                break;
              }
            }
            payload.groups = payload.groups.filter(g => g.complaints.length > 0);
            payload.total_count = payload.groups.reduce((s, g) => s + g.complaints.length, 0);
            payload.group_count = payload.groups.length;
          }

          closeResolveModal();
          setBanner("success", "<strong>Resolved.</strong> Complaint #" + esc(savedComplaintNo || savedAPIID) + " marked as resolved on the portal.");
        } catch (err) {
          setBanner("error", "<strong>Resolve failed.</strong> " + esc(err.message));
          closeResolveModal();
        } finally {
          modalConfirmBtn.disabled = false;
          modalCancelBtn.disabled = false;
          modalConfirmBtn.innerHTML = "Mark Resolved";
        }
      });

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

      // Initialise the date inputs from URL state on boot.
      if (activeFromDate) fromDateEl.value = activeFromDate;
      if (activeToDate) toDateEl.value = activeToDate;
      updateDateFilterChrome();

      function syncDateFilterURL() {
        const u = new URL(location.href);
        if (activeFromDate) u.searchParams.set("from", activeFromDate); else u.searchParams.delete("from");
        if (activeToDate) u.searchParams.set("to", activeToDate); else u.searchParams.delete("to");
        history.replaceState(null, "", u.toString());
      }

      // updateDateFilterChrome flips the active styling on the wrapper and
      // shows/hides the clear button so the operator can tell at a glance
      // whether a date filter is in effect.
      function updateDateFilterChrome() {
        const active = !!(activeFromDate || activeToDate);
        if (dateFilterEl) dateFilterEl.classList.toggle("active", active);
        if (dateClearBtn) dateClearBtn.hidden = !active;
      }

      fromDateEl.addEventListener("change", () => {
        activeFromDate = fromDateEl.value || "";
        syncDateFilterURL();
        updateDateFilterChrome();
        render();
      });
      toDateEl.addEventListener("change", () => {
        activeToDate = toDateEl.value || "";
        syncDateFilterURL();
        updateDateFilterChrome();
        render();
      });
      dateClearBtn.addEventListener("click", () => {
        activeFromDate = "";
        activeToDate = "";
        fromDateEl.value = "";
        toDateEl.value = "";
        syncDateFilterURL();
        updateDateFilterChrome();
        render();
      });

      debugToggleBtn.addEventListener("click", () => {
        document.body.classList.toggle("show-debug");
        debugToggleBtn.classList.toggle("active", document.body.classList.contains("show-debug"));
      });

      refreshBtn.addEventListener("click", () => loadData({ scrape: true }));
      printBtn.addEventListener("click", () => {
        const now = new Date();
        const stamp = now.toLocaleString(undefined, {
          day: "2-digit", month: "short", year: "numeric",
          hour: "2-digit", minute: "2-digit"
        });
        const pd = $("printDate"); if (pd) pd.textContent = stamp;
        window.print();
      });

      // Keyboard shortcuts
      document.addEventListener("keydown", (e) => {
        if (e.key === "/" && document.activeElement !== searchInput && !e.ctrlKey && !e.metaKey) {
          e.preventDefault();
          searchInput.focus();
        }
        if (e.key === "Escape") {
          if (villagesPopover) { closeVillagesPopover(); return; }
          if (resolveModal.classList.contains("open")) { closeResolveModal(); return; }
          if (document.activeElement === searchInput) {
            searchInput.value = "";
            searchInput.blur();
            render();
          }
        }
      });

      // Boot
      setChip("loading");
      loadData({ silent: true });
    })();
  </script>
</body>
</html>`))
