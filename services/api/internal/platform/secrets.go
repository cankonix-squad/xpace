package platform

import (
	"fmt"
	"os"
)

func ValidateRuntimeSecrets() error {
	required := []struct {
		name      string
		minimum   int
		allowWeak bool
	}{
		{"API_SESSION_SIGNING_KEY", 32, false},
		{"LIVEKIT_API_KEY", 3, true},
		{"LIVEKIT_API_SECRET", 32, false},
		{"MINIO_ROOT_PASSWORD", 16, false},
	}
	for _, secret := range required {
		value := os.Getenv(secret.name)
		if len(value) < secret.minimum {
			return fmt.Errorf("%s must contain at least %d characters", secret.name, secret.minimum)
		}
		if !secret.allowWeak && (value == "replace-with-a-local-secret" || value == "replace-with-a-32-byte-minimum-local-secret" || value == "replace-with-a-32-byte-minimum-livekit-secret") {
			return fmt.Errorf("%s still uses a documented placeholder", secret.name)
		}
	}
	return nil
}
