// Package logging configures the application-wide structured logger.
//
// The default text mode (LOG_FORMAT=text or unset) installs slog's text
// handler so terminal output stays human-readable. LOG_FORMAT=json installs
// slog.JSONHandler instead so log aggregators can parse every line.
//
// In both modes, the standard library's package-level `log` logger is
// rerouted through slog so packages still using `log.Printf` produce output
// in the chosen format. As packages migrate to slog directly they gain
// structured attributes; until then they still emit valid logfmt/json with
// the formatted message preserved as the slog message field.
package logging

import (
	"io"
	"log"
	"log/slog"
	"os"
	"strings"
)

// Setup installs the slog default handler implied by format and re-routes
// the stdlib log package to it. format is matched case-insensitively; an
// unrecognised value falls back to text mode.
func Setup(format string) {
	var handler slog.Handler
	switch strings.ToLower(strings.TrimSpace(format)) {
	case "json":
		handler = slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	default:
		handler = slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	slog.SetDefault(slog.New(handler))

	// Strip the stdlib log prefix/timestamp — the slog handler adds its own
	// — and forward each log line through slog at INFO level.
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(slogWriter{})
}

// slogWriter forwards each Write to slog.Default at INFO level. The stdlib
// log package writes one full line per Write call, so this is one log entry.
type slogWriter struct{}

func (slogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\r\n")
	if msg == "" {
		return len(p), nil
	}
	slog.Info(msg)
	return len(p), nil
}

// Discard is exported for tests that want to silence both slog and stdlib log
// output. Call as the very first thing in a test that exercises logging code.
var Discard io.Writer = io.Discard
