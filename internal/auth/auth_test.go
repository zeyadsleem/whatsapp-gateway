package auth

import (
	"strings"
	"testing"
)

func TestGenerateShopKey(t *testing.T) {
	raw, hash, prefix, err := GenerateShopKey()
	if err != nil {
		t.Fatalf("GenerateShopKey: %v", err)
	}
	if !strings.HasPrefix(raw, ShopKeyPrefix) {
		t.Fatalf("key missing prefix: %s", raw)
	}
	if len(raw) < 40 {
		t.Fatalf("key too short: %d", len(raw))
	}
	if hash != HashKey(raw) {
		t.Fatal("hash does not match")
	}
	if hash == raw {
		t.Fatal("hash must not equal raw key")
	}
	if prefix != raw[:8] {
		t.Fatalf("unexpected prefix %q", prefix)
	}

	raw2, hash2, _, err := GenerateShopKey()
	if err != nil {
		t.Fatalf("GenerateShopKey: %v", err)
	}
	if raw == raw2 || hash == hash2 {
		t.Fatal("keys must be unique")
	}
}

func TestVerifyKey(t *testing.T) {
	raw, hash, _, err := GenerateShopKey()
	if err != nil {
		t.Fatalf("GenerateShopKey: %v", err)
	}
	if !VerifyKey(raw, hash) {
		t.Fatal("expected valid key to verify")
	}
	if VerifyKey(raw+"x", hash) {
		t.Fatal("tampered key must not verify")
	}
	if VerifyKey("", hash) {
		t.Fatal("empty key must not verify")
	}
	if VerifyKey(raw, "") {
		t.Fatal("empty hash must not verify")
	}
	if VerifyKey(raw, HashKey("other")) {
		t.Fatal("wrong hash must not verify")
	}
}

func TestExtractBearer(t *testing.T) {
	cases := []struct {
		header string
		want   string
		ok     bool
	}{
		{"Bearer abc123", "abc123", true},
		{"bearer abc123", "abc123", true},
		{"BEARER abc123", "abc123", true},
		{"Bearer   abc123  ", "abc123", true},
		{"Basic abc123", "", false},
		{"Bearer ", "", false},
		{"", "", false},
		{"abc123", "", false},
	}
	for _, tc := range cases {
		got, ok := ExtractBearer(tc.header)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("ExtractBearer(%q) = (%q, %v), want (%q, %v)", tc.header, got, ok, tc.want, tc.ok)
		}
	}
}
