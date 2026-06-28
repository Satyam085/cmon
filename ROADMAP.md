# cmon — Repo Analysis & Improvement Suggestions

## Context

A structural analysis of the `cmon` repo and a menu of code-quality improvements + new features. This is a **prioritized recommendation list** — pick what's most valuable; each item links to a concrete file/line so it's actionable.

The repo is in good shape overall: clean package layout, sensible session-retry logic, dual-tier storage, no browser dependency. The pressure points are: a few oversized files, missing tests on IO-heavy packages, some silent error swallowing, and a handful of features that are 80% wired up but not surfaced to users.

---

## Architecture (one-paragraph mental model)

`main.go` boots config → storage (SQLite + in-memory cache) → session.Client (rate-limited HTTP + cookie jar replacing browser automation) → Telegram + WhatsApp clients → Gemini translator → background HTTP/WebSocket dashboard. A 15-minute ticker drives `complaint.Fetcher`, which scrapes the DGVCL portal, fans out per-complaint detail fetches via a worker pool, dedupes against storage, persists, and notifies both messaging channels. Resolutions flow back via Telegram inline buttons or WhatsApp resolve-by-reply, hit the DGVCL resolve API, edit the original message, and remove from storage. Dashboard reads from the in-memory map and pushes WebSocket updates after every fetch cycle.

**File-size hot spots** (worth knowing before refactoring):
- `internal/health/complaints.go` — **1798 lines** (dashboard + JSON + HTML template all in one)
- `internal/telegram/client.go` — 1227 lines
- `internal/storage/storage.go` — 863 lines
- `internal/whatsapp/client.go` — 656 lines
- `internal/session/client.go` — 495 lines
- `main.go` — 391 lines

---

## Code-Quality Improvements (prioritized)

### Tier 1 — Correctness fixes (do first, low effort)

| # | Issue | File:line | Fix |
|---|-------|-----------|-----|
| 1 | Silent error swallowing on response read | `internal/session/client.go:207`, `internal/telegram/client.go:533` | Capture and log `err` from `io.ReadAll(resp.Body)` instead of `_`. |
| 2 | Telegram unmarshal error discarded | `internal/telegram/client.go:535` | Check `json.Unmarshal` err — currently `result["ok"]` check runs on possibly empty map. |
| 3 | `log.Fatalf` outside init path | `internal/storage/storage.go:860` (column-ensure) | Return error and let caller decide. The init-path Fatalfs (102/122/145/259) are acceptable but document them. |
| 4 | `context.Background()` for network I/O with no deadline | `internal/whatsapp/client.go:175,208,222,227,508`; `internal/complaint/fetcher.go:215` (Gemini batch) | Wrap each in `context.WithTimeout(parent, 30s)`; pass parent ctx through `HandleEvents` → `handle*Command`. |
| 5 | New `http.Client` per Telegram photo upload | `internal/telegram/client.go:526` | Reuse a package-level `*http.Client` like the rest of the package does. |
| 6 | WhatsApp `go c.handle*Command(...)` spawned with no ctx | `internal/whatsapp/client.go:299–308` | Pass the `HandleEvents` ctx into handlers; respect `<-ctx.Done()` for clean shutdown. |

### Tier 2 — Test coverage gaps (most-risky-first)

Untested packages:
- `internal/auth/` — login/captcha (security-critical)
- `internal/api/` — DGVCL resolve calls (state mutation)
- `internal/whatsapp/` — message send + handler routing
- `internal/summary/` — image render + grouping (now with two new code paths)
- `internal/translate/` — Gemini call + caching
- `internal/config/` — env precedence

Suggested order: **auth → api → summary → whatsapp → translate → config**. Auth and api are the riskiest to be untested because they mutate external state. Summary moved up because of recent changes (`/summarybelt`).

### Tier 3 — Structural cleanups

| # | Cleanup | Where | Why |
|---|---------|-------|-----|
| 7 | Split `internal/health/complaints.go` | 1798-line file → e.g. `dashboard_html.go`, `dashboard_json.go`, `dashboard_render.go` | File is 33% of the codebase; hard to navigate. |
| 8 | Extract goroutine boot + ticker loop from `main()` | `main.go:52` (193-line `main`) | Separate `startBackgroundWorkers(...)` and `runFetchLoop(...)`. |
| 9 | Simplify `fetchWithRetry` | `main.go:245` (~85 lines) | Pull session-reset path into `resetAndRelogin()`. |
| 10 | Move `handleSummaryCommand`/`handleSummaryBeltCommand` into a `telegram/commands.go` | `internal/telegram/client.go` | `client.go` now mixes transport + 6+ command handlers. |
| 11 | Extract complaint-no parser | `telegram/client.go` and `whatsapp/client.go` both have local extractors | Move to `internal/complaint/parse.go` and reuse. |

