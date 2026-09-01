package httpapi

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"
)

const maxRequestBody = 64 << 10

func decodeJSON(writer http.ResponseWriter, request *http.Request, value any) error {
	if contentType := request.Header.Get("Content-Type"); contentType != "" {
		mediaType, _, err := mime.ParseMediaType(contentType)
		if err != nil || mediaType != "application/json" {
			return errors.New("Content-Type must be application/json")
		}
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBody)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("request body must not exceed %d bytes", maxRequestBody)
		}
		if errors.Is(err, io.EOF) {
			return errors.New("request body must contain one JSON object")
		}
		return errors.New("request body must be valid JSON and contain only supported fields")
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return errors.New("request body must contain one JSON object")
	}
	return nil
}

func errorJSON(writer http.ResponseWriter, status int, code, message string) {
	respondJSON(writer, status, map[string]any{"error": map[string]string{"code": code, "message": message}})
}

func randomToken(byteCount int) (string, error) {
	value := make([]byte, byteCount)
	if _, err := rand.Read(value); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(value), nil
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

func clientIP(request *http.Request) string {
	remote := normalizedIP(request.RemoteAddr)
	remoteIP := net.ParseIP(remote)
	if remoteIP == nil || !isTrustedProxyIP(remoteIP) {
		return remote
	}
	forwarded := strings.Split(request.Header.Get("X-Forwarded-For"), ",")
	for index := len(forwarded) - 1; index >= 0; index-- {
		candidate := net.ParseIP(strings.TrimSpace(forwarded[index]))
		if isPublicNetworkIP(candidate) {
			return candidate.String()
		}
	}
	if candidate := net.ParseIP(strings.TrimSpace(request.Header.Get("X-Real-IP"))); isPublicNetworkIP(candidate) {
		return candidate.String()
	}
	return remote
}

func normalizedIP(address string) string {
	host, _, err := net.SplitHostPort(address)
	if err == nil {
		return strings.Trim(host, "[]")
	}
	return strings.Trim(address, "[]")
}

func isTrustedProxyIP(ip net.IP) bool {
	return ip != nil && (ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast())
}

func isPublicNetworkIP(ip net.IP) bool {
	return ip != nil && ip.IsGlobalUnicast() && !ip.IsPrivate() && !ip.IsLoopback() && !ip.IsLinkLocalUnicast()
}
