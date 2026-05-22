package main

import "testing"

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
