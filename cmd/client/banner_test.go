package main

import (
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout runs fn while capturing everything written to os.Stdout and
// returns it as a string. printBanner writes to stdout via fmt.Println.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	os.Stdout = w
	// Suppress the update hint's network check so the banner is deterministic.
	t.Setenv("TUNND_NO_UPDATE_CHECK", "1")

	done := make(chan string, 1)
	go func() {
		var sb strings.Builder
		_, _ = io.Copy(&sb, r)
		done <- sb.String()
	}()

	fn()
	w.Close()
	os.Stdout = orig
	return <-done
}

func TestPrintBanner_WSModeShowsWSSAndHTTP(t *testing.T) {
	out := captureStdout(t, func() {
		printBanner("https://icy-creek.example.com", 3000, 4040, "http", true)
	})

	if !strings.Contains(out, "wss://icy-creek.example.com") {
		t.Errorf("ws banner should show wss:// URL; got:\n%s", out)
	}
	if !strings.Contains(out, "https://icy-creek.example.com (same port)") {
		t.Errorf("ws banner should note HTTP works on the same URL; got:\n%s", out)
	}
}

func TestPrintBanner_HTTPModeShowsHTTPSOnly(t *testing.T) {
	out := captureStdout(t, func() {
		printBanner("https://fancy-harbor.example.com", 3000, 4040, "http", false)
	})

	if !strings.Contains(out, "https://fancy-harbor.example.com") {
		t.Errorf("http banner should show https:// URL; got:\n%s", out)
	}
	if strings.Contains(out, "wss://") {
		t.Errorf("http banner must not show a wss:// URL; got:\n%s", out)
	}
	if strings.Contains(out, "(same port)") {
		t.Errorf("http banner must not show the ws-mode HTTP line; got:\n%s", out)
	}
}

func TestPrintBanner_TCPModeUnaffectedByWSMode(t *testing.T) {
	out := captureStdout(t, func() {
		printBanner("tcp://example.com:20000", 5432, 0, "tcp", false)
	})

	if !strings.Contains(out, "tcp://example.com:20000") {
		t.Errorf("tcp banner should show the tcp:// URL; got:\n%s", out)
	}
	if strings.Contains(out, "Inspector") {
		t.Errorf("tcp banner must not show an inspector line; got:\n%s", out)
	}
}
