// Package whatsapp provides WhatsApp messaging integration for the CMON application.
//
// This package handles:
//   - Connecting to WhatsApp via the multi-device API (using whatsmeow)
//   - QR-code pairing on first run (stored session prevents re-pairing)
//   - Sending plain-text complaint notifications to a configured recipient
//   - Listening for incoming messages (/summary command and resolve-by-reply)
//
// Architecture:
//   - Client: Main struct wrapping whatsmeow.Client + recipient JID + message tracker
//   - NewClient(): Reads env, opens SQLite device store, connects (prints QR if needed)
//   - SendComplaintMessage(): Sends + tracks message ID for resolve-by-reply
//   - SendMessage(): Sends a plain-text message (non-tracked)
//   - HandleEvents(): Background goroutine listening for incoming messages
//   - Disconnect(): Graceful shutdown
//
// Configuration (environment variables):
//   - WHATSAPP_RECIPIENT_JID:     Target JID (e.g. 919876543210@s.whatsapp.net or group@g.us)
//   - WHATSAPP_DB_PATH:           Path to SQLite session DB (default: ./whatsapp.db)
//   - WHATSAPP_RESOLVE_ENABLED:   Allow resolve-by-reply (default: false)
package whatsapp

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"strings"

	_ "modernc.org/sqlite"

	"cmon/internal/session"

	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waCommon"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Client wraps a whatsmeow client and target recipient JID.
//
// Fields:
//   - wm:            The underlying whatsmeow client
//   - recipientJID:  The parsed JID to send messages to
//   - wa_message_id string (via stor interface)
type Client struct {
	wm           *whatsmeow.Client
	recipientJID types.JID
}

// NewClient creates a new WhatsApp client from environment variables.
//
// Configuration:
//   - WHATSAPP_RECIPIENT_JID: Required. Target JID for sending messages.
//   - WHATSAPP_DB_PATH:       Optional. SQLite DB file path (default: whatsapp.db).
//
// On first run (no session stored), a QR code is printed to stdout.
// Scan it with WhatsApp mobile → Linked Devices → Link a Device.
// The session is then persisted to the SQLite DB for subsequent runs.
//
// Returns:
//   - *Client: Ready-to-use WhatsApp client, or nil if not configured / error
func NewClient() *Client {
	recipientJIDStr := os.Getenv("WHATSAPP_RECIPIENT_JID")
	if recipientJIDStr == "" {
		log.Println("⚠️  WHATSAPP_RECIPIENT_JID not set. WhatsApp notifications disabled.")
		return nil
	}

	dbPath := os.Getenv("WHATSAPP_DB_PATH")
	if dbPath == "" {
		dbPath = "whatsapp.db"
	}

	// Parse the recipient JID
	recipientJID, err := types.ParseJID(recipientJIDStr)
	if err != nil {
		log.Printf("⚠️  Invalid WHATSAPP_RECIPIENT_JID %q: %v. WhatsApp disabled.", recipientJIDStr, err)
		return nil
	}

	// Open (or create) the SQLite device store
	dbLog := waLog.Noop
	container, err := sqlstore.New(context.Background(), "sqlite", "file:"+dbPath+"?_pragma=foreign_keys(1)", dbLog)
	if err != nil {
		log.Printf("⚠️  Failed to open WhatsApp SQLite store (%s): %v. WhatsApp disabled.", dbPath, err)
		return nil
	}

	// Get or create device record
	deviceStore, err := container.GetFirstDevice(context.Background())
	if err != nil {
		log.Printf("⚠️  Failed to get WhatsApp device store: %v. WhatsApp disabled.", err)
		return nil
	}

	wmClient := whatsmeow.NewClient(deviceStore, waLog.Noop)

	c := &Client{
		wm:           wmClient,
		recipientJID: recipientJID,
	}

	// Connect to WhatsApp
	if wmClient.Store.ID == nil {
		// No session stored → need to pair via QR code
		log.Println("📱 No WhatsApp session found. Starting QR code pairing...")
		log.Println("   Scan the QR code below with WhatsApp → Linked Devices → Link a Device")

		qrChan, _ := wmClient.GetQRChannel(context.Background())
		if err := wmClient.Connect(); err != nil {
			log.Printf("⚠️  Failed to connect to WhatsApp for QR pairing: %v. WhatsApp disabled.", err)
			return nil
		}

		// Block until QR is scanned or pairing times out
		for evt := range qrChan {
			switch evt.Event {
			case "code":
				printQR(evt.Code)
			case "success":
				log.Println("✓ WhatsApp QR pairing successful!")
			case "timeout":
				log.Println("⚠️  WhatsApp QR pairing timed out. WhatsApp disabled for this run.")
				wmClient.Disconnect()
				return nil
			case "error":
				log.Printf("⚠️  WhatsApp QR pairing error: %v. WhatsApp disabled.", evt.Error)
				wmClient.Disconnect()
				return nil
			}
		}
	} else {
		// Session exists → reconnect without QR
		log.Println("📱 Reconnecting to existing WhatsApp session...")
		if err := wmClient.Connect(); err != nil {
			log.Printf("⚠️  Failed to reconnect to WhatsApp: %v. WhatsApp disabled.", err)
			return nil
		}
		log.Println("✓ WhatsApp reconnected successfully")

		// Print all joined groups so user can easily find the group JID
		c.ListGroups()
	}

	log.Printf("✓ WhatsApp configured successfully (recipient: %s)", recipientJIDStr)
	return c
}

