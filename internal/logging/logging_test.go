package logging

import (
	"bytes"
	"encoding/json"
	"log"
	"log/slog"
	"strings"
	"testing"
)

// withCapture installs a slog handler that writes to buf so we can read what
// was emitted, restores the previous default + stdlib log state on cleanup.
func withCapture(t *testing.T, jsonMode bool) *bytes.Buffer {
	t.Helper()

	prevDefault := slog.Default()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	prevWriter := log.Writer()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
		log.SetOutput(prevWriter)
	})

	var buf bytes.Buffer
	var handler slog.Handler
	if jsonMode {
		handler = slog.NewJSONHandler(&buf, nil)
	} else {
		handler = slog.NewTextHandler(&buf, nil)
	}
	slog.SetDefault(slog.New(handler))
	log.SetFlags(0)
	log.SetPrefix("")
	log.SetOutput(slogWriter{})

	return &buf
}

func TestStdlibLogIsForwardedAsJSONInJSONMode(t *testing.T) {
	buf := withCapture(t, true)

	log.Printf("hello %s", "world")

	line := strings.TrimSpace(buf.String())
	if line == "" {
		t.Fatal("no log output captured")
	}

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(line), &got); err != nil {
		t.Fatalf("output is not valid JSON: %v\nline: %s", err, line)
	}
	if msg, _ := got["msg"].(string); msg != "hello world" {
		t.Errorf("msg: got %q, want %q", msg, "hello world")
	}
	if lvl, _ := got["level"].(string); lvl != "INFO" {
		t.Errorf("level: got %q, want INFO", lvl)
	}
}

func TestStdlibLogIsForwardedAsTextInTextMode(t *testing.T) {
	buf := withCapture(t, false)

	log.Println("hi there")

	out := buf.String()
	if !strings.Contains(out, `msg="hi there"`) {
		t.Errorf("expected text-mode logfmt msg field; got: %s", out)
	}
	if !strings.Contains(out, "level=INFO") {
		t.Errorf("expected level=INFO; got: %s", out)
	}
}

func TestSlogWriterStripsTrailingNewline(t *testing.T) {
	buf := withCapture(t, true)

	// log.Println always appends a newline. The trim must drop it so the
	// message field doesn't carry a stray \n that breaks parsing tools.
	log.Println("trailing")

	var got map[string]interface{}
	if err := json.Unmarshal([]byte(strings.TrimSpace(buf.String())), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if msg, _ := got["msg"].(string); msg != "trailing" {
		t.Errorf("msg: got %q, want %q (newline not stripped?)", msg, "trailing")
	}
}

func TestSetupUnknownFormatFallsBackToText(t *testing.T) {
	prevDefault := slog.Default()
	prevFlags := log.Flags()
	prevPrefix := log.Prefix()
	prevWriter := log.Writer()
	t.Cleanup(func() {
		slog.SetDefault(prevDefault)
		log.SetFlags(prevFlags)
		log.SetPrefix(prevPrefix)
		log.SetOutput(prevWriter)
	})

	// Setup() writes to os.Stderr; we just want to confirm it doesn't panic
	// for a garbage value and that the resulting handler is non-nil.
	Setup("not-a-real-format")
	if slog.Default() == nil {
		t.Fatal("Setup left slog.Default nil")
	}
}
