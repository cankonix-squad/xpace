package httpapi

import "net/http"

func (api *API) leaveMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE meeting_participants SET status='LEFT',left_at=NOW() WHERE tenant_id=$3 AND id=(SELECT id FROM meeting_participants WHERE meeting_id=$1 AND user_id=$2 AND tenant_id=$3 AND status='JOINED' ORDER BY created_at DESC LIMIT 1)`, meeting.ID, user.ID, user.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update participant lifecycle")
		return
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		api.auditModeration(request, user, "participant.leave", user.ID)
	}
	writer.WriteHeader(http.StatusNoContent)
}
