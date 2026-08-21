package httpapi

import (
	"database/sql"
	"fmt"
	"net/http"
	"strings"
	"time"
)

type meetingResponse struct {
	ID                 string     `json:"id"`
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
	if !user.Role.canCreateMeeting() {
		errorJSON(writer, http.StatusForbidden, "INSUFFICIENT_ROLE", "guests cannot create meetings")
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
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,title,join_code,room_name,status,scheduled_at,waiting_room_enabled,host_id,created_at FROM meetings WHERE tenant_id=$1 AND status NOT IN ('CANCELLED') ORDER BY COALESCE(scheduled_at,created_at) DESC LIMIT 50`, user.TenantID)
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
	respondJSON(writer, 200, map[string]any{"meetings": items})
}

func (api *API) getMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	respondJSON(writer, 200, map[string]any{"meeting": meeting})
}

func (api *API) joinMeeting(writer http.ResponseWriter, request *http.Request, user currentUser) {
	meeting, err := api.findMeeting(request, user)
	if err == sql.ErrNoRows {
		errorJSON(writer, 404, "MEETING_NOT_FOUND", "meeting code is invalid")
		return
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load meeting")
		return
	}
	policy, err := api.loadMeetingPolicy(request.Context(), user.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting policy")
		return
	}
	if user.Role == roleGuest && !policy.GuestAccessEnabled {
		errorJSON(writer, http.StatusForbidden, "GUEST_ACCESS_DISABLED", "guest access is disabled for this workspace")
		return
	}
	if meeting.Status == "ENDED" || meeting.Status == "CANCELLED" {
		errorJSON(writer, 409, "MEETING_UNAVAILABLE", "meeting is no longer available")
		return
	}
	var locked bool
	if err = api.database.QueryRowContext(request.Context(), "SELECT locked_at IS NOT NULL FROM meetings WHERE id=$1 AND tenant_id=$2", meeting.ID, user.TenantID).Scan(&locked); err != nil {
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
	} else if user.Role == roleGuest {
		participantRole = "GUEST"
	}
	err = api.database.QueryRowContext(request.Context(), `INSERT INTO meeting_participants (meeting_id,user_id,tenant_id,display_name,role,status,joined_at) VALUES ($1,$2,$3,$4,$5,$6::participant_status,CASE WHEN $6::participant_status='JOINED'::participant_status THEN NOW() ELSE NULL END) RETURNING id`, meeting.ID, user.ID, user.TenantID, user.DisplayName, participantRole, status).Scan(&participantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not join meeting")
		return
	}
	if meeting.HostID == user.ID && meeting.Status != "ACTIVE" {
		if _, err = api.database.ExecContext(request.Context(), `UPDATE meetings SET status='ACTIVE',started_at=COALESCE(started_at,NOW()),updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND status IN ('SCHEDULED','WAITING')`, meeting.ID, user.TenantID); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not start meeting")
			return
		}
		meeting.Status = "ACTIVE"
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "meeting.join", "meeting", meeting.ID, map[string]any{"participantId": participantID, "status": status, "role": participantRole})
	respondJSON(writer, 201, map[string]any{"meeting": meeting, "participant": map[string]string{"id": participantID, "status": status}})
}

func (api *API) findMeeting(request *http.Request, user currentUser) (meetingResponse, error) {
	var m meetingResponse
	code := strings.ToUpper(strings.TrimSpace(request.PathValue("joinCode")))
	err := api.database.QueryRowContext(request.Context(), `SELECT id,title,join_code,room_name,status,scheduled_at,waiting_room_enabled,host_id,created_at FROM meetings WHERE tenant_id=$1 AND join_code=$2`, user.TenantID, code).Scan(&m.ID, &m.Title, &m.JoinCode, &m.RoomName, &m.Status, &m.ScheduledAt, &m.WaitingRoomEnabled, &m.HostID, &m.CreatedAt)
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
