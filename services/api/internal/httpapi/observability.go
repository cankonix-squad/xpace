package httpapi

import (
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "xpace", Subsystem: "api", Name: "http_requests_total",
		Help: "Total HTTP requests handled by the Xpace API.",
	}, []string{"method", "route", "status"})
	httpDuration = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Namespace: "xpace", Subsystem: "api", Name: "http_request_duration_seconds",
		Help:    "HTTP request duration in seconds.",
		Buckets: prometheus.DefBuckets,
	}, []string{"method", "route"})
)

func init() {
	prometheus.MustRegister(httpRequests, httpDuration)
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (recorder *statusRecorder) WriteHeader(status int) {
	recorder.status = status
	recorder.ResponseWriter.WriteHeader(status)
}

func observeRequests(logger *slog.Logger, next http.Handler) http.Handler {
	if logger == nil {
		logger = slog.Default()
	}
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		started := time.Now()
		recorder := &statusRecorder{ResponseWriter: writer, status: http.StatusOK}
		next.ServeHTTP(recorder, request)

		route := request.Pattern
		if route == "" {
			route = "unmatched"
		}
		duration := time.Since(started)
		httpRequests.WithLabelValues(request.Method, route, strconv.Itoa(recorder.status)).Inc()
		httpDuration.WithLabelValues(request.Method, route).Observe(duration.Seconds())
		logger.Info("http request",
			"method", request.Method,
			"route", route,
			"status", recorder.status,
			"duration_ms", duration.Milliseconds(),
			"remote_addr", request.RemoteAddr,
		)
	})
}
