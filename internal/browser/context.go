// Package browser provides browser automation context management for the CMON application.
//
// This package handles Chrome/Chromium browser lifecycle management using ChromeDP.
// It provides thread-safe context creation, cancellation, and restart capabilities.
//
// Key features:
//   - Thread-safe context holder for sharing across goroutines
//   - Automatic cleanup on cancellation
//   - Browser restart capability for error recovery
package browser

import (
	"context"
	"log"
	"sync"

	"github.com/chromedp/chromedp"
)

// ContextHolder provides thread-safe access to a browser context.
//
// This struct allows multiple goroutines to safely share and update
// the browser context. It's particularly useful when the browser needs
// to be restarted due to errors or session issues.
//
// Thread-safety:
//   - All methods use mutex locking
//   - Safe for concurrent access from multiple goroutines
//   - Context updates are atomic
type ContextHolder struct {
	mu     sync.RWMutex      // Protects ctx and cancel
	ctx    context.Context   // Current browser context
	cancel context.CancelFunc // Function to cancel current context
}

// NewContextHolder creates a new browser context holder with an initialized context.
//
// Flow:
//   1. Create new Chrome browser context
//   2. Wrap in thread-safe holder
//   3. Return holder for shared use
//
// Returns:
//   - *ContextHolder: Thread-safe wrapper around browser context
func NewContextHolder() *ContextHolder {
	ctx, cancel := NewContext()
	return &ContextHolder{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Get returns the current browser context.
//
// This method is safe to call from multiple goroutines.
// Uses read lock for optimal performance when multiple readers exist.
//
// Returns:
//   - context.Context: Current browser context for ChromeDP operations
func (h *ContextHolder) Get() context.Context {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ctx
}

// Set updates the browser context with a new one.
//
// This is typically called when restarting the browser after errors.
// The old context is cancelled before setting the new one.
//
// Flow:
//   1. Acquire write lock
//   2. Cancel old context (cleanup resources)
//   3. Set new context and cancel function
//   4. Release lock
//
// Thread-safety:
//   - Uses write lock to ensure exclusive access during update
//   - Atomic swap of context and cancel function
//
// Parameters:
//   - ctx: New browser context
//   - cancel: Cancel function for the new context
func (h *ContextHolder) Set(ctx context.Context, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Cancel old context to free resources
	if h.cancel != nil {
		h.cancel()
	}

	// Set new context
	h.ctx = ctx
	h.cancel = cancel
}

// Cancel cancels the current browser context and cleans up resources.
//
// This should be called:
//   - On application shutdown
//   - Before restarting the browser
//   - When the context is no longer needed
//
// Thread-safety:
//   - Uses write lock to prevent concurrent cancellation
func (h *ContextHolder) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()

	if h.cancel != nil {
		h.cancel()
		h.cancel = nil
	}
}

// NewContext creates a new Chrome browser context.
//
// Browser configuration:
//   - Headless mode (no GUI)
//   - Logging enabled for debugging
//   - Standard Chrome flags
//
// Flow:
//   1. Create ChromeDP context with options
//   2. Return context and cancel function
//
// Returns:
//   - context.Context: Browser context for automation
//   - context.CancelFunc: Function to cancel and cleanup
func NewContext() (context.Context, context.CancelFunc) {
	log.Println("  → Creating new browser context...")

	// Create Chrome context with logging
	// WithLogf enables ChromeDP debug logging
	ctx, cancel := chromedp.NewContext(
		context.Background(),
		chromedp.WithLogf(log.Printf),
	)

	log.Println("  ✓ Browser context created successfully")
	return ctx, cancel
}

// RestartContext cancels the old context and creates a new one.
//
// This is used for error recovery when:
//   - Browser becomes unresponsive
//   - Session cannot be recovered
//   - Memory leaks are suspected
//
// Flow:
//   1. Cancel old context (cleanup)
//   2. Create new browser context
//   3. Return new context and cancel function
//
// Parameters:
//   - oldCancel: Cancel function for the old context (can be nil)
//
// Returns:
//   - context.Context: New browser context
//   - context.CancelFunc: Cancel function for new context
func RestartContext(oldCancel context.CancelFunc) (context.Context, context.CancelFunc) {
	log.Println("  ⚠️  Restarting browser context...")

	// Cancel old context to free resources
	if oldCancel != nil {
		oldCancel()
		log.Println("  ✓ Old browser context cancelled")
	}

	// Create new context
	return NewContext()
}
