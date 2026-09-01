package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type incidentResponse struct {
	ID             string     `json:"id"`
	Title          string     `json:"title"`
	Description    string     `json:"description"`
	Source         string     `json:"source"`
	Severity       string     `json:"severity"`
	Status         string     `json:"status"`
	AssigneeUserID *string    `json:"assigneeUserId"`
	AssigneeName   string     `json:"assigneeName"`
	CreatedByName  string     `json:"createdByName"`
	AcknowledgedAt *time.Time `json:"acknowledgedAt"`
	ResolvedAt     *time.Time `json:"resolvedAt"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
}

type incidentEventResponse struct {
	ID        string         `json:"id"`
	EventType string         `json:"eventType"`
	ActorName string         `json:"actorName"`
	Note      string         `json:"note"`
	Metadata  map[string]any `json:"metadata"`
	CreatedAt time.Time      `json:"createdAt"`
}

func (api *API) adminIncidents(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "audit.read") {
		errorJSON(writer, http.StatusForbidden, "PERMISSION_REQUIRED", "audit.read permission is required")
		return
	}
	if request.Method == http.MethodPost {
		api.createAdminIncident(writer, request, actor)
		return
	}
	status := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("status")))
	severity := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("severity")))
	if status != "" && !validIncidentStatus(status) || severity != "" && !validIncidentSeverity(severity) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_FILTER", "invalid incident status or severity")
		return
	}
	limit := 50
	if raw := request.URL.Query().Get("limit"); raw != "" {
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 {
			errorJSON(writer, http.StatusBadRequest, "INVALID_FILTER", "limit must be between 1 and 100")
			return
		}
		limit = value
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT i.id,i.title,i.description,i.source,i.severity,i.status,i.assignee_user_id,COALESCE(a.display_name,''),COALESCE(c.display_name,'System'),i.acknowledged_at,i.resolved_at,i.created_at,i.updated_at FROM incidents i LEFT JOIN users a ON a.tenant_id=i.tenant_id AND a.id=i.assignee_user_id LEFT JOIN users c ON c.tenant_id=i.tenant_id AND c.id=i.created_by WHERE i.tenant_id=$1 AND ($2='' OR i.status=$2) AND ($3='' OR i.severity=$3) ORDER BY CASE i.status WHEN 'OPEN' THEN 0 WHEN 'ACKNOWLEDGED' THEN 1 WHEN 'INVESTIGATING' THEN 2 WHEN 'RESOLVED' THEN 3 ELSE 4 END,CASE i.severity WHEN 'P1' THEN 0 WHEN 'P2' THEN 1 WHEN 'P3' THEN 2 ELSE 3 END,i.updated_at DESC LIMIT $4`, actor.TenantID, status, severity, limit)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load incidents")
		return
	}
	defer rows.Close()
	items := []incidentResponse{}
	for rows.Next() {
		var item incidentResponse
		if err = scanIncident(rows, &item); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load incidents")
			return
		}
		items = append(items, item)
	}
	var open, acknowledged, investigating, resolved int
	if err = api.database.QueryRowContext(request.Context(), `SELECT COUNT(*) FILTER(WHERE status='OPEN'),COUNT(*) FILTER(WHERE status='ACKNOWLEDGED'),COUNT(*) FILTER(WHERE status='INVESTIGATING'),COUNT(*) FILTER(WHERE status='RESOLVED') FROM incidents WHERE tenant_id=$1`, actor.TenantID).Scan(&open, &acknowledged, &investigating, &resolved); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load incident summary")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"incidents": items, "summary": map[string]int{"open": open, "acknowledged": acknowledged, "investigating": investigating, "resolved": resolved}})
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanIncident(row rowScanner, item *incidentResponse) error {
	return row.Scan(&item.ID, &item.Title, &item.Description, &item.Source, &item.Severity, &item.Status, &item.AssigneeUserID, &item.AssigneeName, &item.CreatedByName, &item.AcknowledgedAt, &item.ResolvedAt, &item.CreatedAt, &item.UpdatedAt)
}

