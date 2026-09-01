package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func NewRouter(database *sql.DB, logger *slog.Logger) http.Handler {
	api := &API{database: database, chat: newChatHub()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", health)
	platformRegistry := prometheus.NewRegistry()
	platformRegistry.MustRegister(newPlatformMetricsCollector(database))
	metricsGatherer := prometheus.Gatherers{prometheus.DefaultGatherer, platformRegistry}
	mux.Handle("GET /metrics", promhttp.HandlerFor(metricsGatherer, promhttp.HandlerOpts{}))
	mux.HandleFunc("GET /api/v1/health", readiness(database))
	mux.HandleFunc("GET /api/v1/plans", api.publicPlans)
	mux.HandleFunc("POST /api/v1/billing/webhooks/{provider}", api.billingWebhook)
	mux.HandleFunc("POST /api/v1/billing/webhooks/xendit/native", api.xenditWebhook)
	mux.HandleFunc("POST /api/v1/integrations/alertmanager", api.alertmanagerWebhook)
	mux.HandleFunc("POST /api/v1/auth/bootstrap", api.bootstrap)
	mux.HandleFunc("POST /api/v1/auth/login", api.login)
	mux.HandleFunc("POST /api/v1/auth/signup", api.signup)
	mux.HandleFunc("POST /api/v1/auth/verify-email", api.verifyEmail)
	mux.HandleFunc("POST /api/v1/auth/forgot-password", api.forgotPassword)
	mux.HandleFunc("POST /api/v1/auth/reset-password", api.resetPassword)
	mux.HandleFunc("GET /api/v1/auth/sso/oidc/start", api.oidcStart)
	mux.HandleFunc("GET /api/v1/auth/sso/oidc/callback", api.oidcCallback)
	mux.HandleFunc("POST /api/v1/auth/invitations/accept", api.acceptInvitation)
	mux.HandleFunc("POST /api/v1/auth/logout", api.logout)
	mux.HandleFunc("GET /api/v1/auth/me", api.requireSession(api.me))
	mux.HandleFunc("GET /api/v1/profile", api.requireSession(api.profile))
	mux.HandleFunc("PATCH /api/v1/profile", api.requireSession(api.profile))
	mux.HandleFunc("GET /api/v1/profile/avatar", api.requireSession(api.profileAvatar))
	mux.HandleFunc("PUT /api/v1/profile/avatar", api.requireSession(api.profileAvatar))
	mux.HandleFunc("DELETE /api/v1/profile/avatar", api.requireSession(api.profileAvatar))
	mux.HandleFunc("POST /api/v1/errors/client", api.requireSession(api.clientError))
	mux.HandleFunc("GET /api/v1/directory/users", api.requireSession(api.directoryUsers))
	mux.HandleFunc("GET /api/v1/directory/users/{userID}/avatar", api.requireSession(api.directoryUserAvatar))
	mux.HandleFunc("GET /api/v1/search", api.requireSession(api.globalSearch))
	mux.HandleFunc("GET /api/v1/activity", api.requireSession(api.workspaceActivity))
	mux.HandleFunc("GET /api/v1/security/mfa", api.requireSession(api.mfaSettings))
	mux.HandleFunc("POST /api/v1/security/mfa", api.requireSession(api.mfaSettings))
	mux.HandleFunc("POST /api/v1/security/mfa/confirm", api.requireSession(api.mfaConfirm))
	mux.HandleFunc("DELETE /api/v1/security/mfa", api.requireSession(api.mfaDisable))
	mux.HandleFunc("GET /api/v1/security/sessions", api.requireSession(api.sessionDevices))
	mux.HandleFunc("DELETE /api/v1/security/sessions", api.requireSession(api.sessionDevices))
	mux.HandleFunc("DELETE /api/v1/security/sessions/{sessionID}", api.requireSession(api.revokeSessionDevice))
	mux.HandleFunc("GET /api/v1/calendar/events", api.requireSession(api.calendarEvents))
	mux.HandleFunc("POST /api/v1/calendar/events", api.requireSession(api.calendarEvents))
	mux.HandleFunc("PATCH /api/v1/calendar/events/{eventID}/response", api.requireSession(api.calendarResponse))
	mux.HandleFunc("GET /api/v1/rooms", api.requireSession(api.rooms))
	mux.HandleFunc("POST /api/v1/rooms", api.requireSession(api.rooms))
	mux.HandleFunc("GET /api/v1/rooms/{roomID}", api.requireSession(api.roomDetail))
	mux.HandleFunc("PUT /api/v1/rooms/{roomID}/members/{userID}", api.requireSession(api.roomMember))
	mux.HandleFunc("POST /api/v1/rooms/{roomID}/meetings", api.requireSession(api.roomMeeting))
	mux.HandleFunc("GET /api/v1/drive/nodes", api.requireSession(api.driveNodes))
	mux.HandleFunc("POST /api/v1/drive/folders", api.requireSession(api.driveNodes))
	mux.HandleFunc("POST /api/v1/drive/files", api.requireSession(api.driveUpload))
	mux.HandleFunc("GET /api/v1/drive/nodes/{nodeID}/download", api.requireSession(api.driveDownload))
	mux.HandleFunc("PATCH /api/v1/drive/nodes/{nodeID}", api.requireSession(api.driveNodeUpdate))
	mux.HandleFunc("DELETE /api/v1/drive/nodes/{nodeID}", api.requireSession(api.driveNodeUpdate))
	mux.HandleFunc("PUT /api/v1/drive/nodes/{nodeID}/shares/{userID}", api.requireSession(api.driveShare))
	mux.HandleFunc("GET /api/v1/drive/nodes/{nodeID}/versions", api.requireSession(api.driveVersions))
	mux.HandleFunc("POST /api/v1/drive/nodes/{nodeID}/versions", api.requireSession(api.driveVersions))
	mux.HandleFunc("POST /api/v1/drive/nodes/{nodeID}/versions/{version}/restore", api.requireSession(api.driveVersionRestore))
	mux.HandleFunc("GET /api/v1/notifications", api.requireSession(api.notifications))
	mux.HandleFunc("POST /api/v1/notifications/{notificationID}/read", api.requireSession(api.notificationRead))
	mux.HandleFunc("POST /api/v1/notifications/read-all", api.requireSession(api.notificationRead))
	mux.HandleFunc("GET /api/v1/chat/conversations", api.requireSession(api.chatConversations))
	mux.HandleFunc("POST /api/v1/chat/conversations", api.requireSession(api.chatConversations))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/clear", api.requireSession(api.chatConversationClear))
	mux.HandleFunc("DELETE /api/v1/chat/conversations/{conversationID}", api.requireSession(api.chatConversationDelete))
	mux.HandleFunc("GET /api/v1/chat/conversations/{conversationID}/messages", api.requireSession(api.chatMessages))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/messages", api.requireSession(api.chatMessages))
	mux.HandleFunc("PATCH /api/v1/chat/conversations/{conversationID}/messages/{messageID}", api.requireSession(api.chatMessages))
	mux.HandleFunc("DELETE /api/v1/chat/conversations/{conversationID}/messages/{messageID}", api.requireSession(api.chatMessages))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/messages/{messageID}/reactions", api.requireSession(api.chatReaction))
	mux.HandleFunc("GET /api/v1/chat/conversations/{conversationID}/messages/{messageID}/attachments", api.requireSession(api.chatAttachment))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/messages/{messageID}/attachments", api.requireSession(api.chatAttachment))
	mux.HandleFunc("GET /api/v1/chat/conversations/{conversationID}/messages/{messageID}/attachments/{attachmentID}/download", api.requireSession(api.chatAttachmentDownload))
	mux.HandleFunc("DELETE /api/v1/chat/conversations/{conversationID}/messages/{messageID}/reactions", api.requireSession(api.chatReaction))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/messages/{messageID}/pin", api.requireSession(api.chatPin))
	mux.HandleFunc("DELETE /api/v1/chat/conversations/{conversationID}/messages/{messageID}/pin", api.requireSession(api.chatPin))
	mux.HandleFunc("GET /api/v1/chat/conversations/{conversationID}/events", api.requireSession(api.chatEvents))
	mux.HandleFunc("GET /api/v1/chat/search", api.requireSession(api.chatSearch))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/read", api.requireSession(api.chatRead))
	mux.HandleFunc("POST /api/v1/chat/conversations/{conversationID}/presence", api.requireSession(api.chatPresence))
	mux.HandleFunc("GET /api/v1/admin/dashboard", api.requireSession(api.adminDashboard))
	mux.HandleFunc("GET /api/v1/admin/subscription", api.requireSession(api.adminSubscription))
	mux.HandleFunc("GET /api/v1/admin/billing/invoices", api.requireSession(api.adminBillingInvoices))
	mux.HandleFunc("POST /api/v1/admin/billing/checkout", api.requireSession(api.adminBillingCheckout))
	mux.HandleFunc("POST /api/v1/admin/billing/subscription/cancel", api.requireSession(api.adminBillingCancellation))
	mux.HandleFunc("POST /api/v1/admin/billing/subscription/resume", api.requireSession(api.adminBillingCancellation))
	mux.HandleFunc("GET /api/v1/admin/users", api.requireSession(api.adminUsers))
	mux.HandleFunc("POST /api/v1/admin/users", api.requireSession(api.adminUsers))
	mux.HandleFunc("PATCH /api/v1/admin/users/{userID}", api.requireSession(api.updateAdminUser))
	mux.HandleFunc("DELETE /api/v1/admin/users/{userID}", api.requireSession(api.deleteAdminUser))
	mux.HandleFunc("GET /api/v1/admin/groups", api.requireSession(api.adminGroups))
	mux.HandleFunc("POST /api/v1/admin/groups", api.requireSession(api.adminGroups))
	mux.HandleFunc("PATCH /api/v1/admin/groups/{groupID}", api.requireSession(api.updateAdminGroup))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{groupID}", api.requireSession(api.updateAdminGroup))
	mux.HandleFunc("PUT /api/v1/admin/groups/{groupID}/members/{userID}", api.requireSession(api.updateAdminGroupMember))
	mux.HandleFunc("DELETE /api/v1/admin/groups/{groupID}/members/{userID}", api.requireSession(api.updateAdminGroupMember))
	mux.HandleFunc("GET /api/v1/admin/meetings", api.requireSession(api.adminMeetings))
	mux.HandleFunc("GET /api/v1/admin/meetings/{meetingID}", api.requireSession(api.adminMeetingAnalytics))
	mux.HandleFunc("GET /api/v1/admin/audit-events", api.requireSession(api.adminAuditLog))
	mux.HandleFunc("GET /api/v1/admin/incidents", api.requireSession(api.adminIncidents))
	mux.HandleFunc("POST /api/v1/admin/incidents", api.requireSession(api.adminIncidents))
	mux.HandleFunc("GET /api/v1/admin/incidents/{incidentID}", api.requireSession(api.adminIncidentDetail))
	mux.HandleFunc("PATCH /api/v1/admin/incidents/{incidentID}", api.requireSession(api.updateAdminIncident))
	mux.HandleFunc("POST /api/v1/admin/incidents/{incidentID}/timeline", api.requireSession(api.addIncidentNote))
	mux.HandleFunc("POST /api/v1/admin/incidents/{incidentID}/{action}", api.requireSession(api.transitionAdminIncident))
	mux.HandleFunc("GET /api/v1/admin/meeting-policy", api.requireSession(api.adminMeetingPolicy))
	mux.HandleFunc("PUT /api/v1/admin/meeting-policy", api.requireSession(api.adminMeetingPolicy))
	mux.HandleFunc("GET /api/v1/admin/system-configuration", api.requireSession(api.adminSystemConfiguration))
	mux.HandleFunc("PUT /api/v1/admin/system-configuration", api.requireSession(api.adminSystemConfiguration))
	mux.HandleFunc("GET /api/v1/admin/identity/oidc", api.requireSession(api.adminOIDCConfiguration))
	mux.HandleFunc("PUT /api/v1/admin/identity/oidc", api.requireSession(api.adminOIDCConfiguration))
	mux.HandleFunc("GET /api/v1/admin/identity/scim", api.requireSession(api.adminSCIMConfiguration))
	mux.HandleFunc("POST /api/v1/admin/identity/scim", api.requireSession(api.adminSCIMConfiguration))
	mux.HandleFunc("DELETE /api/v1/admin/identity/scim", api.requireSession(api.adminSCIMConfiguration))
	mux.HandleFunc("GET /api/v1/scim/v2/{tenant}/ServiceProviderConfig", api.scimServiceProvider)
	mux.HandleFunc("GET /api/v1/scim/v2/{tenant}/Users", api.scimUsers)
	mux.HandleFunc("POST /api/v1/scim/v2/{tenant}/Users", api.scimUsers)
	mux.HandleFunc("GET /api/v1/scim/v2/{tenant}/Users/{resourceID}", api.scimUser)
	mux.HandleFunc("PUT /api/v1/scim/v2/{tenant}/Users/{resourceID}", api.scimUser)
	mux.HandleFunc("PATCH /api/v1/scim/v2/{tenant}/Users/{resourceID}", api.scimUser)
	mux.HandleFunc("DELETE /api/v1/scim/v2/{tenant}/Users/{resourceID}", api.scimUser)
	mux.HandleFunc("GET /api/v1/scim/v2/{tenant}/Groups", api.scimGroups)
	mux.HandleFunc("POST /api/v1/scim/v2/{tenant}/Groups", api.scimGroups)
	mux.HandleFunc("GET /api/v1/scim/v2/{tenant}/Groups/{resourceID}", api.scimGroup)
	mux.HandleFunc("PUT /api/v1/scim/v2/{tenant}/Groups/{resourceID}", api.scimGroup)
	mux.HandleFunc("PATCH /api/v1/scim/v2/{tenant}/Groups/{resourceID}", api.scimGroup)
	mux.HandleFunc("DELETE /api/v1/scim/v2/{tenant}/Groups/{resourceID}", api.scimGroup)
	mux.HandleFunc("GET /api/v1/admin/roles", api.requireSession(api.adminCustomRoles))
	mux.HandleFunc("POST /api/v1/admin/roles", api.requireSession(api.adminCustomRoles))
	mux.HandleFunc("PUT /api/v1/admin/roles/{roleID}", api.requireSession(api.updateCustomRole))
	mux.HandleFunc("DELETE /api/v1/admin/roles/{roleID}", api.requireSession(api.updateCustomRole))
	mux.HandleFunc("PUT /api/v1/admin/roles/{roleID}/users/{userID}", api.requireSession(api.customRoleAssignment))
	mux.HandleFunc("DELETE /api/v1/admin/roles/{roleID}/users/{userID}", api.requireSession(api.customRoleAssignment))
	mux.HandleFunc("GET /api/v1/admin/governance/policy", api.requireSession(api.adminGovernancePolicy))
	mux.HandleFunc("PUT /api/v1/admin/governance/policy", api.requireSession(api.adminGovernancePolicy))
	mux.HandleFunc("GET /api/v1/admin/governance/holds", api.requireSession(api.adminLegalHolds))
	mux.HandleFunc("POST /api/v1/admin/governance/holds", api.requireSession(api.adminLegalHolds))
	mux.HandleFunc("POST /api/v1/admin/governance/holds/{holdID}/release", api.requireSession(api.releaseLegalHold))
	mux.HandleFunc("PUT /api/v1/admin/governance/holds/{holdID}/resources/{resourceType}/{resourceID}", api.requireSession(api.legalHoldResource))
	mux.HandleFunc("DELETE /api/v1/admin/governance/holds/{holdID}/resources/{resourceType}/{resourceID}", api.requireSession(api.legalHoldResource))
	mux.HandleFunc("POST /api/v1/admin/governance/retention/run", api.requireSession(api.runGovernanceRetention))
	mux.HandleFunc("GET /api/v1/admin/governance/exports", api.requireSession(api.adminDataExports))
	mux.HandleFunc("POST /api/v1/admin/governance/exports", api.requireSession(api.adminDataExports))
	mux.HandleFunc("POST /api/v1/admin/governance/exports/{exportID}/approve", api.requireSession(api.reviewDataExport))
	mux.HandleFunc("POST /api/v1/admin/governance/exports/{exportID}/reject", api.requireSession(api.reviewDataExport))
	mux.HandleFunc("GET /api/v1/admin/governance/exports/{exportID}/download", api.requireSession(api.downloadDataExport))
	mux.HandleFunc("GET /api/v1/platform/overview", api.requireSession(api.platformOverview))
	mux.HandleFunc("GET /api/v1/platform/tenants", api.requireSession(api.platformTenants))
	mux.HandleFunc("GET /api/v1/platform/tenants/{tenantID}", api.requireSession(api.platformTenantDetail))
	mux.HandleFunc("POST /api/v1/platform/tenants/{tenantID}/{action}", api.requireSession(api.platformTenantLifecycle))
	mux.HandleFunc("POST /api/v1/platform/tenants/{tenantID}/support-access", api.requireSession(api.platformSupportAccess))
	mux.HandleFunc("DELETE /api/v1/platform/tenants/{tenantID}/support-access", api.requireSession(api.platformSupportAccess))
	mux.HandleFunc("GET /api/v1/platform/tenants/{tenantID}/support-view", api.requireSession(api.platformSupportView))
	mux.HandleFunc("GET /api/v1/meetings", api.requireSession(api.listMeetings))
	mux.HandleFunc("GET /api/v1/recordings", api.requireSession(api.recordingLibrary))
	mux.HandleFunc("GET /api/v1/recordings/{recordingID}/file", api.requireSession(api.recordingLibraryFile))
	mux.HandleFunc("DELETE /api/v1/recordings/{recordingID}", api.requireSession(api.deleteLibraryRecording))
	mux.HandleFunc("DELETE /api/v1/meetings/{joinCode}", api.requireSession(api.deleteMeeting))
	mux.HandleFunc("POST /api/v1/meetings", api.requireSession(api.createMeeting))
	mux.HandleFunc("GET /api/v1/meetings/history", api.requireSession(api.meetingHistory))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}", api.requireSession(api.getMeeting))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/join", api.requireSession(api.joinMeeting))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/token", api.requireSession(api.liveKitToken))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/participants", api.requireSession(api.listParticipants))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/participant", api.requireSession(api.currentParticipant))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/participants/{participantID}/avatar", api.requireSession(api.participantAvatar))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/participants/{participantID}/{action}", api.requireSession(api.moderateParticipant))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/moderation/{action}", api.requireSession(api.moderateMeeting))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/leave", api.requireSession(api.leaveMeeting))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/recordings", api.requireSession(api.recordings))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/recordings", api.requireSession(api.recordings))
	mux.HandleFunc("POST /api/v1/meetings/{joinCode}/recordings/stop", api.requireSession(api.stopRecording))
	mux.HandleFunc("GET /api/v1/meetings/{joinCode}/recordings/{recordingID}/download", api.requireSession(api.recordingDownload))
	mux.HandleFunc("DELETE /api/v1/meetings/{joinCode}/recordings/{recordingID}", api.requireSession(api.deleteRecording))
	mux.HandleFunc("PUT /api/v1/meetings/{joinCode}/recordings/{recordingID}/access/{userID}", api.requireSession(api.recordingAccess))
	mux.HandleFunc("DELETE /api/v1/meetings/{joinCode}/recordings/{recordingID}/access/{userID}", api.requireSession(api.recordingAccess))

	secured := withSecurityHeaders(newRateLimiter().middleware(mux))
	return observeRequests(logger, recoverPanics(logger, secured))
}

type API struct {
	database *sql.DB
	chat     *chatHub
}

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
