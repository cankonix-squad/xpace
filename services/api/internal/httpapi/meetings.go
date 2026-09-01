package httpapi

import (
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"
)

type meetingResponse struct {
	ID                 string     `json:"id"`
	TenantID           string     `json:"-"`
	WorkspaceSlug      string     `json:"workspaceSlug"`
	WorkspaceName      string     `json:"workspaceName"`
	ExternalGuest      bool       `json:"externalGuest"`
	Title              string     `json:"title"`
	JoinCode           string     `json:"joinCode"`
	RoomName           string     `json:"roomName"`
	Status             string     `json:"status"`
	ScheduledAt        *time.Time `json:"scheduledAt"`
	WaitingRoomEnabled bool       `json:"waitingRoomEnabled"`
	HostID             string     `json:"hostId"`
	CreatedAt          time.Time  `json:"createdAt"`
}

func (api *API) createMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	if !api.hasPermission(request.Context(), user, "meeting.create") {
		errorJSON(writer, http.StatusForbidden, "INSUFFICIENT_ROLE", "guests cannot create meetings")
		return
	}
	if err := api.enforceTenantQuota(request.Context(), user.TenantID, "meetings", 1); err != nil {
		if !respondEntitlementError(writer, err) {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
		}
		return
	}
	var input struct {
		Title              string     `json:"title"`
		ScheduledAt        *time.Time `json:"scheduledAt"`
		WaitingRoomEnabled *bool      `json:"waitingRoomEnabled"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Title = strings.TrimSpace(input.Title)
	if len(input.Title) < 3 || len(input.Title) > 120 {
		errorJSON(writer, 400, "INVALID_TITLE", "meeting title must be between 3 and 120 characters")
		return
	}
	policy, err := api.loadMeetingPolicy(request.Context(), user.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
		return
	}
	waiting := policy.WaitingRoomDefault
	if input.WaitingRoomEnabled != nil {
		waiting = *input.WaitingRoomEnabled
	}
	status := "WAITING"
	if input.ScheduledAt != nil && input.ScheduledAt.After(time.Now()) {
		status = "SCHEDULED"
	}
	var meeting meetingResponse
	for attempt := 0; attempt < 3; attempt++ {
		code, err := meetingCode()
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not generate meeting code")
			return
		}
		roomToken, _ := randomToken(12)
		meeting.RoomName = "xpace-" + strings.ToLower(roomToken)
		meeting.JoinCode = code
		err = api.database.QueryRowContext(request.Context(), `INSERT INTO meetings (tenant_id,host_id,room_name,join_code,title,scheduled_at,status,waiting_room_enabled) VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING id,status,created_at`, user.TenantID, user.ID, meeting.RoomName, meeting.JoinCode, input.Title, input.ScheduledAt, status, waiting).Scan(&meeting.ID, &meeting.Status, &meeting.CreatedAt)
		if err == nil {
			break
		}
		if attempt == 2 {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create meeting")
			return
		}
	}
	meeting.Title = input.Title
	meeting.ScheduledAt = input.ScheduledAt
	meeting.WaitingRoomEnabled = waiting
	meeting.HostID = user.ID
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "meeting.create", "meeting", meeting.ID, nil)
	respondJSON(writer, 201, map[string]any{"meeting": meeting})
}

func (api *API) listMeetings(writer http.ResponseWriter, request *http.Request, user currentUser) {
	limit, offset := 10, 0
	if raw := request.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 50 {
			errorJSON(writer, http.StatusBadRequest, "INVALID_PAGINATION", "limit must be between 1 and 50")
			return
		}
		limit = value
	}
	if raw := request.URL.Query().Get("offset"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 0 {
			errorJSON(writer, http.StatusBadRequest, "INVALID_PAGINATION", "offset must be zero or greater")
			return
		}
		offset = value
	}
	var total int
	if err := api.database.QueryRowContext(request.Context(), `SELECT COUNT(*) FROM meetings WHERE tenant_id=$1 AND status!='CANCELLED'`, user.TenantID).Scan(&total); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not count meetings")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,title,join_code,room_name,status,scheduled_at,waiting_room_enabled,host_id,created_at FROM meetings WHERE tenant_id=$1 AND status!='CANCELLED' ORDER BY COALESCE(scheduled_at,created_at) DESC LIMIT $2 OFFSET $3`, user.TenantID, limit, offset)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meetings")
		return
	}
	defer rows.Close()
	items := make([]meetingResponse, 0)
	for rows.Next() {
		var m meetingResponse
		if err = rows.Scan(&m.ID, &m.Title, &m.JoinCode, &m.RoomName, &m.Status, &m.ScheduledAt, &m.WaitingRoomEnabled, &m.HostID, &m.CreatedAt); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meetings")
			return
		}
		items = append(items, m)
	}
	respondJSON(writer, 200, map[string]any{"meetings": items, "pagination": map[string]any{"limit": limit, "offset": offset, "total": total, "hasMore": offset+len(items) < total}})
}

