package httpapi

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSecurityHeaders(t *testing.T) {
	handler := withSecurityHeaders(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
	request := httptest.NewRequest(http.MethodGet, "https://xpace.test/api/v1/health", nil)
	request.TLS = &tls.ConnectionState{}
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, request)
	for _, header := range []string{"Cache-Control", "Content-Security-Policy", "Cross-Origin-Resource-Policy", "Permissions-Policy", "Referrer-Policy", "Strict-Transport-Security", "X-Content-Type-Options", "X-Frame-Options"} {
		if writer.Header().Get(header) == "" {
			t.Fatalf("%s must be set", header)
		}
	}
}

func TestLoginRateLimit(t *testing.T) {
	now := time.Now()
	limiter := newRateLimiter()
	limiter.now = func() time.Time { return now }
	handler := limiter.middleware(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) { writer.WriteHeader(http.StatusNoContent) }))
	for attempt := 0; attempt < 11; attempt++ {
		request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
		request.RemoteAddr = "192.0.2.10:1234"
		writer := httptest.NewRecorder()
		handler.ServeHTTP(writer, request)
		if attempt < 10 && writer.Code != http.StatusNoContent {
			t.Fatalf("attempt %d unexpectedly returned %d", attempt+1, writer.Code)
		}
		if attempt == 10 && writer.Code != http.StatusTooManyRequests {
			t.Fatalf("rate-limited request returned %d", writer.Code)
		}
	}
	now = now.Add(time.Minute)
	request := httptest.NewRequest(http.MethodPost, "/api/v1/auth/login", nil)
	request.RemoteAddr = "192.0.2.10:1234"
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, request)
	if writer.Code != http.StatusNoContent {
		t.Fatalf("new window returned %d", writer.Code)
	}
}

func TestDecodeJSONLimitsAndSanitizes(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"value":"`+strings.Repeat("x", maxRequestBody)+`"}`))
	request.Header.Set("Content-Type", "application/json")
	writer := httptest.NewRecorder()
	var value map[string]any
	if err := decodeJSON(writer, request, &value); err == nil || !strings.Contains(err.Error(), "must not exceed") {
		t.Fatalf("expected bounded body error, got %v", err)
	}

	request = httptest.NewRequest(http.MethodPost, "/", strings.NewReader(`{"password":`))
	request.Header.Set("Content-Type", "application/json")
	writer = httptest.NewRecorder()
	if err := decodeJSON(writer, request, &value); err == nil || strings.Contains(err.Error(), "password") {
		t.Fatalf("parser detail must not be reflected: %v", err)
	}
}

func TestRecoverPanicsReturnsSafeError(t *testing.T) {
	handler := recoverPanics(nil, http.HandlerFunc(func(http.ResponseWriter, *http.Request) { panic("database password leaked") }))
	writer := httptest.NewRecorder()
	handler.ServeHTTP(writer, httptest.NewRequest(http.MethodGet, "/", nil))
	if writer.Code != http.StatusInternalServerError || strings.Contains(writer.Body.String(), "password") {
		t.Fatalf("unsafe panic response: %d %s", writer.Code, writer.Body.String())
	}
}

func TestRequestRateLimitClasses(t *testing.T) {
	tests := []struct {
		method string
		path   string
		limit  int
		bucket string
	}{
		{http.MethodPost, "/api/v1/auth/login", 10, "/api/v1/auth/login"},
		{http.MethodPost, "/api/v1/auth/bootstrap", 5, "/api/v1/auth/bootstrap"},
		{http.MethodPost, "/api/v1/meetings", 60, "write"},
		{http.MethodGet, "/api/v1/meetings", 300, "read"},
	}
	for _, test := range tests {
		request := httptest.NewRequest(test.method, test.path, nil)
		limit, duration := requestLimit(request)
		if limit != test.limit || duration != time.Minute || rateLimitBucket(request) != test.bucket {
			t.Fatalf("unexpected class for %s %s", test.method, test.path)
		}
	}
}
