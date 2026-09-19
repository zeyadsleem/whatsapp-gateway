package message

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"testing"
)

func makePNG(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatalf("encode png: %v", err)
	}
	return buf.Bytes()
}

func TestValidateImageData(t *testing.T) {
	png := makePNG(t)
	mime, err := ValidateImageData(png)
	if err != nil {
		t.Fatalf("valid png rejected: %v", err)
	}
	if mime != "image/png" {
		t.Fatalf("unexpected mime %q", mime)
	}
	if _, err := ValidateImageData([]byte("not an image")); !errors.Is(err, ErrUnsupportedMediaType) {
		t.Fatalf("expected unsupported media type, got %v", err)
	}
	if _, err := ValidateImageData(nil); err == nil {
		t.Fatal("empty image must be rejected")
	}
}

func TestNormalizeDocumentMime(t *testing.T) {
	ok := []string{"application/pdf", "text/csv", "application/vnd.ms-excel", "APPLICATION/PDF; charset=binary"}
	for _, mime := range ok {
		if _, err := NormalizeDocumentMime(mime); err != nil {
			t.Fatalf("NormalizeDocumentMime(%q): %v", mime, err)
		}
	}
	blocked := []string{
		"application/x-msdownload",
		"application/x-executable",
		"application/x-sh",
		"application/java-archive",
		"text/x-shellscript",
	}
	for _, mime := range blocked {
		if _, err := NormalizeDocumentMime(mime); !errors.Is(err, ErrUnsupportedMediaType) {
			t.Fatalf("NormalizeDocumentMime(%q) must be blocked, got %v", mime, err)
		}
	}
	if mime, err := NormalizeDocumentMime(""); err != nil || mime != "application/octet-stream" {
		t.Fatalf("generic mime should default to octet-stream, got %q err=%v", mime, err)
	}
}
