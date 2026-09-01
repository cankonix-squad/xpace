package httpapi

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

type workspaceRoom struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Description    string    `json:"description"`
	Visibility     string    `json:"visibility"`
	Role           string    `json:"role"`
	ConversationID *string   `json:"conversationId,omitempty"`
	MemberCount    int       `json:"memberCount"`
	CreatedAt      time.Time `json:"createdAt"`
}
type roomActivity struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	ActorName string         `json:"actorName"`
	Payload   map[string]any `json:"payload"`
	CreatedAt time.Time      `json:"createdAt"`
}

func (api *API) isRoomMember(request *http.Request, actor currentUser, roomID string) bool {
	var member bool
	return api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM workspace_room_members WHERE room_id=$1 AND tenant_id=$2 AND user_id=$3)`, roomID, actor.TenantID, actor.ID).Scan(&member) == nil && member
}

func (api *API) rooms(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	switch request.Method {
	case http.MethodGet:
		rows, err := api.database.QueryContext(request.Context(), `SELECT r.id,r.name,r.description,r.visibility,m.role,r.conversation_id,(SELECT COUNT(*) FROM workspace_room_members x WHERE x.room_id=r.id AND x.tenant_id=r.tenant_id),r.created_at FROM workspace_rooms r JOIN workspace_room_members m ON m.room_id=r.id AND m.tenant_id=r.tenant_id AND m.user_id=$2 WHERE r.tenant_id=$1 ORDER BY r.updated_at DESC`, actor.TenantID, actor.ID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load rooms")
			return
		}
		defer rows.Close()
		items := make([]workspaceRoom, 0)
		for rows.Next() {
			var item workspaceRoom
			if err := rows.Scan(&item.ID, &item.Name, &item.Description, &item.Visibility, &item.Role, &item.ConversationID, &item.MemberCount, &item.CreatedAt); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load rooms")
				return
			}
			items = append(items, item)
		}
		respondJSON(writer, 200, map[string]any{"rooms": items})
	case http.MethodPost:
		var input struct {
			Name        string   `json:"name"`
			Description string   `json:"description"`
			Visibility  string   `json:"visibility"`
			MemberIDs   []string `json:"memberIds"`
		}
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Name = strings.TrimSpace(input.Name)
		input.Description = strings.TrimSpace(input.Description)
		input.Visibility = strings.ToUpper(strings.TrimSpace(input.Visibility))
		if len(input.Name) < 2 || len(input.Name) > 120 {
			errorJSON(writer, 400, "INVALID_INPUT", "room name must be between 2 and 120 characters")
			return
		}
		if input.Visibility == "" {
			input.Visibility = "PRIVATE"
		}
		if input.Visibility != "PRIVATE" && input.Visibility != "TENANT" {
			errorJSON(writer, 400, "INVALID_INPUT", "visibility is invalid")
			return
		}
		tx, err := api.database.BeginTx(request.Context(), nil)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create room")
			return
		}
		defer tx.Rollback()
		var conversationID string
		if err = tx.QueryRowContext(request.Context(), `INSERT INTO chat_conversations(tenant_id,type,name,created_by) VALUES($1,'CHANNEL',$2,$3) RETURNING id`, actor.TenantID, input.Name, actor.ID).Scan(&conversationID); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create room channel")
			return
		}
		var item workspaceRoom
		err = tx.QueryRowContext(request.Context(), `INSERT INTO workspace_rooms(tenant_id,name,description,created_by,conversation_id,visibility) VALUES($1,$2,$3,$4,$5,$6) RETURNING id,name,description,visibility,conversation_id,created_at`, actor.TenantID, input.Name, input.Description, actor.ID, conversationID, input.Visibility).Scan(&item.ID, &item.Name, &item.Description, &item.Visibility, &item.ConversationID, &item.CreatedAt)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create room")
			return
		}
		members := uniqueIDs(append(input.MemberIDs, actor.ID))
		for _, id := range members {
			role := "MEMBER"
			if id == actor.ID {
				role = "OWNER"
			}
			result, e := tx.ExecContext(request.Context(), `INSERT INTO workspace_room_members(room_id,tenant_id,user_id,role) SELECT $1,$2,id,$4 FROM users WHERE id=$3 AND tenant_id=$2 AND status='ACTIVE'`, item.ID, actor.TenantID, id, role)
			if e != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not add room member")
				return
			}
			count, _ := result.RowsAffected()
			if count > 0 {
				item.MemberCount++
				_, _ = tx.ExecContext(request.Context(), `INSERT INTO chat_members(conversation_id,tenant_id,user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, conversationID, actor.TenantID, id)
			}
		}
		_, _ = tx.ExecContext(request.Context(), `INSERT INTO workspace_room_activity(room_id,tenant_id,actor_id,type,payload) VALUES($1,$2,$3,'ROOM_CREATED',jsonb_build_object('name',$4::text))`, item.ID, actor.TenantID, actor.ID, item.Name)
		if err = tx.Commit(); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create room")
			return
		}
		item.Role = "OWNER"
		respondJSON(writer, 201, map[string]any{"room": item})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) roomDetail(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	roomID := request.PathValue("roomID")
	var role string
	if err := api.database.QueryRowContext(request.Context(), `SELECT role FROM workspace_room_members WHERE room_id=$1 AND tenant_id=$2 AND user_id=$3`, roomID, actor.TenantID, actor.ID).Scan(&role); err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "room not found")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT a.id,a.type,u.display_name,a.payload,a.created_at FROM workspace_room_activity a JOIN users u ON u.id=a.actor_id AND u.tenant_id=a.tenant_id WHERE a.room_id=$1 AND a.tenant_id=$2 ORDER BY a.created_at DESC LIMIT 100`, roomID, actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load activity")
		return
	}
	defer rows.Close()
	items := make([]roomActivity, 0)
	for rows.Next() {
		var item roomActivity
		var payload []byte
		if err := rows.Scan(&item.ID, &item.Type, &item.ActorName, &payload, &item.CreatedAt); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load activity")
			return
		}
		_ = json.Unmarshal(payload, &item.Payload)
		items = append(items, item)
	}
	respondJSON(writer, 200, map[string]any{"role": role, "activity": items})
}

func (api *API) roomMember(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	roomID, userID := request.PathValue("roomID"), request.PathValue("userID")
	var actorRole, conversationID string
	if err := api.database.QueryRowContext(request.Context(), `SELECT m.role,r.conversation_id FROM workspace_room_members m JOIN workspace_rooms r ON r.id=m.room_id AND r.tenant_id=m.tenant_id WHERE m.room_id=$1 AND m.tenant_id=$2 AND m.user_id=$3`, roomID, actor.TenantID, actor.ID).Scan(&actorRole, &conversationID); err != nil || actorRole != "OWNER" && actorRole != "ADMIN" {
		errorJSON(writer, 403, "FORBIDDEN", "room admin access is required")
		return
	}
	var input struct {
		Role string `json:"role"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Role = strings.ToUpper(strings.TrimSpace(input.Role))
	if input.Role != "ADMIN" && input.Role != "MEMBER" && input.Role != "GUEST" {
		errorJSON(writer, 400, "INVALID_INPUT", "role is invalid")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not add member")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(request.Context(), `INSERT INTO workspace_room_members(room_id,tenant_id,user_id,role) SELECT $1,$2,id,$4 FROM users WHERE id=$3 AND tenant_id=$2 AND status='ACTIVE' ON CONFLICT(room_id,user_id) DO UPDATE SET role=EXCLUDED.role`, roomID, actor.TenantID, userID, input.Role)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not add member")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, 404, "NOT_FOUND", "user not found")
		return
	}
	_, _ = tx.ExecContext(request.Context(), `INSERT INTO chat_members(conversation_id,tenant_id,user_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, conversationID, actor.TenantID, userID)
	_, _ = tx.ExecContext(request.Context(), `INSERT INTO workspace_room_activity(room_id,tenant_id,actor_id,type,payload) VALUES($1,$2,$3,'MEMBER_UPDATED',jsonb_build_object('userId',$4::text,'role',$5::text))`, roomID, actor.TenantID, actor.ID, userID, input.Role)
	if err = tx.Commit(); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not add member")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) roomMeeting(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	roomID := request.PathValue("roomID")
	var roomName, role string
	if err := api.database.QueryRowContext(request.Context(), `SELECT r.name,m.role FROM workspace_rooms r JOIN workspace_room_members m ON m.room_id=r.id AND m.tenant_id=r.tenant_id AND m.user_id=$3 WHERE r.id=$1 AND r.tenant_id=$2`, roomID, actor.TenantID, actor.ID).Scan(&roomName, &role); err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "room not found")
		return
	}
	if role == "GUEST" {
		errorJSON(writer, 403, "FORBIDDEN", "guests cannot start room meetings")
		return
	}
	if err := api.enforceTenantQuota(request.Context(), actor.TenantID, "meetings", 1); err != nil {
		if !respondEntitlementError(writer, err) {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
		}
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not start meeting")
		return
	}
	defer tx.Rollback()
	var meetingID, joinCode string
	for attempt := 0; attempt < 3; attempt++ {
		code, _ := meetingCode()
		token, _ := randomToken(12)
		joinCode = code
		err = tx.QueryRowContext(request.Context(), `INSERT INTO meetings(tenant_id,host_id,room_name,join_code,title,status,waiting_room_enabled) VALUES($1,$2,$3,$4,$5,'WAITING',TRUE) RETURNING id`, actor.TenantID, actor.ID, "xpace-"+strings.ToLower(token), joinCode, roomName+" meeting").Scan(&meetingID)
		if err == nil {
			break
		}
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not start meeting")
		return
	}
	_, err = tx.ExecContext(request.Context(), `INSERT INTO workspace_room_meetings(room_id,tenant_id,meeting_id) VALUES($1,$2,$3)`, roomID, actor.TenantID, meetingID)
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO workspace_room_activity(room_id,tenant_id,actor_id,type,payload) VALUES($1,$2,$3,'MEETING_CREATED',jsonb_build_object('meetingId',$4::text,'joinCode',$5::text))`, roomID, actor.TenantID, actor.ID, meetingID, joinCode)
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not start meeting")
		return
	}
	respondJSON(writer, 201, map[string]any{"meeting": map[string]string{"id": meetingID, "joinCode": joinCode}})
}
