package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type rateWindow struct {
	started time.Time
	count   int
}

type rateLimiter struct {
	mu      sync.Mutex
	windows map[string]rateWindow
	now     func() time.Time
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{windows: make(map[string]rateWindow), now: time.Now}
}

func (limiter *rateLimiter) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		limit, window := requestLimit(request)
		key := clientIP(request) + "|" + rateLimitBucket(request)
		allowed, retryAfter := limiter.allow(key, limit, window)
		writer.Header().Set("RateLimit-Limit", strconv.Itoa(limit))
		if !allowed {
			slog.Warn("request rate limited", "remote_addr", request.RemoteAddr, "bucket", rateLimitBucket(request), "method", request.Method)
			writer.Header().Set("Retry-After", strconv.Itoa(max(1, int(retryAfter.Seconds()))))
			errorJSON(writer, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests; retry later")
			return
		}
		next.ServeHTTP(writer, request)
	})
}

func (limiter *rateLimiter) allow(key string, limit int, duration time.Duration) (bool, time.Duration) {
	now := limiter.now()
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	window := limiter.windows[key]
	if window.started.IsZero() || now.Sub(window.started) >= duration {
		limiter.windows[key] = rateWindow{started: now, count: 1}
		return true, 0
	}
	if window.count >= limit {
		return false, duration - now.Sub(window.started)
	}
	window.count++
	limiter.windows[key] = window
	if len(limiter.windows) > 10000 {
		for candidate, value := range limiter.windows {
			if now.Sub(value.started) >= duration {
				delete(limiter.windows, candidate)
			}
		}
	}
	return true, 0
}

func requestLimit(request *http.Request) (int, time.Duration) {
	if request.URL.Path == "/api/v1/auth/login" || request.URL.Path == "/api/v1/auth/signup" || request.URL.Path == "/api/v1/auth/forgot-password" || request.URL.Path == "/api/v1/auth/reset-password" {
		return 10, time.Minute
	}
	if request.URL.Path == "/api/v1/auth/bootstrap" {
		return 5, time.Minute
	}
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		return 60, time.Minute
	}
	return 300, time.Minute
}

func rateLimitBucket(request *http.Request) string {
	if strings.HasPrefix(request.URL.Path, "/api/v1/auth/") {
		return request.URL.Path
	}
	if request.Method == http.MethodGet || request.Method == http.MethodHead {
		return "read"
	}
	return "write"
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Cache-Control", "no-store")
		writer.Header().Set("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		writer.Header().Set("Cross-Origin-Resource-Policy", "same-origin")
		writer.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=(), payment=(), usb=()")
		writer.Header().Set("Referrer-Policy", "no-referrer")
		writer.Header().Set("X-Content-Type-Options", "nosniff")
		writer.Header().Set("X-Frame-Options", "DENY")
		writer.Header().Set("X-Permitted-Cross-Domain-Policies", "none")
		if request.TLS != nil {
			writer.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		}
		next.ServeHTTP(writer, request)
	})
}

func recoverPanics(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logger.Error("panic recovered while handling request", "method", request.Method, "route", request.Pattern)
				errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "the request could not be completed")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}