func (api *API) createAdminIncident(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "incident.manage") {
		errorJSON(writer, http.StatusForbidden, "PERMISSION_REQUIRED", "incident.manage permission is required")
		return
	}
	var input struct {
		Title       string `json:"title"`
		Description string `json:"description"`
		Severity    string `json:"severity"`
		Source      string `json:"source"`
	}
	if decodeJSON(writer, request, &input) != nil {
		return
	}
	input.Title, input.Description = strings.TrimSpace(input.Title), strings.TrimSpace(input.Description)
	input.Severity, input.Source = strings.ToUpper(strings.TrimSpace(input.Severity)), strings.ToUpper(strings.TrimSpace(input.Source))
	if input.Source == "" {
		input.Source = "MANUAL"
	}
	if len(input.Title) < 3 || len(input.Title) > 160 || len(input.Description) > 4000 || !validIncidentSeverity(input.Severity) || !validIncidentSource(input.Source) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INCIDENT", "title, severity, source, or description is invalid")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create incident")
		return
	}
	defer tx.Rollback()
	var id string
	if err = tx.QueryRowContext(request.Context(), `INSERT INTO incidents(tenant_id,title,description,source,severity,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, actor.TenantID, input.Title, input.Description, input.Source, input.Severity, actor.ID).Scan(&id); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create incident")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO incident_events(tenant_id,incident_id,actor_user_id,event_type,note,metadata) VALUES($1,$2,$3,'CREATED',$4,$5)`, actor.TenantID, id, actor.ID, input.Description, mustJSON(map[string]any{"severity": input.Severity, "source": input.Source})); err != nil || tx.Commit() != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create incident")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "incident.create", "incident", id, map[string]any{"severity": input.Severity, "source": input.Source})
	respondJSON(writer, http.StatusCreated, map[string]string{"id": id})
}

