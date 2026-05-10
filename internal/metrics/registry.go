package metrics

import (
	"net/http"
)

// Default is the process-wide registry used by the package-level helpers
// below. Other packages instrument by importing this package and calling
// metrics.FetchAttempts.Inc() etc. Tests that need isolation can build a
// fresh Registry with NewRegistry().
var Default = NewRegistry()

// Counters and gauges exposed at /metrics. New ones added here are picked
// up automatically by Handler() — no separate registration step required.
var (
	FetchAttemptsTotal = Default.NewCounter(
		"cmon_fetch_attempts_total",
		"Total number of complaint fetch cycles attempted.",
	)
	FetchFailuresTotal = Default.NewCounter(
		"cmon_fetch_failures_total",
		"Total number of complaint fetch cycles that ended in error.",
	)
	ComplaintsSeenTotal = Default.NewCounter(
		"cmon_complaints_seen_total",
		"Total number of new complaints observed (post-dedupe).",
	)
	ResolveCallsTotal = Default.NewCounter(
		"cmon_resolve_calls_total",
		"Total number of DGVCL resolve API calls made.",
	)
	ResolveFailuresTotal = Default.NewCounter(
		"cmon_resolve_failures_total",
		"Total number of DGVCL resolve API calls that failed.",
	)
	TelegramSendsTotal = Default.NewCounter(
		"cmon_telegram_sends_total",
		"Total number of successful Telegram outbound API calls (sendMessage / sendPhoto / editMessageText).",
	)
	TelegramSendFailuresTotal = Default.NewCounter(
		"cmon_telegram_send_failures_total",
		"Total number of failed Telegram outbound API calls.",
	)
	WhatsAppSendsTotal = Default.NewCounter(
		"cmon_whatsapp_sends_total",
		"Total number of successful WhatsApp outbound messages or images.",
	)
	WhatsAppSendFailuresTotal = Default.NewCounter(
		"cmon_whatsapp_send_failures_total",
		"Total number of failed WhatsApp outbound sends.",
	)

	LastFetchSuccessUnixSeconds = Default.NewGauge(
		"cmon_last_fetch_success_unix_seconds",
		"Unix timestamp of the most recent successful fetch cycle (0 if never).",
	)
)

// RegisterOpenComplaintsByBelt wires the `cmon_open_complaints` gauge family
// to a live storage query. Call once during startup after storage is ready.
func RegisterOpenComplaintsByBelt(fn func() map[string]int) {
	if fn == nil {
		return
	}
	Default.RegisterLabelledGauge(
		"cmon_open_complaints",
		"Number of currently-open (unresolved) complaints, partitioned by belt.",
		"belt",
		func() map[string]float64 {
			counts := fn()
			out := make(map[string]float64, len(counts))
			for k, v := range counts {
				out[k] = float64(v)
			}
			return out
		},
	)
}

// Handler returns an http.Handler that serves the default registry.
func Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_ = Default.Encode(w)
	})
}
