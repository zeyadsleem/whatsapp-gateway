package safehttp

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestValidateURLString(t *testing.T) {
	if err := ValidateURLString("https://example.com/hook"); err != nil {
		t.Fatalf("public url rejected: %v", err)
	}
	for _, raw := range []string{
		"http://127.0.0.1/hook",
		"http://169.254.169.254/latest/meta-data",
		"http://192.168.0.1/",
		"http://[fe80::1]/",
		"http://0.0.0.0/",
		"file:///etc/passwd",
	} {
		if err := ValidateURLString(raw); err == nil {
			t.Fatalf("expected %q to be blocked", raw)
		}
	}
}

func TestSafeDialContextBlocksPrivate(t *testing.T) {
	// httptest serves on 127.0.0.1; the safe transport must refuse it.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	client := NewClient(time.Second)
	resp, err := client.Get(srv.URL)
	if err == nil {
		resp.Body.Close()
		t.Fatal("expected private address to be blocked")
	}
}

func TestSafeDialContextAllowsPublic(t *testing.T) {
	// Simulate a public address by bypassing the IP check through a hostname
	// that resolves to a public IP is not possible offline; instead verify the
	// dialer logic directly.
	if isDisallowedIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("8.8.8.8 must be allowed")
	}
	if !isDisallowedIP(net.ParseIP("10.0.0.1")) {
		t.Fatal("10.0.0.1 must be blocked")
	}
	if !isDisallowedIP(net.ParseIP("100.64.0.1")) {
		t.Fatal("CGNAT range must be blocked")
	}
	_ = context.Background()
}
