package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

type platformMetricsCollector struct {
	database            *sql.DB
	databaseConnections *prometheus.Desc
	databaseWaitTotal   *prometheus.Desc
	databaseWaitSeconds *prometheus.Desc
	activeMeetings      *prometheus.Desc
	waitingParticipants *prometheus.Desc
	joinedParticipants  *prometheus.Desc
	failedRecordings    *prometheus.Desc
	storageBytes        *prometheus.Desc
	chatMessages24Hours *prometheus.Desc
	clientErrors24Hours *prometheus.Desc
	scrapeErrors        *prometheus.Desc
}

func newPlatformMetricsCollector(database *sql.DB) prometheus.Collector {
	return &platformMetricsCollector{
		database:            database,
		databaseConnections: prometheus.NewDesc("xpace_database_connections", "Current PostgreSQL connection-pool usage.", []string{"state"}, nil),
		databaseWaitTotal:   prometheus.NewDesc("xpace_database_connection_wait_total", "Total number of waits for a PostgreSQL connection.", nil, nil),
		databaseWaitSeconds: prometheus.NewDesc("xpace_database_connection_wait_seconds_total", "Total time spent waiting for a PostgreSQL connection.", nil, nil),
		activeMeetings:      prometheus.NewDesc("xpace_platform_active_meetings", "Active meeting rooms across the platform.", nil, nil),
		waitingParticipants: prometheus.NewDesc("xpace_platform_waiting_participants", "Participants currently in meeting waiting rooms.", nil, nil),
		joinedParticipants:  prometheus.NewDesc("xpace_platform_joined_participants", "Participants currently joined to meetings.", nil, nil),
		failedRecordings:    prometheus.NewDesc("xpace_platform_failed_recordings", "Recordings currently in a failed state.", nil, nil),
		storageBytes:        prometheus.NewDesc("xpace_platform_storage_bytes", "Stored content size across the platform.", []string{"kind"}, nil),
		chatMessages24Hours: prometheus.NewDesc("xpace_platform_chat_messages_24h", "Chat messages created in the last 24 hours.", nil, nil),
		clientErrors24Hours: prometheus.NewDesc("xpace_platform_client_errors_24h", "Client error events reported in the last 24 hours.", nil, nil),
		scrapeErrors:        prometheus.NewDesc("xpace_platform_metrics_scrape_error", "Whether the latest platform database metrics scrape failed.", nil, nil),
	}
}

func (collector *platformMetricsCollector) Describe(channel chan<- *prometheus.Desc) {
	channel <- collector.databaseConnections
	channel <- collector.databaseWaitTotal
	channel <- collector.databaseWaitSeconds
	channel <- collector.activeMeetings
	channel <- collector.waitingParticipants
	channel <- collector.joinedParticipants
	channel <- collector.failedRecordings
	channel <- collector.storageBytes
	channel <- collector.chatMessages24Hours
	channel <- collector.clientErrors24Hours
	channel <- collector.scrapeErrors
}

func (collector *platformMetricsCollector) Collect(channel chan<- prometheus.Metric) {
	stats := collector.database.Stats()
	channel <- prometheus.MustNewConstMetric(collector.databaseConnections, prometheus.GaugeValue, float64(stats.OpenConnections), "open")
	channel <- prometheus.MustNewConstMetric(collector.databaseConnections, prometheus.GaugeValue, float64(stats.InUse), "in_use")
	channel <- prometheus.MustNewConstMetric(collector.databaseConnections, prometheus.GaugeValue, float64(stats.Idle), "idle")
	channel <- prometheus.MustNewConstMetric(collector.databaseConnections, prometheus.GaugeValue, float64(stats.MaxOpenConnections), "maximum")
	channel <- prometheus.MustNewConstMetric(collector.databaseWaitTotal, prometheus.CounterValue, float64(stats.WaitCount))
	channel <- prometheus.MustNewConstMetric(collector.databaseWaitSeconds, prometheus.CounterValue, stats.WaitDuration.Seconds())

	context, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	var activeMeetings, waitingParticipants, joinedParticipants, failedRecordings int
	var driveStorage, chatStorage, recordingStorage int64
	var chatMessages, clientErrors int
	err := collector.database.QueryRowContext(context, `SELECT
		(SELECT COUNT(*) FROM meetings WHERE status='ACTIVE'),
		(SELECT COUNT(*) FROM meeting_participants WHERE status='WAITING_ROOM'),
		(SELECT COUNT(*) FROM meeting_participants WHERE status='JOINED'),
		(SELECT COUNT(*) FROM recordings WHERE status='FAILED'),
		(SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes WHERE kind='FILE' AND deleted_at IS NULL),
		(SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments),
		(SELECT COALESCE(SUM(size_bytes),0) FROM recordings WHERE status='READY'),
		(SELECT COUNT(*) FROM chat_messages WHERE created_at>=NOW()-INTERVAL '24 hours'),
		(SELECT COUNT(*) FROM error_events WHERE created_at>=NOW()-INTERVAL '24 hours')`).Scan(
		&activeMeetings, &waitingParticipants, &joinedParticipants, &failedRecordings,
		&driveStorage, &chatStorage, &recordingStorage, &chatMessages, &clientErrors,
	)
	if err != nil {
		channel <- prometheus.MustNewConstMetric(collector.scrapeErrors, prometheus.GaugeValue, 1)
		return
	}
	channel <- prometheus.MustNewConstMetric(collector.activeMeetings, prometheus.GaugeValue, float64(activeMeetings))
	channel <- prometheus.MustNewConstMetric(collector.waitingParticipants, prometheus.GaugeValue, float64(waitingParticipants))
	channel <- prometheus.MustNewConstMetric(collector.joinedParticipants, prometheus.GaugeValue, float64(joinedParticipants))
	channel <- prometheus.MustNewConstMetric(collector.failedRecordings, prometheus.GaugeValue, float64(failedRecordings))
	channel <- prometheus.MustNewConstMetric(collector.storageBytes, prometheus.GaugeValue, float64(driveStorage), "drive")
	channel <- prometheus.MustNewConstMetric(collector.storageBytes, prometheus.GaugeValue, float64(chatStorage), "chat")
	channel <- prometheus.MustNewConstMetric(collector.storageBytes, prometheus.GaugeValue, float64(recordingStorage), "recordings")
	channel <- prometheus.MustNewConstMetric(collector.chatMessages24Hours, prometheus.GaugeValue, float64(chatMessages))
	channel <- prometheus.MustNewConstMetric(collector.clientErrors24Hours, prometheus.GaugeValue, float64(clientErrors))
	channel <- prometheus.MustNewConstMetric(collector.scrapeErrors, prometheus.GaugeValue, 0)
}

var (
	httpRequests = prometheus.NewCounterVec(prometheus.CounterOpts{
		Namespace: "xpace", Subsystem: "api", Name: "http_requests_total",
		Help: "Total HTTP requests handled by the Xspace API.",
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