// SendComplaintMessage sends a complaint notification and tracks the message ID
// so that a reply of "resolve <remark>" can be matched back to this complaint.
//
// Parameters:
//   - text:            Plain-text complaint message to send
//   - stor:            storage interface to save the wa_message_id
//
// Returns:
//   - error: Send error, or nil on success
func (c *Client) SendComplaintMessage(text, complaintNumber string, storI interface{}) error {
	if c == nil {
		return nil
	}

	resp, err := c.wm.SendMessage(
		context.Background(),
		c.recipientJID,
		&waProto.Message{Conversation: proto.String(text)},
	)
	if err != nil {
		return fmt.Errorf("failed to send WhatsApp complaint message: %w", err)
	}

	// Persist WA Message ID to SQLite for reply-to-resolve
	if stor, ok := storI.(storageSetter); ok {
		if err := stor.SetWAMessageID(complaintNumber, resp.ID); err != nil {
			log.Printf("⚠️  WhatsApp message sent, but failed to save tracking ID: %v", err)
		}
	} else {
		log.Printf("⚠️  WhatsApp storage type mismatch, cannot track message ID")
	}

	return nil
}

// SendMessage sends a plain-text message (non-tracked; for alerts, resolved notices, etc.)
//
// Parameters:
//   - text: Plain text message to send
//
// Returns:
//   - error: Send error, or nil on success
func (c *Client) SendMessage(text string) error {
	if c == nil {
		return nil
	}

	_, err := c.wm.SendMessage(
		context.Background(),
		c.recipientJID,
		&waProto.Message{Conversation: proto.String(text)},
	)
	if err != nil {
		return fmt.Errorf("failed to send WhatsApp message: %w", err)
	}

	return nil
}

// sendImage uploads and sends a PNG image to the recipient.
// Falls back to sending plain-text summary if upload fails.
func (c *Client) sendImage(imgBytes []byte, caption string) error {
	uploaded, err := c.wm.Upload(context.Background(), imgBytes, whatsmeow.MediaImage)
	if err != nil {
		return fmt.Errorf("failed to upload image: %w", err)
	}

	_, err = c.wm.SendMessage(context.Background(), c.recipientJID, &waProto.Message{
		ImageMessage: &waProto.ImageMessage{
			Caption:       proto.String(caption),
			Mimetype:      proto.String("image/png"),
			URL:           proto.String(uploaded.URL),
			DirectPath:    proto.String(uploaded.DirectPath),
			MediaKey:      uploaded.MediaKey,
			FileEncSHA256: uploaded.FileEncSHA256,
			FileSHA256:    uploaded.FileSHA256,
			FileLength:    proto.Uint64(uploaded.FileLength),
		},
	})
	return err
}

