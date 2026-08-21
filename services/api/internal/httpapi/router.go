package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(database *sql.DB, logger *slog.Logger) http.Handler {
	api := &API{database: database}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	mux.Handle("GET /metrics", promhttp.Handler())
	mux.HandleFunc("GET /api/v1/health", readiness(database))
	mux.HandleFunc("POST /api/v1/auth/bootstrap", api.bootstrap)
	mux.HandleFunc("POST /api/v1/auth/login", api.login)
	mux.HandleFunc("POST /api/v1/auth/logout", api.logout)
	mux.HandleFunc("GET /api/v1/auth/me", api.requireSession(api.me))
	mux.HandleFunc("GET /api/v1/profile", api.requireSession(api.profile))
	mux.HandleFunc("PATCH /api/v1/profile", api.requireSession(api.profile))
	mux.HandleFunc("GET /api/v1/admin/dashboard", api.requireSession(api.adminDashboard))
	mux.HandleFunc("GET /api/v1/admin/users", api.requireSession(api.adminUsers))
	mux.HandleFunc("POST /api/v1/admin/users", api.requireSession(api.adminUsers))
	mux.HandleFunc("PATCH /api/v1/admin/users/{userID}", api.requireSession(api.updateAdminUser))
	mux.HandleFunc("GET /api/v1/admin/groups", api.requireSession(api.adminGroups))
	mux.HandleFunc("POST /api/v1/admin/groups", api.requireSession(api.adminGroups))
	mux.HandleFunc("PATCH /api/v1/admin/groups/{groupID}", api.requireSession(api.updateAdminGroup))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{groupID}", api.requireSession(api.updateAdminGroup))
	mux.HandleFunc("PUT /api/v1/admin/groups/{groupID}/members/{userID}", api.requireSession(api.updateAdminGroupMember))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{groupID}/members/{userID}", api.requireSession(api.updateAdminGroupMember))
	mux.HandleFunc("GET /api/v1/admin/meetings", api.requireSession(api.adminMeetings))
	mux.HandleFunc("GET /api/v1/admin/meetings/{meetingID}", api.requireSession(api.adminMeetingAnalytics))
	mux.HandleFunc("GET /api/v1/admin/audit-events", api.requireSession(api.adminAuditLog))
	mux.HandleFunc("GET /api/v1/admin/meeting-policy", api.requireSession(api.adminMeetingPolicy))
	mux.HandleFunc("PUT /api/v1/admin/meeting-policy", api.requireSession(api.adminMeetingPolicy))
	mux.HandleFunc("GET /api/v1/admin/system-configuration", api.requireSession(api.adminSystemConfiguration))
	mux.HandleFunc("PUT /api/v1/admin/system-configuration", api.requireSession(api.adminSystemConfiguration))
	mux.HandleFunc("GET /api/v1/meetings", api.requireSession(api.listMeetings))
	mux.HandleFunc("POST /api/v1/meetings", api.requireSession(api.createMeeting))
	mux.HandleFunc("GET /api/v1/meetings/history", api.requireSession(api.meetingHistory))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}", api.requireSession(api.getMeeting))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/join", api.requireSession(api.joinMeeting))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/token", api.requireSession(api.liveKitToken))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/participants", api.requireSession(api.listParticipants))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/participant", api.requireSession(api.currentParticipant))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/participants/{participantID}/{action}", api.requireSession(api.moderateParticipant))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/moderation/{action}", api.requireSession(api.moderateMeeting))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/leave", api.requireSession(api.leaveMeeting))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/recordings", api.requireSession(api.recordings))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/recordings", api.requireSession(api.recordings))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/recordings/stop", api.requireSession(api.stopRecording))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/recordings/{recordingID}/download", api.requireSession(api.recordingDownload))
	mux.HandleFunc("PUT /api/v1/meetings/{joinCode}/recordings/{recordingID}/access/{userID}", api.requireSession(api.recordingAccess))
	mux.HandleFunc("DELETE /api/v1/meetings/{joinCode}/recordings/{recordingID}/access/{userID}", api.requireSession(api.recordingAccess))

	secured := withSecurityHeaders(newRateLimiter().middleware(mux))
	return observeRequests(logger, recoverPanics(logger, secured))
}

type API struct{ database *sql.DB }

func readiness(database *sql.DB) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		context, cancel := context.WithTimeout(request.Context(), 2*time.Second)
		defer cancel()
		if err := database.PingContext(context); err != nil {
			respondJSON(writer, http.StatusServiceUnavailable, map[string]any{
				"service": "xpace-api", "status": "degraded",
				"checks": map[string]string{"postgres": "unavailable"},
				"time":   time.Now().UTC().Format(time.RFC3339),
			})
			return
		}
		respondJSON(writer, http.StatusOK, map[string]any{
			"service": "xpace-api", "status": "ready",
			"checks": map[string]string{"postgres": "ok"},
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	}
}

func health(writer http.ResponseWriter, request *http.Request) {
	respondJSON(writer, http.StatusOK, map[string]string{
		"service": "xpace-api",
		"status":  "ok",
		"time":    time.Now().UTC().Format(time.RFC3339),
	})
}

func respondJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
