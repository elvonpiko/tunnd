package ratelimit

import (
	"sync"
	"testing"
	"time"
)

func TestLimiter_BasicBudget(t *testing.T) {
	l := New(50*time.Millisecond, 3)
	defer l.Stop()

	for i := 0; i < 3; i++ {
		if !l.Allow("ip1") {
			t.Fatalf("strike %d: key should still be allowed", i)
		}
		l.Strike("ip1")
	}
	if l.Allow("ip1") {
		t.Fatal("after 3 strikes the key must be refused")
	}
	// Other keys are unaffected.
	if !l.Allow("ip2") {
		t.Fatal("unrelated key must be allowed")
	}
}

func TestLimiter_WindowRollsOver(t *testing.T) {
	l := New(40*time.Millisecond, 2)
	defer l.Stop()

	l.Strike("ip1")
	l.Strike("ip1")
	if l.Allow("ip1") {
		t.Fatal("key over budget")
	}
	time.Sleep(60 * time.Millisecond)
	if !l.Allow("ip1") {
		t.Fatal("window must roll over and allow again")
	}
}

func TestLimiter_ResetClearsStrikes(t *testing.T) {
	l := New(time.Minute, 2)
	defer l.Stop()

	l.Strike("ip1")
	l.Strike("ip1")
	if l.Allow("ip1") {
		t.Fatal("key over budget")
	}
	l.Reset("ip1")
	if !l.Allow("ip1") {
		t.Fatal("Reset must clear the budget")
	}
}

func TestLimiter_RetryAfter(t *testing.T) {
	l := New(time.Minute, 1)
	defer l.Stop()

	if ra := l.RetryAfter("ip1"); ra != 0 {
		t.Fatalf("unknown key RetryAfter = %v, want 0", ra)
	}
	l.Strike("ip1")
	l.Strike("ip1")
	ra := l.RetryAfter("ip1")
	if ra <= 0 || ra > time.Minute {
		t.Fatalf("RetryAfter = %v, want (0, 1m]", ra)
	}
}

func TestLimiter_JanitorEvictsExpiredWindows(t *testing.T) {
	l := New(30*time.Millisecond, 1)
	defer l.Stop()

	l.Strike("stale")
	l.mu.Lock()
	if len(l.counts) != 1 {
		l.mu.Unlock()
		t.Fatalf("counts = %d, want 1", len(l.counts))
	}
	l.mu.Unlock()

	// Wait past one janitor tick.
	time.Sleep(80 * time.Millisecond)
	l.mu.Lock()
	n := len(l.counts)
	l.mu.Unlock()
	if n != 0 {
		t.Fatalf("janitor did not evict the expired window, counts = %d", n)
	}
}

func TestLimiter_ConcurrentStrikes(t *testing.T) {
	l := New(time.Minute, 1000)
	defer l.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			l.Strike("shared")
			l.Allow("shared")
			l.RetryAfter("shared")
		}()
	}
	wg.Wait()

	l.mu.Lock()
	got := l.counts["shared"].count
	l.mu.Unlock()
	if got != 50 {
		t.Fatalf("concurrent strikes lost updates: count = %d, want 50", got)
	}
}