### Tier 4 — Config / hardcoded values

| # | Value | Where | Suggested home |
|---|-------|-------|----------------|
| 12 | Telegram rate-limit `35*time.Millisecond` | `internal/telegram/client.go:221` | Add `TELEGRAM_RATE_INTERVAL_MS` to `internal/config`. |
| 13 | Long-poll timeouts (60s / 30) duplicated | `internal/telegram/client.go:192,565` | Single const at top of file. |
| 14 | Resolve API URL `https://complaint.dgvcl.com/api/...` | `internal/api/resolve.go:39` | Already partly in config; finish the migration so prod can point at staging. |

### Tier 5 — Operations / observability (lower urgency, real value)

- **Structured logging**: emoji-prefixed `log.Printf` is friendly in a terminal but unparseable in any log aggregator. Switch to `log/slog` with JSON handler when `LOG_FORMAT=json`. Plug-in: replace `log` import package-by-package, starting in `complaint/` and `session/`.
- **Prometheus metrics endpoint**: `health/server.go` already serves HTTP. Add `/metrics` with counters for fetch attempts/failures, complaints seen, resolve calls, message sends. ~80 LOC.
- **Health endpoint richness**: `/health` returns JSON status. Add last-fetch-success timestamp + error count so an external probe can detect a stuck scraper.
- **Graceful shutdown audit**: `main.go` has signal handling, but verify each goroutine (Telegram poll, WhatsApp event handler, ticker) actually exits before the DB closes — otherwise you can lose the in-flight fetch.

---

## Feature Suggestions (prioritized by ROI ÷ effort)

Difficulty: **S** = ≤½ day, **M** = 1–2 days, **L** = ≥3 days.

### Tier A — Quick wins (S, high visibility)

