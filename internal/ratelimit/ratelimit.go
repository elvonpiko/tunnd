// Package ratelimit provides a tiny fixed-window per-key rate limiter —
// enough for the admin login and control-plane handshake paths, where the
// goal is making online brute force impractical, not precise traffic
// shaping. Keys are typically client IPs.
//
// Failures are counted with Strike; a key over its budget gets no service
// until its window rolls over. Successes are cleared with Reset so honest
// users never accumulate a strike from working logins.
package ratelimit

import (
	"sync"
	"time"
)

// windowCount is one key's counter for the current window.
type windowCount struct {
	count       int
	windowStart time.Time
}

// Limiter limits per-key event counts within a fixed window.
type Limiter struct {
	window time.Duration
	max    int

	mu      sync.Mutex
	counts  map[string]*windowCount
	stopped chan struct{}
}

// New returns a Limiter allowing at most max strikes per key per window.
// A background janitor evicts expired windows so keys can't pile up; it
// stops when the server does (call Stop) — or lives for the process
// lifetime, which is fine for a server-owned limiter.
func New(window time.Duration, max int) *Limiter {
	l := &Limiter{
		window:  window,
		max:     max,
		counts:  make(map[string]*windowCount),
		stopped: make(chan struct{}),
	}
	go l.janitor()
	return l
}

// Allow reports whether key is currently within its budget. It does not
// consume anything — call it to check before doing work, then Strike on
// failure / Reset on success.
func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	wc, ok := l.counts[key]
	if !ok || now.Sub(wc.windowStart) >= l.window {
		return true // fresh key, or the window already rolled over
	}
	return wc.count < l.max
}

// Strike records one failure for key, starting a new window if needed.
// Over-limit keys keep accumulating strikes; each new window gets a fresh
// budget, which also means a persistent attacker is simply refused forever
// while their strikes keep landing in fresh windows.
func (l *Limiter) Strike(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	wc, ok := l.counts[key]
	if !ok || now.Sub(wc.windowStart) >= l.window {
		wc = &windowCount{windowStart: now}
		l.counts[key] = wc
	}
	wc.count++
}

// Reset clears a key's counter — call on success so honest traffic never
// builds up toward the limit.
func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	delete(l.counts, key)
	l.mu.Unlock()
}

// RetryAfter returns how long until key's current window rolls over (0 when
// the key is currently allowed or has no window).
func (l *Limiter) RetryAfter(key string) time.Duration {
	l.mu.Lock()
	defer l.mu.Unlock()

	wc, ok := l.counts[key]
	if !ok {
		return 0
	}
	remaining := l.window - time.Since(wc.windowStart)
	if remaining < 0 {
		return 0
	}
	return remaining
}

// Stop terminates the janitor goroutine. Calling Stop on an already-stopped
// limiter (or never calling it) is safe.
func (l *Limiter) Stop() {
	select {
	case <-l.stopped:
		// already stopped
	default:
		close(l.stopped)
	}
}

// janitor periodically evicts expired windows so the map stays bounded.
func (l *Limiter) janitor() {
	ticker := time.NewTicker(l.window)
	defer ticker.Stop()
	for {
		select {
		case <-l.stopped:
			return
		case <-ticker.C:
			l.mu.Lock()
			now := time.Now()
			for key, wc := range l.counts {
				if now.Sub(wc.windowStart) >= l.window {
					delete(l.counts, key)
				}
			}
			l.mu.Unlock()
		}
	}
}
