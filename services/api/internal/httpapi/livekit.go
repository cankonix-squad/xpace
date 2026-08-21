package httpapi

import (
	"database/sql"
	"net/http"
	"os"
	"time"

	"github.com/livekit/protocol/auth"
	"github.com/livekit/protocol/livekit"
)

func (api *API) liveKitToken(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	var participantID, status string
	err = api.database.QueryRowContext(request.Context(), `SELECT id,status FROM meeting_participants WHERE meeting_id=$1 AND user_id=$2 AND tenant_id=$3 ORDER BY created_at DESC LIMIT 1`, meeting.ID, user.ID, user.TenantID).Scan(&participantID, &status)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusForbidden, "JOIN_REQUIRED", "complete the pre-join flow first")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not verify participant")
		return
	}
	if status != "JOINED" {
		errorJSON(writer, http.StatusForbidden, "WAITING_FOR_HOST", "the host has not admitted you yet")
		return
	}
	apiKey, apiSecret := os.Getenv("LIVEKIT_API_KEY"), os.Getenv("LIVEKIT_API_SECRET")
	if apiKey == "" || apiSecret == "" {
		errorJSON(writer, http.StatusServiceUnavailable, "REALTIME_UNAVAILABLE", "realtime service is not configured")
		return
	}
	policy, err := api.loadMeetingPolicy(request.Context(), user.TenantID)
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
	token := auth.NewAccessToken(apiKey, apiSecret).SetIdentity(user.ID + ":" + participantID).SetName(user.DisplayName).SetValidFor(15 * time.Minute).SetVideoGrant(grant)
	jwt, err := token.ToJWT()
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not issue realtime token")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "realtime.token.issue", "meeting", meeting.ID, map[string]any{"participantId": participantID})
	respondJSON(writer, http.StatusOK, map[string]any{"token": jwt, "serverUrl": envOr("LIVEKIT_WS_URL", "ws://localhost:7880"), "roomName": meeting.RoomName, "screenShareEnabled": policy.ScreenShareEnabled})
}

func boolPointer(value bool) *bool { return &value }
func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
