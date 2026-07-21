package integration

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
)

const encryptionKeyEnvironment = "FORGEREVIEW_ENCRYPTION_KEY"

// EncryptedSecrets encrypts integration credentials with AES-256-GCM. The
// integration key is authenticated additional data, preventing ciphertext from
// being moved between integration records.
type EncryptedSecrets struct{ gcm cipher.AEAD }

// NewEncryptedSecrets parses a canonical base64-encoded, 32-byte AES-256 key.
func NewEncryptedSecrets(encodedKey string) (*EncryptedSecrets, error) {
	decoded, err := base64.StdEncoding.DecodeString(encodedKey)
	if err != nil || base64.StdEncoding.EncodeToString(decoded) != encodedKey || len(decoded) != 32 {
		return nil, errors.New("FORGEREVIEW_ENCRYPTION_KEY must be a base64-encoded 32-byte key")
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, fmt.Errorf("initialize encryption cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("initialize authenticated encryption: %w", err)
	}
	return &EncryptedSecrets{gcm: gcm}, nil
}

// NewEncryptedSecretsFromEnvironment is the only runtime configuration path
// for the master key.
func NewEncryptedSecretsFromEnvironment() (*EncryptedSecrets, error) {
	key, ok := os.LookupEnv(encryptionKeyEnvironment)
	if !ok {
		return nil, errors.New("FORGEREVIEW_ENCRYPTION_KEY is required")
	}
	return NewEncryptedSecrets(key)
}

func (s *EncryptedSecrets) Encrypt(integrationKey, value string) (string, error) {
	if value == "" {
		return "", errors.New("integration secret is required")
	}
	nonce := make([]byte, s.gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("generate encryption nonce: %w", err)
	}
	ciphertext := s.gcm.Seal(nil, nonce, []byte(value), []byte(integrationKey))
	return base64.StdEncoding.EncodeToString(append(nonce, ciphertext...)), nil
}

func (s *EncryptedSecrets) Resolve(item Integration) (string, error) {
	payload, err := base64.StdEncoding.DecodeString(item.SecretCiphertext)
	if err != nil || len(payload) < s.gcm.NonceSize()+s.gcm.Overhead() {
		return "", errors.New("stored integration secret cannot be decrypted")
	}
	nonce, ciphertext := payload[:s.gcm.NonceSize()], payload[s.gcm.NonceSize():]
	plaintext, err := s.gcm.Open(nil, nonce, ciphertext, []byte(item.Key))
	if err != nil {
		return "", errors.New("stored integration secret cannot be decrypted")
	}
	return string(plaintext), nil
}
