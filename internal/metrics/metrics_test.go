package metrics

import (
	"bytes"
	"strings"
	"testing"
)

func TestEncodeCounter(t *testing.T) {
	r := NewRegistry()
	c := r.NewCounter("foo_total", "foo description")
	c.Inc()
	c.Add(4)

	var buf bytes.Buffer
	if err := r.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := "# HELP foo_total foo description\n# TYPE foo_total counter\nfoo_total 5\n"
	if got := buf.String(); got != want {
		t.Errorf("counter exposition mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestEncodeGauge(t *testing.T) {
	r := NewRegistry()
	g := r.NewGauge("bar_seconds", "bar description")
	g.Set(42)

	var buf bytes.Buffer
	if err := r.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	want := "# HELP bar_seconds bar description\n# TYPE bar_seconds gauge\nbar_seconds 42\n"
	if got := buf.String(); got != want {
		t.Errorf("gauge exposition mismatch:\nwant:\n%s\ngot:\n%s", want, got)
	}
}

func TestEncodeLabelledGaugeSorted(t *testing.T) {
	r := NewRegistry()
	r.RegisterLabelledGauge("baz", "baz description", "kind", func() map[string]float64 {
		return map[string]float64{
			"zulu":  3,
			"alpha": 1,
			"bravo": 2,
		}
	})

	var buf bytes.Buffer
	if err := r.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	out := buf.String()
	// Header lines first.
	if !strings.HasPrefix(out, "# HELP baz baz description\n# TYPE baz gauge\n") {
		t.Errorf("missing/incorrect HELP+TYPE preamble:\n%s", out)
	}

	// Label lines must be in sorted order so scrape diffs stay readable.
	wantOrder := []string{
		`baz{kind="alpha"} 1`,
		`baz{kind="bravo"} 2`,
		`baz{kind="zulu"} 3`,
	}
	idxs := make([]int, len(wantOrder))
	for i, line := range wantOrder {
		idxs[i] = strings.Index(out, line)
		if idxs[i] < 0 {
			t.Fatalf("missing line %q in:\n%s", line, out)
		}
	}
	for i := 1; i < len(idxs); i++ {
		if idxs[i] <= idxs[i-1] {
			t.Errorf("labels not in sorted order; got:\n%s", out)
		}
	}
}

func TestLabelValueEscaping(t *testing.T) {
	r := NewRegistry()
	r.RegisterLabelledGauge("escaped", "test", "key", func() map[string]float64 {
		return map[string]float64{
			`he said "hi"\there` + "\nnewline": 1,
		}
	})

	var buf bytes.Buffer
	if err := r.Encode(&buf); err != nil {
		t.Fatalf("Encode: %v", err)
	}

	// Backslash, double quote, and newline must all be escaped per the spec.
	want := `escaped{key="he said \"hi\"\\there\nnewline"} 1`
	if !strings.Contains(buf.String(), want) {
		t.Errorf("escaping wrong; want substring %q in:\n%s", want, buf.String())
	}
}

func TestDuplicateNamePanics(t *testing.T) {
	r := NewRegistry()
	r.NewCounter("x", "")
	defer func() {
		if recover() == nil {
			t.Error("expected panic on duplicate counter name")
		}
	}()
	r.NewCounter("x", "")
}
