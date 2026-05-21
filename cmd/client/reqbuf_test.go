package main

import (
	"io"
	"testing"
	"time"
)

func TestReqBuffer_WriteBeforeRead(t *testing.T) {
	b := newReqBuffer()
	b.Write([]byte("hello "))
	b.Write([]byte("world"))
	b.Close()
	out, err := io.ReadAll(b)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(out) != "hello world" {
		t.Fatalf("got %q", out)
	}
}

func TestReqBuffer_ReadBlocksUntilWrite(t *testing.T) {
	b := newReqBuffer()
	go func() {
		time.Sleep(20 * time.Millisecond)
		b.Write([]byte("late"))
		b.Close()
	}()
	out, _ := io.ReadAll(b)
	if string(out) != "late" {
		t.Fatalf("got %q", out)
	}
}

func TestReqBuffer_WriteToClosedReturnsError(t *testing.T) {
	b := newReqBuffer()
	b.Close()
	if _, err := b.Write([]byte("x")); err == nil {
		t.Fatal("expected error on write to closed buffer")
	}
}

func TestReqBuffer_ConcurrentWriters(t *testing.T) {
	b := newReqBuffer()
	go func() {
		for i := 0; i < 100; i++ {
			b.Write([]byte{byte(i)})
		}
		b.Close()
	}()
	out, _ := io.ReadAll(b)
	if len(out) != 100 {
		t.Fatalf("got len %d", len(out))
	}
}
