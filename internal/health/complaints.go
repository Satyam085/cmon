package health

import (
	"encoding/json"
	"fmt"
	"html/template"
	"image/color"
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
  <title>CMON Pending Complaints</title>
  <style>
    :root {
      --bg: #eef3ea;
      --bg-accent: #f8fbf3;
      --paper: rgba(255, 255, 255, 0.88);
      --paper-strong: rgba(255, 255, 255, 0.96);
      --ink: #1d2935;
      --muted: #607284;
      --line: rgba(83, 102, 117, 0.18);
      --brand: #195a95;
      --brand-strong: #0f3f69;
      --brand-soft: rgba(25, 90, 149, 0.12);
      --accent: #d4892f;
      --danger: #a63b36;
      --shadow: 0 24px 70px rgba(19, 36, 53, 0.12);
      --radius-xl: 28px;
      --radius-lg: 18px;
      --radius-md: 14px;
    }

    * {
      box-sizing: border-box;
    }

    html {
      scroll-behavior: smooth;
    }

    body {
      margin: 0;
      min-height: 100vh;
      color: var(--ink);
      background:
        radial-gradient(circle at top left, rgba(212, 137, 47, 0.18), transparent 28rem),
        radial-gradient(circle at top right, rgba(25, 90, 149, 0.16), transparent 24rem),
        linear-gradient(180deg, #f7f8f2 0%, var(--bg) 56%, #e4ece6 100%);
      font-family: "Trebuchet MS", "Lucida Sans Unicode", "Lucida Grande", sans-serif;
      line-height: 1.45;
    }

    .page-shell {
      width: min(1440px, calc(100% - 32px));
      margin: 0 auto;
      padding: 28px 0 48px;
    }

    .hero {
      position: relative;
      overflow: hidden;
      background:
        linear-gradient(135deg, rgba(15, 63, 105, 0.96), rgba(25, 90, 149, 0.92)),
        linear-gradient(180deg, rgba(255, 255, 255, 0.08), transparent);
      border-radius: var(--radius-xl);
      box-shadow: var(--shadow);
      padding: 30px;
      color: #f7fafc;
      display: grid;
      grid-template-columns: minmax(0, 1.6fr) minmax(280px, 1fr);
      gap: 24px;
      isolation: isolate;
    }

    .hero::after {
      content: "";
      position: absolute;
      inset: auto -80px -90px auto;
      width: 320px;
      height: 320px;
      border-radius: 50%;
      background: radial-gradient(circle, rgba(255, 221, 173, 0.4), transparent 70%);
      z-index: -1;
    }

    .eyebrow {
      margin: 0 0 8px;
      text-transform: uppercase;
      letter-spacing: 0.18em;
      font-size: 0.78rem;
      opacity: 0.8;
    }

    h1 {
      margin: 0;
      font-family: Georgia, "Times New Roman", serif;
      font-size: clamp(2.15rem, 3vw, 3.4rem);
      line-height: 1.05;
      letter-spacing: -0.03em;
    }

    .hero-copy {
      max-width: 58rem;
    }

    .hero-copy p {
      margin: 14px 0 0;
      max-width: 48rem;
      color: rgba(241, 245, 249, 0.88);
      font-size: 1.02rem;
    }

    .hero-meta {
      display: grid;
      grid-template-columns: repeat(2, minmax(0, 1fr));
      gap: 14px;
      align-content: start;
    }

    .meta-card {
      padding: 16px 18px;
      border-radius: 18px;
      background: rgba(255, 255, 255, 0.11);
      border: 1px solid rgba(255, 255, 255, 0.12);
      backdrop-filter: blur(10px);
    }

    .meta-card span {
      display: block;
      font-size: 0.78rem;
      letter-spacing: 0.08em;
      text-transform: uppercase;
      color: rgba(226, 232, 240, 0.7);
    }

    .meta-card strong {
      display: block;
      margin-top: 7px;
      font-size: 1.05rem;
      color: #fff8e8;
      word-break: break-word;
    }

    .controls {
      margin-top: 22px;
      padding: 18px;
      border-radius: var(--radius-lg);
      background: var(--paper);
      border: 1px solid rgba(255, 255, 255, 0.8);
      box-shadow: 0 18px 44px rgba(24, 39, 56, 0.08);
      display: grid;
      grid-template-columns: minmax(240px, 1.5fr) repeat(3, auto);
      gap: 14px;
      align-items: center;
      backdrop-filter: blur(14px);
    }

    .search-wrap,
    .toggle-wrap {
      display: flex;
      align-items: center;
      gap: 10px;
    }

    .search-wrap input {
      width: 100%;
      border: 1px solid var(--line);
      border-radius: 999px;
      padding: 13px 16px;
      background: var(--paper-strong);
      color: var(--ink);
      font: inherit;
      box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.55);
    }

    .search-wrap input:focus {
      outline: 2px solid rgba(25, 90, 149, 0.18);
      border-color: rgba(25, 90, 149, 0.4);
    }

    .toggle-wrap {
      justify-self: start;
      padding: 8px 12px;
      border-radius: 999px;
      background: rgba(25, 90, 149, 0.08);
      color: var(--brand-strong);
      font-size: 0.96rem;
      white-space: nowrap;
    }

    .toggle-wrap input {
      width: 18px;
      height: 18px;
      accent-color: var(--brand);
    }

    .refresh-btn {
      justify-self: end;
      border: 0;
      border-radius: 999px;
      padding: 12px 18px;
      font: inherit;
      font-weight: 700;
      color: white;
      background: linear-gradient(135deg, var(--brand), var(--brand-strong));
      box-shadow: 0 12px 24px rgba(17, 63, 103, 0.24);
      cursor: pointer;
      transition: transform 140ms ease, box-shadow 140ms ease;
    }

    .refresh-btn:hover {
      transform: translateY(-1px);
      box-shadow: 0 16px 28px rgba(17, 63, 103, 0.26);
    }

    .auto-note {
      justify-self: end;
      font-size: 0.92rem;
      color: var(--muted);
      text-align: right;
      min-width: 10rem;
    }

    .status-banner {
      margin-top: 18px;
      padding: 14px 18px;
      border-radius: 16px;
      background: var(--paper);
      border: 1px solid rgba(255, 255, 255, 0.86);
      color: var(--muted);
      box-shadow: 0 10px 28px rgba(24, 39, 56, 0.08);
    }

    .status-banner strong {
      color: var(--ink);
    }

    .status-banner.is-error {
      border-color: rgba(166, 59, 54, 0.2);
      background: rgba(255, 246, 245, 0.94);
      color: #7f2f2b;
    }

    .group-stack {
      margin-top: 22px;
      display: grid;
      gap: 18px;
    }

    .group-card {
      background: var(--paper);
      border: 1px solid rgba(255, 255, 255, 0.86);
      border-radius: 22px;
      box-shadow: 0 18px 44px rgba(24, 39, 56, 0.08);
      overflow: hidden;
      backdrop-filter: blur(14px);
      animation: rise-in 240ms ease;
    }

    .group-head {
      display: flex;
      align-items: center;
      justify-content: space-between;
      gap: 12px;
      padding: 16px 18px;
      border-bottom: 1px solid rgba(17, 24, 39, 0.08);
    }

    .group-title {
      display: flex;
      align-items: center;
      gap: 12px;
      min-width: 0;
    }

    .group-dot {
      width: 14px;
      height: 14px;
      border-radius: 50%;
      flex: 0 0 auto;
      box-shadow: inset 0 0 0 2px rgba(255, 255, 255, 0.5);
    }

    .group-title h2 {
      margin: 0;
      font-size: 1.1rem;
      font-family: Georgia, "Times New Roman", serif;
      letter-spacing: -0.02em;
    }

    .group-count {
      color: var(--muted);
      font-size: 0.94rem;
      white-space: nowrap;
    }

    .table-shell {
      overflow: auto;
      padding: 6px 10px 12px;
    }

    table {
      width: 100%;
      border-collapse: separate;
      border-spacing: 0;
      min-width: 980px;
    }

    thead th {
      position: sticky;
      top: 0;
      z-index: 1;
      padding: 14px 12px;
      background: #edf4f8;
      color: #25425a;
      font-size: 0.8rem;
      text-transform: uppercase;
      letter-spacing: 0.08em;
      text-align: left;
      border-bottom: 1px solid rgba(83, 102, 117, 0.16);
    }

    tbody td {
      padding: 12px;
      border-bottom: 1px solid rgba(83, 102, 117, 0.1);
      vertical-align: top;
      color: var(--ink);
      background: rgba(255, 255, 255, 0.72);
    }

    tbody tr:nth-child(even) td {
      background: rgba(237, 244, 248, 0.56);
    }

    tbody tr:hover td {
      background: rgba(250, 238, 216, 0.58);
    }

    tbody td.debug-col,
    thead th.debug-col {
      display: none;
    }

    body.show-debug tbody td.debug-col,
    body.show-debug thead th.debug-col {
      display: table-cell;
    }

    .pill {
      display: inline-flex;
      align-items: center;
      gap: 8px;
      padding: 6px 11px;
      border-radius: 999px;
      background: rgba(25, 90, 149, 0.08);
      color: var(--brand-strong);
      font-size: 0.9rem;
      white-space: nowrap;
    }

    .mono {
      font-family: "Courier New", Courier, monospace;
      font-size: 0.92rem;
      white-space: nowrap;
    }

    .muted {
      color: var(--muted);
    }

    .empty-state {
      margin-top: 22px;
      padding: 48px 24px;
      text-align: center;
      border-radius: 22px;
      color: var(--muted);
      background: rgba(255, 255, 255, 0.82);
      border: 1px dashed rgba(83, 102, 117, 0.24);
      box-shadow: 0 18px 44px rgba(24, 39, 56, 0.08);
    }

    .empty-state strong {
      display: block;
      color: var(--ink);
      font-size: 1.08rem;
      margin-bottom: 8px;
    }

    .error-box {
      margin-top: 22px;
      padding: 18px;
      border-radius: 18px;
      border: 1px solid rgba(166, 59, 54, 0.18);
      background: rgba(255, 245, 244, 0.95);
      color: #7f2f2b;
      box-shadow: 0 18px 44px rgba(24, 39, 56, 0.08);
    }

    .error-box strong {
      display: block;
      margin-bottom: 6px;
      color: var(--danger);
    }

    @keyframes rise-in {
      from {
        opacity: 0;
        transform: translateY(8px);
      }
      to {
        opacity: 1;
        transform: translateY(0);
      }
    }

    @media (max-width: 1080px) {
      .hero,
      .controls {
        grid-template-columns: 1fr;
      }

      .refresh-btn,
      .auto-note {
        justify-self: start;
      }
    }

    @media (max-width: 760px) {
      .page-shell {
        width: min(100% - 18px, 100%);
        padding-top: 12px;
      }

      .hero {
        padding: 22px 18px;
      }

      .hero-meta {
        grid-template-columns: 1fr 1fr;
      }

      .controls {
        padding: 14px;
      }

      .table-shell {
        padding: 10px;
      }

      table {
        min-width: 100%;
      }

      thead {
        display: none;
      }

      tbody,
      tbody tr,
      tbody td {
        display: block;
        width: 100%;
      }

      body.show-debug tbody td.debug-col {
        display: block;
      }

      tbody tr {
        padding: 10px;
        border-radius: 16px;
        margin-bottom: 10px;
        background: rgba(255, 255, 255, 0.78);
        border: 1px solid rgba(83, 102, 117, 0.08);
      }

      tbody td {
        border: 0;
        background: transparent !important;
        padding: 8px 0;
      }

      tbody td::before {
        content: attr(data-label);
        display: block;
        margin-bottom: 4px;
        color: var(--muted);
        font-size: 0.76rem;
        font-weight: 700;
        text-transform: uppercase;
        letter-spacing: 0.08em;
      }
    }
  </style>
</head>
<body>
  <main class="page-shell">
    <section class="hero">
      <div class="hero-copy">
        <p class="eyebrow">CMON Monitoring Desk</p>
        <h1>Pending complaints, sorted live by belt</h1>
        <p>
          This view follows the same grouping and ordering rules as the <code>/summary</code> image.
          Telegram and WhatsApp message IDs stay tucked away until you need them for debugging.
        </p>
      </div>
      <div class="hero-meta">
        <div class="meta-card">
          <span>Total complaints</span>
          <strong id="totalCount">-</strong>
        </div>
        <div class="meta-card">
          <span>Belt groups</span>
          <strong id="groupCount">-</strong>
        </div>
        <div class="meta-card">
          <span>Last fetch</span>
          <strong id="lastFetchTime">-</strong>
        </div>
        <div class="meta-card">
          <span>Fetch status</span>
          <strong id="lastFetchStatus">-</strong>
        </div>
      </div>
    </section>

    <section class="controls">
      <label class="search-wrap" aria-label="Search complaints">
        <input id="searchInput" type="search" placeholder="Search by complaint, consumer, address, description, or debug IDs">
      </label>
      <label class="toggle-wrap" for="debugToggle">
        <input id="debugToggle" type="checkbox">
        <span>Show debug IDs</span>
      </label>
      <button id="refreshBtn" class="refresh-btn" type="button">Refresh now</button>
      <div class="auto-note" id="autoNote">Manual refresh only</div>
    </section>

    <section id="statusBanner" class="status-banner">
      <strong>Loading complaints...</strong>
      <span id="statusDetail">Please wait while the dashboard fetches the latest pending complaints.</span>
    </section>

    <section id="content"></section>
  </main>

  <script>
    (() => {
      const dataUrl = {{.DataURL}};
      const searchInput = document.getElementById("searchInput");
      const debugToggle = document.getElementById("debugToggle");
      const refreshBtn = document.getElementById("refreshBtn");
      const autoNote = document.getElementById("autoNote");
      const statusBanner = document.getElementById("statusBanner");
      const statusDetail = document.getElementById("statusDetail");
      const content = document.getElementById("content");

      let payload = null;
      let isLoading = false;

      const esc = (value) => String(value ?? "")
        .replaceAll("&", "&amp;")
        .replaceAll("<", "&lt;")
        .replaceAll(">", "&gt;")
        .replaceAll('"', "&quot;")
        .replaceAll("'", "&#39;");

      const formatCount = (count) => String(count);

      function setMetric(id, value) {
        const el = document.getElementById(id);
        if (el) {
          el.textContent = value;
        }
      }

      function setBanner(kind, headline, detail) {
        statusBanner.classList.toggle("is-error", kind === "error");
        statusBanner.querySelector("strong").textContent = headline;
        statusDetail.textContent = detail;
      }

      function complaintMatches(complaint, term) {
        if (!term) {
          return true;
        }

        const haystack = [
          complaint.complain_no,
          complaint.name,
          complaint.consumer_no,
          complaint.mobile_no,
          complaint.address,
          complaint.area,
          complaint.village,
          complaint.belt,
          complaint.description,
          complaint.complain_date,
          complaint.telegram_message_id,
          complaint.whatsapp_message_id
        ].join(" ").toLowerCase();

        return haystack.includes(term);
      }

      function buildRow(complaint) {
        const tgID = complaint.telegram_message_id || "Not stored";
        const waID = complaint.whatsapp_message_id || "Not stored";

        return (
          '<tr>' +
            '<td data-label="Complaint" class="mono">' + esc(complaint.complain_no || "-") + '</td>' +
            '<td data-label="Name">' + esc(complaint.name || "-") + '</td>' +
            '<td data-label="Consumer">' + esc(complaint.consumer_no || "-") + '</td>' +
            '<td data-label="Mobile">' + esc(complaint.mobile_no || "-") + '</td>' +
            '<td data-label="Address">' + esc(complaint.address || "-") + '</td>' +
            '<td data-label="Area">' + esc(complaint.area || "-") + '</td>' +
            '<td data-label="Description">' + esc(complaint.description || "-") + '</td>' +
            '<td data-label="Date">' + esc(complaint.complain_date || "-") + '</td>' +
            '<td data-label="Telegram ID" class="debug-col mono">' + esc(tgID) + '</td>' +
            '<td data-label="WhatsApp ID" class="debug-col mono">' + esc(waID) + '</td>' +
          '</tr>'
        );
      }

      function buildGroup(group, complaints) {
        const rows = complaints.map((complaint) => buildRow(complaint)).join("");
        return (
          '<section class="group-card">' +
            '<header class="group-head" style="background: linear-gradient(180deg, ' + group.fill_color + ', rgba(255,255,255,0.7)); color:' + group.text_color + ';">' +
              '<div class="group-title">' +
                '<span class="group-dot" style="background:' + group.text_color + ';"></span>' +
                '<h2>' + esc(group.label) + ' Belt</h2>' +
              '</div>' +
              '<div class="group-count">' + complaints.length + ' complaint' + (complaints.length === 1 ? "" : "s") + '</div>' +
            '</header>' +
            '<div class="table-shell">' +
              '<table>' +
                '<thead>' +
                  '<tr>' +
                    '<th>Complaint</th>' +
                    '<th>Name</th>' +
                    '<th>Consumer</th>' +
                    '<th>Mobile</th>' +
                    '<th>Address</th>' +
                    '<th>Area</th>' +
                    '<th>Description</th>' +
                    '<th>Date</th>' +
                    '<th class="debug-col">Telegram ID</th>' +
                    '<th class="debug-col">WhatsApp ID</th>' +
                  '</tr>' +
                '</thead>' +
                '<tbody>' + rows + '</tbody>' +
              '</table>' +
            '</div>' +
          '</section>'
        );
      }

      function render() {
        if (!payload) {
          content.innerHTML = "";
          return;
        }

        const query = searchInput.value.trim().toLowerCase();
        const visibleGroups = payload.groups
          .map((group) => ({
            ...group,
            complaints: group.complaints.filter((complaint) => complaintMatches(complaint, query))
          }))
          .filter((group) => group.complaints.length > 0);

        const visibleCount = visibleGroups.reduce((sum, group) => sum + group.complaints.length, 0);

        setMetric("totalCount", query ? formatCount(visibleCount) + " filtered" : formatCount(payload.total_count));
        setMetric("groupCount", query ? String(visibleGroups.length) + " visible" : formatCount(payload.group_count));
        setMetric("lastFetchTime", payload.status.last_fetch_time || "Not yet fetched");
        setMetric("lastFetchStatus", payload.status.last_fetch_status || "Unknown");

        if (visibleGroups.length === 0) {
          content.innerHTML =
            '<section class="empty-state">' +
              '<strong>' + esc(query ? "No complaints matched your search." : "No pending complaints right now.") + '</strong>' +
              '<span>' + esc(query ? "Try a broader search term or disable the debug ID filter." : "The dashboard will populate as soon as pending complaints are present in storage.") + '</span>' +
            '</section>';
          return;
        }

        content.innerHTML = '<div class="group-stack">' + visibleGroups.map((group) => buildGroup(group, group.complaints)).join("") + '</div>';
      }

      async function loadData(options = {}) {
        const silent = !!options.silent;
        if (isLoading) {
          return;
        }

        isLoading = true;
        refreshBtn.disabled = true;
        refreshBtn.textContent = "Refreshing...";

        if (!silent) {
          setBanner("info", "Refreshing complaints...", "Fetching the latest pending complaint details from the live session.");
        }

        try {
          const response = await fetch(dataUrl + "?ts=" + Date.now(), {
            headers: {
              "Accept": "application/json"
            }
          });

          const data = await response.json().catch(() => ({}));
          if (!response.ok) {
            throw new Error(data.error || ("Request failed with status " + response.status));
          }

          payload = data;
          render();

          const fetchStatus = payload.status.last_fetch_status || "unknown";
          const bannerKind = payload.status.status === "unhealthy" ? "error" : "info";
          setBanner(
            bannerKind,
            payload.status.status === "unhealthy" ? "Last fetch needs attention." : "Dashboard is up to date.",
            "Updated " + payload.generated_at + ". Last fetch status: " + fetchStatus + "."
          );
        } catch (error) {
          content.innerHTML =
            '<section class="error-box">' +
              '<strong>Unable to load complaint dashboard</strong>' +
              '<span>' + esc(error.message || "Unknown error") + '</span>' +
            '</section>';
          setMetric("totalCount", "-");
          setMetric("groupCount", "-");
          setBanner("error", "Dashboard refresh failed.", error.message || "Unknown error");
        } finally {
          isLoading = false;
          refreshBtn.disabled = false;
          refreshBtn.textContent = "Refresh now";
        }
      }

      searchInput.addEventListener("input", render);
      debugToggle.addEventListener("change", () => {
        document.body.classList.toggle("show-debug", debugToggle.checked);
      });
      refreshBtn.addEventListener("click", () => loadData());

      loadData();
    })();
  </script>
</body>
</html>`))

func registerComplaintDashboard(mux *http.ServeMux, monitor *Monitor, sc *session.Client, stor *storage.Storage) {
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