// HandleEvents starts the incoming message event loop in a background goroutine.
//
// This listens for:
//   - "/summary" command → fetches pending complaints and sends a summary image
//   - "resolve <remark>" reply to a tracked complaint message → resolves the complaint
//     (only active when resolveEnabled is true)
//
// Parameters:
//   - ctx:            Context for cancellation (stops the loop when cancelled)
//   - browserCtxHolder: Provides browser context for API calls and summary fetch
//   - stor:           Storage for complaint data
//   - resolveEnabled: Whether reply-to-resolve is active
func (c *Client) HandleEvents(ctx context.Context, sc *session.Client, stor interface{}, resolveEnabled bool, debugMode bool) {
	if c == nil {
		return
	}

	log.Println("✓ Starting WhatsApp event handler...")

	handlerID := c.wm.AddEventHandler(func(evt interface{}) {
		// Auto-reconnect on disconnection (network blip, server reset, etc.)
		if _, ok := evt.(*events.Disconnected); ok {
			log.Println("⚠️  WhatsApp disconnected — attempting reconnect...")
			if err := c.wm.Connect(); err != nil {
				log.Printf("⚠️  WhatsApp reconnect failed: %v (will retry on next event)", err)
			} else {
				log.Println("✓ WhatsApp reconnected successfully")
			}
			return
		}

		msg, ok := evt.(*events.Message)
		if !ok {
			return
		}

		// Debug: log every incoming message event so issues are visible in logs
		log.Printf("   [WA] msg from chat=%s isFromMe=%v text=%q", msg.Info.Chat, msg.Info.IsFromMe,
			func() string {
				t := msg.Message.GetConversation()
				if t == "" {
					t = msg.Message.GetExtendedTextMessage().GetText()
				}
				return t
			}(),
		)

		// Accept messages from the configured recipient chat.
		// NOTE: Modern WhatsApp uses LIDs (Linked IDs) as JIDs, which don't
		// match the phone-based JID in WHATSAPP_RECIPIENT_JID. We accept from
		// any chat and rely on command specificity (/summary, resolve) for safety.

		// Extract message text (plain or quoted reply)
		text := msg.Message.GetConversation()
		if text == "" {
			// ExtendedTextMessage covers quoted replies and longer text
			text = msg.Message.GetExtendedTextMessage().GetText()
		}
		text = strings.TrimSpace(text)
		if text == "" || msg.Info.IsFromMe {
			return
		}

		lower := strings.ToLower(text)

		// Handle /summary command
		if lower == "/summary" {
			log.Println("📊 WhatsApp /summary command received")
			go c.handleSummaryCommand(sc, stor)
			return
		}

		// Handle resolve-by-reply (only if enabled)
		if resolveEnabled && strings.HasPrefix(lower, "resolve ") {
			remark := strings.TrimSpace(text[len("resolve "):])
			if remark == "" {
				c.SendMessage("⚠️ Usage: reply to a complaint message with:\nresolve <your remark here>")
				return
			}

			// Get the ID of the message being replied to (quoted message stanza ID)
			quotedID := msg.Message.GetExtendedTextMessage().GetContextInfo().GetStanzaID()
			if quotedID == "" {
				c.SendMessage("⚠️ To resolve a complaint, *reply to the complaint message* with:\nresolve <your remark here>")
				return
			}

			storRslv, ok := stor.(resolveStorage)
			if !ok {
				c.SendMessage("❌ Internal error: storage type mismatch.")
				return
			}

			complaintNumber, tracked := storRslv.GetComplaintIDByWAMessageID(quotedID)
			if !tracked {
				c.SendMessage("⚠️ That message is not a tracked complaint, or it was already resolved.")
				return
			}

			log.Printf("📝 WhatsApp resolve request for complaint %s (remark: %s)", complaintNumber, remark)
			go c.handleResolve(sc, storRslv, complaintNumber, quotedID, remark, debugMode)
		}
	})

	// Wait for context cancellation, then remove handler
	<-ctx.Done()
	c.wm.RemoveEventHandler(handlerID)
	log.Println("🛑 WhatsApp event handler stopped")
}

// handleSummaryCommand fetches all pending complaints and sends a summary image.
func (c *Client) handleSummaryCommand(sc *session.Client, storI interface{}) {
	c.SendMessage("📊 Generating summary... please wait.")

	// Type-assert storage
	stor, ok := storI.(summaryStorage)
	if !ok {
		c.SendMessage("❌ Internal error: storage type mismatch.")
		return
	}

	// Fetch pending complaint details
	complaints, err := fetchPendingSummary(sc, stor)
	if err != nil {
		log.Printf("⚠️  WhatsApp summary fetch failed: %v", err)
		c.SendMessage("ℹ️ No pending complaints found.")
		return
	}

	// Render table as PNG
	imgBytes, err := renderSummaryImage(complaints)
	if err != nil {
		log.Printf("⚠️  WhatsApp summary render failed: %v", err)
		c.SendMessage(fmt.Sprintf("❌ Failed to render summary image: %v", err))
		return
	}

	caption := fmt.Sprintf("📋 %d Pending Complaints", len(complaints))
	if err := c.sendImage(imgBytes, caption); err != nil {
		log.Printf("⚠️  WhatsApp summary image send failed: %v", err)
		// Fallback to text summary
		c.SendMessage(buildTextSummary(complaints))
		return
	}

}

// handleResolve resolves a complaint via the DGVCL API and updates tracking.
func (c *Client) handleResolve(sc *session.Client, stor resolveStorage, complaintNumber, waMessageID, remark string, debugMode bool) {
	// Look up API ID
	apiID := stor.GetAPIID(complaintNumber)
	if apiID == "" {
		c.SendMessage(fmt.Sprintf("❌ Cannot resolve complaint %s: API ID not found.", complaintNumber))
		return
	}

	// Check still pending
	if !stor.Exists(complaintNumber) {
		c.SendMessage(fmt.Sprintf("ℹ️ Complaint %s was already resolved.", complaintNumber))
		return
	}

	// Call DGVCL API (respects DEBUG_MODE — will simulate without real call if true)
	if err := resolveComplaintAPI(sc, apiID, remark, debugMode); err != nil {
		log.Printf("⚠️  WhatsApp resolve API call failed for %s: %v", complaintNumber, err)
		c.SendMessage(fmt.Sprintf("❌ Failed to resolve complaint %s on website:\n%v\n\nPlease resolve manually.", complaintNumber, err))
		return
	}

	// Remove from storage so the automatic markResolvedComplaints loop doesn't
	// attempt to double-resolve or edit a stale Telegram message.
	if err := stor.Remove(complaintNumber); err != nil {
		log.Printf("⚠️  Resolved on website but failed to remove %s from storage: %v", complaintNumber, err)
	}

	c.SendMessage(fmt.Sprintf("✅ RESOLVED\n\nComplaint #%s\n💬 %s", complaintNumber, remark))
	log.Printf("✅ WhatsApp: resolved complaint %s", complaintNumber)
}

