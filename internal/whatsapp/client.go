// Package whatsapp provides WhatsApp messaging integration for the CMON application.
//
// This package handles:
//   - Connecting to WhatsApp via the multi-device API (using whatsmeow)
//   - QR-code pairing on first run (stored session prevents re-pairing)
//   - Sending plain-text complaint notifications to a configured recipient
//
// Architecture:
//   - Client: Main struct wrapping whatsmeow.Client + recipient JID
//   - NewClient(): Reads env, opens SQLite device store, connects (prints QR if needed)
//   - SendMessage(): Sends a plain-text message to the recipient
//   - Disconnect(): Graceful shutdown
//
// Configuration (environment variables):
//   - WHATSAPP_RECIPIENT_JID: Target JID (e.g. 919876543210@s.whatsapp.net)
//   - WHATSAPP_DB_PATH: Path to SQLite session DB (default: ./whatsapp.db)
package whatsapp

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"

	_ "modernc.org/sqlite"

	"go.mau.fi/whatsmeow"
	waProto "go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"
)

// Client wraps a whatsmeow client and target recipient JID.
//
// Fields:
//   - wm:           The underlying whatsmeow client
//   - recipientJID: The parsed types.JID to send messages to
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
	// whatsmeow uses this to persist the cryptographic session keys
	// so that QR pairing is only needed once.
	dbLog := waLog.Noop // suppress internal whatsmeow DB debug logs
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

	// Create whatsmeow client
	// Use Noop logger to keep logs clean; change to waLog.Stdout for debugging
	clientLog := waLog.Noop
	wmClient := whatsmeow.NewClient(deviceStore, clientLog)

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
			if evt.Event == "code" {
				// Print QR code as ASCII to terminal
				printQR(evt.Code)
			} else if evt.Event == "success" {
				log.Println("✓ WhatsApp QR pairing successful!")
				break
			} else if evt.Event == "timeout" {
				log.Println("⚠️  WhatsApp QR pairing timed out. WhatsApp disabled for this run.")
				wmClient.Disconnect()
				return nil
			} else if evt.Event == "error" {
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
	}

	log.Printf("✓ WhatsApp configured successfully (recipient: %s)", recipientJIDStr)
	return &Client{
		wm:           wmClient,
		recipientJID: recipientJID,
	}
}

// SendMessage sends a plain-text message to the configured recipient JID.
//
// WhatsApp does not support HTML formatting, so this accepts plain text only.
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
		&waProto.Message{
			Conversation: proto.String(text),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to send WhatsApp message: %w", err)
	}

	return nil
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

// printQR renders the QR code string in the terminal using qrencode.
func printQR(code string) {
	fmt.Println()
	renderQRCodeASCII(code)
}

// renderQRCodeASCII tries to render the QR code using the `qrencode` binary.
// Uses ansiutf8 format with size=1 and margin=1 for a compact, scannable output.
// Falls back to printing the raw code string (copy + pipe to qrencode manually).
func renderQRCodeASCII(code string) {
	// Try qrencode with UTF-8 block characters — compact and scannable
	cmd := exec.Command("qrencode", "-t", "ansiutf8", "-s", "1", "-m", "1", "-o", "-", "--", code)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		// qrencode not found or failed — print raw code as fallback
		log.Printf("QR code (pipe to qrencode manually):\n%s", code)
		log.Println("Install qrencode: sudo apt install qrencode  OR  sudo dnf install qrencode")
	}
}
