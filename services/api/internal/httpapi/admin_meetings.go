package httpapi

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

type adminMeetingListItem struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	JoinCode         string     `json:"joinCode"`
	Status           string     `json:"status"`
	HostName         string     `json:"hostName"`
	ScheduledAt      *time.Time `json:"scheduledAt"`
	StartedAt        *time.Time `json:"startedAt"`
	EndedAt          *time.Time `json:"endedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	ParticipantCount int        `json:"participantCount"`
	RecordingCount   int        `json:"recordingCount"`
	DurationSeconds  int64      `json:"durationSeconds"`
}

func (api *API) adminMeetings(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	limit, offset, status, search, err := adminMeetingFilters(request)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_FILTER", "invalid status, limit, or offset")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT m.id,m.title,m.join_code,m.status,u.display_name,m.scheduled_at,m.started_at,m.ended_at,m.created_at,
		       (SELECT COUNT(DISTINCT COALESCE(p.user_id::text,p.id::text)) FROM meeting_participants p WHERE p.meeting_id=m.id AND p.tenant_id=m.tenant_id),
		       (SELECT COUNT(*) FROM recordings r WHERE r.meeting_id=m.id AND r.tenant_id=m.tenant_id),
		       GREATEST(0,EXTRACT(EPOCH FROM (COALESCE(m.ended_at,NOW())-COALESCE(m.started_at,m.created_at))))::bigint
		FROM meetings m JOIN users u ON u.id=m.host_id AND u.tenant_id=m.tenant_id
		WHERE m.tenant_id=$1 AND ($2='' OR m.status::text=$2)
		  AND ($3='' OR m.title ILIKE '%%'||$3||'%%' OR m.join_code ILIKE '%%'||$3||'%%' OR u.display_name ILIKE '%%'||$3||'%%')
		ORDER BY COALESCE(m.scheduled_at,m.created_at) DESC,m.id DESC LIMIT $4 OFFSET $5`, actor.TenantID, status, search, limit+1, offset)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meetings")
		return
	}
	defer rows.Close()
	items := make([]adminMeetingListItem, 0, 100)
	for rows.Next() {
		var item adminMeetingListItem
		if err = rows.Scan(&item.ID, &item.Title, &item.JoinCode, &item.Status, &item.HostName, &item.ScheduledAt, &item.StartedAt, &item.EndedAt, &item.CreatedAt, &item.ParticipantCount, &item.RecordingCount, &item.DurationSeconds); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meetings")
			return
		}
		items = append(items, item)
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	respondJSON(writer, http.StatusOK, map[string]any{"meetings": items, "pagination": map[string]any{"limit": limit, "offset": offset, "hasMore": hasMore, "nextOffset": offset + len(items)}})
}

func (api *API) adminMeetingAnalytics(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	meetingID := request.PathValue("meetingID")
	var meeting adminMeetingListItem
	err := api.database.QueryRowContext(request.Context(), `
		SELECT m.id,m.title,m.join_code,m.status,u.display_name,m.scheduled_at,m.started_at,m.ended_at,m.created_at,
		       (SELECT COUNT(DISTINCT COALESCE(p.user_id::text,p.id::text)) FROM meeting_participants p WHERE p.meeting_id=m.id AND p.tenant_id=m.tenant_id),
		       (SELECT COUNT(*) FROM recordings r WHERE r.meeting_id=m.id AND r.tenant_id=m.tenant_id),
		       GREATEST(0,EXTRACT(EPOCH FROM (COALESCE(m.ended_at,NOW())-COALESCE(m.started_at,m.created_at))))::bigint
		FROM meetings m JOIN users u ON u.id=m.host_id AND u.tenant_id=m.tenant_id
		WHERE m.id=$1 AND m.tenant_id=$2`, meetingID, actor.TenantID).Scan(&meeting.ID, &meeting.Title, &meeting.JoinCode, &meeting.Status, &meeting.HostName, &meeting.ScheduledAt, &meeting.StartedAt, &meeting.EndedAt, &meeting.CreatedAt, &meeting.ParticipantCount, &meeting.RecordingCount, &meeting.DurationSeconds)
	if err != nil {
		errorJSON(writer, http.StatusNotFound, "MEETING_NOT_FOUND", "meeting was not found")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT role,status,COUNT(*),COALESCE(AVG(EXTRACT(EPOCH FROM (COALESCE(left_at,NOW())-COALESCE(joined_at,created_at)))),0)::bigint FROM meeting_participants WHERE meeting_id=$1 AND tenant_id=$2 GROUP BY role,status ORDER BY role,status`, meetingID, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting analytics")
		return
	}
	defer rows.Close()
	breakdown := make([]map[string]any, 0)
	for rows.Next() {
		var role, status string
		var count int
		var averageDuration int64
		if err = rows.Scan(&role, &status, &count, &averageDuration); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting analytics")
			return
		}
		breakdown = append(breakdown, map[string]any{"role": role, "status": status, "count": count, "averageDurationSeconds": averageDuration})
	}
	respondJSON(writer, http.StatusOK, map[string]any{"meeting": meeting, "participantBreakdown": breakdown})
}

func adminMeetingFilters(request *http.Request) (int, int, string, string, error) {
	limit, offset := 25, 0
	var err error
	if raw := request.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, "", "", strconv.ErrSyntax
		}
	}
	if raw := request.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, "", "", strconv.ErrSyntax
		}
	}
	status := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("status")))
	if status != "" && status != "SCHEDULED" && status != "WAITING" && status != "ACTIVE" && status != "ENDED" && status != "CANCELLED" {
		return 0, 0, "", "", strconv.ErrSyntax
	}
	search := strings.TrimSpace(request.URL.Query().Get("search"))
	if len(search) > 120 {
		return 0, 0, "", "", strconv.ErrSyntax
	}
	return limit, offset, status, search, nil
}
