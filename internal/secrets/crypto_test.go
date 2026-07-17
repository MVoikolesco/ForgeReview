package secrets

import "testing"

func TestEncryptionBindsConnectionAADAndPreservesLegacyFormat(t *testing.T) {
	t.Setenv("GITEA_TOKEN_ENCRYPTION_KEY", "01234567890123456789012345678901")
	ciphertext, err := Encrypt("secret", ConnectionKeyAAD(7))
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Decrypt(ciphertext, ConnectionKeyAAD(7)); err != nil || got != "secret" {
		t.Fatalf("decrypt=%q err=%v", got, err)
	}
	if _, err := Decrypt(ciphertext, ConnectionKeyAAD(8)); err == nil {
		t.Fatal("ciphertext was transferable")
	}
	legacy, err := Encrypt("gitea-token", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := Decrypt(legacy, nil); err != nil || got != "gitea-token" {
		t.Fatalf("legacy decrypt=%q err=%v", got, err)
	}
}
