package config

import (
	"bytes"
	"testing"
)

func TestEncryptDecryptConfigData(t *testing.T) {
	plain := []byte(`{"version":1,"servers":{"test":{"name":"test","host":"1.2.3.4"}}}`)
	password := "my-secret-master-pass-123!"

	encrypted, err := EncryptConfigData(plain, password)
	if err != nil {
		t.Fatalf("EncryptConfigData failed: %v", err)
	}

	decrypted, err := DecryptConfigData(encrypted, password)
	if err != nil {
		t.Fatalf("DecryptConfigData failed: %v", err)
	}

	if !bytes.Equal(decrypted, plain) {
		t.Errorf("decrypted %s does not match original %s", string(decrypted), string(plain))
	}

	// Test wrong password
	_, err = DecryptConfigData(encrypted, "wrong-password")
	if err == nil {
		t.Errorf("expected decryption error with wrong password, got nil")
	}
}
