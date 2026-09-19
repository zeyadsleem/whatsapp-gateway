package crypto

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"
)

type Box struct {
	aead cipher.AEAD
}

// New derives a 32-byte AES-256-GCM key from the configured secret.
func New(secret string) (*Box, error) {
	if secret == "" {
		return nil, errors.New("encryption secret is empty")
	}
	key := sha256.Sum256([]byte(secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create gcm: %w", err)
	}
	return &Box{aead: aead}, nil
}

func (b *Box) Encrypt(plaintext []byte) ([]byte, error) {
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return b.aead.Seal(nonce, nonce, plaintext, nil), nil
}

func (b *Box) Decrypt(data []byte) ([]byte, error) {
	ns := b.aead.NonceSize()
	if len(data) < ns {
		return nil, errors.New("ciphertext too short")
	}
	plaintext, err := b.aead.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}