func (api *API) deleteMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	code := strings.ToUpper(strings.TrimSpace(request.PathValue("joinCode")))
	var id, hostID, status string
	err := api.database.QueryRowContext(request.Context(), `SELECT id,host_id,status::text FROM meetings WHERE tenant_id=$1 AND join_code=$2 AND status!='CANCELLED'`, user.TenantID, code).Scan(&id, &hostID, &status)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting does not exist or was already deleted")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	if hostID != user.ID && !user.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "INSUFFICIENT_ROLE", "only the host or a workspace administrator can delete this meeting")
		return
	}
	if status == "ACTIVE" {
		errorJSON(writer, http.StatusConflict, "MEETING_ACTIVE", "end the active meeting before deleting it")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE meetings SET status='CANCELLED',updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status!='ACTIVE'`, id, user.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not delete meeting")
		return
	}
	if affected, _ := result.RowsAffected(); affected == 0 {
		errorJSON(writer, http.StatusConflict, "MEETING_ACTIVE", "end the active meeting before deleting it")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "meeting.delete", "meeting", id, map[string]any{"joinCode": code, "previousStatus": status})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) getMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeetingForJoin(request)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	meeting.ExternalGuest = meeting.TenantID != user.TenantID
	if meeting.ExternalGuest {
		policy, policyErr := api.loadMeetingPolicy(request.Context(), meeting.TenantID)
		if policyErr != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
			return
		}
		if !policy.GuestAccessEnabled {
			errorJSON(writer, http.StatusForbidden, "GUEST_ACCESS_DISABLED", "external guests are disabled for this workspace")
			return
		}
	}
	respondJSON(writer, 200, map[string]any{"meeting": meeting})
}

func (api *API) joinMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	var input struct {
		RecordingNoticeAcknowledged bool   `json:"recordingNoticeAcknowledged"`
		RecordingConsentVersion     string `json:"recordingConsentVersion"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "RECORDING_NOTICE_REQUIRED", "review and acknowledge the recording notice before joining")
		return
	}
	if !input.RecordingNoticeAcknowledged || strings.TrimSpace(input.RecordingConsentVersion) != currentRecordingConsentVersion() {
		errorJSON(writer, http.StatusBadRequest, "RECORDING_NOTICE_REQUIRED", "review and acknowledge the current recording notice before joining")
		return
	}
	meeting, err := api.findMeetingForJoin(request)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	meeting.ExternalGuest = meeting.TenantID != user.TenantID
	policy, err := api.loadMeetingPolicy(request.Context(), meeting.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
		return
	}
	if (meeting.ExternalGuest || user.Role == roleGuest) && !policy.GuestAccessEnabled {
		errorJSON(writer, http.StatusForbidden, "GUEST_ACCESS_DISABLED", "external guests are disabled for this workspace")
		return
	}
	if meeting.Status == "ENDED" || meeting.Status == "CANCELLED" {
		errorJSON(writer, 409, "MEETING_UNAVAILABLE", "meeting is no longer available")
		return
	}
	var locked bool
	if err = api.database.QueryRowContext(request.Context(), "SELECT locked_at IS NOT NULL FROM meetings WHERE id=$1 AND tenant_id=$2", meeting.ID, meeting.TenantID).Scan(&locked); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify meeting policy")
		return
	}
	if locked && meeting.HostID != user.ID {
		errorJSON(writer, 423, "MEETING_LOCKED", "the host has locked this meeting")
		return
	}
	status := "JOINED"
	if meeting.WaitingRoomEnabled && meeting.HostID != user.ID {
		status = "WAITING_ROOM"
	}
	var participantID string
	participantRole := "MEMBER"
	if meeting.HostID == user.ID {
		participantRole = "HOST"
	} else if meeting.ExternalGuest || user.Role == roleGuest {
		participantRole = "GUEST"
	}
	if meeting.ExternalGuest {
		err = api.database.QueryRowContext(request.Context(), `
			INSERT INTO meeting_participants (meeting_id,user_id,tenant_id,external_user_id,external_tenant_id,display_name,role,status,joined_at)
			VALUES ($1,NULL,$2,$3,$4,$5,$6,$7::participant_status,CASE WHEN $7::participant_status='JOINED'::participant_status THEN NOW() ELSE NULL END)
			ON CONFLICT (meeting_id,external_user_id)
			  WHERE external_user_id IS NOT NULL AND status NOT IN ('LEFT','REMOVED')
			DO UPDATE SET display_name=EXCLUDED.display_name
			RETURNING id,status::text,role`, meeting.ID, meeting.TenantID, user.ID, user.TenantID, user.DisplayName, participantRole, status).Scan(&participantID, &status, &participantRole)
	} else {
		err = api.database.QueryRowContext(request.Context(), `
			INSERT INTO meeting_participants (meeting_id,user_id,tenant_id,display_name,role,status,joined_at)
			VALUES ($1,$2,$3,$4,$5,$6::participant_status,CASE WHEN $6::participant_status='JOINED'::participant_status THEN NOW() ELSE NULL END)
			ON CONFLICT (meeting_id,user_id)
			  WHERE user_id IS NOT NULL AND status NOT IN ('LEFT','REMOVED')
			DO UPDATE SET
			  display_name=EXCLUDED.display_name,
			  role=CASE WHEN meeting_participants.role='CO_HOST' THEN meeting_participants.role ELSE EXCLUDED.role END
			RETURNING id,status::text,role`, meeting.ID, user.ID, meeting.TenantID, user.DisplayName, participantRole, status).Scan(&participantID, &status, &participantRole)
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not join meeting")
		return
	}
	if meeting.HostID == user.ID && meeting.Status != "ACTIVE" {
		if _, err = api.database.ExecContext(request.Context(), `UPDATE meetings SET status='ACTIVE',started_at=COALESCE(started_at,NOW()),updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status IN ('SCHEDULED','WAITING')`, meeting.ID, meeting.TenantID); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not start meeting")
			return
		}
		meeting.Status = "ACTIVE"
	}
	auditMetadata := map[string]any{"participantId": participantID, "status": status, "role": participantRole, "recordingNoticeAcknowledged": true, "recordingConsentVersion": currentRecordingConsentVersion()}
	if meeting.ExternalGuest {
		auditMetadata["externalUserId"] = user.ID
		auditMetadata["externalTenantId"] = user.TenantID
	}
	_ = api.writeAuditEvent(request.Context(), request, meeting.TenantID, user.ID, "meeting.join", "meeting", meeting.ID, auditMetadata)
	respondJSON(writer, 201, map[string]any{"meeting": meeting, "participant": map[string]string{"id": participantID, "status": status}})
}

