package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	ShopKeyPrefix = "wgs_"
	shopKeyBytes  = 32
	keyPrefixLen  = 8
)

// GenerateShopKey returns a new high-entropy API key. The raw value is shown to
// the operator exactly once; only the SHA-256 hash and a short display prefix
// are persisted.
func GenerateShopKey() (raw, hash, prefix string, err error) {
	buf := make([]byte, shopKeyBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", "", "", fmt.Errorf("generate api key: %w", err)
	}
	raw = ShopKeyPrefix + base64.RawURLEncoding.EncodeToString(buf)
	return raw, HashKey(raw), raw[:keyPrefixLen], nil
}

func HashKey(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

// VerifyKey compares a raw API key against a stored hash in constant time.
func VerifyKey(raw, expectedHash string) bool {
	if raw == "" || expectedHash == "" {
		return false
	}
	actual := HashKey(raw)
	return subtle.ConstantTimeCompare([]byte(actual), []byte(expectedHash)) == 1
}

func ExtractBearer(header string) (string, bool) {
	const scheme = "Bearer "
	if len(header) <= len(scheme) || !strings.EqualFold(header[:len(scheme)], scheme) {
		return "", false
	}
	key := strings.TrimSpace(header[len(scheme):])
	if key == "" {
		return "", false
	}
	return key, true
}
