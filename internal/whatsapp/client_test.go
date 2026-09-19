package whatsapp

import (
	"testing"
	"time"

	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

func TestReconnectDelayBounds(t *testing.T) {
	for attempt := 0; attempt < 12; attempt++ {
		d := reconnectDelay(attempt)
		if d < 3*time.Second {
			t.Fatalf("attempt %d delay too small: %v", attempt, d)
		}
		if d > 6*time.Minute {
			t.Fatalf("attempt %d delay too large: %v", attempt, d)
		}
	}
	first := reconnectDelay(0)
	if first < 4*time.Second || first > 6*time.Second {
		t.Fatalf("first delay out of expected range: %v", first)
	}
	late := reconnectDelay(10)
	if late < 4*time.Minute {
		t.Fatalf("late delay should approach the 5m cap (with jitter), got %v", late)
	}
}

func TestExtractContent(t *testing.T) {
	cases := []struct {
		name     string
		msg      *waE2E.Message
		wantType string
		wantText string
	}{
		{"nil", nil, "", ""},
		{"conversation", &waE2E.Message{Conversation: proto.String("1")}, "text", "1"},
		{"extended", &waE2E.Message{ExtendedTextMessage: &waE2E.ExtendedTextMessage{Text: proto.String("موعدك غدًا")}}, "text", "موعدك غدًا"},
		{"image", &waE2E.Message{ImageMessage: &waE2E.ImageMessage{Caption: proto.String("صورة")}}, "image", "صورة"},
		{"video", &waE2E.Message{VideoMessage: &waE2E.VideoMessage{}}, "video", ""},
		{"audio", &waE2E.Message{AudioMessage: &waE2E.AudioMessage{}}, "audio", ""},
		{"document", &waE2E.Message{DocumentMessage: &waE2E.DocumentMessage{FileName: proto.String("f.pdf")}}, "document", ""},
		{"sticker", &waE2E.Message{StickerMessage: &waE2E.StickerMessage{}}, "sticker", ""},
		{"location", &waE2E.Message{LocationMessage: &waE2E.LocationMessage{}}, "location", ""},
		{"protocol", &waE2E.Message{ProtocolMessage: &waE2E.ProtocolMessage{}}, "protocol", ""},
		{"empty", &waE2E.Message{}, "", ""},
	}
	for _, tc := range cases {
		gotType, gotText := extractContent(tc.msg)
		if gotType != tc.wantType || gotText != tc.wantText {
			t.Fatalf("%s: extractContent = (%q, %q), want (%q, %q)", tc.name, gotType, gotText, tc.wantType, tc.wantText)
		}
	}
}

func TestStatusConstants(t *testing.T) {
	statuses := []string{
		StatusDisconnected, StatusConnecting, StatusQRRequired, StatusConnected,
		StatusReconnecting, StatusLoggedOut, StatusError,
	}
	seen := make(map[string]bool)
	for _, s := range statuses {
		if s == "" || seen[s] {
			t.Fatalf("invalid or duplicate status %q", s)
		}
		seen[s] = true
	}
	if StatusConnected != "connected" || StatusQRRequired != "qr_required" {
		t.Fatal("status constants changed unexpectedly")
	}
}
