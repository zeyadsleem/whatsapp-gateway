package logger

import (
	"log/slog"
	"os"
	"strings"
	"testing"
)

func TestMaskPhoneNumbers(t *testing.T) {
	cases := map[string]string{
		"message from 201001234567":        "message from 2010******67",
		"short 12345":                      "short 12345",
		"no digits here":                   "no digits here",
		"two 201001234567 and 15551234567": "two 2010******67 and 1555*****67",
	}
	for input, want := range cases {
		if got := MaskPhoneNumbers(input); got != want {
			t.Fatalf("MaskPhoneNumbers(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestWhatsAppLoggerRedactsQR(t *testing.T) {
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelDebug}))
	adapter := WhatsAppLogger(log, "QRChannel")
	if got := adapter.(*waLogAdapter).sanitize("Emitting QR code 2@abc"); !strings.Contains(got, "redacted") {
		t.Fatalf("QR payload must be redacted, got %q", got)
	}

	clientLog := WhatsAppLogger(log, "Client")
	got := clientLog.(*waLogAdapter).sanitize("failed to send to 201001234567")
	if strings.Contains(got, "201001234567") {
		t.Fatalf("phone number must be masked, got %q", got)
	}
}
