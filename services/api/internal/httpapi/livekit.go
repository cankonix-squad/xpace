package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"os"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
)

func (api *API) liveKitToken(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeetingForJoin(request)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	var participantID, status string
	err = api.database.QueryRowContext(request.Context(), `
		SELECT id,status FROM meeting_participants
		WHERE meeting_id=$1 AND tenant_id=$2
		  AND ((user_id=$3 AND $2=$4) OR (external_user_id=$3 AND external_tenant_id=$4))
		ORDER BY created_at DESC LIMIT 1`, meeting.ID, meeting.TenantID, user.ID, user.TenantID).Scan(&participantID, &status)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusForbidden, "JOIN_REQUIRED", "complete the pre-join flow first")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not verify participant")
		return
	}
	if status != "JOINED" && status != "DISCONNECTED" {
		errorJSON(writer, http.StatusForbidden, "WAITING_FOR_HOST", "the host has not admitted you yet")
		return
	}
	if status == "DISCONNECTED" {
		result, updateErr := api.database.ExecContext(request.Context(), `UPDATE meeting_participants SET status='JOINED',joined_at=NOW(),left_at=NULL WHERE id=$1 AND meeting_id=$2 AND tenant_id=$3 AND status='DISCONNECTED'`, participantID, meeting.ID, meeting.TenantID)
		if updateErr != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not restore participant session")
			return
		}
		if updated, _ := result.RowsAffected(); updated == 0 {
			errorJSON(writer, http.StatusConflict, "REJOIN_REQUIRED", "return to device preview to rejoin the meeting")
			return
		}
	}
	apiKey, apiSecret := os.Getenv("LIVEKIT_API_KEY"), os.Getenv("LIVEKIT_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		errorJSON(writer, http.StatusServiceUnavailable, "REALTIME_UNAVAILABLE", "realtime service is not configured")
		return
	}
	policy, err := api.loadMeetingPolicy(request.Context(), meeting.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
		return
	}
	grant := &auth.VideoGrant{RoomJoin: true, Room: meeting.RoomName, CanPublish: boolPointer(true), CanSubscribe: boolPointer(true), CanPublishData: boolPointer(true)}
	sources := []livekit.TrackSource{livekit.TrackSource_CAMERA, livekit.TrackSource_MICROPHONE}
	if policy.ScreenShareEnabled {
		sources = append(sources, livekit.TrackSource_SCREEN_SHARE, livekit.TrackSource_SCREEN_SHARE_AUDIO)
	}
	grant.SetCanPublishSources(sources)
	participantMetadata := map[string]string{}
	var hasAvatar bool
	if queryErr := api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM user_profiles WHERE user_id=$1 AND tenant_id=$2 AND NULLIF(BTRIM(avatar_url),'') IS NOT NULL)`, user.ID, user.TenantID).Scan(&hasAvatar); queryErr == nil && hasAvatar {
		participantMetadata["avatarUrl"] = "/api/v1/meetings/" + meeting.JoinCode + "/participants/" + participantID + "/avatar"
	}
	metadataJSON, _ := json.Marshal(participantMetadata)
	// Keep the realtime identity stable across leave/rejoin cycles. LiveKit
	// replaces an older connection that uses the same identity, which prevents
	// one account from appearing twice when a browser reconnects quickly.
	token := auth.NewAccessToken(apiKey, apiSecret).SetIdentity(user.ID).SetName(user.DisplayName).SetMetadata(string(metadataJSON)).SetValidFor(15 * time.Minute).SetVideoGrant(grant)
	jwt, err := token.ToJWT()
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not issue realtime token")
		return
	}
	metadata := map[string]any{"participantId": participantID}
	if meeting.TenantID != user.TenantID {
		metadata["externalUserId"] = user.ID
		metadata["externalTenantId"] = user.TenantID
	}
	_ = api.writeAuditEvent(request.Context(), request, meeting.TenantID, user.ID, "realtime.token.issue", "meeting", meeting.ID, metadata)
	respondJSON(writer, http.StatusOK, map[string]any{"token": jwt, "serverUrl": envOr("LIVEKIT_WS_URL", "ws://localhost:7880"), "roomName": meeting.RoomName, "screenShareEnabled": policy.ScreenShareEnabled})
}

func boolPointer(value bool) *bool { return &value }
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