func currentRecordingConsentVersion() string {
	if value := strings.TrimSpace(os.Getenv("RECORDING_CONSENT_VERSION")); value != "" {
		return value
	}
	return "2026-08-29"
}

func (api *API) findMeeting(request *http.Request, user currentUser) (meetingResponse, error) {
	var m meetingResponse
	code := strings.ToUpper(strings.TrimSpace(request.PathValue("joinCode")))
	err := api.database.QueryRowContext(request.Context(), `SELECT id,title,join_code,room_name,status,scheduled_at,waiting_room_enabled,host_id,created_at FROM meetings WHERE tenant_id=$1 AND join_code=$2`, user.TenantID, code).Scan(&m.ID, &m.Title, &m.JoinCode, &m.RoomName, &m.Status, &m.ScheduledAt, &m.WaitingRoomEnabled, &m.HostID, &m.CreatedAt)
	m.TenantID, m.WorkspaceSlug, m.WorkspaceName = user.TenantID, user.TenantSlug, user.TenantName
	return m, err
}

func (api *API) findMeetingForJoin(request *http.Request) (meetingResponse, error) {
	var m meetingResponse
	code := strings.ToUpper(strings.TrimSpace(request.PathValue("joinCode")))
	err := api.database.QueryRowContext(request.Context(), `
		SELECT m.id,m.tenant_id,t.slug,t.name,m.title,m.join_code,m.room_name,m.status,
		       m.scheduled_at,m.waiting_room_enabled,m.host_id,m.created_at
		FROM meetings m JOIN tenants t ON t.id=m.tenant_id
		WHERE m.join_code=$1 AND m.status!='CANCELLED'`, code).Scan(
		&m.ID, &m.TenantID, &m.WorkspaceSlug, &m.WorkspaceName, &m.Title, &m.JoinCode,
		&m.RoomName, &m.Status, &m.ScheduledAt, &m.WaitingRoomEnabled, &m.HostID, &m.CreatedAt,
	)
	return m, err
}
func meetingCode() (string, error) {
	token, err := randomToken(8)
	if err != nil {
		return "", err
	}
	token = strings.ToUpper(strings.NewReplacer("-", "A", "_", "B").Replace(token))
	if len(token) < 9 {
		return "", fmt.Errorf("short random token")
	}
	return fmt.Sprintf("%s-%s-%s", token[:3], token[3:6], token[6:9]), nil
}
