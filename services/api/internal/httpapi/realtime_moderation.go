package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"os"

	"github.com/livekit/protocol/livekit"
	lksdk "github.com/livekit/server-sdk-go/v2"
)

func liveKitRoomClient() (*lksdk.RoomServiceClient, error) {
	key, secret := os.Getenv("LIVEKIT_API_KEY"), os.Getenv("LIVEKIT_API_SECRET")
	if key == "" || secret == "" {
		return nil, fmt.Errorf("livekit credentials missing")
	}
	return lksdk.NewRoomServiceClient(envOr("LIVEKIT_API_URL", "http://localhost:7880"), key, secret), nil
}

func (api *API) syncParticipantAction(ctx context.Context, meeting meetingResponse, userID, participantID, action string) error {
	client, err := liveKitRoomClient()
	if err != nil {
		return err
	}
	identity := userID
	response, listErr := client.ListParticipants(ctx, &livekit.ListParticipantsRequest{Room: meeting.RoomName})
	if listErr == nil {
		for _, participant := range response.Participants {
			if logicalRealtimeIdentity(participant.Identity) == userID {
				identity = participant.Identity
				break
			}
		}
	}
	switch action {
	case "remove":
		_, err = client.RemoveParticipant(ctx, &livekit.RoomParticipantIdentity{Room: meeting.RoomName, Identity: identity})
	case "promote":
		_, err = client.UpdateParticipant(ctx, &livekit.UpdateParticipantRequest{Room: meeting.RoomName, Identity: identity, Metadata: `{"role":"CO_HOST"}`, Permission: &livekit.ParticipantPermission{CanPublish: true, CanSubscribe: true, CanPublishData: true, CanUpdateMetadata: true}})
	case "mute":
		if listErr == nil {
			found := false
			for _, participant := range response.Participants {
				if participant.Identity == identity {
					for _, track := range participant.Tracks {
						if track.Type == livekit.TrackType_AUDIO {
							_, err = client.MutePublishedTrack(ctx, &livekit.MuteRoomTrackRequest{Room: meeting.RoomName, Identity: identity, TrackSid: track.Sid, Muted: true})
							if err != nil {
								return err
							}
							found = true
						}
					}
				}
			}
			if !found {
				return fmt.Errorf("audio track not found")
			}
		}
		err = listErr
	}
	return err
}

func (api *API) moderateMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err != nil {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if meeting.HostID != user.ID && !api.hasPermission(request.Context(), user, "meeting.moderate") {
		errorJSON(writer, 403, "HOST_REQUIRED", "only the host can change meeting policy")
		return
	}
	action := request.PathValue("action")
	switch action {
	case "lock":
		_, err = api.database.ExecContext(request.Context(), "UPDATE meetings SET locked_at=NOW(),updated_at=NOW() WHERE id=$1 AND tenant_id=$2", meeting.ID, user.TenantID)
	case "unlock":
		_, err = api.database.ExecContext(request.Context(), "UPDATE meetings SET locked_at=NULL,updated_at=NOW() WHERE id=$1 AND tenant_id=$2", meeting.ID, user.TenantID)
	case "end":
		_, err = api.database.ExecContext(request.Context(), "UPDATE meetings SET status='ENDED',ended_at=NOW(),updated_at=NOW() WHERE id=$1 AND tenant_id=$2", meeting.ID, user.TenantID)
		if err == nil {
			if client, clientErr := liveKitRoomClient(); clientErr == nil {
				_, _ = client.DeleteRoom(request.Context(), &livekit.DeleteRoomRequest{Room: meeting.RoomName})
			}
		}
	default:
		errorJSON(writer, 400, "INVALID_ACTION", "unsupported meeting action")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update meeting")
		return
	}
	api.auditModeration(request, user, "meeting."+action, meeting.ID)
	respondJSON(writer, 200, map[string]string{"status": "ok"})
}