func (api *API) adminIncidentDetail(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "audit.read") {
		errorJSON(writer, http.StatusForbidden, "PERMISSION_REQUIRED", "audit.read permission is required")
		return
	}
	incidentID := request.PathValue("incidentID")
	var item incidentResponse
	row := api.database.QueryRowContext(request.Context(), `SELECT i.id,i.title,i.description,i.source,i.severity,i.status,i.assignee_user_id,COALESCE(a.display_name,''),COALESCE(c.display_name,'System'),i.acknowledged_at,i.resolved_at,i.created_at,i.updated_at FROM incidents i LEFT JOIN users a ON a.tenant_id=i.tenant_id AND a.id=i.assignee_user_id LEFT JOIN users c ON c.tenant_id=i.tenant_id AND c.id=i.created_by WHERE i.tenant_id=$1 AND i.id=$2`, actor.TenantID, incidentID)
	if err := scanIncident(row, &item); err != nil {
		if err == sql.ErrNoRows {
			errorJSON(writer, http.StatusNotFound, "NOT_FOUND", "incident was not found")
		} else {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load incident")
		}
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT e.id,e.event_type,COALESCE(u.display_name,'System'),e.note,e.metadata,e.created_at FROM incident_events e LEFT JOIN users u ON u.tenant_id=e.tenant_id AND u.id=e.actor_user_id WHERE e.tenant_id=$1 AND e.incident_id=$2 ORDER BY e.created_at,e.id`, actor.TenantID, incidentID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load incident timeline")
		return
	}
	defer rows.Close()
	events := []incidentEventResponse{}
	for rows.Next() {
		var event incidentEventResponse
		var metadata []byte
		if rows.Scan(&event.ID, &event.EventType, &event.ActorName, &event.Note, &metadata, &event.CreatedAt) != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load incident timeline")
			return
		}
		event.Metadata = map[string]any{}
		_ = json.Unmarshal(metadata, &event.Metadata)
		events = append(events, event)
	}
	respondJSON(writer, http.StatusOK, map[string]any{"incident": item, "timeline": events})
}

func (api *API) updateAdminIncident(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "incident.manage") {
		errorJSON(writer, http.StatusForbidden, "PERMISSION_REQUIRED", "incident.manage permission is required")
		return
	}
	var input struct {
		Severity       *string `json:"severity"`
		AssigneeUserID *string `json:"assigneeUserId"`
	}
	if decodeJSON(writer, request, &input) != nil {
		return
	}
	incidentID := request.PathValue("incidentID")
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update incident")
		return
	}
	defer tx.Rollback()
	if input.Severity != nil {
		severity := strings.ToUpper(strings.TrimSpace(*input.Severity))
		if !validIncidentSeverity(severity) {
			errorJSON(writer, 400, "INVALID_INCIDENT", "invalid severity")
			return
		}
		result, updateErr := tx.ExecContext(request.Context(), `UPDATE incidents SET severity=$3,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`, actor.TenantID, incidentID, severity)
		if updateErr != nil || affectedRows(result) != 1 {
			errorJSON(writer, 404, "NOT_FOUND", "incident was not found")
			return
		}
		_, err = tx.ExecContext(request.Context(), `INSERT INTO incident_events(tenant_id,incident_id,actor_user_id,event_type,metadata) VALUES($1,$2,$3,'SEVERITY_CHANGED',$4)`, actor.TenantID, incidentID, actor.ID, mustJSON(map[string]string{"severity": severity}))
	}
	if err == nil && input.AssigneeUserID != nil {
		assignee := strings.TrimSpace(*input.AssigneeUserID)
		if assignee != "" {
			var active bool
			if tx.QueryRowContext(request.Context(), `SELECT status='ACTIVE' FROM users WHERE tenant_id=$1 AND id=$2`, actor.TenantID, assignee).Scan(&active) != nil || !active {
				errorJSON(writer, 400, "INVALID_ASSIGNEE", "assignee must be an active workspace user")
				return
			}
		}
		_, err = tx.ExecContext(request.Context(), `UPDATE incidents SET assignee_user_id=NULLIF($3,''),updated_at=NOW() WHERE tenant_id=$1 AND id=$2`, actor.TenantID, incidentID, assignee)
		if err == nil {
			_, err = tx.ExecContext(request.Context(), `INSERT INTO incident_events(tenant_id,incident_id,actor_user_id,event_type,metadata) VALUES($1,$2,$3,'ASSIGNED',$4)`, actor.TenantID, incidentID, actor.ID, mustJSON(map[string]string{"assigneeUserId": assignee}))
		}
	}
	if input.Severity == nil && input.AssigneeUserID == nil {
		errorJSON(writer, 400, "INVALID_INCIDENT", "severity or assigneeUserId is required")
		return
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update incident")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "incident.update", "incident", incidentID, map[string]any{})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) transitionAdminIncident(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "incident.manage") {
		errorJSON(writer, 403, "PERMISSION_REQUIRED", "incident.manage permission is required")
		return
	}
	action := strings.ToLower(request.PathValue("action"))
	target, eventType, allowed := incidentTransition(action)
	if target == "" {
		errorJSON(writer, 400, "INVALID_TRANSITION", "unsupported incident transition")
		return
	}
	incidentID := request.PathValue("incidentID")
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update incident")
		return
	}
	defer tx.Rollback()
	var current string
	if err = tx.QueryRowContext(request.Context(), `SELECT status FROM incidents WHERE tenant_id=$1 AND id=$2 FOR UPDATE`, actor.TenantID, incidentID).Scan(&current); err != nil {
		errorJSON(writer, 404, "NOT_FOUND", "incident was not found")
		return
	}
	if !allowed[current] {
		errorJSON(writer, 409, "INVALID_TRANSITION", "incident cannot transition from its current status")
		return
	}
	_, err = tx.ExecContext(request.Context(), `UPDATE incidents SET status=$3,acknowledged_by=CASE WHEN $3='ACKNOWLEDGED' THEN $4 ELSE acknowledged_by END,acknowledged_at=CASE WHEN $3='ACKNOWLEDGED' THEN NOW() ELSE acknowledged_at END,resolved_by=CASE WHEN $3='RESOLVED' THEN $4 WHEN $3='OPEN' THEN NULL ELSE resolved_by END,resolved_at=CASE WHEN $3='RESOLVED' THEN NOW() WHEN $3='OPEN' THEN NULL ELSE resolved_at END,updated_at=NOW() WHERE tenant_id=$1 AND id=$2`, actor.TenantID, incidentID, target, actor.ID)
	if err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO incident_events(tenant_id,incident_id,actor_user_id,event_type,metadata) VALUES($1,$2,$3,$4,$5)`, actor.TenantID, incidentID, actor.ID, eventType, mustJSON(map[string]string{"from": current, "to": target}))
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update incident")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "incident."+action, "incident", incidentID, map[string]any{"from": current, "to": target})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) addIncidentNote(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "incident.manage") {
		errorJSON(writer, 403, "PERMISSION_REQUIRED", "incident.manage permission is required")
		return
	}
	var input struct {
		Note string `json:"note"`
	}
	if decodeJSON(writer, request, &input) != nil {
		return
	}
	input.Note = strings.TrimSpace(input.Note)
	if input.Note == "" || len(input.Note) > 4000 {
		errorJSON(writer, 400, "INVALID_NOTE", "note must be between 1 and 4000 characters")
		return
	}
	result, err := api.database.ExecContext(request.Context(), `INSERT INTO incident_events(tenant_id,incident_id,actor_user_id,event_type,note) SELECT tenant_id,id,$3,'NOTE',$4 FROM incidents WHERE tenant_id=$1 AND id=$2`, actor.TenantID, request.PathValue("incidentID"), actor.ID, input.Note)
	if err != nil || affectedRows(result) != 1 {
		errorJSON(writer, 404, "NOT_FOUND", "incident was not found")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `UPDATE incidents SET updated_at=NOW() WHERE tenant_id=$1 AND id=$2`, actor.TenantID, request.PathValue("incidentID"))
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "incident.note", "incident", request.PathValue("incidentID"), map[string]any{})
	writer.WriteHeader(http.StatusNoContent)
}

