// CMON - Complaint Monitoring System
//
// This application monitors the DGVCL complaint portal and sends
// real-time notifications via Telegram when new complaints are filed.
//
// Architecture:
//   - Main thread: Orchestrates fetch loop and error recovery
//   - Health check server: Background HTTP server for monitoring
//   - Telegram handler: Background goroutine for processing callbacks
//   - Worker pool: Concurrent complaint processing (created per fetch)
//
// Flow:
//  1. Load configuration and initialize components
//  2. Login to DGVCL portal via HTTP (no browser required)
//  3. Initial fetch of complaints
//  4. Start periodic refresh loop (every 15 minutes by default)
//  5. Handle errors with retry logic and session reset
//  6. Graceful shutdown on SIGTERM/SIGINT
//
// Error recovery strategy:
//   - Session expired → Re-login
//   - Re-login failed → Reset session (new cookie jar) and re-login
//   - All retries failed → Send critical alert to Telegram
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	_ "time/tzdata"

	"cmon/internal/api"
	"cmon/internal/auth"
	"cmon/internal/belt"
	"cmon/internal/complaint"
	"cmon/internal/config"
	"cmon/internal/errors"
	"cmon/internal/health"
	"cmon/internal/logging"
	"cmon/internal/metrics"
	"cmon/internal/session"
	"cmon/internal/storage"
	"cmon/internal/telegram"
	"cmon/internal/translate"
	"cmon/internal/whatsapp"
)

// fetchMu prevents concurrent scrape cycles (ticker vs dashboard refresh).
var fetchMu sync.Mutex

// daemonDeps bundles every long-lived dependency the daemon's hot paths
// (fetch, login retry, scheduler, dashboard refresh) need. Pulled out to
// keep helper signatures readable — fetchWithRetry would otherwise take 13
// positional arguments.
type daemonDeps struct {
	cfg           *config.Config
	sc            *session.Client
	stor          *storage.Storage
	tg            *telegram.Client
	wa            *whatsapp.Client
	translator    *translate.Translator
	healthMonitor *health.Monitor
}