| # | Feature | Plug-in point | Notes |
|---|---------|---------------|-------|
| F1 (already exist dont implement) | **Show consumer account #** in Telegram messages and dashboard rows | `Complaint.ConsumerNo` already in storage; surface in `telegram/client.go:SendComplaintMessage` and the dashboard row template in `health/complaints.go` | Field is parsed and stored but dropped at render. |
| F2 | **Complaint age** ("pending for 3d 4h") on dashboard + Telegram | Compute `now - parsed(ComplainDate)` in `summary/fetcher.go` (add `AgeMinutes` field to `Complaint`) and render in dashboard + summary image | Needed for ops triage. |
| F3 | **Per-belt dashboard tabs** — filter pill row at top of dashboard | `health/complaints.go` template + a `?belt=` query param on `/data/complaints-dashboard` | Belt grouping already exists server-side; this is a thin UI/JS toggle. |
| F4 | **CSV/JSON export** endpoint at `/export.csv` and `/export.json` | New handler in `health/server.go`; reuse `summary.GroupComplaints` | Useful for audits and ad-hoc analysis. |
| F5 | **Belt move command discoverability** — `/move` with no/bad args prints usage with valid belt names | `telegram/client.go:handleMoveCommand` | Currently silent on bad input. |
| F6 | **WhatsApp resolve-by-reply default-on (or doc'd toggle)** | `WHATSAPP_RESOLVE_ENABLED` flag in `config/` and `whatsapp/client.go:HandleEvents` | Code is fully scaffolded; only a flag flip + readme update. |

### Tier B — Operational features (M)

| # | Feature | Plug-in point | Notes |
|---|---------|---------------|-------|
| F7 (dont need)| **Resolution audit trail** — who resolved what, when, what note | New `resolutions` table in `internal/storage/storage.go` + write site in `telegram/client.go:handleResolveText` and `whatsapp/client.go:handleResolve` | Currently complaints just disappear from storage on resolve. Audit is one INSERT and an extra dashboard tab. |
| F8 | **Time-to-resolve metric** per belt | Combine F2 (created_at) with F7 (resolved_at); compute avg/p50/p95 in `health/complaints.go` | Real ops value; only possible after F7. |
| F9 | **Translation cache** — store Gujarati translation in DB so the same description isn't re-translated | New `translations` table keyed by hash of source text; check before calling Gemini in `complaint/fetcher.go:215` | Cuts Gemini cost roughly to "one call per unique description". |
| F10 | **Dashboard complaint detail modal** — click row → side panel with full description, history, age | `health/complaints.go` template (modal HTML) + a `/complaints/{id}` JSON endpoint | Replaces the cramped table cells with a full view. |
| F11 | **Village drill-down** — click belt → list villages with counts | `health/complaints.go` (`/villages?belt=X` endpoint) + JS click handler; village field is stored, never displayed | Free win — data is already there. |
| F12 | **Per-belt routing rules** — different Telegram chats/threads per belt | Add `BeltRoutes map[string]string` to config; `telegram/client.go:SendComplaintMessage` picks chat by `complaint.Belt` | Useful when different field teams own different belts. |

### Tier C — Bigger features (L)

| # | Feature | Plug-in point | Notes |
|---|---------|---------------|-------|
| F13 (dont implement) | **WhatsApp inline buttons (interactive messages)** | `internal/whatsapp/client.go:sendImage`/`SendComplaintMessage` — switch from plain text to `InteractiveMessage` template | whatsmeow supports it; would let WhatsApp reach Telegram parity for one-tap resolve. |
| F14 (dont implement)| **Batch resolve** (`/resolve_all <belt> "<note>"`) | `telegram/client.go` new command; loops over `storage.GetByBelt` and calls `api.ResolveComplaint` paced by the existing rate limiter | Powerful but high-blast-radius — gate behind a confirmation flow. |
| F15 | **Scheduled summaries** — auto-post `/summary` at 09:00/18:00 IST | `main.go` ticker → reuse Telegram + WhatsApp send paths | Infra is already there; just a cron-style trigger. |
| F16 | **Multi-tenant config** (multiple subdivisions) | `config/` becomes a slice; storage gets a `tenant_id`; each tenant has its own session.Client | Only worth doing if you actually need to monitor a second SDn. |

---

## Suggested rollout order

1. **Week 1 — Tier 1 fixes** (#1–#6) plus **F5** (move-command UX) and **F1** (show consumer #). Pure low-risk wins.
2. **Week 2 — Tests** (auth + api + summary) and **F2** (age) + **F3** (belt tabs).
3. **Week 3 — Storage refactor** (#11) + **F7** (audit trail) → unlocks **F8** (time-to-resolve).
4. **Week 4 — Observability** (slog + Prometheus) + split `health/complaints.go` (#7).
5. **Later** — Tier C features when there's a real driver (multiple SDns, scheduled posts, etc.).

---

## Critical files referenced

| Purpose | Path |
|---------|------|
| Entry point | `main.go` |
| Dashboard | `internal/health/complaints.go`, `internal/health/server.go`, `internal/health/wshub.go` |
| Telegram | `internal/telegram/client.go` |
| WhatsApp | `internal/whatsapp/client.go`, `internal/whatsapp/bridge.go` |
| Scraping | `internal/complaint/fetcher.go`, `internal/complaint/worker.go` |
| Session/HTTP | `internal/session/client.go` |
| Storage | `internal/storage/storage.go` |
| Resolve API | `internal/api/resolve.go` |
| Config | `internal/config/config.go` |
| Translation | `internal/translate/translator.go` |
| Belt domain | `internal/belt/style.go`, `internal/belt/resolver.go` |
| Summary render | `internal/summary/image.go`, `internal/summary/fetcher.go`, `internal/summary/grouping.go` |

## Verification (how to validate any chosen item)

- **Code-quality fixes**: `go build ./... && go vet ./... && go test ./...` before/after; add a focused test where one is missing.
- **Feature work**: run the binary against a staging session (or with mocked `session.Client`), trigger via Telegram `/summary`, `/summarybelt`, dashboard, or the new endpoint; eyeball storage row + UI render.
- **End-to-end**: scrape cycle → confirm new field flows from `complaint.Fetcher` → storage → notification → dashboard JSON, all in one run.
- **Pre-existing failure** in `internal/belt/resolver_test.go::TestResolveFuzzyVillage` is unrelated to anything above; flag separately.
