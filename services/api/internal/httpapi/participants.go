package httpapi

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/livekit/protocol/livekit"
)

type participantResponse struct {
	ID          string  `json:"id"`
	UserID      *string `json:"userId"`
	AvatarURL   *string `json:"avatarUrl"`
	HasAvatar   bool    `json:"-"`
	DisplayName string  `json:"displayName"`
	Role        string  `json:"role"`
	Status      string  `json:"status"`
}

func (api *API) currentParticipant(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeetingForJoin(request)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	var p participantResponse
	err = api.database.QueryRowContext(request.Context(), `
		SELECT participant.id,
		       COALESCE(participant.user_id,participant.external_user_id),
		       NULLIF(BTRIM(profile.avatar_url),'') IS NOT NULL,
		       participant.display_name,
		       participant.role,
		       participant.status
		FROM meeting_participants AS participant
		LEFT JOIN user_profiles AS profile
		  ON profile.user_id=COALESCE(participant.user_id,participant.external_user_id)
		 AND profile.tenant_id=COALESCE(participant.external_tenant_id,participant.tenant_id)
		WHERE participant.meeting_id=$1 AND participant.tenant_id=$2
		  AND ((participant.user_id=$3 AND $2=$4)
		       OR (participant.external_user_id=$3 AND participant.external_tenant_id=$4))
		ORDER BY participant.created_at DESC LIMIT 1`, meeting.ID, meeting.TenantID, user.ID, user.TenantID).Scan(&p.ID, &p.UserID, &p.HasAvatar, &p.DisplayName, &p.Role, &p.Status)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "PARTICIPANT_NOT_FOUND", "complete the join flow first")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load participant")
		return
	}
	if p.HasAvatar {
		p.AvatarURL = stringPointer("/api/v1/meetings/" + meeting.JoinCode + "/participants/" + p.ID + "/avatar")
	}
	respondJSON(writer, 200, map[string]any{"participant": p, "isHost": api.canModerate(request, meeting, user)})
}

