package telegram

import (
	"testing"
	"time"
)

func TestParseRateInterval(t *testing.T) {
	cases := []struct {
		in   string
		want time.Duration
	}{
		{"", 0},
		{"35", 35 * time.Millisecond},
		{"100", 100 * time.Millisecond},
		{"0", 0},   // non-positive → fall back
		{"-5", 0},  // negative → fall back
		{"abc", 0}, // garbage → fall back
		{"3.5", 0}, // float-shaped → strconv.Atoi rejects → fall back
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			if got := parseRateInterval(tc.in); got != tc.want {
				t.Errorf("parseRateInterval(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestEffectiveRateInterval(t *testing.T) {
	// Zero on the client → use defaultRateInterval.
	c := &Client{}
	if got := c.effectiveRateInterval(); got != defaultRateInterval {
		t.Errorf("zero override should yield default; got %v", got)
	}

	// Positive override wins.
	c2 := &Client{rateInterval: 100 * time.Millisecond}
	if got := c2.effectiveRateInterval(); got != 100*time.Millisecond {
		t.Errorf("override should win; got %v", got)
	}
}

func TestChatIDForBelt(t *testing.T) {
	c := &Client{
		ChatID: "default-chat",
		BeltRoutes: map[string]string{
			"dahod":    "-1001234",
			"bajipura": "-1005678",
		},
	}

	cases := []struct {
		name string
		belt string
		want string
	}{
		{"known belt routes to override", "dahod", "-1001234"},
		{"second known belt", "bajipura", "-1005678"},
		{"belt name with whitespace is trimmed", "  dahod  ", "-1001234"},
		{"belt name uppercase still matches", "DAHOD", "-1001234"},
		{"unknown belt falls back to default", "songadh", "default-chat"},
		{"empty belt falls back to default", "", "default-chat"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.ChatIDForBelt(tc.belt); got != tc.want {
				t.Errorf("ChatIDForBelt(%q) = %q, want %q", tc.belt, got, tc.want)
			}
		})
	}
}

func TestChatIDForBeltWithoutRoutesAlwaysReturnsDefault(t *testing.T) {
	c := &Client{ChatID: "default-chat"} // no BeltRoutes
	if got := c.ChatIDForBelt("dahod"); got != "default-chat" {
		t.Errorf("nil routes should always return ChatID; got %q", got)
	}
	if got := c.ChatIDForBelt(""); got != "default-chat" {
		t.Errorf("empty belt + nil routes should return ChatID; got %q", got)
	}
}

func TestChatIDForBeltOnNilClientReturnsEmpty(t *testing.T) {
	var c *Client
	if got := c.ChatIDForBelt("dahod"); got != "" {
		t.Errorf("nil client should return empty string; got %q", got)
	}
}

func TestIsMoveCommand(t *testing.T) {
	tests := []struct {
		name string
		text string
		want bool
	}{
		{name: "exact command", text: "/move", want: true},
		{name: "command with args", text: "/move 123 red", want: true},
		{name: "command in reply with spacing", text: "  /move yellow  ", want: true},
		{name: "prefixed lookalike", text: "/moved 123 red", want: false},
		{name: "telegram bot suffix", text: "/move@cmon_bot 123 red", want: false},
		{name: "plain text", text: "please move this", want: false},
		{name: "empty", text: "   ", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isMoveCommand(tt.text); got != tt.want {
				t.Fatalf("isMoveCommand(%q) = %v, want %v", tt.text, got, tt.want)
			}
		})
	}
}
