package platform

import (
	"strings"
	"testing"
)

func TestValidateRuntimeSecrets(t *testing.T) {
	t.Setenv("API_SESSION_SIGNING_KEY", strings.Repeat("s", 32))
	t.Setenv("LIVEKIT_API_KEY", "key")
	t.Setenv("LIVEKIT_API_SECRET", strings.Repeat("l", 32))
	t.Setenv("MINIO_ROOT_PASSWORD", strings.Repeat("m", 16))
	if err := ValidateRuntimeSecrets(); err != nil {
		t.Fatalf("valid secrets rejected: %v", err)
	}
	t.Setenv("API_SESSION_SIGNING_KEY", "short")
	if err := ValidateRuntimeSecrets(); err == nil {
		t.Fatal("short signing key must be rejected")
	}
}
