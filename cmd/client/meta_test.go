package main

import "testing"

func TestParseRequestLine(t *testing.T) {
	method, path := parseRequestLine("POST /v1/hooks?x=1 HTTP/1.1\r\n")
	if method != "POST" || path != "/v1/hooks?x=1" {
		t.Fatalf("got %s %s", method, path)
	}
	method, path = parseRequestLine("nope")
	if method != "GET" || path != "/" {
		t.Fatalf("fallback got %s %s", method, path)
	}
}

func TestParseStatusCode(t *testing.T) {
	if got := parseStatusCode("HTTP/1.0 204 No Content\r\n"); got != 204 {
		t.Fatalf("got %d", got)
	}
	if got := parseStatusCode("short"); got != 0 {
		t.Fatalf("got %d", got)
	}
}