func (api *API) listParticipants(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeetingForJoin(request)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if !api.canModerate(request, meeting, user) {
		errorJSON(writer, 403, "HOST_REQUIRED", "only the meeting host can view the waiting room")
		return
	}
	// The database is the durable meeting history, while LiveKit is the source
	// of truth for who is online right now. Reconcile stale JOINED rows left by
	// a closed tab or interrupted leave request before returning the host list.
	api.reconcileRealtimeParticipants(request.Context(), meeting)
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT participant.id,
		       COALESCE(participant.user_id,participant.external_user_id),
		       NULLIF(BTRIM(profile.avatar_url),'') IS NOT NULL,
		       participant.display_name,
		       participant.role,
		       participant.status
		FROM meeting_participants AS participant
		LEFT JOIN user_profiles AS profile
		  ON profile.user_id=COALESCE(participant.user_id,participant.external_user_id)
		 AND profile.tenant_id=COALESCE(participant.external_tenant_id,participant.tenant_id)
		WHERE participant.meeting_id=$1 AND participant.tenant_id=$2
		  AND participant.status NOT IN ('LEFT','REMOVED')
		ORDER BY participant.created_at`, meeting.ID, meeting.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load participants")
		return
	}
	defer rows.Close()
	items := make([]participantResponse, 0)
	for rows.Next() {
		var p participantResponse
		if err = rows.Scan(&p.ID, &p.UserID, &p.HasAvatar, &p.DisplayName, &p.Role, &p.Status); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load participants")
			return
		}
		if p.HasAvatar {
			p.AvatarURL = stringPointer("/api/v1/meetings/" + meeting.JoinCode + "/participants/" + p.ID + "/avatar")
		}
		items = append(items, p)
	}
	respondJSON(writer, 200, map[string]any{"participants": items})
}

func (api *API) reconcileRealtimeParticipants(ctx context.Context, meeting meetingResponse) {
	client, err := liveKitRoomClient()
	if err != nil {
		return
	}
	response, err := client.ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: meeting.RoomName})
	if err != nil {
		return
	}
	liveUsers := make(map[string]struct{}, len(response.Participants))
	for _, participant := range response.Participants {
		if identity := logicalRealtimeIdentity(participant.Identity); identity != "" {
			liveUsers[identity] = struct{}{}
		}
	}
	rows, err := api.database.QueryContext(ctx, `
		SELECT id,COALESCE(user_id::text,external_user_id::text)
		FROM meeting_participants
		WHERE meeting_id=$1 AND tenant_id=$2 AND status='JOINED'
		  AND joined_at < $3`, meeting.ID, meeting.TenantID, time.Now().Add(-20*time.Second))
	if err != nil {
		return
	}
	defer rows.Close()
	type staleCandidate struct{ id, userID string }
	candidates := make([]staleCandidate, 0)
	for rows.Next() {
		var candidate staleCandidate
		if rows.Scan(&candidate.id, &candidate.userID) == nil {
			candidates = append(candidates, candidate)
		}
	}
	for _, candidate := range candidates {
		if _, online := liveUsers[candidate.userID]; online {
			continue
		}
		_, _ = api.database.ExecContext(ctx, `UPDATE meeting_participants SET status='LEFT',left_at=COALESCE(left_at,NOW()) WHERE id=$1 AND status='JOINED'`, candidate.id)
	}
}

func logicalRealtimeIdentity(identity string) string {
	// Accept the previous userId:participantId format during rolling deploys,
	// while all newly issued tokens use the stable user UUID directly.
	if index := strings.IndexByte(identity, ':'); index >= 0 {
		return identity[:index]
	}
	return identity
}

func (api *API) participantAvatar(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeetingForJoin(request)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	participantID := request.PathValue("participantID")
	var sourceUserID, sourceTenantID string
	err = api.database.QueryRowContext(request.Context(), `
		SELECT COALESCE(user_id::text,external_user_id::text),
		       COALESCE(external_tenant_id::text,tenant_id::text)
		FROM meeting_participants
		WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3`, participantID, meeting.ID, meeting.TenantID).Scan(&sourceUserID, &sourceTenantID)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "AVATAR_NOT_FOUND", "profile picture is not available")
		return
	}
	allowed := sourceUserID == user.ID && sourceTenantID == user.TenantID
	moderator := api.canModerate(request, meeting, user)
	if !allowed && !moderator {
		_ = api.database.QueryRowContext(request.Context(), `
			SELECT EXISTS(
				SELECT 1 FROM meeting_participants
				WHERE meeting_id=$1 AND tenant_id=$2 AND status='JOINED'
				  AND ((user_id=$3 AND $2=$4) OR (external_user_id=$3 AND external_tenant_id=$4))
			)`, meeting.ID, meeting.TenantID, user.ID, user.TenantID).Scan(&allowed)
	}
	if !allowed && !moderator {
		errorJSON(writer, http.StatusForbidden, "PARTICIPANT_ACCESS_REQUIRED", "profile picture is only available to meeting participants")
		return
	}
	api.streamProfileAvatar(writer, request, sourceTenantID, sourceUserID)
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
	if err = api.database.QueryRowContext(request.Context(), `SELECT COALESCE(user_id::text,external_user_id::text,''),role,status FROM meeting_participants WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3`, participantID, meeting.ID, user.TenantID).Scan(&targetUserID, &targetRole, &targetStatus); err != nil {
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
	if meeting.TenantID != "" && meeting.TenantID != user.TenantID {
		return false
	}
	if api.hasPermission(request.Context(), user, "meeting.moderate") || meeting.HostID == user.ID {
		return true
	}
	var exists bool
	_ = api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM meeting_participants WHERE meeting_id=$1 AND user_id=$2 AND tenant_id=$3 AND role='CO_HOST' AND status='JOINED')`, meeting.ID, user.ID, user.TenantID).Scan(&exists)
	return exists
}
