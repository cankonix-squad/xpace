package httpapi

import (
	"context"
	"database/sql"
	"net/http"
)

type meetingPolicy struct {
	GuestAccessEnabled bool `json:"guestAccessEnabled"`
	WaitingRoomDefault bool `json:"waitingRoomDefault"`
	RecordingEnabled   bool `json:"recordingEnabled"`
	ScreenShareEnabled bool `json:"screenShareEnabled"`
}

func (api *API) loadMeetingPolicy(ctx context.Context, tenantID string) (meetingPolicy, error) {
	policy := meetingPolicy{GuestAccessEnabled: true, WaitingRoomDefault: true, RecordingEnabled: true, ScreenShareEnabled: true}
	err := api.database.QueryRowContext(ctx, `SELECT guest_access_enabled,waiting_room_default,recording_enabled,screen_share_enabled FROM tenant_meeting_policies WHERE tenant_id=$1`, tenantID).Scan(&policy.GuestAccessEnabled, &policy.WaitingRoomDefault, &policy.RecordingEnabled, &policy.ScreenShareEnabled)
	if err == sql.ErrNoRows {
		_, err = api.database.ExecContext(ctx, `INSERT INTO tenant_meeting_policies (tenant_id) VALUES ($1) ON CONFLICT (tenant_id) DO NOTHING`, tenantID)
	}
	return policy, err
}

func (api *API) adminMeetingPolicy(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	if request.Method == http.MethodGet {
		policy, err := api.loadMeetingPolicy(request.Context(), actor.TenantID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
			return
		}
		respondJSON(writer, http.StatusOK, map[string]any{"policy": policy})
		return
	}
	var policy meetingPolicy
	if err := decodeJSON(writer, request, &policy); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	_, err := api.database.ExecContext(request.Context(), `INSERT INTO tenant_meeting_policies (tenant_id,guest_access_enabled,waiting_room_default,recording_enabled,screen_share_enabled,updated_by) VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT (tenant_id) DO UPDATE SET guest_access_enabled=EXCLUDED.guest_access_enabled,waiting_room_default=EXCLUDED.waiting_room_default,recording_enabled=EXCLUDED.recording_enabled,screen_share_enabled=EXCLUDED.screen_share_enabled,updated_by=EXCLUDED.updated_by,updated_at=NOW()`, actor.TenantID, policy.GuestAccessEnabled, policy.WaitingRoomDefault, policy.RecordingEnabled, policy.ScreenShareEnabled, actor.ID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update meeting policy")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "meeting.policy.update", "tenant", actor.TenantID, map[string]any{"guestAccess": policy.GuestAccessEnabled, "waitingRoomDefault": policy.WaitingRoomDefault, "recording": policy.RecordingEnabled, "screenShare": policy.ScreenShareEnabled})
	respondJSON(writer, http.StatusOK, map[string]any{"policy": policy})
}
