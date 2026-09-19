package message

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	MinPhoneDigits = 7
	MaxPhoneDigits = 15
	MaxTextRunes   = 20000
)

var (
	ErrInvalidPhone = errors.New("invalid phone number")
	ErrEmptyText    = errors.New("text must not be empty")
	ErrTextTooLong  = errors.New("text is too long")
)

// NormalizePhone accepts an E.164 number with optional +, spaces, dashes,
// parentheses and dots, and returns the bare international digits.
func NormalizePhone(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", ErrInvalidPhone
	}
	var b strings.Builder
	b.Grow(len(raw))
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '+' || r == ' ' || r == '-' || r == '(' || r == ')' || r == '.':
		default:
			return "", fmt.Errorf("%w: unexpected character %q", ErrInvalidPhone, r)
		}
	}
	digits := b.String()
	if strings.HasPrefix(digits, "00") {
		digits = strings.TrimPrefix(digits, "00")
	}
	if len(digits) < MinPhoneDigits || len(digits) > MaxPhoneDigits {
		return "", fmt.Errorf("%w: expected %d-%d digits, got %d", ErrInvalidPhone, MinPhoneDigits, MaxPhoneDigits, len(digits))
	}
	if digits[0] == '0' {
		return "", fmt.Errorf("%w: number must start with a country code, not 0", ErrInvalidPhone)
	}
	return digits, nil
}

func ValidateText(text string) error {
	if strings.TrimSpace(text) == "" {
		return ErrEmptyText
	}
	if utf8.RuneCountInString(text) > MaxTextRunes {
		return fmt.Errorf("%w: maximum %d characters", ErrTextTooLong, MaxTextRunes)
	}
	return nil
}

func MaskPhone(phone string) string {
	if len(phone) <= 6 {
		return phone
	}
	return phone[:4] + strings.Repeat("*", len(phone)-6) + phone[len(phone)-2:]
}
