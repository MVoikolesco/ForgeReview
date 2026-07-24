package integration

import (
	"encoding/base64"
	"os"
	"strings"
	"testing"
)

const testEncryptionKey = "MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY="

func TestEncryptedSecretsRoundTrip(t *testing.T) {
	secrets, err := NewEncryptedSecrets(testEncryptionKey)
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := secrets.Encrypt("gitea-main", "token-value")
	if err != nil {
		t.Fatal(err)
	}
	if ciphertext == "token-value" || strings.Contains(ciphertext, "token-value") {
		t.Fatalf("ciphertext exposes plaintext: %q", ciphertext)
	}
	value, err := secrets.Resolve(Integration{Key: "gitea-main", SecretCiphertext: ciphertext})
	if err != nil || value != "token-value" {
		t.Fatalf("round trip = %q, %v", value, err)
	}
	if _, err = secrets.Resolve(Integration{Key: "other", SecretCiphertext: ciphertext}); err == nil {
		t.Fatal("ciphertext was accepted for a different integration")
	}
}

func TestEncryptedSecretsRejectsMissingAndInvalidEnvironmentKey(t *testing.T) {
	original, wasSet := os.LookupEnv(encryptionKeyEnvironment)
	if err := os.Unsetenv(encryptionKeyEnvironment); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if wasSet {
			_ = os.Setenv(encryptionKeyEnvironment, original)
			return
		}
		_ = os.Unsetenv(encryptionKeyEnvironment)
	})
	if _, err := NewEncryptedSecretsFromEnvironment(); err == nil {
		t.Fatal("missing key was accepted")
	}
	for _, key := range []string{"not-base64", base64.StdEncoding.EncodeToString(make([]byte, 31))} {
		if _, err := NewEncryptedSecrets(key); err == nil {
			t.Fatalf("invalid key %q was accepted", key)
		}
	}
}
