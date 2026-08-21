package httpapi

import (
	"strings"
	"testing"
)

func TestSignedSessionToken(t *testing.T) {
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("a", 32))
	value := signedSessionToken("opaque-token")
	if token, valid := verifySignedSessionToken(value); !valid || token != "opaque-token" {
		t.Fatal("signed session token must verify")
	}
	if _, valid := verifySignedSessionToken(value + "tampered"); valid {
		t.Fatal("tampered session token must be rejected")
	}
}

func TestPreviousSessionSigningKeySupportsRotation(t *testing.T) {
	oldKey := strings.Repeat("o", 32)
	t.Setenv("API_SESSION_SIGNING_KEY", oldKey)
	value := signedSessionToken("rotating-token")
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("n", 32))
	t.Setenv("API_SESSION_SIGNING_KEY_PREVIOUS", oldKey)
	if _, valid := verifySignedSessionToken(value); !valid {
		t.Fatal("previous signing key must remain valid during rotation")
	}
	t.Setenv("API_SESSION_SIGNING_KEY_PREVIOUS", "")
}
