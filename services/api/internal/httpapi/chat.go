package httpapi

import (
	"database/sql"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type chatConversation struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Name        string    `json:"name"`
	CreatedAt   time.Time `json:"createdAt"`
	MemberCount int       `json:"memberCount"`
	UnreadCount int       `json:"unreadCount"`
	OnlineCount int       `json:"onlineCount"`
}

type chatMessage struct {
	ID             string           `json:"id"`
	ConversationID string           `json:"conversationId"`
	Body           string           `json:"body"`
	SenderID       string           `json:"senderId"`
	SenderName     string           `json:"senderName"`
	CreatedAt      time.Time        `json:"createdAt"`
	ParentID       *string          `json:"parentId,omitempty"`
	EditedAt       *time.Time       `json:"editedAt,omitempty"`
	DeletedAt      *time.Time       `json:"deletedAt,omitempty"`
	PinnedAt       *time.Time       `json:"pinnedAt,omitempty"`
	ReactionCount  int              `json:"reactionCount"`
	Attachments    []chatAttachment `json:"attachments,omitempty"`
}

func (api *API) chatConversations(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	switch request.Method {
	case http.MethodGet:
		rows, err := api.database.QueryContext(request.Context(), `
			SELECT c.id,c.type::text,
			  CASE WHEN c.type='DIRECT' THEN COALESCE(MAX(u2.display_name) FILTER (WHERE m2.user_id<>$2),'Direct message') ELSE COALESCE(c.name,'') END,
			  c.created_at,COUNT(DISTINCT m2.user_id),
			  COUNT(DISTINCT msg.id) FILTER (WHERE msg.created_at>GREATEST(COALESCE(m.last_read_at,'epoch'::timestamptz),COALESCE(m.cleared_at,'epoch'::timestamptz))),
			  COUNT(DISTINCT m2.user_id) FILTER (WHERE m2.last_seen_at>NOW()-INTERVAL '60 seconds')
			FROM chat_conversations c
			JOIN chat_members m ON m.conversation_id=c.id AND m.tenant_id=c.tenant_id AND m.user_id=$2
			LEFT JOIN chat_members m2 ON m2.conversation_id=c.id AND m2.tenant_id=c.tenant_id
			LEFT JOIN users u2 ON u2.id=m2.user_id AND u2.tenant_id=m2.tenant_id
			LEFT JOIN chat_messages msg ON msg.conversation_id=c.id AND msg.tenant_id=c.tenant_id AND msg.sender_id<>$2
			WHERE c.tenant_id=$1 AND (m.hidden_at IS NULL OR c.updated_at>m.hidden_at)
			GROUP BY c.id ORDER BY c.updated_at DESC,c.created_at DESC`, actor.TenantID, actor.ID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load conversations")
			return
		}
		defer rows.Close()
		items := make([]chatConversation, 0)
		for rows.Next() {
			var item chatConversation
			if err := rows.Scan(&item.ID, &item.Type, &item.Name, &item.CreatedAt, &item.MemberCount, &item.UnreadCount, &item.OnlineCount); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load conversations")
				return
			}
			items = append(items, item)
		}
		if err := rows.Err(); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load conversations")
			return
		}
		respondJSON(writer, 200, map[string]any{"conversations": items})
	case http.MethodPost:
		var input struct {
			Type      string   `json:"type"`
			Name      string   `json:"name"`
			MemberIDs []string `json:"memberIds"`
		}
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Type = strings.ToUpper(strings.TrimSpace(input.Type))
		input.Name = strings.TrimSpace(input.Name)
		if input.Type != "CHANNEL" && input.Type != "DIRECT" {
			errorJSON(writer, 400, "INVALID_INPUT", "type must be CHANNEL or DIRECT")
			return
		}
		if input.Type == "CHANNEL" && (len(input.Name) < 2 || len(input.Name) > 120) {
			errorJSON(writer, 400, "INVALID_INPUT", "channel name must be between 2 and 120 characters")
			return
		}
		members := uniqueIDs(append(input.MemberIDs, actor.ID))
		if input.Type == "DIRECT" && len(members) != 2 {
			errorJSON(writer, 400, "INVALID_INPUT", "direct conversations require exactly one other member")
			return
		}
		if len(members) > 100 {
			errorJSON(writer, 400, "INVALID_INPUT", "a conversation cannot have more than 100 members")
			return
		}
		if input.Type == "DIRECT" {
			otherID := ""
			for _, memberID := range members {
				if memberID != actor.ID {
					otherID = memberID
					break
				}
			}
			var otherName string
			if err := api.database.QueryRowContext(request.Context(), `SELECT display_name FROM users WHERE id=$1 AND tenant_id=$2 AND status='ACTIVE'`, otherID, actor.TenantID).Scan(&otherName); err != nil {
				if errors.Is(err, sql.ErrNoRows) {
					errorJSON(writer, 400, "INVALID_INPUT", "selected workspace user is not active")
					return
				}
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not validate workspace user")
				return
			}
			var existing chatConversation
			err := api.database.QueryRowContext(request.Context(), `
				SELECT c.id,c.type::text,$4,c.created_at,2,0,
				  COUNT(DISTINCT member.user_id) FILTER (WHERE member.last_seen_at>NOW()-INTERVAL '60 seconds')
				FROM chat_conversations c
				JOIN chat_members mine ON mine.conversation_id=c.id AND mine.tenant_id=c.tenant_id AND mine.user_id=$2
				JOIN chat_members theirs ON theirs.conversation_id=c.id AND theirs.tenant_id=c.tenant_id AND theirs.user_id=$3
				LEFT JOIN chat_members member ON member.conversation_id=c.id AND member.tenant_id=c.tenant_id
				WHERE c.tenant_id=$1 AND c.type='DIRECT'
				  AND (SELECT COUNT(*) FROM chat_members all_members WHERE all_members.conversation_id=c.id AND all_members.tenant_id=c.tenant_id)=2
				GROUP BY c.id LIMIT 1`, actor.TenantID, actor.ID, otherID, otherName).Scan(&existing.ID, &existing.Type, &existing.Name, &existing.CreatedAt, &existing.MemberCount, &existing.UnreadCount, &existing.OnlineCount)
			if err == nil {
				respondJSON(writer, 200, map[string]any{"conversation": existing, "existing": true})
				return
			}
			if !errors.Is(err, sql.ErrNoRows) {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not find direct conversation")
				return
			}
			input.Name = otherName
		}
		tx, err := api.database.BeginTx(request.Context(), nil)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create conversation")
			return
		}
		defer tx.Rollback()
		var item chatConversation
		databaseName := input.Name
		if input.Type == "DIRECT" {
			databaseName = ""
		}
		if err = tx.QueryRowContext(request.Context(), `INSERT INTO chat_conversations (tenant_id,type,name,created_by) VALUES ($1,$2,NULLIF($3,''),$4) RETURNING id,type::text,COALESCE(name,''),created_at`, actor.TenantID, input.Type, databaseName, actor.ID).Scan(&item.ID, &item.Type, &item.Name, &item.CreatedAt); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create conversation")
			return
		}
		for _, memberID := range members {
			var result sql.Result
			if result, err = tx.ExecContext(request.Context(), `INSERT INTO chat_members (conversation_id,tenant_id,user_id) SELECT $1,$2,id FROM users WHERE id=$3 AND tenant_id=$2 AND status='ACTIVE'`, item.ID, actor.TenantID, memberID); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not add conversation member")
				return
			}
			if affected, affectedErr := result.RowsAffected(); affectedErr != nil || affected != 1 {
				errorJSON(writer, 400, "INVALID_INPUT", "every conversation member must be an active workspace user")
				return
			}
		}
		if input.Type == "DIRECT" {
			item.Name = input.Name
		}
		item.MemberCount = len(members)
		if err = tx.Commit(); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not create conversation")
			return
		}
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "chat.conversation.created", "conversation", item.ID, map[string]any{"type": item.Type})
		respondJSON(writer, 201, map[string]any{"conversation": item})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func (api *API) chatEvents(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID := request.PathValue("conversationID")
	if !api.isChatMember(request, conversationID, actor) {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		errorJSON(writer, 500, "UNSUPPORTED", "realtime streaming is unavailable")
		return
	}
	channel, unsubscribe := api.chat.subscribe(conversationID)
	defer unsubscribe()
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache")
	writer.Header().Set("Connection", "keep-alive")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write([]byte(": connected\n\n"))
	flusher.Flush()
	heartbeat := time.NewTicker(25 * time.Second)
	defer heartbeat.Stop()
	for {
		select {
		case <-request.Context().Done():
			return
		case <-heartbeat.C:
			_, _ = writer.Write([]byte(": keep-alive\n\n"))
			flusher.Flush()
		case event, open := <-channel:
			if !open {
				return
			}
			data, err := encodeChatEvent(event)
			if err != nil {
				continue
			}
			_, _ = writer.Write([]byte("event: " + event.Type + "\ndata: "))
			_, _ = writer.Write(data)
			_, _ = writer.Write([]byte("\n\n"))
			flusher.Flush()
		}
	}
}

