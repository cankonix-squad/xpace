package httpapi

import (
	"database/sql"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

type activityItem struct {
	ID          string    `json:"id"`
	Type        string    `json:"type"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	ActorName   string    `json:"actorName"`
	Href        string    `json:"href"`
	CreatedAt   time.Time `json:"createdAt"`
}

type activityQuery struct {
	typeName string
	sql      string
	href     func(string, string) string
	adminArg bool
}

func (api *API) workspaceActivity(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	limit := 12
	if raw := request.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 5 || parsed > 50 {
			errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "activity limit must be between 5 and 50")
			return
		}
		limit = parsed
	}
	before := time.Now().Add(time.Minute)
	if raw := strings.TrimSpace(request.URL.Query().Get("before")); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "activity cursor is invalid")
			return
		}
		before = parsed
	}
	queries := []activityQuery{
		{"meeting", `SELECT m.id::text,m.title,CASE m.status::text WHEN 'ACTIVE' THEN 'Meeting is live' WHEN 'ENDED' THEN 'Meeting ended' WHEN 'SCHEDULED' THEN 'Meeting scheduled' ELSE 'Meeting is ready' END,u.display_name,m.join_code,m.updated_at FROM meetings m JOIN users u ON u.id=m.host_id AND u.tenant_id=m.tenant_id WHERE m.tenant_id=$1 AND $2::uuid IS NOT NULL AND m.status!='CANCELLED' AND m.updated_at<$3 ORDER BY m.updated_at DESC LIMIT $4`, func(_ string, link string) string { return "/meet/" + url.PathEscape(link) + "/prejoin" }, false},
		{"chat", `SELECT message.id::text,COALESCE(conversation.name,'Direct message'),CASE WHEN message.deleted_at IS NULL THEN LEFT(message.body,180) ELSE 'Message deleted' END,sender.display_name,conversation.id::text,message.created_at FROM chat_messages message JOIN chat_conversations conversation ON conversation.id=message.conversation_id AND conversation.tenant_id=message.tenant_id JOIN chat_members viewer ON viewer.conversation_id=message.conversation_id AND viewer.tenant_id=message.tenant_id AND viewer.user_id=$2 JOIN users sender ON sender.id=message.sender_id AND sender.tenant_id=message.tenant_id WHERE message.tenant_id=$1 AND message.created_at>COALESCE(viewer.cleared_at,'epoch'::timestamptz) AND (viewer.hidden_at IS NULL OR conversation.updated_at>viewer.hidden_at) AND message.created_at<$3 ORDER BY message.created_at DESC LIMIT $4`, func(id, link string) string {
			return "/chat?conversationId=" + url.QueryEscape(link) + "&messageId=" + url.QueryEscape(id)
		}, false},
		{"room", `SELECT activity.id::text,room.name,INITCAP(REPLACE(activity.type,'_',' ')),member.display_name,room.id::text,activity.created_at FROM workspace_room_activity activity JOIN workspace_rooms room ON room.id=activity.room_id AND room.tenant_id=activity.tenant_id JOIN workspace_room_members viewer ON viewer.room_id=activity.room_id AND viewer.tenant_id=activity.tenant_id AND viewer.user_id=$2 JOIN users member ON member.id=activity.actor_id AND member.tenant_id=activity.tenant_id WHERE activity.tenant_id=$1 AND activity.created_at<$3 ORDER BY activity.created_at DESC LIMIT $4`, func(_ string, link string) string { return "/rooms?roomId=" + url.QueryEscape(link) }, false},
		{"drive", `SELECT DISTINCT node.id::text,node.name,INITCAP(LOWER(node.kind))||' updated',owner.display_name,node.name,node.updated_at FROM drive_nodes node JOIN users owner ON owner.id=node.owner_id AND owner.tenant_id=node.tenant_id LEFT JOIN drive_shares share ON share.node_id=node.id AND share.tenant_id=node.tenant_id AND share.user_id=$2 WHERE node.tenant_id=$1 AND node.deleted_at IS NULL AND (node.owner_id=$2 OR share.user_id=$2) AND node.updated_at<$3 ORDER BY node.updated_at DESC LIMIT $4`, func(_ string, link string) string { return "/drive?search=" + url.QueryEscape(link) }, false},
		{"calendar", `SELECT event.id::text,event.title,'Event · '||to_char(event.starts_at AT TIME ZONE event.timezone,'Mon DD, HH24:MI'),organizer.display_name,event.id::text,event.updated_at FROM calendar_events event JOIN users organizer ON organizer.id=event.organizer_id AND organizer.tenant_id=event.tenant_id LEFT JOIN calendar_event_attendees attendee ON attendee.event_id=event.id AND attendee.tenant_id=event.tenant_id AND attendee.user_id=$2 WHERE event.tenant_id=$1 AND (event.organizer_id=$2 OR attendee.user_id=$2) AND event.updated_at<$3 ORDER BY event.updated_at DESC LIMIT $4`, func(_ string, link string) string { return "/calendar?eventId=" + url.QueryEscape(link) }, false},
		{"recording", `SELECT recording.id::text,meeting.title,'Recording · '||INITCAP(LOWER(recording.status::text)),starter.display_name,recording.id::text,recording.updated_at FROM recordings recording JOIN meetings meeting ON meeting.id=recording.meeting_id AND meeting.tenant_id=recording.tenant_id JOIN users starter ON starter.id=recording.started_by AND starter.tenant_id=recording.tenant_id WHERE recording.tenant_id=$1 AND recording.retention_expired_at IS NULL AND ($3 OR meeting.host_id=$2 OR recording.started_by=$2 OR EXISTS(SELECT 1 FROM recording_access_grants access WHERE access.recording_id=recording.id AND access.tenant_id=recording.tenant_id AND access.user_id=$2)) AND recording.updated_at<$4 ORDER BY recording.updated_at DESC LIMIT $5`, func(_ string, link string) string { return "/recordings?recordingId=" + url.QueryEscape(link) }, true},
	}

	items := make([]activityItem, 0, limit*2)
	for _, query := range queries {
		args := []any{actor.TenantID, actor.ID, before, limit + 1}
		if query.adminArg {
			args = []any{actor.TenantID, actor.ID, actor.Role.isWorkspaceAdmin(), before, limit + 1}
		}
		rows, err := api.database.QueryContext(request.Context(), query.sql, args...)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "workspace activity is temporarily unavailable")
			return
		}
		found, err := scanActivityRows(rows, query)
		rows.Close()
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "workspace activity is temporarily unavailable")
			return
		}
		items = append(items, found...)
	}
	sort.SliceStable(items, func(left, right int) bool { return items[left].CreatedAt.After(items[right].CreatedAt) })
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	nextBefore := ""
	if hasMore && len(items) > 0 {
		nextBefore = items[len(items)-1].CreatedAt.Format(time.RFC3339Nano)
	}
	respondJSON(writer, http.StatusOK, map[string]any{"activity": items, "pagination": map[string]any{"limit": limit, "hasMore": hasMore, "nextBefore": nextBefore}})
}

func scanActivityRows(rows *sql.Rows, query activityQuery) ([]activityItem, error) {
	items := make([]activityItem, 0)
	for rows.Next() {
		var item activityItem
		var link string
		if err := rows.Scan(&item.ID, &item.Title, &item.Description, &item.ActorName, &link, &item.CreatedAt); err != nil {
			return nil, err
		}
		item.Type = query.typeName
		item.Href = query.href(item.ID, link)
		items = append(items, item)
	}
	return items, rows.Err()
}
