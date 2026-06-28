# ROADMAP Implementation Progress

Tracking implementation of `ROADMAP.md`. Tick items as `[x]` when done. Continue across sessions.

**User decisions baked in:**
- Skipped: F1 (already exists), F7 (no audit trail needed), F8 (depends on F7), F13 (no WhatsApp inline buttons), F14 (no batch resolve).
- Prometheus-style `/metrics` endpoint is wanted (Tier 5 item promoted).
- Code will be reviewed by codex.
- Logic preservation matters more than exhaustive new tests — write tests only where they protect a real invariant or risky path.

---

## Tier 1 — Correctness fixes

- [x] **T1.1** Capture/log `err` from `io.ReadAll(resp.Body)` — `internal/session/client.go:207`, `internal/telegram/client.go:533`
- [x] **T1.2** Check `json.Unmarshal` err in Telegram long-poll — `internal/telegram/client.go:535` (sendPhoto path; covered by T1.1 edit)
- [x] **T1.3** Replace runtime `log.Fatalf` (column-ensure) with returned error — `internal/storage/storage.go:860`. `New()` now returns `(*Storage, error)`; init-path Fatalfs documented in the New() doc comment.
- [x] **T1.4** Add timeouts to `context.Background()` network I/O — wrapped each call site with `WithTimeout(...,30s)`. WhatsApp uses pkg-level `requestTimeout = 30s`. Parent-ctx threading deferred to T1.6.
- [x] **T1.5** Reuse package-level `*http.Client` for Telegram photo upload — `internal/telegram/client.go:526` now uses `c.httpClient`.
- [x] **T1.6** Pass `HandleEvents` ctx into WhatsApp goroutine handlers. `handle*Command` and `handleResolve` now take `ctx` first; `sendImage` accepts a parent ctx and layers `requestTimeout` on top; `ctx.Err()` checks at major boundaries; per-belt loop bails on cancellation. handleResolve only checks ctx at entry — once the resolve API call has gone out, post-call cleanup must complete to avoid leaking state.

## Tier 2 — Tests (selective; only risky paths)

- [x] **T2.1** Login + captcha tests. Captcha solver covered with 16 sub-cases (every operator variant + 6 sad paths). End-to-end Login fixture (httptest server) covers happy path + 4 sad paths (unsolvable captcha, missing captcha element, bad credentials, missing token in API response). `IsSessionExpired` covered. Auth shim covered with 2 forwarding tests. All tests live in `session/client_test.go` and `auth/login_test.go`.
- [x] **T2.2** Resolve API tests. 8 cases against an httptest fake DGVCL: form shape (POST + Content-Type + 3 form fields), `ERROR:` response, HTTP 500, debug-mode skip + no metric movement, success metric increments, error-response failure metric, transport-error failure metric. Added `metrics.Counter.Value()` for clean test assertions and a tiny `var resolveEndpoint` package-level override hook in `api/resolve.go` (real config migration still tracked in T4.3).
- [x] **T2.3** Summary grouping/sorting tests. `parseComplaintDate` covered with 13 sub-cases including all 9 accepted layouts + parse failures + IST locality. `complaintDateLess` covered for the 4 ordering branches (both parseable / equal / one parseable / neither). `groupComplaints` covered for empty-belt → "Unknown" fallback, intra-group date+number sort, inter-group oldest-first sort with alphabetical tie-break, and empty-input contract. Public `GroupComplaints` smoke test locks in the dashboard-visible shape.
- [x] **T2.4** Config tests. Helper parsers covered exhaustively (`getEnvOrDefault`, `getEnvInt`, `getEnvFloat`, `getEnvDuration` — happy + invalid + empty cases). `Validate` covered for all required-field branches (username, password, URLs, MaxPages, WorkerPoolSize). LoadConfig precedence chain locked in: env-var override (top), embedded `.env` fallback (middle), hard-coded default (bottom). WHATSAPP_RESOLVE_ENABLED bool is strict `"true"` literal — `True`/`TRUE`/`1`/`yes`/`false` all yield false. Empty/unset values fall through to embedded layer (per the precedence rule).
- [ ] (skip if logic untouched) `internal/whatsapp/`, `internal/translate/`

