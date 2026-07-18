package security

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

// Encrypt protects value with AES-256-GCM using GITEA_TOKEN_ENCRYPTION_KEY.
// It returns a base64 ciphertext containing the random nonce.
func Encrypt(value string) (string, error) {
	key := []byte(os.Getenv("GITEA_TOKEN_ENCRYPTION_KEY"))
	if len(key) != 32 {
		return "", fmt.Errorf("GITEA_TOKEN_ENCRYPTION_KEY must have 32 bytes")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err = io.ReadFull(rand.Reader, nonce); err != nil {
		return "", err
	}
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(value), nil)), nil
}

// Decrypt decodes and opens an AES-256-GCM ciphertext produced by Encrypt. It
// returns the original plaintext or an error for invalid key/ciphertext data.
func Decrypt(value string) (string, error) {
	key := []byte(os.Getenv("GITEA_TOKEN_ENCRYPTION_KEY"))
	if len(key) != 32 {
		return "", fmt.Errorf("encryption key is invalid")
	}
	raw, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		return "", err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil || len(raw) < gcm.NonceSize() {
		return "", fmt.Errorf("invalid ciphertext")
	}
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], nil)
	return string(plain), err
}
