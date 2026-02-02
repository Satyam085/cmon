package main

import (
	"context"
	"sync"
)

// BrowserContextHolder holds the browser context with thread-safe access
// This prevents race conditions when the browser context is restarted
type BrowserContextHolder struct {
	ctx    context.Context
	cancel context.CancelFunc
	mu     sync.RWMutex
}

// NewBrowserContextHolder creates a new context holder with an initial context
func NewBrowserContextHolder() *BrowserContextHolder {
	ctx, cancel := NewBrowserContext()
	return &BrowserContextHolder{
		ctx:    ctx,
		cancel: cancel,
	}
}

// Get returns the current browser context (read-only)
func (h *BrowserContextHolder) Get() context.Context {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.ctx
}

// Set updates the browser context (cancels old one if exists)
func (h *BrowserContextHolder) Set(ctx context.Context, cancel context.CancelFunc) {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	// Cancel old context if it exists
	if h.cancel != nil {
		h.cancel()
	}
	
	h.ctx = ctx
	h.cancel = cancel
}

// Cancel cancels the current browser context
func (h *BrowserContextHolder) Cancel() {
	h.mu.Lock()
	defer h.mu.Unlock()
	
	if h.cancel != nil {
		h.cancel()
	}
}
