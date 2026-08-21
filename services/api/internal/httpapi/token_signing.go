package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"os"
	"strings"
)

func signedSessionToken(token string) string {
	return token + "." + sessionSignature(token, os.Getenv("API_SESSION_SIGNING_KEY"))
}

func verifySignedSessionToken(value string) (string, bool) {
	token, signature, found := strings.Cut(value, ".")
	if !found || token == "" || signature == "" {
		return "", false
	}
	keys := []string{os.Getenv("API_SESSION_SIGNING_KEY"), os.Getenv("API_SESSION_SIGNING_KEY_PREVIOUS")}
	for _, key := range keys {
		if key != "" && hmac.Equal([]byte(signature), []byte(sessionSignature(token, key))) {
			return token, true
		}
	}
	return "", false
}

func sessionSignature(token, key string) string {
	mac := hmac.New(sha256.New, []byte(key))
	_, _ = mac.Write([]byte(token))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
