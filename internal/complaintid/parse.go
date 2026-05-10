// Package complaintid extracts the DGVCL complaint number from a notification
// message body. Both the Telegram and WhatsApp clients render outgoing
// complaint messages with a "📋 Complaint" line and parse the number back out
// when a user replies. Historically each side maintained its own parser with
// a slightly different format expectation, which meant a Telegram /move
// reply to a WhatsApp-formatted quote wouldn't recover the complaint number.
// This package is the single source of truth for the format.
package complaintid

import "strings"

// prefix is the literal that begins a complaint header line in messages
// emitted by both telegram.SendComplaintMessage and the WhatsApp text built
// in complaint/fetcher.go. Both flavors are accepted on parse:
//
//	"📋 Complaint : 12345"  — Telegram (spaces around the colon)
//	"📋 Complaint: 12345"   — WhatsApp (no leading space before the colon)
const prefix = "📋 Complaint"

// FromText scans every line of text and returns the first complaint number
// it finds, or "" if no header line is present. Lines that start with
// prefix (any amount of whitespace before the colon) are matched; the
// content after the colon is trimmed and returned.
//
// Scans every line — not just the first — so a quoted reply that includes
// arbitrary preamble before the original message still parses cleanly.
func FromText(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}

	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, prefix) {
			continue
		}

		rest := strings.TrimPrefix(line, prefix)
		// Allow any whitespace between "Complaint" and ":".
		rest = strings.TrimLeft(rest, " \t")
		if !strings.HasPrefix(rest, ":") {
			// Header line without a colon — not actually a complaint number.
			continue
		}
		rest = strings.TrimPrefix(rest, ":")
		number := strings.TrimSpace(rest)
		if number != "" {
			return number
		}
	}

	return ""
}
