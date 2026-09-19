package ratelimit

import (
	"testing"
)

func TestLimiterAllow(t *testing.T) {
	l := New(1, 2)
	if !l.Allow("key-a") {
		t.Fatal("first request must pass")
	}
	if !l.Allow("key-a") {
		t.Fatal("second request must pass (burst=2)")
	}
	if l.Allow("key-a") {
		t.Fatal("third request must be limited")
	}
	if !l.Allow("key-b") {
		t.Fatal("other keys must not be affected")
	}
}

func TestLimiterIsolation(t *testing.T) {
	l := New(100, 1)
	for i := 0; i < 10; i++ {
		if !l.Allow("a") && i == 0 {
			t.Fatal("first request must pass")
		}
	}
	if l.Allow("a") {
		t.Fatal("key a must be limited after burst")
	}
	if !l.Allow("b") {
		t.Fatal("key b must start with a full bucket")
	}
}
