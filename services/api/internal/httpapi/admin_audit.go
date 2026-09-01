package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type auditEventResponse struct {
	ID           string         `json:"id"`
	Action       string         `json:"action"`
	ResourceType string         `json:"resourceType"`
	ResourceID   *string        `json:"resourceId"`
	ActorUserID  *string        `json:"actorUserId"`
	ActorName    string         `json:"actorName"`
	IPAddress    *string        `json:"ipAddress"`
	UserAgent    *string        `json:"userAgent"`
	Metadata     map[string]any `json:"metadata"`
	CreatedAt    time.Time      `json:"createdAt"`
}

func (api *API) adminAuditLog(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "audit.read") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	limit, offset, action, resource, actorID, err := auditFilters(request)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_FILTER", "invalid audit filter, limit, or offset")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT a.id,a.action,a.resource_type,a.resource_id,a.actor_user_id,
		       COALESCE(u.display_name,'System'),a.ip_address::text,a.user_agent,a.metadata,a.created_at
		FROM audit_events a LEFT JOIN users u ON u.id=a.actor_user_id AND u.tenant_id=a.tenant_id
		WHERE a.tenant_id=$1 AND ($2='' OR a.action ILIKE $2||'%%')
		  AND ($3='' OR a.resource_type=$3) AND ($4='' OR a.actor_user_id::text=$4)
		ORDER BY a.created_at DESC,a.id DESC LIMIT $5 OFFSET $6`, actor.TenantID, action, resource, actorID, limit+1, offset)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load audit events")
		return
	}
	defer rows.Close()
	items := make([]auditEventResponse, 0, 100)
	for rows.Next() {
		var item auditEventResponse
		var metadata []byte
		if err = rows.Scan(&item.ID, &item.Action, &item.ResourceType, &item.ResourceID, &item.ActorUserID, &item.ActorName, &item.IPAddress, &item.UserAgent, &metadata, &item.CreatedAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load audit events")
			return
		}
		item.Metadata = map[string]any{}
		_ = json.Unmarshal(metadata, &item.Metadata)
		items = append(items, item)
	}
	hasMore := len(items) > limit
	if hasMore {
		items = items[:limit]
	}
	respondJSON(writer, http.StatusOK, map[string]any{"events": items, "pagination": map[string]any{"limit": limit, "offset": offset, "hasMore": hasMore, "nextOffset": offset + len(items)}})
}

func auditFilters(request *http.Request) (int, int, string, string, string, error) {
	limit, offset := 50, 0
	var err error
	if raw := request.URL.Query().Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
		if err != nil || limit < 1 || limit > 100 {
			return 0, 0, "", "", "", strconv.ErrSyntax
		}
	}
	if raw := request.URL.Query().Get("offset"); raw != "" {
		offset, err = strconv.Atoi(raw)
		if err != nil || offset < 0 {
			return 0, 0, "", "", "", strconv.ErrSyntax
		}
	}
	action := strings.TrimSpace(request.URL.Query().Get("action"))
	resource := strings.TrimSpace(request.URL.Query().Get("resource"))
	actorID := strings.TrimSpace(request.URL.Query().Get("actorId"))
	if len(action) > 100 || len(resource) > 50 || len(actorID) > 64 {
		return 0, 0, "", "", "", strconv.ErrSyntax
	}
	return limit, offset, action, resource, actorID, nil
}
