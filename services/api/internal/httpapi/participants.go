package httpapi

import (
	"database/sql"
	"net/http"
)

type participantResponse struct {
	ID          string  `json:"id"`
	UserID      *string `json:"userId"`
	DisplayName string  `json:"displayName"`
	Role        string  `json:"role"`
	Status      string  `json:"status"`
}

func (api *API) currentParticipant(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	var p participantResponse
	err = api.database.QueryRowContext(request.Context(), `SELECT id,user_id,display_name,role,status FROM meeting_participants WHERE meeting_id=$1 AND user_id=$2 AND tenant_id=$3 ORDER BY created_at DESC LIMIT 1`, meeting.ID, user.ID, user.TenantID).Scan(&p.ID, &p.UserID, &p.DisplayName, &p.Role, &p.Status)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "PARTICIPANT_NOT_FOUND", "complete the join flow first")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load participant")
		return
	}
	respondJSON(writer, 200, map[string]any{"participant": p, "isHost": api.canModerate(request, meeting, user)})
}

func (api *API) listParticipants(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if !api.canModerate(request, meeting, user) {
		errorJSON(writer, 403, "HOST_REQUIRED", "only the meeting host can view the waiting room")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,user_id,display_name,role,status FROM meeting_participants WHERE meeting_id=$1 AND tenant_id=$2 AND status NOT IN ('LEFT','REMOVED') ORDER BY created_at`, meeting.ID, user.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load participants")
		return
	}
	defer rows.Close()
	items := make([]participantResponse, 0)
	for rows.Next() {
		var p participantResponse
		if err = rows.Scan(&p.ID, &p.UserID, &p.DisplayName, &p.Role, &p.Status); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load participants")
			return
		}
		items = append(items, p)
	}
	respondJSON(writer, 200, map[string]any{"participants": items})
}

func (api *API) moderateParticipant(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if !api.canModerate(request, meeting, user) {
		errorJSON(writer, 403, "HOST_REQUIRED", "only the meeting host can moderate participants")
		return
	}
	participantID, action := request.PathValue("participantID"), request.PathValue("action")
	var targetUserID, targetRole, targetStatus string
	if err = api.database.QueryRowContext(request.Context(), `SELECT COALESCE(user_id::text,''),role,status FROM meeting_participants WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3`, participantID, meeting.ID, user.TenantID).Scan(&targetUserID, &targetRole, &targetStatus); err != nil {
		errorJSON(writer, 404, "PARTICIPANT_NOT_FOUND", "participant was not found")
		return
	}
	if action == "mute" {
		if targetRole == "HOST" || targetStatus != "JOINED" {
			errorJSON(writer, 409, "ACTION_NOT_ALLOWED", "participant cannot be muted")
			return
		}
		if err = api.syncParticipantAction(request.Context(), meeting, targetUserID, participantID, action); err != nil {
			errorJSON(writer, 502, "REALTIME_ERROR", "could not mute participant in realtime room")
			return
		}
		api.auditModeration(request, user, "participant.mute", participantID)
		respondJSON(writer, 200, map[string]string{"status": "ok"})
		return
	}
	var query, audit string
	switch action {
	case "admit":
		query = `UPDATE meeting_participants SET status='JOINED',joined_at=COALESCE(joined_at,NOW()) WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3 AND status='WAITING_ROOM'`
		audit = "participant.admit"
	case "reject", "remove":
		query = `UPDATE meeting_participants SET status='REMOVED',left_at=NOW() WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3 AND status NOT IN ('LEFT','REMOVED') AND role!='HOST'`
		audit = "participant." + action
	case "promote":
		query = `UPDATE meeting_participants SET role='CO_HOST' WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3 AND status='JOINED' AND role='MEMBER'`
		audit = "participant.promote"
	default:
		errorJSON(writer, 400, "INVALID_ACTION", "unsupported moderation action")
		return
	}
	result, err := api.database.ExecContext(request.Context(), query, participantID, meeting.ID, user.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update participant")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		errorJSON(writer, 409, "ACTION_NOT_ALLOWED", "participant state does not allow this action")
		return
	}
	if (action == "remove" || action == "promote") && targetUserID != "" {
		_ = api.syncParticipantAction(request.Context(), meeting, targetUserID, participantID, action)
	}
	api.auditModeration(request, user, audit, participantID)
	respondJSON(writer, 200, map[string]string{"status": "ok"})
}

func (api *API) auditModeration(request *http.Request, user currentUser, action, participantID string) {
	resourceType := "participant"
	if action == "meeting.lock" || action == "meeting.unlock" || action == "meeting.end" {
		resourceType = "meeting"
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, action, resourceType, participantID, nil)
}

func (api *API) canModerate(request *http.Request, meeting meetingResponse, user currentUser) bool {
	if user.Role.isWorkspaceAdmin() || meeting.HostID == user.ID {
		return true
	}
	var exists bool
	_ = api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM meeting_participants WHERE meeting_id=$1 AND user_id=$2 AND tenant_id=$3 AND role='CO_HOST' AND status='JOINED')`, meeting.ID, user.ID, user.TenantID).Scan(&exists)
	return exists
}
