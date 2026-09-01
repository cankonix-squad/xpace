package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestTOTPMatchesRFC6238SHA1Vector(t *testing.T) {
	secret := "GEZDGNBVGY3TQOJQGEZDGNBVGY3TQOJQ"
	if got := totp(secret, time.Unix(59, 0)); got != "287082" {
		t.Fatalf("unexpected TOTP: got %s want 287082", got)
	}
}

func TestMFASecretEncryptionRoundTrip(t *testing.T) {
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("k", 32))
	encrypted, err := encryptMFASecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	if encrypted == "JBSWY3DPEHPK3PXP" {
		t.Fatal("MFA secret must not be stored as plaintext")
	}
	plain, err := decryptMFASecret(encrypted)
	if err != nil || plain != "JBSWY3DPEHPK3PXP" {
		t.Fatalf("round trip failed: %q, %v", plain, err)
	}
}

func TestVerifyMFAConsumesRecoveryCode(t *testing.T) {
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("k", 32))
	encrypted, err := encryptMFASecret("JBSWY3DPEHPK3PXP")
	if err != nil {
		t.Fatal(err)
	}
	code := "RECOVERY_123"
	hash := sha256.Sum256([]byte(code))
	encoded, _ := json.Marshal([]string{hex.EncodeToString(hash[:]), "another-hash"})
	valid, remaining := verifyMFA(encrypted, encoded, strings.ToLower(code))
	if !valid || len(remaining) != 1 || remaining[0] != "another-hash" {
		t.Fatalf("recovery code was not consumed: valid=%v remaining=%v", valid, remaining)
	}
}
