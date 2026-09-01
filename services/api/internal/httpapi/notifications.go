package httpapi

import (
	"encoding/json"
	"net/http"
)

type notification struct {
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	ConversationID string         `json:"conversationId,omitempty"`
	MessageID      string         `json:"messageId,omitempty"`
	ActorName      string         `json:"actorName,omitempty"`
	Payload        map[string]any `json:"payload"`
	ReadAt         *string        `json:"readAt,omitempty"`
	CreatedAt      string         `json:"createdAt"`
}

func (api *API) notifications(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	rows, err := api.database.QueryContext(request.Context(), `SELECT n.id,n.type,COALESCE(n.conversation_id::text,''),COALESCE(n.message_id::text,''),COALESCE(u.display_name,''),n.payload,COALESCE(n.read_at::text,''),n.created_at::text FROM notifications n LEFT JOIN users u ON u.id=n.actor_id AND u.tenant_id=n.tenant_id WHERE n.tenant_id=$1 AND n.recipient_id=$2 AND ($3='false' OR n.read_at IS NULL) ORDER BY n.created_at DESC LIMIT 100`, actor.TenantID, actor.ID, request.URL.Query().Get("unreadOnly"))
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load notifications")
		return
	}
	defer rows.Close()
	items := make([]notification, 0)
	for rows.Next() {
		var item notification
		var payload []byte
		if err := rows.Scan(&item.ID, &item.Type, &item.ConversationID, &item.MessageID, &item.ActorName, &payload, &item.ReadAt, &item.CreatedAt); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load notifications")
			return
		}
		_ = json.Unmarshal(payload, &item.Payload)
		if item.Payload == nil {
			item.Payload = map[string]any{}
		}
		if item.ReadAt != nil && *item.ReadAt == "" {
			item.ReadAt = nil
		}
		items = append(items, item)
	}
	var unread int
	_ = api.database.QueryRowContext(request.Context(), `SELECT COUNT(*) FROM notifications WHERE tenant_id=$1 AND recipient_id=$2 AND read_at IS NULL`, actor.TenantID, actor.ID).Scan(&unread)
	respondJSON(writer, 200, map[string]any{"notifications": items, "unreadCount": unread})
}

func (api *API) notificationRead(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	id := request.PathValue("notificationID")
	if id == "" {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE notifications SET read_at=NOW() WHERE tenant_id=$1 AND recipient_id=$2 AND read_at IS NULL`, actor.TenantID, actor.ID)
	} else {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE notifications SET read_at=NOW() WHERE id=$1 AND tenant_id=$2 AND recipient_id=$3`, id, actor.TenantID, actor.ID)
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) createNotification(request *http.Request, tenantID, recipientID, actorID, notificationType, conversationID, messageID string, payload map[string]any) {
	if recipientID == "" || recipientID == actorID {
		return
	}
	encoded, _ := json.Marshal(payload)
	_, _ = api.database.ExecContext(request.Context(), `INSERT INTO notifications (tenant_id,recipient_id,actor_id,type,conversation_id,message_id,payload) VALUES ($1,$2,NULLIF($3,'')::uuid,$4,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7::jsonb)`, tenantID, recipientID, actorID, notificationType, conversationID, messageID, string(encoded))
}
