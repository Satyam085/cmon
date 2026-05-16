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