func incidentTransition(action string) (string, string, map[string]bool) {
	switch action {
	case "acknowledge":
		return "ACKNOWLEDGED", "ACKNOWLEDGED", map[string]bool{"OPEN": true}
	case "investigate":
		return "INVESTIGATING", "INVESTIGATING", map[string]bool{"OPEN": true, "ACKNOWLEDGED": true}
	case "resolve":
		return "RESOLVED", "RESOLVED", map[string]bool{"OPEN": true, "ACKNOWLEDGED": true, "INVESTIGATING": true}
	case "close":
		return "CLOSED", "CLOSED", map[string]bool{"RESOLVED": true}
	case "reopen":
		return "OPEN", "REOPENED", map[string]bool{"RESOLVED": true, "CLOSED": true}
	default:
		return "", "", nil
	}
}

func validIncidentSeverity(value string) bool {
	return value == "P1" || value == "P2" || value == "P3" || value == "P4"
}
func validIncidentStatus(value string) bool {
	return value == "OPEN" || value == "ACKNOWLEDGED" || value == "INVESTIGATING" || value == "RESOLVED" || value == "CLOSED"
}
func validIncidentSource(value string) bool {
	return value == "MANUAL" || value == "PROMETHEUS" || value == "CLIENT_ERROR"
}

func mustJSON(value any) []byte { encoded, _ := json.Marshal(value); return encoded }
func affectedRows(result sql.Result) int64 {
	if result == nil {
		return 0
	}
	count, _ := result.RowsAffected()
	return count
}
