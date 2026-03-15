package telegram

import "testing"

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
