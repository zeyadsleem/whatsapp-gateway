package webhook

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/zeyad/whatsapp-gateway/internal/safehttp"
)

func TestSign(t *testing.T) {
	secret := []byte("whsec_test_secret")
	body := []byte(`{"event":"message.received","text":"1"}`)
	ts := "2026-09-18T12:00:00Z"

	got := Sign(secret, ts, body)
	if !strings.HasPrefix(got, "sha256=") {
		t.Fatalf("signature must be prefixed with sha256=, got %q", got)
	}

	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(ts + "." + string(body)))
	want := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	if got != want {
		t.Fatalf("Sign = %q, want %q", got, want)
	}

	if Sign(secret, ts, body) == Sign([]byte("other"), ts, body) {
		t.Fatal("different secrets must produce different signatures")
	}
	if Sign(secret, ts, body) == Sign(secret, "2026-09-18T12:00:01Z", body) {
		t.Fatal("different timestamps must produce different signatures")
	}
}

func TestValidateTarget(t *testing.T) {
	valid := []string{
		"https://example.com/webhooks",
		"http://example.com:8080/hook",
	}
	for _, raw := range valid {
		if err := safehttp.ValidateURLString(raw); err != nil {
			t.Fatalf("ValidateURLString(%q): %v", raw, err)
		}
	}
	invalid := []string{
		"",
		"ftp://example.com",
		"https://",
		"http://127.0.0.1/hook",
		"http://localhost:8080/hook",
		"http://10.1.2.3/hook",
		"http://192.168.1.10/hook",
		"http://[::1]/hook",
	}
	for _, raw := range invalid {
		if err := safehttp.ValidateURLString(raw); err == nil {
			t.Fatalf("ValidateURLString(%q) expected error", raw)
		}
	}
}