func main() {
	// Force Indian Standard Time (IST) for all time operations
	ist, err := time.LoadLocation("Asia/Kolkata")
	if err != nil {
		log.Fatal("❌ Failed to load IST timezone:", err)
	}
	time.Local = ist

	log.Println("🚀 Starting CMON...")

	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatal("❌ Configuration error:", err)
	}

	// Install slog as the application-wide structured logger and reroute the
	// stdlib log package through it. Done as soon as config is parsed so every
	// subsequent log line is in the configured format.
	logging.Setup(cfg.LogFormat)

	// Point the DGVCL resolve client at the configured endpoint. Default
	// matches production; override via DGVCL_RESOLVE_URL for staging.
	api.SetResolveEndpoint(cfg.ResolveURL)

	// Initialize storage. Closed at the very end of the graceful shutdown
	// sequence — never via defer — so it cannot run while a goroutine is
	// still mid-write. See the explicit shutdown block at the bottom of main.
	stor, err := storage.New()
	if err != nil {
		log.Fatalf("❌ Failed to initialize storage: %v", err)
	}

	// Live gauge: cmon_open_complaints{belt=...}. Read from storage at scrape
	// time so the value can never drift from the source of truth.
	metrics.RegisterOpenComplaintsByBelt(stor.GetPendingCountsByBelt)

	// Step 3: Initialize Telegram client (optional)
	tg := telegram.NewClient()
	if tg != nil && len(cfg.TelegramBeltRoutes) > 0 {
		tg.BeltRoutes = cfg.TelegramBeltRoutes
		log.Printf("✓ Telegram per-belt routing enabled for %d belt(s)", len(cfg.TelegramBeltRoutes))
	}

	// Step 3a: Initialize WhatsApp client (optional)
	wa := whatsapp.NewClient()

	// Step 3b: Initialize Gemini Translator (optional)
	translator, err := translate.NewTranslator(context.Background(), cfg.GeminiAPIKey, cfg)
	if err != nil {
		log.Printf("⚠️  Translator init failed (translation disabled): %v", err)
	}

	// Step 4: Initialize health monitor
	healthMonitor := health.NewMonitor()

	// Step 5: Create authenticated session client (replaces browser context)
	sc, err := session.New(cfg.APIRateLimitRPS, cfg.APIRateLimitBurst, cfg.APIMaxRetries429)
	if err != nil {
		log.Fatal("❌ Failed to create session client:", err)
	}
	log.Println("✓ Session client created")

	// Bundle the long-lived state so helpers don't take 13 positional args.
	deps := &daemonDeps{
		cfg:           cfg,
		sc:            sc,
		stor:          stor,
		tg:            tg,
		wa:            wa,
		translator:    translator,
		healthMonitor: healthMonitor,
	}

	// Build the refresh function that the dashboard can call to trigger a scrape.
	// Uses TryLock so concurrent refresh requests return immediately instead of queuing.
	refreshFn := func() error {
		if !fetchMu.TryLock() {
			return fmt.Errorf("a scrape cycle is already in progress, please wait")
		}
		defer fetchMu.Unlock()
		// silent: don't send critical Telegram alerts for dashboard-triggered scrapes
		return fetchWithRetry(deps, true)
	}

	resolveFn := func(apiID string, remark string) error {
		lowerAPIID := strings.ToLower(apiID)
		if strings.HasPrefix(lowerAPIID, "local") || strings.HasPrefix(lowerAPIID, "l-") || strings.HasPrefix(lowerAPIID, "vld") {
			log.Printf("✅ Resolving local complaint %s...", apiID)

			messageID := stor.GetMessageID(apiID)
			consumerName := stor.GetConsumerName(apiID)
			if consumerName == "" {
				consumerName = "Unknown"
			}

			resolvedMessage := fmt.Sprintf(
				"✅ <b>RESOLVED (LOCAL)</b>\n\n"+
					"Complaint #%s\n"+
					"👤 %s\n"+
					"🕐 %s",
				apiID,
				consumerName,
				time.Now().Format("02 Jan 2006, 03:04 PM"),
			)

			if tg != nil && messageID != "" {
				if err := tg.EditMessageText(tg.ChatIDForBelt(stor.GetBelt(apiID)), messageID, resolvedMessage); err != nil {
					log.Printf("⚠️  Failed to edit Telegram message for local complaint %s: %v", apiID, err)
				}
			}

			if wa != nil {
				waResolvedMsg := fmt.Sprintf(
					"✅ RESOLVED (LOCAL)\n\nComplaint #%s\n👤 %s\n🕐 %s",
					apiID,
					consumerName,
					time.Now().Format("02 Jan 2006, 03:04 PM"),
				)
				if waErr := wa.SendMessage(waResolvedMsg); waErr != nil {
					log.Printf("⚠️  Failed to send WhatsApp resolved notice: %v", waErr)
				}
			}

			if err := stor.Remove(apiID); err != nil {
				return fmt.Errorf("failed to remove local complaint from storage: %w", err)
			}
			return nil
		}

		return api.ResolveComplaint(sc, apiID, remark, cfg.DebugMode)
	}

	registerLocalFn := func(complainantName, mobileNo, consumerNo, village, beltName, address, area, description string) (string, error) {
		// Generate custom VLDYYYYMMDDSR ID
		complaintID, err := stor.GenerateLocalComplaintID()
		if err != nil {
			return "", fmt.Errorf("failed to generate complaint ID: %w", err)
		}
		complainDate := time.Now().Format("02/01/2006 15:04:05")

		// Handle Auto Assign belt
		var canonicalBelt string
		if beltName == "" || strings.ToLower(beltName) == "auto" {
			resolved := belt.Resolve(area, address, description)
			canonicalBelt = resolved.Belt
			village = resolved.Village
		} else {
			var ok bool
			canonicalBelt, ok = belt.Canonicalize(beltName)
			if !ok {
				canonicalBelt = "Unknown"
			}
			// Attempt to resolve village
			resolved := belt.Resolve(area, address, description)
			if resolved.Belt == canonicalBelt {
				village = resolved.Village
			}
		}

		record := storage.Record{
			ComplaintID:  complaintID,
			APIID:        complaintID,
			ConsumerName: complainantName,
			Village:      village,
			Belt:         canonicalBelt,
			ConsumerNo:   consumerNo,
			MobileNo:     mobileNo,
			Address:      address,
			Area:         area,
			Description:  description,
			ComplainDate: complainDate,
		}

		// Translate details
		translatedName := record.ConsumerName
		translatedDesc := record.Description
		translatedAddr := fmt.Sprintf("%s, %s", record.Address, record.Area)

		if translator != nil {
			texts := []string{translatedName, translatedDesc, translatedAddr}
			out, err := translator.BatchTranslateToGujarati(context.Background(), texts)
			if err == nil {
				translatedName = out[0]
				translatedDesc = out[1]
				translatedAddr = out[2]
			}
		}

		gujaratiText := ""
		if translatedName != "" || translatedDesc != "" || translatedAddr != "" {
			gujaratiText = fmt.Sprintf("👤 %s\n💬 %s\n📍 %s", translatedName, translatedDesc, translatedAddr)
		}

		// Persist to DB
		if err := stor.SaveMultiple([]storage.Record{record}); err != nil {
			return "", fmt.Errorf("failed to save local complaint: %w", err)
		}
		metrics.ComplaintsSeenTotal.Inc()

		// Send Telegram notification
		details := complaint.Details{
			ComplainNo:      record.ComplaintID,
			ConsumerNo:      record.ConsumerNo,
			ComplainantName: record.ConsumerName,
			MobileNo:        record.MobileNo,
			Description:     record.Description,
			ComplainDate:    record.ComplainDate,
			ExactLocation:   record.Address,
			Area:            record.Area,
			Village:         record.Village,
			Belt:            record.Belt,
		}
		prettyJSON, _ := json.MarshalIndent(details, "  ", "  ")

		if tg != nil {
			msgID, err := tg.SendComplaintMessage(string(prettyJSON), record.ComplaintID, gujaratiText)
			if err == nil && msgID != "" {
				_ = stor.SetMessageID(record.ComplaintID, msgID)
			}
		}

		// Send WhatsApp notification
		if wa != nil {
			waText := complaint.BuildWhatsAppMessage(details, gujaratiText)
			_ = wa.SendComplaintMessage(waText, record.ComplaintID, stor)
		}

		// Refresh Dashboard WebSockets
		if health.WSHub != nil {
			health.WSHub.BroadcastRefresh()
		}

		return complaintID, nil
	}

	// Step 6: Start health check server in background. Returned *http.Server
	// is shut down explicitly at the end of main so in-flight requests
	// (notably /refresh, which holds fetchMu) finish before storage closes.
	httpServer := health.StartServer(healthMonitor, cfg.HealthCheckPort, sc, stor, refreshFn, resolveFn, registerLocalFn)

	// bgWg tracks long-lived background goroutines that must finish before
	// storage closes. Telegram + WhatsApp handlers can be mid-DB-write when a
	// shutdown signal arrives; we wait for them rather than racing.
	var bgWg sync.WaitGroup

	// Step 7: Telegram + WhatsApp event handlers
	callbackCancel, waCancel := startBackgroundHandlers(deps, &bgWg)
	defer callbackCancel()
	defer waCancel()

	// Run initial login and fetch in a background goroutine so startup is instant and non-blocking
	go func() {
		log.Println("🔐 Logging in...")
		if err := loginWithRetry(deps); err != nil {
			log.Printf("⚠️  Initial login failed: %v. Continuing in offline mode.", err)
			healthMonitor.UpdateFetchStatus(fmt.Sprintf("error: login failed: %v", err))
			if tg != nil {
				_ = tg.SendCriticalAlert(
					"Startup Login Failure",
					fmt.Sprintf("Unable to log in during startup: %v", err),
					cfg.MaxLoginRetries,
				)
			}
		} else {
			log.Println("✓ Logged in")
			log.Println("📬 Fetching complaints...")
			if err := triggerFetch(deps, false); err != nil {
				log.Printf("⚠️  Failed initial fetch: %v. Continuing in offline mode.", err)
				healthMonitor.UpdateFetchStatus(fmt.Sprintf("error: initial fetch failed: %v", err))
			} else {
				healthMonitor.UpdateFetchStatus("success")
				if health.WSHub != nil {
					health.WSHub.BroadcastRefresh()
				}
			}
		}
	}()

	log.Printf("⏰ Running — next check in %v\n", cfg.FetchInterval)
	log.Println("═══════════════════════════════════════════════════════════")

	// Step 11: Set up graceful shutdown
	shutdownCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Step 11a: Scheduled summaries (cfg.ScheduledSummaries empty → no-op)
	if len(cfg.ScheduledSummaries) > 0 {
		bgWg.Add(1)
		go func() {
			defer bgWg.Done()
			runScheduledSummaries(shutdownCtx, cfg.ScheduledSummaries, tg, wa, sc, stor)
		}()
	}

	// Step 12: Periodic fetch ticker — blocks until shutdownCtx fires.
	runFetchLoop(shutdownCtx, deps)

	// Graceful shutdown — explicit, ordered, never via defer for state that
	// matters. Each step has a short timeout so a stuck goroutine cannot
	// indefinitely block process exit.
	log.Println("🛑 Shutdown signal received, cleaning up...")

	// 1. Stop accepting new HTTP requests; wait briefly for in-flight ones
	//    (notably /refresh, which may hold fetchMu) to drain.
	httpShutdownCtx, httpCancel := context.WithTimeout(context.Background(), 10*time.Second)
	if err := httpServer.Shutdown(httpShutdownCtx); err != nil {
		log.Printf("⚠️  HTTP server shutdown error: %v", err)
	}
	httpCancel()

	// 2. Cancel handler contexts so Telegram long-poll and WhatsApp event
	//    loop start unwinding.
	callbackCancel()
	waCancel()

	// 3. Wait for the handler goroutines to actually exit. Telegram long-poll
	//    can hang for up to ~30s on its current request; cap the wait so we
	//    don't block the operator forever on a wedged upstream.
	if waited := waitWithTimeout(&bgWg, 35*time.Second); !waited {
		log.Println("⚠️  Background handlers did not exit within 35s; closing storage anyway")
	}

	// 4. Acquire fetchMu to make sure no scrape (ticker- or dashboard-triggered)
	//    is still mid-DB-write. Lock — not TryLock — so this blocks until the
	//    in-flight scrape finishes. Then we hold it until storage closes.
	fetchMu.Lock()

	// 5. Disconnect WhatsApp + close translator before storage. WhatsApp's own
	//    sqlite store is independent of complaint storage, but ordering keeps
	//    the shutdown log readable.
	if wa != nil {
		wa.Disconnect()
	}
	if translator != nil {
		translator.Close()
	}

	// 6. Close the complaint database last.
	if err := stor.Close(); err != nil {
		log.Printf("⚠️  Failed to close database: %v", err)
	}

	log.Println("✅ Cleanup complete, shutting down")
}

