// Package metrics exposes lightweight Prometheus-compatible counters and
// gauges without pulling in the upstream client_golang dependency.
//
// The exposition format follows the Prometheus text format spec:
//
//	# HELP <name> <help>
//	# TYPE <name> counter|gauge
//	<name>{<label>="<value>"} <number>
//
// Counters are monotonically increasing uint64s. Gauges are int64 settable
// from anywhere. Labelled gauges are populated at scrape time by a callback
// so they always reflect live state.
package metrics

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
)

// Counter is a monotonically increasing 64-bit counter.
type Counter struct {
	name, help string
	value      atomic.Uint64
}

// Inc adds one to the counter.
func (c *Counter) Inc() {
	if c == nil {
		return
	}
	c.value.Add(1)
}

// Add adds n to the counter.
func (c *Counter) Add(n uint64) {
	if c == nil {
		return
	}
	c.value.Add(n)
}

// Value returns the counter's current value. Intended for tests that want to
// assert "this counter advanced by 1" without parsing the exposition format.
func (c *Counter) Value() uint64 {
	if c == nil {
		return 0
	}
	return c.value.Load()
}

// Gauge is a 64-bit signed gauge whose value can move up or down.
type Gauge struct {
	name, help string
	value      atomic.Int64
}

// Set replaces the gauge's current value.
func (g *Gauge) Set(v int64) {
	if g == nil {
		return
	}
	g.value.Store(v)
}

// SetToCurrentTime is a convenience for time-based gauges.
func (g *Gauge) SetToCurrentTime(unixSeconds int64) {
	g.Set(unixSeconds)
}

// labelledGauge is a gauge family populated by a callback at scrape time.
// fn returns label-value → numeric-value; the label key is fixed at construction.
type labelledGauge struct {
	name, help, labelKey string
	fn                   func() map[string]float64
}

// Registry is a thread-safe collection of metrics.
type Registry struct {
	mu             sync.RWMutex
	counters       []*Counter
	gauges         []*Gauge
	labelledGauges []*labelledGauge
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{}
}

// NewCounter creates and registers a counter. Panics on duplicate name.
func (r *Registry) NewCounter(name, help string) *Counter {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, c := range r.counters {
		if c.name == name {
			panic("metrics: duplicate counter " + name)
		}
	}
	c := &Counter{name: name, help: help}
	r.counters = append(r.counters, c)
	return c
}

// NewGauge creates and registers a gauge. Panics on duplicate name.
func (r *Registry) NewGauge(name, help string) *Gauge {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, g := range r.gauges {
		if g.name == name {
			panic("metrics: duplicate gauge " + name)
		}
	}
	g := &Gauge{name: name, help: help}
	r.gauges = append(r.gauges, g)
	return g
}

// RegisterLabelledGauge registers a callback-based gauge family. The callback
// is invoked on every scrape and must return label-value → numeric-value.
// Use this for metrics derived from live storage.
func (r *Registry) RegisterLabelledGauge(name, help, labelKey string, fn func() map[string]float64) {
	if fn == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.labelledGauges = append(r.labelledGauges, &labelledGauge{
		name:     name,
		help:     help,
		labelKey: labelKey,
		fn:       fn,
	})
}

// Encode emits all registered metrics in Prometheus text-exposition format.
func (r *Registry) Encode(w io.Writer) error {
	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, c := range r.counters {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s counter\n%s %d\n",
			c.name, c.help, c.name, c.name, c.value.Load()); err != nil {
			return err
		}
	}
	for _, g := range r.gauges {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n%s %d\n",
			g.name, g.help, g.name, g.name, g.value.Load()); err != nil {
			return err
		}
	}
	for _, lg := range r.labelledGauges {
		if _, err := fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s gauge\n",
			lg.name, lg.help, lg.name); err != nil {
			return err
		}
		values := lg.fn()
		// Stable label order so scrape diffs stay readable.
		keys := make([]string, 0, len(values))
		for k := range values {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if _, err := fmt.Fprintf(w, "%s{%s=\"%s\"} %g\n",
				lg.name, lg.labelKey, escapeLabelValue(k), values[k]); err != nil {
				return err
			}
		}
	}
	return nil
}

// escapeLabelValue escapes the three characters the Prometheus text format
// requires escaping inside a label value: backslash, double quote, newline.
func escapeLabelValue(v string) string {
	r := strings.NewReplacer(
		`\`, `\\`,
		`"`, `\"`,
		"\n", `\n`,
	)
	return r.Replace(v)
}
