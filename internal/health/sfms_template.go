package health

import "html/template"

var sfmsPageTemplate = template.Must(template.New("sfms-page").Parse(`<!DOCTYPE html>
<html lang="en">
<head>
  <meta charset="UTF-8">
  <meta name="viewport" content="width=device-width, initial-scale=1.0">
  <title>CMON — SFMS Feeder Monitor</title>
  <link rel="preconnect" href="https://fonts.googleapis.com">
  <link rel="preconnect" href="https://fonts.gstatic.com" crossorigin>
  <link href="https://fonts.googleapis.com/css2?family=JetBrains+Mono:wght@400;500;700&family=DM+Sans:wght@400;500;600;700&display=swap" rel="stylesheet">
  <style>
    :root {
      --bg: #f1ede4;
      --surface: #ffffff;
      --surface-raised: #fbf8f2;
      --surface-bright: #ece6d8;
      --surface-deep: #e3ddcd;

      --border: rgba(40,32,20,0.09);
      --border-bright: rgba(40,32,20,0.16);
      --border-strong: rgba(40,32,20,0.28);

      --text: #14181f;
      --text-2: #2f3742;
      --text-dim: #5b6472;
      --text-faint: #8b95a5;

      --accent: #1f5fe8;
      --accent-hover: #1648c9;
      --accent-dim: rgba(31,95,232,0.10);

      --danger: #dc2626;
      --danger-dim: rgba(220,38,38,0.10);
      --danger-bg: #fef2f2;
      --danger-border: #fecaca;

      --success: #15803d;
      --success-dim: rgba(21,128,61,0.10);
      --success-bg: #f0fdf4;
      --success-border: #bbf7d0;

      --warn: #b45309;
      --warn-dim: rgba(180,83,9,0.10);
      --warn-bg: #fffbeb;
      --warn-border: #fde68a;

      --shadow-sm: 0 1px 2px rgba(20,16,8,0.04);
      --shadow-md: 0 1px 3px rgba(20,16,8,0.06), 0 1px 2px rgba(20,16,8,0.03);
      --shadow-lg: 0 6px 24px rgba(20,16,8,0.07), 0 1px 3px rgba(20,16,8,0.04);

      --font-mono: 'JetBrains Mono', monospace;
      --font-sans: 'DM Sans', -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, sans-serif;

      --r-xs: 4px;
      --r-sm: 6px;
      --r-md: 8px;
      --r-lg: 12px;
    }

    *,*::before,*::after { box-sizing: border-box; }
    html { scroll-behavior: smooth; -webkit-font-smoothing: antialiased; }
    body {
      margin: 0;
      min-height: 100vh;
      background: var(--bg);
      color: var(--text);
      font-family: var(--font-sans);
      font-size: 14px;
      line-height: 1.5;
    }

    .shell {
      max-width: 1400px;
      margin: 0 auto;
      padding: 24px 28px 64px;
    }

    /* ── Topbar ── */
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
      font-size: 18px;
      letter-spacing: -0.02em;
      color: var(--text);
      display: flex;
      align-items: center;
      gap: 8px;
      white-space: nowrap;
    }
    .logo span { color: var(--accent); }
    .nav-tabs {
      display: inline-flex;
      gap: 4px;
      background: var(--surface-bright);
      padding: 3px;
      border-radius: var(--r-md);
      border: 1px solid var(--border);
    }
    .nav-tab {
      display: inline-flex;
      align-items: center;
      justify-content: center;
      padding: 5px 12px;
      border-radius: var(--r-sm);
      text-decoration: none;
      font-size: 12.5px;
      font-weight: 600;
      color: var(--text-dim);
      transition: all 0.15s ease;
      white-space: nowrap;
    }
    .nav-tab:hover {
      color: var(--text);
    }
    .nav-tab.active {
      background: var(--surface);
      color: var(--text);
      box-shadow: var(--shadow-sm);
    }

    .topbar-right {
      display: flex;
      align-items: center;
      gap: 10px;
      flex-shrink: 0;
    }
    .topbar-actions {
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .live-time {
      font-family: var(--font-mono);
      font-size: 13px;
      color: var(--text-dim);
      white-space: nowrap;
    }
    .btn {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 8px 14px;
      font-family: var(--font-sans);
      font-size: 13px;
      font-weight: 600;
      border-radius: var(--r-md);
      border: 1px solid var(--border);
      background: var(--surface);
      color: var(--text);
      cursor: pointer;
      box-shadow: var(--shadow-sm);
      transition: all 0.15s ease;
    }
    .btn:hover {
      background: var(--surface-raised);
      border-color: var(--border-bright);
    }
    .btn-primary {
      background: var(--accent);
      color: #ffffff;
      border-color: var(--accent);
    }
    .btn-primary:hover {
      background: var(--accent-hover);
    }
    .btn:disabled {
      opacity: 0.6;
      cursor: not-allowed;
    }

    /* ── Metrics grid ── */
    .metrics-grid {
      display: grid;
      grid-template-columns: repeat(auto-fit, minmax(200px, 1fr));
      gap: 14px;
      margin-bottom: 24px;
    }
    .metric-card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      padding: 16px 18px;
      box-shadow: var(--shadow-sm);
    }
    .metric-title {
      font-size: 12px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--text-dim);
      margin-bottom: 6px;
    }
    .metric-val {
      font-family: var(--font-mono);
      font-size: 26px;
      font-weight: 700;
      color: var(--text);
    }
    .metric-val.green { color: var(--success); }
    .metric-val.red { color: var(--danger); }
    .metric-val.amber { color: var(--warn); }

    /* ── Token Manager Box ── */
    .token-box {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      padding: 18px 20px;
      margin-bottom: 24px;
      box-shadow: var(--shadow-sm);
    }
    .token-box-header {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      flex-wrap: wrap;
    }
    .token-status-badge {
      display: inline-flex;
      align-items: center;
      gap: 6px;
      padding: 4px 10px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 600;
      font-family: var(--font-mono);
    }
    .token-status-badge.active {
      background: var(--success-bg);
      color: var(--success);
      border: 1px solid var(--success-border);
    }
    .token-status-badge.expired {
      background: var(--danger-bg);
      color: var(--danger);
      border: 1px solid var(--danger-border);
    }
    .token-form {
      margin-top: 14px;
      display: none;
      border-top: 1px dashed var(--border);
      padding-top: 14px;
    }
    .token-form.open {
      display: block;
    }
    .token-textarea {
      width: 100%;
      height: 85px;
      background: var(--surface-raised);
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      color: var(--text);
      padding: 10px 12px;
      font-family: var(--font-mono);
      font-size: 12px;
      resize: vertical;
      margin-bottom: 10px;
    }
    .token-textarea:focus {
      outline: none;
      border-color: var(--accent);
    }
    .alert-msg {
      margin-top: 10px;
      padding: 10px 14px;
      border-radius: var(--r-md);
      font-size: 13px;
      display: none;
    }
    .alert-msg.success {
      display: block;
      background: var(--success-bg);
      color: var(--success);
      border: 1px solid var(--success-border);
    }
    .alert-msg.error {
      display: block;
      background: var(--danger-bg);
      color: var(--danger);
      border: 1px solid var(--danger-border);
    }

    /* ── Controls & Filter Bar ── */
    .control-bar {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      margin-bottom: 20px;
      flex-wrap: wrap;
    }
    .search-box {
      flex: 1;
      min-width: 240px;
      max-width: 400px;
      position: relative;
    }
    .search-input {
      width: 100%;
      padding: 8px 12px 8px 32px;
      border-radius: var(--r-md);
      border: 1px solid var(--border);
      background: var(--surface);
      color: var(--text);
      font-family: var(--font-sans);
      font-size: 13px;
    }
    .search-icon {
      position: absolute;
      left: 10px;
      top: 50%;
      transform: translateY(-50%);
      color: var(--text-faint);
    }
    .filters {
      display: flex;
      gap: 6px;
      flex-wrap: wrap;
    }
    .filter-chip {
      padding: 5px 12px;
      border-radius: 999px;
      font-size: 12px;
      font-weight: 600;
      border: 1px solid var(--border);
      background: var(--surface);
      color: var(--text-dim);
      cursor: pointer;
      transition: all 0.15s ease;
    }
    .filter-chip:hover {
      color: var(--text);
      border-color: var(--border-bright);
    }
    .filter-chip.active {
      background: var(--text);
      color: #ffffff;
      border-color: var(--text);
    }

    /* ── Substation Groups ── */
    .substation-group {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      margin-bottom: 18px;
      box-shadow: var(--shadow-sm);
      overflow: hidden;
    }
    .substation-header {
      padding: 14px 18px;
      background: var(--surface-raised);
      border-bottom: 1px solid var(--border);
      display: flex;
      align-items: center;
      justify-content: space-between;
    }
    .substation-title {
      font-family: var(--font-mono);
      font-size: 15px;
      font-weight: 700;
      color: var(--text);
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .feeders-grid {
      display: grid;
      grid-template-columns: repeat(auto-fill, minmax(280px, 1fr));
      gap: 12px;
      padding: 16px;
    }
    .feeder-card {
      border: 1px solid var(--border);
      border-radius: var(--r-md);
      padding: 12px 14px;
      background: var(--surface);
      display: flex;
      flex-direction: column;
      gap: 6px;
      position: relative;
      transition: transform 0.1s ease, box-shadow 0.1s ease;
    }
    .feeder-card:hover {
      transform: translateY(-1px);
      box-shadow: var(--shadow-md);
    }
    .feeder-card.online {
      border-left: 4px solid var(--success);
    }
    .feeder-card.interrupted {
      border-left: 4px solid var(--danger);
      background: var(--danger-bg);
    }
    .feeder-card.dormant {
      border-left: 4px solid var(--warn);
      opacity: 0.85;
    }
    .feeder-card-top {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 6px;
    }
    .feeder-name {
      font-weight: 700;
      font-size: 14px;
      color: var(--text);
    }
    .feeder-badge {
      font-family: var(--font-mono);
      font-size: 11px;
      font-weight: 600;
      padding: 2px 6px;
      border-radius: var(--r-xs);
    }
    .feeder-badge.online {
      background: var(--success-bg);
      color: var(--success);
      border: 1px solid var(--success-border);
    }
    .feeder-badge.interrupted {
      background: var(--danger-bg);
      color: var(--danger);
      border: 1px solid var(--danger-border);
    }
    .feeder-badge.dormant {
      background: var(--warn-bg);
      color: var(--warn);
      border: 1px solid var(--warn-border);
    }
    .feeder-meta {
      font-size: 12px;
      color: var(--text-dim);
      display: flex;
      flex-wrap: wrap;
      gap: 8px 12px;
      font-family: var(--font-mono);
    }
    .downtime-tag {
      color: var(--danger);
      font-weight: 700;
    }

    /* ── Event Log Table ── */
    .events-card {
      background: var(--surface);
      border: 1px solid var(--border);
      border-radius: var(--r-lg);
      padding: 18px 20px;
      margin-top: 32px;
      box-shadow: var(--shadow-sm);
      overflow: hidden;
    }
    .events-title {
      font-size: 15px;
      font-weight: 700;
      margin-bottom: 14px;
      display: flex;
      align-items: center;
      gap: 8px;
    }
    .tbl-wrap {
      width: 100%;
      overflow-x: auto;
      -webkit-overflow-scrolling: touch;
      scrollbar-width: thin;
      scrollbar-color: var(--border-strong) transparent;
    }
    .tbl-wrap::-webkit-scrollbar {
      height: 6px;
    }
    .tbl-wrap::-webkit-scrollbar-track {
      background: var(--surface-bright);
      border-radius: 3px;
    }
    .tbl-wrap::-webkit-scrollbar-thumb {
      background: var(--border-strong);
      border-radius: 3px;
    }
    .events-table {
      width: 100%;
      min-width: 720px;
      border-collapse: collapse;
      font-size: 13px;
    }
    .events-table th {
      text-align: left;
      padding: 8px 10px;
      font-size: 11px;
      font-weight: 600;
      text-transform: uppercase;
      letter-spacing: 0.05em;
      color: var(--text-dim);
      border-bottom: 1px solid var(--border);
    }
    .events-table td {
      padding: 10px 10px;
      border-bottom: 1px solid var(--border);
      font-family: var(--font-mono);
      font-size: 12px;
    }
    .events-table tr:last-child td {
      border-bottom: none;
    }

    /* ── Mobile responsive ── */
    @media (max-width: 768px) {
      .shell {
        padding: 12px 12px 32px;
        max-width: 100%;
      }
      .topbar {
        display: grid;
        grid-template-columns: 1fr auto;
        grid-template-areas:
          "left time"
          "nav nav"
          "actions actions";
        gap: 10px 8px;
        align-items: center;
        padding-bottom: 14px;
        margin-bottom: 16px;
      }
      .topbar-left {
        grid-area: left;
        min-width: 0;
      }
      .logo { font-size: 15px; }
      .live-time {
        grid-area: time;
        font-size: 11.5px;
        text-align: right;
      }
      .nav-tabs {
        grid-area: nav;
        display: flex;
        width: 100%;
        box-sizing: border-box;
      }
      .nav-tab {
        flex: 1 1 50%;
        text-align: center;
        padding: 7px 8px;
        font-size: 12px;
      }
      .topbar-right {
        display: contents;
      }
      .topbar-actions {
        grid-area: actions;
        display: flex;
        width: 100%;
        gap: 8px;
      }
      .topbar-actions .btn {
        flex: 1 1 50%;
        justify-content: center;
        padding: 8px 10px;
        font-size: 12px;
      }

      .metrics-grid {
        grid-template-columns: repeat(2, 1fr);
        gap: 8px;
        margin-bottom: 16px;
      }
      .metric-card {
        padding: 11px 12px;
      }
      .metric-title {
        font-size: 10px;
        letter-spacing: 0.04em;
        margin-bottom: 4px;
      }
      .metric-val {
        font-size: 22px;
      }

      .control-bar {
        flex-direction: column;
        align-items: stretch;
        gap: 10px;
        margin-bottom: 16px;
      }
      .search-box {
        max-width: 100%;
        min-width: 0;
        width: 100%;
      }
      .search-input {
        font-size: 16px; /* prevents iOS auto-zoom */
        padding: 9px 12px 9px 34px;
      }
      .filters {
        overflow-x: auto;
        flex-wrap: nowrap;
        padding-bottom: 4px;
        -webkit-overflow-scrolling: touch;
        scrollbar-width: none;
      }
      .filters::-webkit-scrollbar { display: none; }
      .filter-chip {
        flex-shrink: 0;
        font-size: 11.5px;
        padding: 4px 10px;
      }

      .feeders-grid {
        grid-template-columns: 1fr;
        padding: 12px;
        gap: 10px;
      }
      .substation-header {
        padding: 11px 14px;
      }
      .substation-title {
        font-size: 13.5px;
      }

      .events-card {
        padding: 14px 14px;
        margin-top: 20px;
      }
      .events-title {
        font-size: 13.5px;
        margin-bottom: 10px;
      }
      .events-table th, .events-table td {
        padding: 7px 9px;
        font-size: 11.5px;
      }
      .events-table th {
        font-size: 10px;
      }
    }
  </style>
</head>
<body>
  <div class="shell">
    <!-- Top Bar -->
    <header class="topbar">
      <div class="topbar-left">
        <div class="logo">⚡ CMON<span>/SFMS</span></div>
      </div>
      <nav class="nav-tabs" aria-label="Page navigation">
        <a href="/" class="nav-tab">📋 Complaints</a>
        <a href="/sfms" class="nav-tab active">⚡ Feeder Monitor</a>
      </nav>
      <div class="topbar-right">
        <span id="liveTime" class="live-time">--:--:--</span>
        <div class="topbar-actions">
          <button class="btn" id="refreshBtn" onclick="triggerRefresh()">🔄 Refresh Data</button>
          <button class="btn btn-primary" onclick="toggleTokenBox()">🔑 Update Token</button>
        </div>
      </div>
    </header>

    <!-- Token Manager Panel -->
    <div class="token-box">
      <div class="token-box-header">
        <div>
          <span style="font-weight: 700; margin-right: 8px;">GETCO SFMS Authentication</span>
          <span id="tokenStatusBadge" class="token-status-badge">Checking...</span>
        </div>
        <button class="btn" style="padding: 4px 10px; font-size: 12px;" onclick="toggleTokenBox()">
          Paste 24h Bearer Token ▾
        </button>
      </div>
      <div id="tokenForm" class="token-form">
        <p style="font-size: 13px; color: var(--text-dim); margin: 0 0 8px;">
          Paste your fresh GETCO SFMS Bearer Token below to update the monitor live without restarting the daemon.
        </p>
        <textarea id="tokenInput" class="token-textarea" placeholder="Bearer lxtqXKSto57Irr1DPxKB..."></textarea>
        <button id="saveTokenBtn" class="btn btn-primary" onclick="saveToken()">⚡ Update & Verify Token</button>
        <div id="tokenAlert" class="alert-msg"></div>
      </div>
    </div>

    <!-- Summary Metrics -->
    <div class="metrics-grid">
      <div class="metric-card">
        <div class="metric-title">Substations</div>
        <div class="metric-val" id="statSubstations">-</div>
      </div>
      <div class="metric-card">
        <div class="metric-title">Total Feeders</div>
        <div class="metric-val" id="statFeeders">-</div>
      </div>
      <div class="metric-card">
        <div class="metric-title">🟢 Online / Closed</div>
        <div class="metric-val green" id="statOnline">-</div>
      </div>
      <div class="metric-card">
        <div class="metric-title">🔴 Interrupted / Trip</div>
        <div class="metric-val red" id="statInterrupted">-</div>
      </div>
      <div class="metric-card">
        <div class="metric-title">🟡 Scheduled Dormant</div>
        <div class="metric-val amber" id="statDormant">-</div>
      </div>
    </div>

    <!-- Control & Search Bar -->
    <div class="control-bar">
      <div class="search-box">
        <span class="search-icon">🔍</span>
        <input type="text" id="searchInput" class="search-input" placeholder="Search feeder name or code..." oninput="renderFeeders()">
      </div>
      <div class="filters">
        <div class="filter-chip active" onclick="setFilter('all', this)">All</div>
        <div class="filter-chip" onclick="setFilter('interrupted', this)">🔴 Interrupted</div>
        <div class="filter-chip" onclick="setFilter('online', this)">🟢 Online</div>
        <div class="filter-chip" onclick="setFilter('dormant', this)">🟡 Dormant</div>
        <div class="filter-chip" onclick="setFilter('jgy', this)">JGY</div>
        <div class="filter-chip" onclick="setFilter('ag', this)">AG</div>
      </div>
    </div>

    <!-- Feeder Substation Groups Container -->
    <div id="substationsContainer">
      <div style="text-align: center; padding: 40px; color: var(--text-dim);">Loading live SCADA feeder data...</div>
    </div>

    <!-- Outage & Event History Log -->
    <div class="events-card">
      <div class="events-title">📜 Recent Interruption & Restoration Log</div>
      <div class="tbl-wrap">
        <table class="events-table">
          <thead>
            <tr>
              <th>Time (IST)</th>
              <th>Event</th>
              <th>Feeder</th>
              <th>Substation</th>
              <th>Category</th>
              <th>Downtime</th>
              <th>Details</th>
            </tr>
          </thead>
          <tbody id="eventsTbody">
            <tr><td colspan="7" style="text-align: center; color: var(--text-faint);">No recent events recorded.</td></tr>
          </tbody>
        </table>
      </div>
    </div>
  </div>

  <script>
    let currentPayload = null;
    let activeFilter = 'all';

    function toggleTokenBox() {
      const form = document.getElementById('tokenForm');
      form.classList.toggle('open');
    }

    function setFilter(filter, el) {
      activeFilter = filter;
      document.querySelectorAll('.filter-chip').forEach(c => c.classList.remove('active'));
      el.classList.add('active');
      renderFeeders();
    }

    async function loadData() {
      try {
        const res = await fetch('/sfms/data');
        if (!res.ok) throw new Error('HTTP ' + res.status);
        const data = await res.json();
        currentPayload = data;
        updateSummary();
        renderFeeders();
        renderEvents();
      } catch (err) {
        console.error('Failed to load SFMS data:', err);
      }
    }

    function updateSummary() {
      if (!currentPayload) return;
      const s = currentPayload.summary || {};
      document.getElementById('statSubstations').innerText = s.total_substations || 0;
      document.getElementById('statFeeders').innerText = s.total_feeders || 0;
      document.getElementById('statOnline').innerText = s.active_online || 0;
      document.getElementById('statInterrupted').innerText = s.interrupted_down || 0;
      document.getElementById('statDormant').innerText = s.dormant || 0;

      const badge = document.getElementById('tokenStatusBadge');
      if (currentPayload.token_active) {
        badge.className = 'token-status-badge active';
        badge.innerHTML = '● Token Active & Verified';
      } else {
        badge.className = 'token-status-badge expired';
        badge.innerHTML = '⚠️ Token Expired / Invalid';
      }
    }

    function renderFeeders() {
      const container = document.getElementById('substationsContainer');
      if (!currentPayload || !currentPayload.groups || currentPayload.groups.length === 0) {
        container.innerHTML = '<div style="text-align: center; padding: 40px; color: var(--text-dim);">No substation data available. Please verify Bearer token.</div>';
        return;
      }

      const q = document.getElementById('searchInput').value.trim().toLowerCase();
      let html = '';

      currentPayload.groups.forEach(grp => {
        const matchingFeeders = grp.feeders.filter(f => {
          // Search query match
          const matchesQuery = !q || f.name.toLowerCase().includes(q) || f.clean_name.toLowerCase().includes(q) || (f.fdr_code && f.fdr_code.includes(q));
          if (!matchesQuery) return false;

          // Filter match
          if (activeFilter === 'all') return true;
          if (activeFilter === 'interrupted') return !f.is_online && !f.is_dormant;
          if (activeFilter === 'online') return f.is_online && !f.is_dormant;
          if (activeFilter === 'dormant') return f.is_dormant;
          if (activeFilter === 'jgy') return (f.category_name || '').toUpperCase().includes('JGY');
          if (activeFilter === 'ag') return (f.category_name || '').toUpperCase().includes('AG');
          return true;
        });

        if (matchingFeeders.length === 0) return;

        html += '<div class="substation-group">';
        html += '  <div class="substation-header">';
        html += '    <div class="substation-title">⚡ ' + grp.clean_name + '</div>';
        html += '    <div style="font-size: 12px; color: var(--text-dim); font-family: var(--font-mono);">' + matchingFeeders.length + ' Feeders</div>';
        html += '  </div>';
        html += '  <div class="feeders-grid">';

        matchingFeeders.forEach(f => {
          let cardClass = 'online';
          let badgeClass = 'online';
          let statusText = '🟢 ONLINE';

          if (f.is_dormant) {
            cardClass = 'dormant';
            badgeClass = 'dormant';
            statusText = '🟡 DORMANT';
          } else if (!f.is_online) {
            cardClass = 'interrupted';
            badgeClass = 'interrupted';
            statusText = '🔴 INTERRUPTED';
          }

          let metaHtml = '';
          if (f.downtime) {
            metaHtml += '<span class="downtime-tag">⏱ Down: ' + f.downtime + '</span>';
          } else if (f.is_dormant && f.schedule_start && f.schedule_end) {
            metaHtml += '<span>🕒 Schedule: ' + f.schedule_start + ' - ' + f.schedule_end + '</span>';
          }

          html += '    <div class="feeder-card ' + cardClass + '">';
          html += '      <div class="feeder-card-top">';
          html += '        <div class="feeder-name">' + f.clean_name + ' <span style="font-size: 11px; font-weight: normal; color: var(--text-dim);">[' + f.category_name + ']</span></div>';
          html += '        <div class="feeder-badge ' + badgeClass + '">' + statusText + '</div>';
          html += '      </div>';
          if (metaHtml) {
            html += '      <div class="feeder-meta">' + metaHtml + '</div>';
          }
          html += '    </div>';
        });

        html += '  </div>';
        html += '</div>';
      });

      if (!html) {
        container.innerHTML = '<div style="text-align: center; padding: 40px; color: var(--text-dim);">No feeders match the selected filters.</div>';
      } else {
        container.innerHTML = html;
      }
    }

    function renderEvents() {
      const tbody = document.getElementById('eventsTbody');
      if (!currentPayload || !currentPayload.events || currentPayload.events.length === 0) {
        tbody.innerHTML = '<tr><td colspan="7" style="text-align: center; color: var(--text-faint);">No recent events recorded.</td></tr>';
        return;
      }

      let rows = '';
      currentPayload.events.forEach(e => {
        let badge = e.event_type === 'interruption' ? '<span style="color: var(--danger); font-weight: 700;">🔴 Interruption</span>' :
                    e.event_type === 'recovery' ? '<span style="color: var(--success); font-weight: 700;">🟢 Restored</span>' :
                    '<span style="color: var(--warn); font-weight: 700;">⚠️ ' + e.event_type + '</span>';

        let ts = e.timestamp ? new Date(e.timestamp).toLocaleTimeString('en-IN', { hour12: false }) : '-';

        rows += '<tr>';
        rows += '  <td>' + ts + '</td>';
        rows += '  <td>' + badge + '</td>';
        rows += '  <td><b>' + (e.feeder_name || '-') + '</b></td>';
        rows += '  <td>' + (e.substation_name || '-') + '</td>';
        rows += '  <td>' + (e.category || '-') + '</td>';
        rows += '  <td>' + (e.downtime || '-') + '</td>';
        rows += '  <td style="color: var(--text-dim);">' + (e.message || '-') + '</td>';
        rows += '</tr>';
      });
      tbody.innerHTML = rows;
    }

    async function triggerRefresh() {
      const btn = document.getElementById('refreshBtn');
      btn.disabled = true;
      btn.innerText = 'Refreshing...';
      try {
        await fetch('/sfms/refresh', { method: 'POST' });
        await loadData();
      } catch (err) {
        console.error('Refresh failed:', err);
      } finally {
        btn.disabled = false;
        btn.innerText = '🔄 Refresh Data';
      }
    }

    async function saveToken() {
      const input = document.getElementById('tokenInput');
      const btn = document.getElementById('saveTokenBtn');
      const alert = document.getElementById('tokenAlert');
      const token = input.value.trim();

      if (!token) {
        alert.className = 'alert-msg error';
        alert.innerText = 'Please paste a token before submitting.';
        return;
      }

      btn.disabled = true;
      btn.innerText = 'Verifying with GETCO API...';
      alert.style.display = 'none';

      try {
        const res = await fetch('/sfms/update-token', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ token: token })
        });
        const data = await res.json();
        if (res.ok) {
          alert.className = 'alert-msg success';
          alert.innerText = '✅ ' + (data.message || 'Token verified and updated successfully!');
          input.value = '';
          await loadData();
        } else {
          throw new Error(data.error || 'Failed to verify token');
        }
      } catch (err) {
        alert.className = 'alert-msg error';
        alert.innerText = '❌ ' + err.message;
      } finally {
        btn.disabled = false;
        btn.innerText = '⚡ Update & Verify Token';
      }
    }

    // Live IST Clock
    function updateClock() {
      const now = new Date();
      const options = { timeZone: 'Asia/Kolkata', hour12: false, hour: '2-digit', minute: '2-digit', second: '2-digit' };
      document.getElementById('liveTime').innerText = 'IST ' + now.toLocaleTimeString('en-GB', options);
    }
    setInterval(updateClock, 1000);
    updateClock();

    // Initial load
    loadData();

    // WebSocket real-time updates
    const wsProtocol = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = wsProtocol + '//' + window.location.host + '/ws';
    const socket = new WebSocket(wsUrl);
    socket.onmessage = function(event) {
      if (event.data === 'refresh') {
        loadData();
      }
    };
  </script>
</body>
</html>`))