// recoverSession is the two-step session recovery the fetch retry loop runs
// when a request comes back with SessionExpiredError. It first attempts a
// plain re-login on the existing cookie jar; if that fails (e.g. because the
// jar is in a stuck state), it resets the jar and re-logs in. Returns true
// when the caller should retry the fetch, false if both attempts failed.
func recoverSession(sc *session.Client, loginURL, username, password string) bool {
	log.Println("🔐 Attempting re-login...")
	if err := auth.Login(sc, loginURL, username, password); err == nil {
		log.Println("✓ Re-login successful, retrying fetch on next loop...")
		return true
	} else {
		log.Println("❌ Re-login failed:", err)
	}

	// Plain re-login failed → reset the jar (the browser-restart equivalent)
	// and try again. If this still fails the caller exits the retry loop.
	log.Println("🔄 Resetting session (clearing cookies)...")
	if err := sc.Reset(); err != nil {
		log.Println("⚠️  Session reset failed:", err)
	}

	log.Println("🔐 Attempting login after session reset...")
	if err := auth.Login(sc, loginURL, username, password); err == nil {
		log.Println("✓ Login successful after session reset, retrying fetch on next loop...")
		return true
	} else {
		log.Println("❌ Login failed even after session reset:", err)
	}
	return false
}

// waitWithTimeout waits for wg with a deadline. Returns true if wg finished
// within the deadline, false on timeout.
func waitWithTimeout(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// fetchWithRetry implements the complete error handling flow with retries.
//
// Retry strategy:
//  1. Attempt fetch
//  2. If session expired → Re-login and retry
//  3. If re-login failed → Reset session (new cookie jar), re-login, and retry
//  4. Repeat up to maxRetries times
//  5. If all retries failed → Send critical alert
//
// silent suppresses the critical-alert Telegram message — used by the
// dashboard refresh path where the operator is already watching the page.
func fetchWithRetry(d *daemonDeps, silent bool) error {
	var lastErr error

	metrics.FetchAttemptsTotal.Inc()

	for attempt := 0; attempt <= d.cfg.MaxFetchRetries; attempt++ {
		if attempt > 0 {
			log.Printf("🔄 Retry attempt %d/%d...", attempt, d.cfg.MaxFetchRetries)
		}

		fetcher := complaint.New(d.sc, d.stor, d.tg, d.wa, d.cfg, d.translator)
		activeComplaintIDs, err := fetcher.FetchAll(d.cfg.ComplaintURL)

		if err == nil {
			markResolvedComplaints(d.stor, d.tg, d.wa, activeComplaintIDs)
			d.healthMonitor.UpdateFetchStatus("success")
			metrics.LastFetchSuccessUnixSeconds.Set(time.Now().Unix())
			return nil
		}

		lastErr = err

		if sessionErr, ok := err.(*errors.SessionExpiredError); ok {
			log.Println("🔄 Session expired:", sessionErr.Message)
			if recoverSession(d.sc, d.cfg.LoginURL, d.cfg.Username, d.cfg.Password) {
				continue
			}
		} else {
			log.Println("⚠️  Error fetching complaints:", err)
			time.Sleep(5 * time.Second)
		}
	}

	log.Println("❌ All retry attempts failed.")

	metrics.FetchFailuresTotal.Inc()
	d.healthMonitor.UpdateFetchStatus(fmt.Sprintf("error: %v", lastErr))

	if !silent && d.tg != nil && d.healthMonitor.GetStatus().ConsecutiveErrors == 1 {
		log.Println("🚨 Sending critical failure alert...")
		alertErr := d.tg.SendCriticalAlert(
			"Fetch/Login Failure",
			fmt.Sprintf("Unable to fetch complaints after %d attempts. Last error: %v", d.cfg.MaxFetchRetries, lastErr),
			d.cfg.MaxFetchRetries,
		)
		if alertErr != nil {
			log.Println("⚠️  Failed to send Telegram alert:", alertErr)
		}
	}

	return fmt.Errorf("all %d retry attempts failed: %w", d.cfg.MaxFetchRetries, lastErr)
}

// triggerFetch wraps fetchWithRetry with the fetchMu lock held. Every scrape
// (initial, ticker, dashboard /refresh, scheduled) goes through this so the
// lock contract is enforced in one place.
func triggerFetch(d *daemonDeps, silent bool) error {
	fetchMu.Lock()
	defer fetchMu.Unlock()
	return fetchWithRetry(d, silent)
}

// loginWithRetry is the boot-time login loop. Runs up to MaxLoginRetries
// times with LoginRetryDelay between attempts. Failure is fatal — the
// caller is expected to log.Fatal on a non-nil return.
func loginWithRetry(d *daemonDeps) error {
	var loginErr error
	for attempt := 1; attempt <= d.cfg.MaxLoginRetries; attempt++ {
		loginErr = auth.Login(d.sc, d.cfg.LoginURL, d.cfg.Username, d.cfg.Password)
		if loginErr == nil {
			return nil
		}
		if attempt < d.cfg.MaxLoginRetries {
			log.Printf("   ❌ Login failed: %v", loginErr)
			log.Printf("   ⏳ Retrying in %v...", d.cfg.LoginRetryDelay)
			time.Sleep(d.cfg.LoginRetryDelay)
		}
	}
	return loginErr
}

// startBackgroundHandlers spawns the long-lived Telegram and WhatsApp event
// goroutines and adds them to bgWg so the shutdown sequence can wait for
// them. Returns the cancel funcs the shutdown sequence calls to start the
// unwind.
func startBackgroundHandlers(d *daemonDeps, bgWg *sync.WaitGroup) (callbackCancel, waCancel context.CancelFunc) {
	callbackCtx, cbCancel := context.WithCancel(context.Background())
	if d.tg != nil {
		bgWg.Add(1)
		go func() {
			defer bgWg.Done()
			d.tg.HandleUpdates(callbackCtx, d.sc, d.stor)
		}()
	}

	waCtx, wCancel := context.WithCancel(context.Background())
	if d.wa != nil {
		bgWg.Add(1)
		go func() {
			defer bgWg.Done()
			d.wa.HandleEvents(waCtx, d.sc, d.stor, d.tg, d.cfg.WhatsAppResolveEnabled, d.cfg.DebugMode)
		}()
	}

	return cbCancel, wCancel
}

// runFetchLoop blocks on the periodic fetch ticker until shutdownCtx is
// cancelled, at which point it returns so the caller can run the graceful
// shutdown sequence.
func runFetchLoop(shutdownCtx context.Context, d *daemonDeps) {
	ticker := time.NewTicker(d.cfg.FetchInterval)
	defer ticker.Stop()

	for {
		select {
		case <-shutdownCtx.Done():
			return
		case <-ticker.C:
			log.Printf("📬 Refreshing — %s", time.Now().Format("15:04:05"))
			if err := triggerFetch(d, false); err != nil {
				log.Println("⚠️  Final error after all retry attempts:", err)
			} else if health.WSHub != nil {
				health.WSHub.BroadcastRefresh()
			}
			log.Println("═══════════════════════════════════════════════════════════")
		}
	}
}

// markResolvedComplaints checks for complaints that were previously seen
// but are no longer on the website, and marks them as resolved in Telegram.
func markResolvedComplaints(stor *storage.Storage, tg *telegram.Client, wa *whatsapp.Client, activeIDs []string) {
	activeIDsMap := make(map[string]bool)
	for _, id := range activeIDs {
		activeIDsMap[id] = true
	}

	allSeen := stor.GetAllSeenComplaints()

	resolvedCount := 0
	for _, complaintID := range allSeen {
		// Skip local complaints from auto-resolution on website sync
		apiID := stor.GetAPIID(complaintID)
		lowerID := strings.ToLower(complaintID)
		lowerAPIID := strings.ToLower(apiID)
		if strings.HasPrefix(lowerAPIID, "local") || 
			strings.HasPrefix(lowerAPIID, "l-") || 
			strings.HasPrefix(lowerAPIID, "vld") ||
			strings.HasPrefix(lowerID, "local") || 
			strings.HasPrefix(lowerID, "l-") ||
			strings.HasPrefix(lowerID, "vld") {
			continue
		}

		if !activeIDsMap[complaintID] {
			log.Printf("✅ Marking complaint %s as resolved", complaintID)

			messageID := stor.GetMessageID(complaintID)
			consumerName := stor.GetConsumerName(complaintID)
			if consumerName == "" {
				consumerName = "Unknown"
			}

			resolvedMessage := fmt.Sprintf(
				"✅ <b>RESOLVED</b>\n\n"+
					"Complaint #%s\n"+
					"👤 %s\n"+
					"🕐 %s",
				complaintID,
				consumerName,
				time.Now().Format("02 Jan 2006, 03:04 PM"),
			)

			if tg != nil {
				if messageID == "" {
					log.Printf("⚠️  Complaint %s has no Telegram message ID; removing from storage based on website state", complaintID)
				} else if err := tg.EditMessageText(tg.ChatIDForBelt(stor.GetBelt(complaintID)), messageID, resolvedMessage); err != nil {
					log.Printf("⚠️  Failed to edit message for complaint %s: %v", complaintID, err)
				}
			}

			if wa != nil {
				waResolvedMsg := fmt.Sprintf(
					"✅ RESOLVED\n\nComplaint #%s\n👤 %s\n🕐 %s",
					complaintID,
					consumerName,
					time.Now().Format("02 Jan 2006, 03:04 PM"),
				)
				if waErr := wa.SendMessage(waResolvedMsg); waErr != nil {
					log.Printf("⚠️  Failed to send WhatsApp resolved notice for %s: %v", complaintID, waErr)
				}
			}

			if rmErr := stor.Remove(complaintID); rmErr != nil {
				log.Printf("⚠️  Failed to remove complaint %s from storage: %v", complaintID, rmErr)
			} else {
				log.Printf("✅ Removed resolved complaint %s from storage", complaintID)
				resolvedCount++
			}
		}
	}

	if resolvedCount > 0 {
		log.Printf("🎉 Marked %d complaints as resolved", resolvedCount)
	}
}

// runScheduledSummaries blocks until ctx is cancelled, firing a Telegram +
// WhatsApp /summary at each configured HH:MM (IST) entry. The schedule is
// re-computed every iteration off time.Now() so a config-driven daemon can
// be paused for a long time and still pick the right next slot.
//
// schedules entries are HH:MM strings; pre-validated by config.parseScheduleList.
func runScheduledSummaries(
	ctx context.Context,
	schedules []string,
	tg *telegram.Client,
	wa *whatsapp.Client,
	sc *session.Client,
	stor *storage.Storage,
) {
	log.Printf("⏰ Scheduled summaries enabled: %v", schedules)
	for {
		nextAt, ok := nextScheduledFire(schedules, time.Now())
		if !ok {
			// No valid schedule entries — bail rather than hot-loop.
			log.Printf("⚠️  No valid scheduled summary times; scheduler exiting")
			return
		}

		wait := time.Until(nextAt)
		log.Printf("⏰ Next scheduled summary at %s (in %s)", nextAt.Format("15:04 MST"), wait.Round(time.Second))

		timer := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}

		log.Printf("📊 Scheduled /summary firing at %s", time.Now().Format("15:04:05"))
		if tg != nil {
			tg.PostScheduledSummary(ctx, sc, stor)
		}
		if wa != nil {
			wa.PostScheduledSummary(ctx, sc, stor)
		}
	}
}

