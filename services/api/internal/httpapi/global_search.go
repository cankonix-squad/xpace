package httpapi

import (
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type globalSearchResult struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Subtitle  string    `json:"subtitle"`
	Href      string    `json:"href"`
	UpdatedAt time.Time `json:"updatedAt"`
}

type globalSearchQuery struct {
	typeName string
	sql      string
	href     func(string, string, string) string
	adminArg bool
}

func (api *API) globalSearch(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	query := strings.TrimSpace(request.URL.Query().Get("q"))
	if len(query) < 2 || len(query) > 120 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "search query must be between 2 and 120 characters")
		return
	}
	pattern := "%" + query + "%"
	admin := actor.Role.isWorkspaceAdmin()
	queries := []globalSearchQuery{
		{"meeting", `SELECT id::text,title,join_code,updated_at FROM meetings WHERE tenant_id=$1 AND status!='CANCELLED' AND (title ILIKE $3 OR join_code ILIKE $3) ORDER BY updated_at DESC LIMIT 6`, func(_ string, _ string, subtitle string) string {
			return "/meet/" + url.PathEscape(subtitle) + "/prejoin"
		}, false},
		{"person", `SELECT id::text,display_name,email||' · @'||username,updated_at FROM users WHERE tenant_id=$1 AND status='ACTIVE' AND (display_name ILIKE $3 OR email ILIKE $3 OR username ILIKE $3 OR role::text ILIKE $3) ORDER BY updated_at DESC LIMIT 6`, func(id, _, _ string) string { return "/people?userId=" + url.QueryEscape(id) }, false},
		{"chat", `SELECT c.id::text,COALESCE(c.name,(SELECT string_agg(u.display_name,', ' ORDER BY u.display_name) FROM chat_members other JOIN users u ON u.id=other.user_id AND u.tenant_id=other.tenant_id WHERE other.conversation_id=c.id AND other.tenant_id=c.tenant_id AND other.user_id<>$2),'Direct conversation'),LOWER(c.type::text),c.updated_at FROM chat_conversations c JOIN chat_members member ON member.conversation_id=c.id AND member.tenant_id=c.tenant_id AND member.user_id=$2 WHERE c.tenant_id=$1 AND (member.hidden_at IS NULL OR c.updated_at>member.hidden_at) AND (COALESCE(c.name,'') ILIKE $3 OR EXISTS(SELECT 1 FROM chat_members other JOIN users u ON u.id=other.user_id AND u.tenant_id=other.tenant_id WHERE other.conversation_id=c.id AND other.tenant_id=c.tenant_id AND other.user_id<>$2 AND (u.display_name ILIKE $3 OR u.email ILIKE $3 OR u.username ILIKE $3))) ORDER BY c.updated_at DESC LIMIT 6`, func(id, _, _ string) string { return "/chat?conversationId=" + url.QueryEscape(id) }, false},
		{"room", `SELECT r.id::text,r.name,COALESCE(NULLIF(r.description,''),'Collaboration room'),r.updated_at FROM workspace_rooms r JOIN workspace_room_members member ON member.room_id=r.id AND member.tenant_id=r.tenant_id AND member.user_id=$2 WHERE r.tenant_id=$1 AND (r.name ILIKE $3 OR r.description ILIKE $3) ORDER BY r.updated_at DESC LIMIT 6`, func(id, _, _ string) string { return "/rooms?roomId=" + url.QueryEscape(id) }, false},
		{"drive", `SELECT DISTINCT n.id::text,n.name,LOWER(n.kind)||CASE WHEN n.content_type IS NULL OR n.content_type='' THEN '' ELSE ' · '||n.content_type END,n.updated_at FROM drive_nodes n LEFT JOIN drive_shares share ON share.node_id=n.id AND share.tenant_id=n.tenant_id AND share.user_id=$2 WHERE n.tenant_id=$1 AND n.deleted_at IS NULL AND (n.owner_id=$2 OR share.user_id=$2) AND n.name ILIKE $3 ORDER BY n.updated_at DESC LIMIT 6`, func(_ string, title string, _ string) string { return "/drive?search=" + url.QueryEscape(title) }, false},
		{"calendar", `SELECT event.id::text,event.title,to_char(event.starts_at AT TIME ZONE event.timezone,'Mon DD, YYYY HH24:MI')||' · '||event.timezone,event.updated_at FROM calendar_events event LEFT JOIN calendar_event_attendees attendee ON attendee.event_id=event.id AND attendee.tenant_id=event.tenant_id AND attendee.user_id=$2 WHERE event.tenant_id=$1 AND (event.organizer_id=$2 OR attendee.user_id=$2) AND (event.title ILIKE $3 OR event.description ILIKE $3) ORDER BY event.updated_at DESC LIMIT 6`, func(id, _, _ string) string { return "/calendar?eventId=" + url.QueryEscape(id) }, false},
		{"recording", `SELECT recording.id::text,meeting.title,recording.status::text||' · '||meeting.join_code,recording.updated_at FROM recordings recording JOIN meetings meeting ON meeting.id=recording.meeting_id AND meeting.tenant_id=recording.tenant_id WHERE recording.tenant_id=$1 AND recording.retention_expired_at IS NULL AND ($4 OR meeting.host_id=$2 OR recording.started_by=$2 OR EXISTS(SELECT 1 FROM recording_access_grants access WHERE access.recording_id=recording.id AND access.tenant_id=recording.tenant_id AND access.user_id=$2)) AND (meeting.title ILIKE $3 OR meeting.join_code ILIKE $3 OR recording.status::text ILIKE $3) ORDER BY recording.updated_at DESC LIMIT 6`, func(id, _, _ string) string { return "/recordings?recordingId=" + url.QueryEscape(id) }, true},
	}

	results := make([]globalSearchResult, 0, 24)
	for _, search := range queries {
		args := []any{actor.TenantID, actor.ID, pattern}
		if search.adminArg {
			args = append(args, admin)
		}
		rows, err := api.database.QueryContext(request.Context(), search.sql, args...)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "workspace search is temporarily unavailable")
			return
		}
		items, err := scanGlobalSearchRows(rows, search)
		rows.Close()
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "workspace search is temporarily unavailable")
			return
		}
		results = append(results, items...)
	}
	respondJSON(writer, http.StatusOK, map[string]any{"query": query, "results": results})
}

func scanGlobalSearchRows(rows *sql.Rows, search globalSearchQuery) ([]globalSearchResult, error) {
	items := make([]globalSearchResult, 0, 6)
	for rows.Next() {
		var item globalSearchResult
		if err := rows.Scan(&item.ID, &item.Title, &item.Subtitle, &item.UpdatedAt); err != nil {
			return nil, err
		}
		item.Type = search.typeName
		item.Href = search.href(item.ID, item.Title, item.Subtitle)
		items = append(items, item)
	}
	return items, rows.Err()
}