## Tier 3 — Structural cleanups

- [x] **T3.1** Split `internal/health/complaints.go` (was 2290 lines) into 4 files. `complaints.go` now keeps only the page template + payload types (1884 lines, dominated by the embedded HTML/CSS/JS). Routes moved to `dashboard_routes.go` (238 lines, all `mux.HandleFunc` registrations). JSON payload + small response helpers (`writeJSONError`, `colorToHex`) live in `dashboard_payload.go` (86 lines). CSV/JSON export flattening in `dashboard_export.go` (125 lines). Pure file-shuffle, no behavior change. Imports re-scoped per file.
- [x] **T3.2** Extracted main goroutine boot + ticker loop. New `daemonDeps` struct bundles long-lived state. New helpers: `loginWithRetry`, `triggerFetch` (single source of truth for fetchMu lock + `fetchWithRetry`), `startBackgroundHandlers` (Telegram + WhatsApp goroutine launches), `runFetchLoop` (the for/select that previously lived inline in `main()`). `fetchWithRetry` now takes `*daemonDeps` instead of 13 positional args. `main()` body reads top-to-bottom: configure → start → wait.
- [x] **T3.3** Extracted `recoverSession(sc, loginURL, user, pass) bool` from `fetchWithRetry`. Two-step recovery (plain re-login → reset-jar + login) lives in one place; the retry loop is back to a clean if/else.
- [x] **T3.4** Moved command handlers into `internal/telegram/commands.go`. New file holds: `handleSummaryCommand`, `handleSummaryBeltCommand`, `handleMoveCommand`, `sendMoveUsage`, `sendTextMessage`, `isMoveCommand`, `extractComplaintIDFromText`, `rewriteComplaintBeltLine`, `htmlEscape`. `client.go` shrunk 1227→1033 lines and now reads as transport + lifecycle (HTTP, long-poll, message router, callback-query handler). Pure file-shuffle, no behavior change. Imports trimmed accordingly.
- [x] **T3.5** Extract complaint-no parser. New leaf package `internal/complaintid` (couldn't live in `complaint` because that package already imports tg + wa — would cycle). Single `FromText(text)` accepts both prefix variants (`"📋 Complaint : "` from Telegram, `"📋 Complaint:"` from WhatsApp) — fixes the latent cross-channel bug where a Telegram /move reply to a WhatsApp-formatted quote couldn't recover the number. tg + wa now both call into it. 13-case test suite covers both formats, multi-line inputs, missing colon, missing number, and rejection of the wrong emoji.

## Tier 4 — Config / hardcoded values

- [x] **T4.1** `TELEGRAM_RATE_INTERVAL_MS` env honoured. New `parseRateInterval` (strict: rejects non-positive / non-integer), `Client.rateInterval`, and `effectiveRateInterval()` so `doRequest` reads one source of truth. Default falls back to `defaultRateInterval = 35ms`. 7 cases + override-vs-default test.
- [x] **T4.2** Top-of-file `const` block in `internal/telegram/client.go` — `httpClientTimeout`, `longPollSeconds`, `defaultRateInterval`. NewClient and getUpdates now reference them instead of inline magic numbers.
- [x] **T4.3** Resolve URL moved to config. New `cfg.ResolveURL` from `DGVCL_RESOLVE_URL` env (default = production URL). `api.SetResolveEndpoint(url)` is the public boot-time installer (empty input is a no-op so misconfig can't blank the endpoint). `main.go` calls it right after `logging.Setup`. `DefaultResolveEndpoint` exported for callers / tests that want the production literal. Test for the setter + no-op contract.

## Tier 5 — Operations / observability

- [x] **T5.1** Structured logging via `log/slog`. New `internal/logging` pkg + `LOG_FORMAT` env var (text|json, default text). `Setup` installs slog handler and reroutes stdlib `log` through slog so legacy call sites keep working in both modes. Migrated `complaint/{fetcher,worker}.go` and `session/client.go` to `slog.Info/Warn/Error` with structured attrs. Tests cover stdlib redirect in both modes, newline trimming, and unknown-format fallback. Other packages still use stdlib log; they emit via slog automatically and can be migrated incrementally.
- [x] **T5.2** **Prometheus `/metrics` endpoint** — new `internal/metrics` pkg (no external dep, hand-rolled text format). Counters: `cmon_fetch_attempts_total`, `cmon_fetch_failures_total`, `cmon_complaints_seen_total`, `cmon_resolve_calls_total`, `cmon_resolve_failures_total`, `cmon_telegram_sends_total`, `cmon_telegram_send_failures_total`, `cmon_whatsapp_sends_total`, `cmon_whatsapp_send_failures_total`. Gauges: `cmon_last_fetch_success_unix_seconds`, `cmon_open_complaints{belt=...}` (live from storage). Endpoint wired in `health/server.go`. Tests cover counter/gauge encoding, label sort order, label escaping, duplicate-name panic.
- [x] **T5.3** Enrich `/health`. New dedicated `/health` JSON endpoint (returns `503` when `unhealthy`, `200` otherwise). Status now includes `last_fetch_success_at` (anchor that doesn't move on failure) and `consecutive_errors` (reset on each success). New `starting` status replaces silent "not started" state. Endpoint registration extracted to `registerStatusEndpoints` for testability; covered by 4 new tests in `server_test.go`.
- [x] **T5.4** Graceful shutdown audit. Background goroutines (`tg.HandleUpdates`, `wa.HandleEvents`) tracked via `sync.WaitGroup`. Explicit ordered shutdown sequence at end of `main()`: HTTP server `Shutdown()` (10s timeout) → cancel handler ctxs → wait for handlers (35s timeout, covers TG long-poll) → acquire `fetchMu` (waits for any in-flight scrape, including dashboard `/refresh`) → disconnect WA → close translator → close storage. `storage.Close` no longer deferred — runs only after the wait sequence so it cannot race with mid-flight DB writes. `health.StartServer` now returns `*http.Server` for `Shutdown`. Added `waitWithTimeout` helper with two unit tests.

## Features

- [x] ~~F1 — Show consumer #~~ (already exists)
- [x] **F2** Complaint age. New `AgeMinutes int64` field on `summary.Complaint`, computed at fetch time from `ComplainDate` (returns 0 for unparseable / future dates). New `Age` column in summary image table; new `Age` column in dashboard with mirrored Go/JS `formatAge` helper ("3d 4h" / "5h 12m" / "23m" form). 16 sub-cases test the formatter + computer.
- [x] **F3** Per-belt dashboard tabs. Pill row above the groups list with one pill per belt (+ "All" pill); active filter syncs to `?belt=` via `history.replaceState` so refresh / share preserves it. Auto-clears when the active belt empties (e.g., last complaint resolved). Pure client-side filter — no server changes needed since `/data` already returns belt-grouped JSON.
- [x] **F4** `/export.csv` and `/export.json` endpoints. Both reuse `buildComplaintDashboardPayload` and flatten its belt-grouped result via a new `exportRow` struct (15 fields incl. computed `age` string). Optional `?belt=<display-name>` query param filters to one belt — same key the F3 dashboard tabs use, so the dashboard URL doubles as an export URL. `Content-Disposition` returns date-stamped filenames (`cmon-complaints-2026-05-10.csv`). 3 tests cover JSON shape, belt-filter contract, and CSV header / quoting / round-trip.
- [x] **F5** `/move` discoverability — `sendMoveUsage` now includes a concrete example; "Could not find the complaint number" path now lists valid belts. (Default-case usage call already existed.)
- [x] **F6** WhatsApp resolve-by-reply default-on. Hard-coded default flipped from `"false"` to `"true"` in config.go (embedded `.env` already shipped `=true`; this just makes the bottom tier match). Doc comment updated.
- [x] ~~F7 — Resolution audit trail~~ (skipped)
- [x] ~~F8 — Time-to-resolve~~ (skipped, depended on F7)
- [x] ~~F9 — Translation cache~~ (implemented and then removed at user request — risk of stale/wrong translations being cached forever wasn't worth the saved Gemini calls; direct call restored).
- [ ] **F10** Dashboard complaint detail modal + `/complaints/{id}` JSON endpoint
- [x] **F11** Village drill-down. New `/villages?belt=X` endpoint returns `{belt, total, villages: [{name, count}]}` sorted by descending count then name. Accepts both display name and canonical key (canonicalises via `belt.Canonicalize`). Storage gained `GetVillageCountsByBelt(canonicalBelt)` (case-insensitive match; empty villages bucketed as "Unknown"). Dashboard adds a small "Villages" pill button in each group header that fetches and renders a positioned popover; click-outside or Escape closes. Three endpoint tests cover happy-path, display-name acceptance, and missing-belt 400.
- [x] **F12** Per-belt Telegram routing. New `TELEGRAM_BELT_ROUTES` env (`belt=chatID,belt=chatID` form) parsed by `parseBeltRoutes` (strict: drops malformed pairs). New `Client.BeltRoutes map[string]string` populated from `cfg.TelegramBeltRoutes` in main. `Client.ChatIDForBelt(belt)` is the lookup; `SendComplaintMessage` and the two cross-channel resolved-edits (`markResolvedComplaints` in main, WhatsApp `handleResolve`) now use it. Doc comment lists the known limitation: interactive callback flows (resolve prompt, /move ack) still post to default chat — explicit follow-up. 11 parser tests + 9 ChatIDForBelt tests + nil-receiver guard.
- [x] ~~F13 — WhatsApp inline buttons~~ (skipped)
- [x] ~~F14 — Batch resolve~~ (skipped)
- [x] **F15** Scheduled summaries. New `SCHEDULED_SUMMARIES` env var, comma-separated HH:MM in IST (e.g. `"09:00,18:00"`). Empty disables. New `runScheduledSummaries` goroutine in main.go computes the next firing each iteration off `time.Now()` so it's robust to long sleeps; fires `tg.PostScheduledSummary` + `wa.PostScheduledSummary` (new public wrappers around the existing `/summary` handlers). Tracked in the same `bgWg` so graceful shutdown waits for an in-flight scheduled summary. Strict HH:MM parsing in config (drops malformed entries silently). Tests cover the parser (10 cases), `parseHHMMToday` (8 cases), and `nextScheduledFire` (5 scenarios incl. all-passed-today rolls to tomorrow, equal-to-now rolls forward, no-valid-entries returns ok=false).
- [ ] **F16** Multi-tenant config (only when a second SDn is real)

---

## Suggested execution order

1. Tier 1 fixes (T1.1–T1.6) + F5 — pure low-risk wins.
2. F2 (age) + F3 (belt tabs) + F11 (village drill-down) — UI uplift batch.
3. T5.2 (`/metrics`) + T5.3 (`/health` richness) + T5.1 (slog).
4. T3.5 (parse extraction) + T3.1 (split `health/complaints.go`).
5. F9 (translation cache) + F4 (export endpoints).
6. T4.1–T4.3 (config), T3.2–T3.4 (main + telegram refactor), T5.4 (shutdown audit).
7. F6, F10, F12, F15.
8. T2 tests filled in alongside the changes that touch each package.
9. F16 only if a second SDn appears.

## Verification each step

`go build ./... && go vet ./... && go test ./...` before/after every change. Run binary against staging session for feature work.
