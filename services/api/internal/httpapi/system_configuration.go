package httpapi

import (
	"database/sql"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"
	_ "time/tzdata"
	"unicode/utf8"
)

type systemConfiguration struct {
	WorkspaceName             string `json:"workspaceName"`
	DefaultTimezone           string `json:"defaultTimezone"`
	DefaultLocale             string `json:"defaultLocale"`
	SupportEmail              string `json:"supportEmail"`
	MaxMeetingDurationMinutes int    `json:"maxMeetingDurationMinutes"`
	RecordingRetentionDays    int    `json:"recordingRetentionDays"`
}

func (api *API) adminSystemConfiguration(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "tenant.manage") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	if request.Method == http.MethodGet {
		configuration, err := api.loadSystemConfiguration(request, actor.TenantID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load system configuration")
			return
		}
		respondJSON(writer, http.StatusOK, map[string]any{"configuration": configuration})
		return
	}

	var configuration systemConfiguration
	if err := decodeJSON(writer, request, &configuration); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	configuration.normalize()
	if code, message := configuration.validate(); code != "" {
		errorJSON(writer, http.StatusBadRequest, code, message)
		return
	}
	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{})
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update system configuration")
		return
	}
	defer tx.Rollback()
	_, err = tx.ExecContext(request.Context(), `UPDATE tenants SET name=$1,updated_at=NOW() WHERE id=$2`, configuration.WorkspaceName, actor.TenantID)
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO tenant_system_configurations (tenant_id,default_timezone,default_locale,support_email,max_meeting_duration_minutes,recording_retention_days,updated_by) VALUES ($1,$2,$3,$4,$5,$6,$7) ON CONFLICT (tenant_id) DO UPDATE SET default_timezone=EXCLUDED.default_timezone,default_locale=EXCLUDED.default_locale,support_email=EXCLUDED.support_email,max_meeting_duration_minutes=EXCLUDED.max_meeting_duration_minutes,recording_retention_days=EXCLUDED.recording_retention_days,updated_by=EXCLUDED.updated_by,updated_at=NOW()`, actor.TenantID, configuration.DefaultTimezone, configuration.DefaultLocale, configuration.SupportEmail, configuration.MaxMeetingDurationMinutes, configuration.RecordingRetentionDays, actor.ID)
	}
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO audit_events (tenant_id,actor_user_id,action,resource_type,resource_id,ip_address,user_agent,metadata) VALUES ($1,$2,'system.configuration.update','tenant',$9,NULLIF($3,'')::inet,$4,jsonb_build_object('timezone',$5::text,'locale',$6::text,'maxMeetingDurationMinutes',$7::integer,'recordingRetentionDays',$8::integer))`, actor.TenantID, actor.ID, clientIP(request), request.UserAgent(), configuration.DefaultTimezone, configuration.DefaultLocale, configuration.MaxMeetingDurationMinutes, configuration.RecordingRetentionDays, actor.TenantID)
	}
	if err != nil {
		slog.Error("system configuration transaction failed", "error", err)
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update system configuration")
		return
	}
	if err = tx.Commit(); err != nil {
		slog.Error("system configuration commit failed", "error", err)
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update system configuration")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"configuration": configuration})
}

func (api *API) loadSystemConfiguration(request *http.Request, tenantID string) (systemConfiguration, error) {
	configuration := systemConfiguration{DefaultTimezone: "Asia/Jakarta", DefaultLocale: "id-ID", MaxMeetingDurationMinutes: 120, RecordingRetentionDays: 30}
	err := api.database.QueryRowContext(request.Context(), `SELECT t.name,COALESCE(c.default_timezone,'Asia/Jakarta'),COALESCE(c.default_locale,'id-ID'),COALESCE(c.support_email,''),COALESCE(c.max_meeting_duration_minutes,120),COALESCE(c.recording_retention_days,30) FROM tenants t LEFT JOIN tenant_system_configurations c ON c.tenant_id=t.id WHERE t.id=$1`, tenantID).Scan(&configuration.WorkspaceName, &configuration.DefaultTimezone, &configuration.DefaultLocale, &configuration.SupportEmail, &configuration.MaxMeetingDurationMinutes, &configuration.RecordingRetentionDays)
	return configuration, err
}

func (configuration *systemConfiguration) normalize() {
	configuration.WorkspaceName = strings.TrimSpace(configuration.WorkspaceName)
	configuration.DefaultTimezone = strings.TrimSpace(configuration.DefaultTimezone)
	configuration.DefaultLocale = strings.TrimSpace(configuration.DefaultLocale)
	configuration.SupportEmail = strings.ToLower(strings.TrimSpace(configuration.SupportEmail))
}

func (configuration systemConfiguration) validate() (string, string) {
	if utf8.RuneCountInString(configuration.WorkspaceName) < 2 || utf8.RuneCountInString(configuration.WorkspaceName) > 120 {
		return "INVALID_WORKSPACE_NAME", "workspaceName must be between 2 and 120 characters"
	}
	if _, err := time.LoadLocation(configuration.DefaultTimezone); err != nil {
		return "INVALID_TIMEZONE", "defaultTimezone must be a valid IANA timezone"
	}
	if !validLocale(configuration.DefaultLocale) {
		return "INVALID_LOCALE", "defaultLocale must use a language or language-region format"
	}
	if configuration.SupportEmail != "" {
		address, err := mail.ParseAddress(configuration.SupportEmail)
		if err != nil || address.Address != configuration.SupportEmail {
			return "INVALID_SUPPORT_EMAIL", "supportEmail must be a valid email address or empty"
		}
	}
	if configuration.MaxMeetingDurationMinutes < 15 || configuration.MaxMeetingDurationMinutes > 1440 {
		return "INVALID_MEETING_DURATION", "maxMeetingDurationMinutes must be between 15 and 1440"
	}
	if configuration.RecordingRetentionDays < 1 || configuration.RecordingRetentionDays > 3650 {
		return "INVALID_RECORDING_RETENTION", "recordingRetentionDays must be between 1 and 3650"
	}
	return "", ""
}