func (api *API) chatConversationClear(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID := request.PathValue("conversationID")
	result, err := api.database.ExecContext(request.Context(), `UPDATE chat_members SET cleared_at=NOW(),last_read_at=NOW() WHERE conversation_id=$1 AND tenant_id=$2 AND user_id=$3`, conversationID, actor.TenantID, actor.ID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not clear conversation")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "chat.conversation.cleared", "conversation", conversationID, map[string]any{"scope": "current_user"})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) chatConversationDelete(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID := request.PathValue("conversationID")
	result, err := api.database.ExecContext(request.Context(), `UPDATE chat_members SET cleared_at=NOW(),hidden_at=NOW(),last_read_at=NOW() WHERE conversation_id=$1 AND tenant_id=$2 AND user_id=$3`, conversationID, actor.TenantID, actor.ID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not delete conversation")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "chat.conversation.deleted", "conversation", conversationID, map[string]any{"scope": "current_user", "mode": "soft_delete"})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) chatRead(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID := request.PathValue("conversationID")
	if _, err := api.database.ExecContext(request.Context(), `UPDATE chat_members SET last_read_at=NOW(),last_seen_at=NOW() WHERE conversation_id=$1 AND tenant_id=$2 AND user_id=$3`, conversationID, actor.TenantID, actor.ID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update read state")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) chatPresence(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID := request.PathValue("conversationID")
	if _, err := api.database.ExecContext(request.Context(), `UPDATE chat_members SET last_seen_at=NOW() WHERE conversation_id=$1 AND tenant_id=$2 AND user_id=$3`, conversationID, actor.TenantID, actor.ID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update presence")
		return
	}
	api.chat.publish(conversationID, "presence", map[string]any{"userId": actor.ID, "displayName": actor.DisplayName, "online": true})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) chatSearch(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 120 {
		errorJSON(writer, 400, "INVALID_INPUT", "search query must be between 2 and 120 characters")
		return
	}
	conversationID := strings.TrimSpace(request.URL.Query().Get("conversationId"))
	args := []any{actor.TenantID, actor.ID, "%" + query + "%"}
	filter := ""
	if conversationID != "" {
		filter = " AND m.conversation_id=$4"
		args = append(args, conversationID)
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT m.id,m.conversation_id,CASE WHEN m.deleted_at IS NULL THEN m.body ELSE '[message deleted]' END,m.sender_id,u.display_name,m.created_at,m.parent_message_id,m.edited_at,m.deleted_at,m.pinned_at,COUNT(r.message_id) FROM chat_messages m JOIN chat_members cm ON cm.conversation_id=m.conversation_id AND cm.tenant_id=m.tenant_id AND cm.user_id=$2 JOIN users u ON u.id=m.sender_id AND u.tenant_id=m.tenant_id LEFT JOIN chat_reactions r ON r.message_id=m.id AND r.tenant_id=m.tenant_id WHERE m.tenant_id=$1 AND m.created_at>COALESCE(cm.cleared_at,'epoch'::timestamptz) AND m.body ILIKE $3`+filter+` GROUP BY m.id,u.display_name ORDER BY m.created_at DESC LIMIT 50`, args...)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not search messages")
		return
	}
	defer rows.Close()
	items := make([]chatMessage, 0)
	for rows.Next() {
		var item chatMessage
		if err := rows.Scan(&item.ID, &item.ConversationID, &item.Body, &item.SenderID, &item.SenderName, &item.CreatedAt, &item.ParentID, &item.EditedAt, &item.DeletedAt, &item.PinnedAt, &item.ReactionCount); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not search messages")
			return
		}
		items = append(items, item)
	}
	respondJSON(writer, 200, map[string]any{"messages": items})
}

func (api *API) isChatMember(request *http.Request, conversationID string, actor currentUser) bool {
	var member bool
	return api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE conversation_id=$1 AND tenant_id=$2 AND user_id=$3)`, conversationID, actor.TenantID, actor.ID).Scan(&member) == nil && member
}

func (api *API) chatMessages(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID := request.PathValue("conversationID")
	var member bool
	if err := api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM chat_members WHERE conversation_id=$1 AND tenant_id=$2 AND user_id=$3)`, conversationID, actor.TenantID, actor.ID).Scan(&member); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not authorize conversation")
		return
	}
	if !member {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	switch request.Method {
	case http.MethodGet:
		limit := 50
		if value, err := strconv.Atoi(request.URL.Query().Get("limit")); err == nil && value > 0 {
			if value > 100 {
				value = 100
			}
			limit = value
		}
		rows, err := api.database.QueryContext(request.Context(), `SELECT m.id,m.conversation_id,CASE WHEN m.deleted_at IS NULL THEN m.body ELSE '[message deleted]' END,m.sender_id,u.display_name,m.created_at,m.parent_message_id,m.edited_at,m.deleted_at,m.pinned_at,COUNT(r.message_id) FROM chat_messages m JOIN chat_members viewer ON viewer.conversation_id=m.conversation_id AND viewer.tenant_id=m.tenant_id AND viewer.user_id=$4 JOIN users u ON u.id=m.sender_id AND u.tenant_id=m.tenant_id LEFT JOIN chat_reactions r ON r.message_id=m.id AND r.tenant_id=m.tenant_id WHERE m.conversation_id=$1 AND m.tenant_id=$2 AND m.created_at>COALESCE(viewer.cleared_at,'epoch'::timestamptz) GROUP BY m.id,u.display_name ORDER BY m.created_at DESC,m.id DESC LIMIT $3`, conversationID, actor.TenantID, limit, actor.ID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load messages")
			return
		}
		defer rows.Close()
		items := make([]chatMessage, 0)
		for rows.Next() {
			var item chatMessage
			if err := rows.Scan(&item.ID, &item.ConversationID, &item.Body, &item.SenderID, &item.SenderName, &item.CreatedAt, &item.ParentID, &item.EditedAt, &item.DeletedAt, &item.PinnedAt, &item.ReactionCount); err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not load messages")
				return
			}
			item.Attachments, _ = api.loadChatAttachments(request, actor.TenantID, item.ID)
			items = append(items, item)
		}
		respondJSON(writer, 200, map[string]any{"messages": items})
	case http.MethodPost:
		var input struct {
			Body     string `json:"body"`
			ParentID string `json:"parentId"`
		}
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Body = strings.TrimSpace(input.Body)
		if len(input.Body) < 1 || len(input.Body) > 4000 {
			errorJSON(writer, 400, "INVALID_INPUT", "message body must be between 1 and 4000 characters")
			return
		}
		if parentID := strings.TrimSpace(input.ParentID); parentID != "" {
			var validParent bool
			if err := api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM chat_messages WHERE id=$1 AND conversation_id=$2 AND tenant_id=$3)`, parentID, conversationID, actor.TenantID).Scan(&validParent); err != nil || !validParent {
				errorJSON(writer, 400, "INVALID_INPUT", "parent message was not found in this conversation")
				return
			}
		}
		var item chatMessage
		err := api.database.QueryRowContext(request.Context(), `INSERT INTO chat_messages (tenant_id,conversation_id,sender_id,body,parent_message_id) VALUES ($1,$2,$3,$4,NULLIF($5,'')::uuid) RETURNING id,conversation_id,body,sender_id,created_at,parent_message_id`, actor.TenantID, conversationID, actor.ID, input.Body, strings.TrimSpace(input.ParentID)).Scan(&item.ID, &item.ConversationID, &item.Body, &item.SenderID, &item.CreatedAt, &item.ParentID)
		if errors.Is(err, sql.ErrNoRows) || err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not send message")
			return
		}
		item.SenderName = actor.DisplayName
		_, _ = api.database.ExecContext(request.Context(), `UPDATE chat_conversations SET updated_at=NOW() WHERE id=$1 AND tenant_id=$2`, conversationID, actor.TenantID)
		if item.ParentID != nil {
			var recipient string
			if api.database.QueryRowContext(request.Context(), `SELECT sender_id FROM chat_messages WHERE id=$1 AND tenant_id=$2`, *item.ParentID, actor.TenantID).Scan(&recipient) == nil {
				api.createNotification(request, actor.TenantID, recipient, actor.ID, "CHAT_REPLY", conversationID, item.ID, map[string]any{"preview": item.Body})
			}
		}
		for _, username := range mentionUsernames(item.Body) {
			var recipient string
			if api.database.QueryRowContext(request.Context(), `SELECT id FROM users WHERE tenant_id=$1 AND username=$2 AND status='ACTIVE'`, actor.TenantID, username).Scan(&recipient) == nil {
				api.createNotification(request, actor.TenantID, recipient, actor.ID, "CHAT_MENTION", conversationID, item.ID, map[string]any{"preview": item.Body, "username": username})
			}
		}
		api.chat.publish(conversationID, "message", item)
		respondJSON(writer, 201, map[string]any{"message": item})
	case http.MethodPatch, http.MethodDelete:
		messageID := request.PathValue("messageID")
		var owner string
		if err := api.database.QueryRowContext(request.Context(), `SELECT sender_id FROM chat_messages WHERE id=$1 AND conversation_id=$2 AND tenant_id=$3 AND deleted_at IS NULL`, messageID, conversationID, actor.TenantID).Scan(&owner); err != nil {
			errorJSON(writer, 404, "NOT_FOUND", "message not found")
			return
		}
		if owner != actor.ID {
			errorJSON(writer, 403, "FORBIDDEN", "only the message author can change this message")
			return
		}
		if request.Method == http.MethodDelete {
			_, err := api.database.ExecContext(request.Context(), `UPDATE chat_messages SET deleted_at=NOW(),body='[message deleted]' WHERE id=$1 AND tenant_id=$2`, messageID, actor.TenantID)
			if err != nil {
				errorJSON(writer, 500, "INTERNAL_ERROR", "could not delete message")
				return
			}
			api.chat.publish(conversationID, "message", map[string]any{"id": messageID, "deleted": true})
			writer.WriteHeader(http.StatusNoContent)
			return
		}
		var input struct {
			Body string `json:"body"`
		}
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Body = strings.TrimSpace(input.Body)
		if len(input.Body) < 1 || len(input.Body) > 4000 {
			errorJSON(writer, 400, "INVALID_INPUT", "message body must be between 1 and 4000 characters")
			return
		}
		var item chatMessage
		err := api.database.QueryRowContext(request.Context(), `UPDATE chat_messages SET body=$1,edited_at=NOW() WHERE id=$2 AND tenant_id=$3 RETURNING id,conversation_id,body,sender_id,created_at,parent_message_id,edited_at,deleted_at,pinned_at`, input.Body, messageID, actor.TenantID).Scan(&item.ID, &item.ConversationID, &item.Body, &item.SenderID, &item.CreatedAt, &item.ParentID, &item.EditedAt, &item.DeletedAt, &item.PinnedAt)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not edit message")
			return
		}
		item.SenderID = actor.ID
		item.SenderName = actor.DisplayName
		api.chat.publish(conversationID, "message", item)
		respondJSON(writer, 200, map[string]any{"message": item})
	default:
		writer.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func uniqueIDs(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value != "" && !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func (api *API) chatReaction(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID, messageID := request.PathValue("conversationID"), request.PathValue("messageID")
	if !api.isChatMember(request, conversationID, actor) {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	var input struct {
		Emoji string `json:"emoji"`
	}
	if request.Method == http.MethodPost {
		if err := decodeJSON(writer, request, &input); err != nil {
			errorJSON(writer, 400, "INVALID_INPUT", err.Error())
			return
		}
		input.Emoji = strings.TrimSpace(input.Emoji)
		if !contains([]string{"👍", "❤️", "😂", "🎉", "😮", "😢"}, input.Emoji) {
			errorJSON(writer, 400, "INVALID_INPUT", "unsupported reaction")
			return
		}
		_, err := api.database.ExecContext(request.Context(), `INSERT INTO chat_reactions (message_id,tenant_id,user_id,emoji) SELECT $1,$2,$3,$4 WHERE EXISTS (SELECT 1 FROM chat_messages WHERE id=$1 AND conversation_id=$5 AND tenant_id=$2) ON CONFLICT DO NOTHING`, messageID, actor.TenantID, actor.ID, input.Emoji, conversationID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not add reaction")
			return
		}
		var recipient string
		if api.database.QueryRowContext(request.Context(), `SELECT sender_id FROM chat_messages WHERE id=$1 AND tenant_id=$2`, messageID, actor.TenantID).Scan(&recipient) == nil {
			api.createNotification(request, actor.TenantID, recipient, actor.ID, "CHAT_REACTION", conversationID, messageID, map[string]any{"emoji": input.Emoji})
		}
		api.chat.publish(conversationID, "reaction", map[string]any{"messageId": messageID, "emoji": input.Emoji, "userId": actor.ID, "active": true})
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.Emoji = strings.TrimSpace(input.Emoji)
	if input.Emoji == "" {
		errorJSON(writer, 400, "INVALID_INPUT", "emoji is required")
		return
	}
	_, err := api.database.ExecContext(request.Context(), `DELETE FROM chat_reactions WHERE message_id=$1 AND tenant_id=$2 AND user_id=$3 AND emoji=$4`, messageID, actor.TenantID, actor.ID, input.Emoji)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not remove reaction")
		return
	}
	api.chat.publish(conversationID, "reaction", map[string]any{"messageId": messageID, "emoji": input.Emoji, "userId": actor.ID, "active": false})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) chatPin(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	conversationID, messageID := request.PathValue("conversationID"), request.PathValue("messageID")
	if !api.isChatMember(request, conversationID, actor) {
		errorJSON(writer, 404, "NOT_FOUND", "conversation not found")
		return
	}
	query := `UPDATE chat_messages SET pinned_at=NOW() WHERE id=$1 AND conversation_id=$2 AND tenant_id=$3`
	if request.Method == http.MethodDelete {
		query = `UPDATE chat_messages SET pinned_at=NULL WHERE id=$1 AND conversation_id=$2 AND tenant_id=$3`
	}
	result, err := api.database.ExecContext(request.Context(), query, messageID, conversationID, actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update pin")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		errorJSON(writer, 404, "NOT_FOUND", "message not found")
		return
	}
	api.chat.publish(conversationID, "pin", map[string]any{"messageId": messageID, "pinned": request.Method == http.MethodPost})
	writer.WriteHeader(http.StatusNoContent)
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func mentionUsernames(body string) []string {
	result := make([]string, 0)
	seen := map[string]bool{}
	for _, token := range strings.Fields(body) {
		if len(token) > 1 && strings.HasPrefix(token, "@") {
			value := strings.Trim(strings.TrimPrefix(token, "@"), ".,!?;:")
			if value != "" && !seen[value] {
				seen[value] = true
				result = append(result, value)
			}
		}
	}
	return result
}
