package httpapi

import "net/http"

func (api *API) leaveMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeetingForJoin(request)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `
		UPDATE meeting_participants SET status='LEFT',left_at=NOW()
		WHERE id=(
			SELECT id FROM meeting_participants
			WHERE meeting_id=$1 AND tenant_id=$2 AND status IN ('JOINED','WAITING_ROOM')
			  AND ((user_id=$3 AND $2=$4) OR (external_user_id=$3 AND external_tenant_id=$4))
			ORDER BY created_at DESC LIMIT 1
		)`, meeting.ID, meeting.TenantID, user.ID, user.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update participant lifecycle")
		return
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		actorID := user.ID
		metadata := map[string]any{}
		if meeting.TenantID != user.TenantID {
			actorID = ""
			metadata["externalUserId"] = user.ID
			metadata["externalTenantId"] = user.TenantID
		}
		_ = api.writeAuditEvent(request.Context(), request, meeting.TenantID, actorID, "participant.leave", "participant", user.ID, metadata)
	}
	writer.WriteHeader(http.StatusNoContent)
}
