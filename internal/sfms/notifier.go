package sfms

import (
	"context"
	"fmt"
	"html"
	"log"
	"sync"
	"time"

	"cmon/internal/storage"
)

// TelegramSender defines the interface needed to send Telegram alerts.
type TelegramSender interface {
	SendHTML(text string) error
	SendRawMessage(chatID, text, parseMode string) error
}

// WhatsAppSender defines the interface needed to send WhatsApp alerts.
type WhatsAppSender interface {
	SendMessage(text string) error
}

// Notifier handles multi-channel alert dispatching and SQLite event logging.
type Notifier struct {
	cfg   *Config
	stor  *storage.Storage
	tg    TelegramSender
	wa    WhatsAppSender
	tgMux sync.Mutex
}

// NewNotifier creates a new SFMS Notifier instance.
func NewNotifier(cfg *Config, stor *storage.Storage, tg TelegramSender, wa WhatsAppSender) *Notifier {
	return &Notifier{
		cfg:  cfg,
		stor: stor,
		tg:   tg,
		wa:   wa,
	}
}

// SendInterruption sends real-time feeder interruption alerts to BOTH Telegram and WhatsApp.
func (n *Notifier) SendInterruption(ctx context.Context, feederName, ssName string, fid int, fdrCode, bmuSerial, category string) {
	ts := FormatNowIST()
	displayName := CleanFeederName(feederName)
	displaySS := CleanSubstationName(ssName)

	catSuffix := ""
	if category != "" {
		catSuffix = fmt.Sprintf(" [%s]", category)
	}

	headerTitle := fmt.Sprintf("🔴 [ALERT] Feeder Interruption: %s%s", displayName, catSuffix)

	tgHTML := fmt.Sprintf(
		"<b>%s</b>\n\n"+
			"<b>Feeder:</b> %s%s \n"+
			"<b>Substation:</b> %s\n"+
			"<b>Time:</b> %s",
		headerTitle, html.EscapeString(displayName), html.EscapeString(catSuffix),
		html.EscapeString(displaySS), ts,
	)

	waText := fmt.Sprintf(
		"🔴 ALERT: Feeder Interruption\n\n"+
			"Feeder: %s%s \n"+
			"Substation: %s\n"+
			"Time: %s",
		displayName, catSuffix, displaySS, ts,
	)

	// Dispatch notifications asynchronously
	if n.tg != nil {
		go func() {
			if err := n.tg.SendHTML(tgHTML); err != nil {
				log.Printf("⚠️  [SFMS] Telegram interruption alert failed: %v", err)
			}
		}()
	}

	if n.wa != nil {
		go func() {
			if err := n.wa.SendMessage(waText); err != nil {
				log.Printf("⚠️  [SFMS] WhatsApp interruption alert failed: %v", err)
			}
		}()
	}

	// Persist to SQLite
	if n.stor != nil {
		_ = n.stor.LogSFMSEvent(storage.SFMSEvent{
			EventType:      "interruption",
			FID:            fid,
			FeederName:     displayName,
			SubstationName: displaySS,
			Category:       category,
			FdrCode:        fdrCode,
			BMUSerial:      bmuSerial,
			Message:        fmt.Sprintf("Feeder %s interrupted at %s", displayName, displaySS),
			Timestamp:      time.Now(),
		})
	}
}

// SendRecovery sends real-time restoration alerts to BOTH Telegram and WhatsApp.
func (n *Notifier) SendRecovery(ctx context.Context, feederName, ssName string, fid int, downtime, category string) {
	ts := FormatNowIST()
	displayName := CleanFeederName(feederName)
	displaySS := CleanSubstationName(ssName)

	catSuffix := ""
	if category != "" {
		catSuffix = fmt.Sprintf(" [%s]", category)
	}

	headerTitle := fmt.Sprintf("🟢 [RESTORED] Feeder Online: %s%s", displayName, catSuffix)

	tgHTML := fmt.Sprintf(
		"<b>%s</b>\n\n"+
			"<b>Feeder:</b> %s%s )\n"+
			"<b>Substation:</b> %s\n"+
			"<b>Downtime:</b> %s\n"+
			"<b>Time:</b> %s",
		headerTitle, html.EscapeString(displayName), html.EscapeString(catSuffix),
		html.EscapeString(displaySS), html.EscapeString(downtime), ts,
	)

	waText := fmt.Sprintf(
		"🟢 RESTORED: Feeder Online\n\n"+
			"Feeder: %s%s \n"+
			"Substation: %s\n"+
			"Downtime: %s\n"+
			"Time: %s",
		displayName, catSuffix, displaySS, downtime, ts,
	)

	// Dispatch notifications asynchronously
	if n.tg != nil {
		go func() {
			if err := n.tg.SendHTML(tgHTML); err != nil {
				log.Printf("⚠️  [SFMS] Telegram recovery alert failed: %v", err)
			}
		}()
	}

	if n.wa != nil {
		go func() {
			if err := n.wa.SendMessage(waText); err != nil {
				log.Printf("⚠️  [SFMS] WhatsApp recovery alert failed: %v", err)
			}
		}()
	}

	// Persist to SQLite
	if n.stor != nil {
		_ = n.stor.LogSFMSEvent(storage.SFMSEvent{
			EventType:      "recovery",
			FID:            fid,
			FeederName:     displayName,
			SubstationName: displaySS,
			Category:       category,
			Downtime:       downtime,
			Message:        fmt.Sprintf("Feeder %s restored at %s (Downtime: %s)", displayName, displaySS, downtime),
			Timestamp:      time.Now(),
		})
	}
}

// SendAuthError sends authentication failure notification to TELEGRAM ONLY.
func (n *Notifier) SendAuthError(ctx context.Context, errStr string) {
	ts := FormatNowIST()

	tgHTML := fmt.Sprintf(
		"⚠️ <b>[AUTH ERROR] SFMS Token Expired</b>\n\n"+
			"<b>Error:</b> %s\n"+
			"<b>Time:</b> %s\n\n"+
			"<i>Please paste the new Bearer Token in the dashboard at /sfms or update token.txt.</i>",
		html.EscapeString(errStr), ts,
	)

	if n.tg != nil {
		go func() {
			if err := n.tg.SendHTML(tgHTML); err != nil {
				log.Printf("⚠️  [SFMS] Telegram auth error notice failed: %v", err)
			}
		}()
	}

	if n.stor != nil {
		_ = n.stor.LogSFMSEvent(storage.SFMSEvent{
			EventType: "auth_error",
			Message:   fmt.Sprintf("Authentication failed: %s", errStr),
			Timestamp: time.Now(),
		})
	}
}

// SendAuthRestored sends token restoration notice to TELEGRAM ONLY.
func (n *Notifier) SendAuthRestored(ctx context.Context) {
	ts := FormatNowIST()

	tgHTML := fmt.Sprintf(
		"✅ <b>[AUTH RESTORED] SFMS Monitoring Active</b>\n\n"+
			"Bearer token updated and verified successfully. Real-time feeder monitoring is active.\n"+
			"<b>Time:</b> %s",
		ts,
	)

	if n.tg != nil {
		go func() {
			if err := n.tg.SendHTML(tgHTML); err != nil {
				log.Printf("⚠️  [SFMS] Telegram auth restored notice failed: %v", err)
			}
		}()
	}

	if n.stor != nil {
		_ = n.stor.LogSFMSEvent(storage.SFMSEvent{
			EventType: "auth_restored",
			Message:   "Authentication restored successfully. Feeder monitoring active.",
			Timestamp: time.Now(),
		})
	}
}
