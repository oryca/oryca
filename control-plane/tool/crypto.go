package tool

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
)

// EncryptAES encrypts plaintext with AES-GCM using the given key (16 or 32 bytes).
// Returns a base64url-encoded string containing nonce+ciphertext.
func EncryptAES(text, key string) (string, error) {
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	ciphertext := gcm.Seal(nonce, nonce, []byte(text), nil)
	return base64.RawURLEncoding.EncodeToString(ciphertext), nil
}

// DecryptAES decrypts a base64url-encoded AES-GCM ciphertext produced by EncryptAES.
func DecryptAES(ct, key string) (string, error) {
	raw, err := base64.RawURLEncoding.DecodeString(ct)
	if err != nil {
		return "", fmt.Errorf("base64 decode: %w", err)
	}
	block, err := aes.NewCipher([]byte(key))
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonceSize := gcm.NonceSize()
	if len(raw) < nonceSize {
		return "", fmt.Errorf("ciphertext too short")
	}
	plaintext, err := gcm.Open(nil, raw[:nonceSize], raw[nonceSize:], nil)
	if err != nil {
		return "", fmt.Errorf("decrypt: %w", err)
	}
	return string(plaintext), nil
}

// EncryptDSN encrypts a DSN string when key is set; returns plaintext as-is when key is empty (dev mode).
func EncryptDSN(dsn, key string) (string, error) {
	if key == "" || dsn == "" {
		return dsn, nil
	}
	return EncryptAES(dsn, key)
}

// DecryptDSN decrypts an encrypted DSN when key is set; returns the value as-is when key is empty (dev mode).
// Returns an error if decryption fails, which typically means the DSN was stored with a different key.
func DecryptDSN(dsn, key string) (string, error) {
	if key == "" || dsn == "" {
		return dsn, nil
	}
	return DecryptAES(dsn, key)
}