// ListGroups fetches all joined WhatsApp groups and prints their name + JID.
//
// Convenience utility for finding the correct group JID to set
// in WHATSAPP_RECIPIENT_JID.
func (c *Client) ListGroups() {
	if c == nil {
		return
	}

	groups, err := c.wm.GetJoinedGroups(context.Background())
	if err != nil {
		log.Printf("⚠️  Could not fetch WhatsApp groups: %v", err)
		return
	}

	if len(groups) == 0 {
		log.Println("📋 No WhatsApp groups found.")
		return
	}

	log.Printf("📋 WhatsApp groups (%d total) — set WHATSAPP_RECIPIENT_JID to a group JID below:", len(groups))
	for _, g := range groups {
		log.Printf("   %-50s  JID: %s", g.Name, g.JID)
	}
}

// Disconnect gracefully disconnects the WhatsApp client.
//
// Should be called via defer in main() for clean shutdown.
func (c *Client) Disconnect() {
	if c == nil {
		return
	}
	c.wm.Disconnect()
	log.Println("✓ WhatsApp disconnected")
}

// ── Interfaces to avoid circular imports ─────────────────────────────────────

// summaryStorage is the subset of storage.Storage needed for /summary.
type summaryStorage interface {
	GetAllSeenComplaints() []string
	GetAPIID(complaintNumber string) string
}

// resolveStorage is the subset of storage.Storage needed for resolve-by-reply.
type resolveStorage interface {
	GetAPIID(complaintNumber string) string
	GetComplaintIDByWAMessageID(waMessageID string) (string, bool)
	Exists(complaintNumber string) bool
	Remove(complaintNumber string) error
}

type storageSetter interface {
	SetWAMessageID(complaintID, waMessageID string) error
}

// ── Thin wrappers to call sibling packages without circular imports ───────────
// These are set via init-style function variables so tests can swap them out.

var (
	fetchPendingSummary = defaultFetchPendingSummary
	renderSummaryImage  = defaultRenderSummaryImage
	resolveComplaintAPI = defaultResolveComplaintAPI
)

func defaultFetchPendingSummary(sc *session.Client, stor summaryStorage) ([]summaryComplaint, error) {
	return fetchSummaryComplaints(sc, stor)
}

func defaultRenderSummaryImage(complaints []summaryComplaint) ([]byte, error) {
	return renderTable(complaints)
}

func defaultResolveComplaintAPI(sc *session.Client, apiID, remark string, debugMode bool) error {
	return resolveOnWebsite(sc, apiID, remark, debugMode)
}

// buildTextSummary produces a plain-text fallback when image sending fails.
func buildTextSummary(complaints []summaryComplaint) string {
	var b bytes.Buffer
	b.WriteString(fmt.Sprintf("📋 *%d Pending Complaints*\n\n", len(complaints)))
	for i, c := range complaints {
		belt := c.Belt
		if strings.TrimSpace(belt) == "" {
			belt = "Unknown"
		}
		b.WriteString(fmt.Sprintf("%d. #%s — %s\n   🏷️ %s\n   📍 %s\n", i+1, c.ComplainNo, c.Name, belt, c.Address))
	}
	return b.String()
}

// ── QR rendering ─────────────────────────────────────────────────────────────

// printQR renders the QR code string in the terminal using qrencode.
func printQR(code string) {
	fmt.Println()
	renderQRCodeASCII(code)
}

// renderQRCodeASCII tries to render the QR code using the `qrencode` binary.
// Uses ansiutf8 format with size=1 and margin=1 for a compact, scannable output.
func renderQRCodeASCII(code string) {
	cmd := exec.Command("qrencode", "-t", "ansiutf8", "-s", "1", "-m", "1", "-o", "-", "--", code)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		log.Printf("QR code (pipe to qrencode manually):\n%s", code)
		log.Println("Install qrencode: sudo apt install qrencode  OR  sudo dnf install qrencode")
	}
}

// ── waCommon import usage (prevents unused import error) ─────────────────────
var _ = waCommon.MessageKey{}
