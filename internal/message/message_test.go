package message

import (
	"errors"
	"strings"
	"testing"
)

func TestNormalizePhone(t *testing.T) {
	valid := map[string]string{
		"201001234567":      "201001234567",
		"+201001234567":     "201001234567",
		" +20 100 123 4567": "201001234567",
		"00201001234567":    "201001234567",
		"(201) 001-234-567": "201001234567",
		"12025550143":       "12025550143",
	}
	for input, want := range valid {
		got, err := NormalizePhone(input)
		if err != nil {
			t.Fatalf("NormalizePhone(%q): %v", input, err)
		}
		if got != want {
			t.Fatalf("NormalizePhone(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		"",
		"   ",
		"12345",
		"1234567890123456",
		"0123456789",
		"2010abc234567",
		"+2010/1234567",
	}
	for _, input := range invalid {
		if _, err := NormalizePhone(input); err == nil {
			t.Fatalf("NormalizePhone(%q) expected error", input)
		} else if !errors.Is(err, ErrInvalidPhone) {
			t.Fatalf("NormalizePhone(%q) unexpected error: %v", input, err)
		}
	}
}

func TestValidateText(t *testing.T) {
	if err := ValidateText("مرحبا"); err != nil {
		t.Fatalf("valid text rejected: %v", err)
	}
	if err := ValidateText("   "); !errors.Is(err, ErrEmptyText) {
		t.Fatalf("blank text must be rejected, got %v", err)
	}
	long := strings.Repeat("ا", MaxTextRunes+1)
	if err := ValidateText(long); !errors.Is(err, ErrTextTooLong) {
		t.Fatalf("long text must be rejected, got %v", err)
	}
}

func TestMaskPhone(t *testing.T) {
	if got := MaskPhone("201001234567"); got != "2010******67" {
		t.Fatalf("MaskPhone = %q", got)
	}
	if got := MaskPhone("123"); got != "123" {
		t.Fatalf("MaskPhone short = %q", got)
	}
}
