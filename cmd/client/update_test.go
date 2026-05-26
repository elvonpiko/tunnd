package main

import (
	"testing"
	"time"
)

func TestNewerThan(t *testing.T) {
	cases := []struct {
		candidate, current string
		want               bool
	}{
		{"v0.1.1", "v0.1.0", true},
		{"v0.1.0", "v0.1.0", false},
		{"v0.1.0", "v0.1.1", false},
		{"v0.2.0", "v0.1.99", true},
		{"v1.0.0", "v0.99.99", true},
		// Identical without v prefix on either side.
		{"0.1.1", "0.1.0", true},
		{"0.1.0", "v0.1.0", false},
		// "dev" or unparseable: fall back to string compare.
		{"dev", "dev", false},
		{"v0.1.1-pre1", "v0.1.0", true}, // candidate parses, current parses
	}
	for _, c := range cases {
		got := newerThan(c.candidate, c.current)
		if got != c.want {
			t.Errorf("newerThan(%q, %q) = %v, want %v", c.candidate, c.current, got, c.want)
		}
	}
}

func TestSplitSemver(t *testing.T) {
	cases := []struct {
		in    string
		want  []int
		nilOK bool
	}{
		{"0.1.0", []int{0, 1, 0}, false},
		{"1.2.3", []int{1, 2, 3}, false},
		{"0.1.1-pre", []int{0, 1, 1}, false},
		{"0.1", nil, true},
		{"abc.def.ghi", nil, true},
		{"", nil, true},
	}
	for _, c := range cases {
		got := splitSemver(c.in)
		if c.nilOK {
			if got != nil {
				t.Errorf("splitSemver(%q) = %v, want nil", c.in, got)
			}
			continue
		}
		if len(got) != 3 || got[0] != c.want[0] || got[1] != c.want[1] || got[2] != c.want[2] {
			t.Errorf("splitSemver(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestPassiveUpdateHint_DoesNotBlock validates Property 11 + Property 15 —
// the passive update hint never blocks the tunnel UX. Confirmation test
// for an already-correct path (clause 1.13): maybePrintUpdateHint()
// short-circuits on `Version == "dev"`, on `TUNND_NO_UPDATE_CHECK=1`,
// and on a fresh-cache hit. When the cache is stale it dispatches the
// network call on a background goroutine with a 4s context.
//
// We exercise the user-facing escape hatch (TUNND_NO_UPDATE_CHECK=1)
// rather than mutating the package-level Version variable, because the
// fire-and-forget goroutine inside maybePrintUpdateHint races with the
// test's cleanup restore of Version under -race. Asserting "the
// disable-check env var actually disables the check and the call
// returns immediately" is a meaningful guard against future regressions
// (e.g. someone moving the env-var check after a synchronous network
// call).
func TestPassiveUpdateHint_DoesNotBlock(t *testing.T) {
	t.Setenv("TUNND_NO_UPDATE_CHECK", "1")

	// Point the on-disk cache at a temp HOME so we don't touch the real
	// ~/.config/tunnd/update-check.json even by accident. We set both
	// HOME and USERPROFILE so the test is portable across Linux, macOS,
	// and Windows (os.UserHomeDir checks USERPROFILE on Windows).
	tmpHome := t.TempDir()
	t.Setenv("HOME", tmpHome)
	t.Setenv("USERPROFILE", tmpHome)

	done := make(chan struct{})
	go func() {
		maybePrintUpdateHint()
		close(done)
	}()

	// Generous ceiling: with TUNND_NO_UPDATE_CHECK=1 the function
	// returns on the second line of its body. Even with a very slow
	// CI runner, well under a second is plenty.
	select {
	case <-done:
		// Returned promptly — that's the property under test.
	case <-time.After(2 * time.Second):
		t.Fatalf("maybePrintUpdateHint did not return within 2s — passive check is blocking")
	}
}
