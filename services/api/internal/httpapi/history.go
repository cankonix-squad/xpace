package httpapi

import (
	"net/http"
	"strconv"
	"time"
)

type meetingHistoryResponse struct {
	ID               string     `json:"id"`
	Title            string     `json:"title"`
	JoinCode         string     `json:"joinCode"`
	Status           string     `json:"status"`
	StartedAt        *time.Time `json:"startedAt"`
	EndedAt          *time.Time `json:"endedAt"`
	CreatedAt        time.Time  `json:"createdAt"`
	Role             string     `json:"role"`
	ParticipantCount int        `json:"participantCount"`
	RecordingCount   int        `json:"recordingCount"`
	DurationSeconds  int64      `json:"durationSeconds"`
}

func (api *API) meetingHistory(writer http.ResponseWriter, request *http.Request, user currentUser) {
	limit, offset, err := historyPage(request)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_PAGINATION", "limit must be 1-100 and offset must be non-negative")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT m.id,m.title,m.join_code,m.status,m.started_at,m.ended_at,m.created_at,
		       CASE WHEN m.host_id=$2 THEN 'HOST' ELSE COALESCE((
		         SELECT mp.role FROM meeting_participants mp
		         WHERE mp.meeting_id=m.id AND mp.tenant_id=m.tenant_id AND mp.user_id=$2
		         ORDER BY mp.created_at DESC LIMIT 1
		       ),'MEMBER') END AS user_role,
		       (SELECT COUNT(DISTINCT COALESCE(mp.user_id::text,mp.id::text))
		        FROM meeting_participants mp WHERE mp.meeting_id=m.id AND mp.tenant_id=m.tenant_id
		          AND mp.status NOT IN ('PRE_JOIN','WAITING_ROOM')) AS participant_count,
		       (SELECT COUNT(*) FROM recordings r
		        WHERE r.meeting_id=m.id AND r.tenant_id=m.tenant_id AND r.status='READY') AS recording_count,
		       GREATEST(0,EXTRACT(EPOCH FROM (COALESCE(m.ended_at,m.updated_at)-COALESCE(m.started_at,m.created_at))))::bigint AS duration_seconds
		FROM meetings m
		WHERE m.tenant_id=$1 AND m.status IN ('ENDED','CANCELLED')
		  AND ($3 OR m.host_id=$2 OR EXISTS (
		    SELECT 1 FROM meeting_participants own
		    WHERE own.meeting_id=m.id AND own.tenant_id=m.tenant_id AND own.user_id=$2
		  ))
		ORDER BY COALESCE(m.ended_at,m.updated_at) DESC,m.id DESC
		LIMIT $4 OFFSET $5`, user.TenantID, user.ID, user.Role.isWorkspaceAdmin(), limit+1, offset)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting history")
		return
	}
	defer rows.Close()
	items := make([]meetingHistoryResponse, 0, 100)
	for rows.Next() {
		var item meetingHistoryResponse
		if err = rows.Scan(&item.ID, &item.Title, &item.JoinCode, &item.Status,
			&item.StartedAt, &item.EndedAt, &item.CreatedAt, &item.Role,
			&item.ParticipantCount, &item.RecordingCount, &item.DurationSeconds); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting history")
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load meeting history")
		return
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	respondJSON(writer, http.StatusOK, map[string]any{
		"meetings":   items,
		"pagination": map[string]any{"limit": limit, "offset": offset, "hasMore": hasMore, "nextOffset": offset + len(items)},
	})
}

func historyPage(request *http.Request) (int, int, error) {
	limit, offset := 25, 0
	var err error
	if raw := request.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, strconv.ErrSyntax
		}
	}
	if raw := request.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, strconv.ErrSyntax
		}
	}
	return limit, offset, nil
}