// nextScheduledFire returns the soonest future time at which any HH:MM in
// schedules will fire, computed in time.Local (IST). Returns ok=false when
// schedules contains no valid entries — the caller treats that as fatal.
func nextScheduledFire(schedules []string, now time.Time) (time.Time, bool) {
	var best time.Time
	have := false
	for _, hhmm := range schedules {
		t, ok := parseHHMMToday(hhmm, now)
		if !ok {
			continue
		}
		if !t.After(now) {
			t = t.Add(24 * time.Hour) // already passed today; schedule for tomorrow
		}
		if !have || t.Before(best) {
			best = t
			have = true
		}
	}
	return best, have
}

// parseHHMMToday converts "09:00" into today's 09:00 in time.Local.
func parseHHMMToday(hhmm string, now time.Time) (time.Time, bool) {
	if len(hhmm) != 5 || hhmm[2] != ':' {
		return time.Time{}, false
	}
	hh, err1 := strconv.Atoi(hhmm[:2])
	mm, err2 := strconv.Atoi(hhmm[3:])
	if err1 != nil || err2 != nil || hh < 0 || hh > 23 || mm < 0 || mm > 59 {
		return time.Time{}, false
	}
	return time.Date(now.Year(), now.Month(), now.Day(), hh, mm, 0, 0, now.Location()), true
}
