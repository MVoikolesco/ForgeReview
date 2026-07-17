// Package secrets provides the shared at-rest encryption format for credentials.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"os"
)

const encryptionKeyEnvironment = "GITEA_TOKEN_ENCRYPTION_KEY"

func key() ([]byte, error) {
	key := []byte(os.Getenv(encryptionKeyEnvironment))
	if len(key) != 32 {
		return nil, fmt.Errorf("%s must have 32 bytes", encryptionKeyEnvironment)
	}
	return key, nil
}

// Encrypt uses the legacy nonce-prefixed base64 AES-256-GCM format. Supplying
// nil AAD preserves readability of existing Gitea token ciphertext.
func Encrypt(plaintext string, aad []byte) (string, error) {
	key, err := key()
	if err != nil {
		return "", err
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
	return base64.StdEncoding.EncodeToString(gcm.Seal(nonce, nonce, []byte(plaintext), aad)), nil
}

func Decrypt(ciphertext string, aad []byte) (string, error) {
	key, err := key()
	if err != nil {
		return "", err
	}
	raw, err := base64.StdEncoding.DecodeString(ciphertext)
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
	plain, err := gcm.Open(nil, raw[:gcm.NonceSize()], raw[gcm.NonceSize():], aad)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func ConnectionKeyAAD(id int64) []byte { return []byte(fmt.Sprintf("ai_connections:%d:api_key", id)) }
