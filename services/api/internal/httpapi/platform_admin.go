package httpapi

import (
	"database/sql"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type platformTenant struct {
	ID                 string     `json:"id"`
	Slug               string     `json:"slug"`
	Name               string     `json:"name"`
	PlatformStatus     string     `json:"platformStatus"`
	SubscriptionStatus string     `json:"subscriptionStatus"`
	PlanKey            string     `json:"planKey"`
	OwnerEmail         string     `json:"ownerEmail"`
	Users              int64      `json:"users"`
	Meetings30Days     int64      `json:"meetings30Days"`
	StorageBytes       int64      `json:"storageBytes"`
	TrialEndsAt        *time.Time `json:"trialEndsAt,omitempty"`
	PeriodEndsAt       *time.Time `json:"periodEndsAt,omitempty"`
	SuspendedAt        *time.Time `json:"suspendedAt,omitempty"`
	SuspensionReason   string     `json:"suspensionReason,omitempty"`
	CreatedAt          time.Time  `json:"createdAt"`
}

func requirePlatformAdmin(writer http.ResponseWriter, actor currentUser) bool {
	if actor.Role != roleSuperAdmin {
		errorJSON(writer, http.StatusForbidden, "PLATFORM_ADMIN_REQUIRED", "SaaS super administrator access is required")
		return false
	}
	return true
}

func (api *API) platformOverview(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !requirePlatformAdmin(writer, actor) {
		return
	}
	var total, active, suspended, trialing, pastDue, users, meetings30Days int64
	var storage int64
	err := api.database.QueryRowContext(request.Context(), `
		SELECT
		  COUNT(*),COUNT(*) FILTER(WHERE t.platform_status='ACTIVE'),COUNT(*) FILTER(WHERE t.platform_status='SUSPENDED'),
		  COUNT(*) FILTER(WHERE s.status='TRIALING'),COUNT(*) FILTER(WHERE s.status='PAST_DUE'),
		  (SELECT COUNT(*) FROM users WHERE status!='DEACTIVATED'),
		  (SELECT COUNT(*) FROM meetings WHERE created_at>=NOW()-INTERVAL '30 days'),
		  (SELECT COALESCE(SUM(size_bytes),0) FROM recordings WHERE retention_expired_at IS NULL)+
		  (SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes WHERE kind='FILE' AND deleted_at IS NULL)+
		  (SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments)
		FROM tenants t JOIN tenant_subscriptions s ON s.tenant_id=t.id`).
		Scan(&total, &active, &suspended, &trialing, &pastDue, &users, &meetings30Days, &storage)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load platform overview")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{
		"tenants": map[string]int64{"total": total, "active": active, "suspended": suspended, "trialing": trialing, "pastDue": pastDue},
		"usage":   map[string]int64{"users": users, "meetings30Days": meetings30Days, "storageBytes": storage},
	})
}

func (api *API) platformTenants(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !requirePlatformAdmin(writer, actor) {
		return
	}
	page, pageSize := platformPagination(request)
	search := strings.TrimSpace(request.URL.Query().Get("search"))
	status := strings.ToUpper(strings.TrimSpace(request.URL.Query().Get("status")))
	if status != "" && status != "ACTIVE" && status != "SUSPENDED" {
		errorJSON(writer, http.StatusBadRequest, "INVALID_STATUS", "status must be ACTIVE or SUSPENDED")
		return
	}
	pattern := "%" + search + "%"
	var total int64
	if err := api.database.QueryRowContext(request.Context(), `SELECT COUNT(*) FROM tenants t WHERE ($1='' OR t.name ILIKE $2 OR t.slug ILIKE $2) AND ($3='' OR t.platform_status=$3)`, search, pattern, status).Scan(&total); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not count workspaces")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT t.id,t.slug,t.name,t.platform_status,s.status,s.plan_key,
		  COALESCE((SELECT u.email FROM users u WHERE u.tenant_id=t.id AND u.status!='DEACTIVATED' ORDER BY CASE u.role WHEN 'SUPER_ADMIN' THEN 0 WHEN 'TENANT_ADMIN' THEN 1 ELSE 2 END,u.created_at LIMIT 1),''),
		  (SELECT COUNT(*) FROM users u WHERE u.tenant_id=t.id AND u.status!='DEACTIVATED'),
		  (SELECT COUNT(*) FROM meetings m WHERE m.tenant_id=t.id AND m.created_at>=NOW()-INTERVAL '30 days'),
		  (SELECT COALESCE(SUM(size_bytes),0) FROM recordings r WHERE r.tenant_id=t.id AND r.retention_expired_at IS NULL)+
		  (SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes d WHERE d.tenant_id=t.id AND d.kind='FILE' AND d.deleted_at IS NULL)+
		  (SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments a WHERE a.tenant_id=t.id),
		  s.trial_ends_at,s.current_period_ends_at,t.suspended_at,t.suspension_reason,t.created_at
		FROM tenants t JOIN tenant_subscriptions s ON s.tenant_id=t.id
		WHERE ($1='' OR t.name ILIKE $2 OR t.slug ILIKE $2) AND ($3='' OR t.platform_status=$3)
		ORDER BY t.created_at DESC,t.id LIMIT $4 OFFSET $5`, search, pattern, status, pageSize, (page-1)*pageSize)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load workspaces")
		return
	}
	defer rows.Close()
	items := make([]platformTenant, 0, pageSize)
	for rows.Next() {
		var item platformTenant
		if err = rows.Scan(&item.ID, &item.Slug, &item.Name, &item.PlatformStatus, &item.SubscriptionStatus, &item.PlanKey, &item.OwnerEmail, &item.Users, &item.Meetings30Days, &item.StorageBytes, &item.TrialEndsAt, &item.PeriodEndsAt, &item.SuspendedAt, &item.SuspensionReason, &item.CreatedAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read workspaces")
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read workspaces")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"tenants": items, "page": page, "pageSize": pageSize, "total": total})
}

func (api *API) platformTenantDetail(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !requirePlatformAdmin(writer, actor) {
		return
	}
	tenantID := request.PathValue("tenantID")
	var item platformTenant
	var meetings, recordings, activeUsers, errors24Hours int64
	err := api.database.QueryRowContext(request.Context(), `
		SELECT t.id,t.slug,t.name,t.platform_status,s.status,s.plan_key,
		  COALESCE((SELECT u.email FROM users u WHERE u.tenant_id=t.id AND u.status!='DEACTIVATED' ORDER BY CASE u.role WHEN 'SUPER_ADMIN' THEN 0 WHEN 'TENANT_ADMIN' THEN 1 ELSE 2 END,u.created_at LIMIT 1),''),
		  (SELECT COUNT(*) FROM users u WHERE u.tenant_id=t.id AND u.status!='DEACTIVATED'),
		  (SELECT COUNT(*) FROM meetings m WHERE m.tenant_id=t.id AND m.created_at>=NOW()-INTERVAL '30 days'),
		  (SELECT COALESCE(SUM(size_bytes),0) FROM recordings r WHERE r.tenant_id=t.id AND r.retention_expired_at IS NULL)+
		  (SELECT COALESCE(SUM(size_bytes),0) FROM drive_nodes d WHERE d.tenant_id=t.id AND d.kind='FILE' AND d.deleted_at IS NULL)+
		  (SELECT COALESCE(SUM(size_bytes),0) FROM chat_attachments a WHERE a.tenant_id=t.id),
		  s.trial_ends_at,s.current_period_ends_at,t.suspended_at,t.suspension_reason,t.created_at,
		  (SELECT COUNT(*) FROM meetings m WHERE m.tenant_id=t.id),
		  (SELECT COUNT(*) FROM recordings r WHERE r.tenant_id=t.id AND r.retention_expired_at IS NULL),
		  (SELECT COUNT(*) FROM users u WHERE u.tenant_id=t.id AND u.status='ACTIVE'),
		  (SELECT COUNT(*) FROM error_events e WHERE e.tenant_id=t.id AND e.created_at>=NOW()-INTERVAL '24 hours')
		FROM tenants t JOIN tenant_subscriptions s ON s.tenant_id=t.id WHERE t.id=$1`, tenantID).
		Scan(&item.ID, &item.Slug, &item.Name, &item.PlatformStatus, &item.SubscriptionStatus, &item.PlanKey, &item.OwnerEmail, &item.Users, &item.Meetings30Days, &item.StorageBytes, &item.TrialEndsAt, &item.PeriodEndsAt, &item.SuspendedAt, &item.SuspensionReason, &item.CreatedAt, &meetings, &recordings, &activeUsers, &errors24Hours)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "TENANT_NOT_FOUND", "workspace was not found")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load workspace details")
		return
	}
	var supportID string
	var supportExpires *time.Time
	_ = api.database.QueryRowContext(request.Context(), `SELECT id,expires_at FROM platform_support_access WHERE actor_user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL AND expires_at>NOW() ORDER BY expires_at DESC LIMIT 1`, actor.ID, tenantID).Scan(&supportID, &supportExpires)
	respondJSON(writer, http.StatusOK, map[string]any{
		"tenant":        item,
		"usage":         map[string]int64{"meetings": meetings, "recordings": recordings, "activeUsers": activeUsers, "errors24Hours": errors24Hours},
		"supportAccess": map[string]any{"active": supportID != "", "id": supportID, "expiresAt": supportExpires},
	})
}

func (api *API) platformTenantLifecycle(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !requirePlatformAdmin(writer, actor) {
		return
	}
	tenantID, action := request.PathValue("tenantID"), request.PathValue("action")
	if tenantID == actor.TenantID {
		errorJSON(writer, http.StatusConflict, "HOME_TENANT_PROTECTED", "your own platform workspace cannot be suspended")
		return
	}
	if action != "suspend" && action != "reactivate" {
		errorJSON(writer, http.StatusNotFound, "ACTION_NOT_FOUND", "workspace action was not found")
		return
	}
	var input struct{ Reason string }
	if err := decodeJSON(writer, request, &input); err != nil {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if len(input.Reason) < 5 || len(input.Reason) > 500 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_REASON", "a reason between 5 and 500 characters is required")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update workspace")
		return
	}
	defer tx.Rollback()
	var previous string
	if err = tx.QueryRowContext(request.Context(), `SELECT platform_status FROM tenants WHERE id=$1 FOR UPDATE`, tenantID).Scan(&previous); err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "TENANT_NOT_FOUND", "workspace was not found")
		return
	} else if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update workspace")
		return
	}
	target := "SUSPENDED"
	if action == "reactivate" {
		target = "ACTIVE"
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE tenants SET platform_status=$1,suspended_at=CASE WHEN $1='SUSPENDED' THEN NOW() ELSE NULL END,suspended_by=CASE WHEN $1='SUSPENDED' THEN $2::uuid ELSE NULL END,suspension_reason=CASE WHEN $1='SUSPENDED' THEN $3 ELSE '' END,updated_at=NOW() WHERE id=$4`, target, actor.ID, input.Reason, tenantID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update workspace")
		return
	}
	if target == "SUSPENDED" {
		if _, err = tx.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id IN(SELECT id FROM users WHERE tenant_id=$1) AND revoked_at IS NULL`, tenantID); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not revoke workspace sessions")
			return
		}
		_, _ = tx.ExecContext(request.Context(), `UPDATE platform_support_access SET revoked_at=NOW() WHERE tenant_id=$1 AND revoked_at IS NULL`, tenantID)
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update workspace")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, tenantID, actor.ID, "platform.tenant."+action, "tenant", tenantID, map[string]any{"reason": input.Reason, "previousStatus": previous, "status": target, "actorTenantId": actor.TenantID})
	respondJSON(writer, http.StatusOK, map[string]string{"status": target})
}

func (api *API) platformSupportAccess(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !requirePlatformAdmin(writer, actor) {
		return
	}
	tenantID := request.PathValue("tenantID")
	if tenantID == actor.TenantID {
		errorJSON(writer, http.StatusConflict, "SUPPORT_ACCESS_NOT_REQUIRED", "support access is not required for your own workspace")
		return
	}
	if request.Method == http.MethodDelete {
		result, err := api.database.ExecContext(request.Context(), `UPDATE platform_support_access SET revoked_at=NOW() WHERE actor_user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL AND expires_at>NOW()`, actor.ID, tenantID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not revoke support access")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			errorJSON(writer, http.StatusNotFound, "SUPPORT_ACCESS_NOT_FOUND", "active support access was not found")
			return
		}
		_ = api.writeAuditEvent(request.Context(), request, tenantID, actor.ID, "platform.support.revoke", "tenant", tenantID, map[string]any{"actorTenantId": actor.TenantID})
		respondJSON(writer, http.StatusOK, map[string]string{"status": "revoked"})
		return
	}
	var input struct {
		Reason          string
		DurationMinutes int
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		return
	}
	input.Reason = strings.TrimSpace(input.Reason)
	if input.DurationMinutes == 0 {
		input.DurationMinutes = 30
	}
	if len(input.Reason) < 5 || len(input.Reason) > 500 || input.DurationMinutes < 15 || input.DurationMinutes > 120 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_SUPPORT_ACCESS", "reason must be 5-500 characters and duration must be 15-120 minutes")
		return
	}
	var status string
	if err := api.database.QueryRowContext(request.Context(), `SELECT platform_status FROM tenants WHERE id=$1`, tenantID).Scan(&status); err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "TENANT_NOT_FOUND", "workspace was not found")
		return
	} else if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not verify workspace")
		return
	}
	if status != "ACTIVE" {
		errorJSON(writer, http.StatusConflict, "TENANT_SUSPENDED", "support access cannot start while the workspace is suspended")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `UPDATE platform_support_access SET revoked_at=NOW() WHERE actor_user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL`, actor.ID, tenantID)
	var id string
	var expires time.Time
	err := api.database.QueryRowContext(request.Context(), `INSERT INTO platform_support_access(tenant_id,actor_user_id,reason,expires_at) VALUES($1,$2,$3,NOW()+$4*INTERVAL '1 minute') RETURNING id,expires_at`, tenantID, actor.ID, input.Reason, input.DurationMinutes).Scan(&id, &expires)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not start support access")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, tenantID, actor.ID, "platform.support.start", "tenant", tenantID, map[string]any{"reason": input.Reason, "durationMinutes": input.DurationMinutes, "actorTenantId": actor.TenantID})
	respondJSON(writer, http.StatusCreated, map[string]any{"id": id, "expiresAt": expires})
}

func (api *API) platformSupportView(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !requirePlatformAdmin(writer, actor) {
		return
	}
	tenantID := request.PathValue("tenantID")
	var accessID string
	if err := api.database.QueryRowContext(request.Context(), `SELECT id FROM platform_support_access WHERE actor_user_id=$1 AND tenant_id=$2 AND revoked_at IS NULL AND expires_at>NOW() ORDER BY expires_at DESC LIMIT 1`, actor.ID, tenantID).Scan(&accessID); err != nil {
		errorJSON(writer, http.StatusForbidden, "SUPPORT_ACCESS_REQUIRED", "active support access is required")
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,email,username,display_name,role,status,created_at FROM users WHERE tenant_id=$1 ORDER BY created_at LIMIT 100`, tenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load support view")
		return
	}
	defer rows.Close()
	users := make([]map[string]any, 0)
	for rows.Next() {
		var id, email, username, name, role, status string
		var createdAt time.Time
		if err = rows.Scan(&id, &email, &username, &name, &role, &status, &createdAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not read support view")
			return
		}
		users = append(users, map[string]any{"id": id, "email": email, "username": username, "displayName": name, "role": role, "status": status, "createdAt": createdAt})
	}
	_ = api.writeAuditEvent(request.Context(), request, tenantID, actor.ID, "platform.support.view", "tenant", tenantID, map[string]any{"supportAccessId": accessID, "actorTenantId": actor.TenantID})
	respondJSON(writer, http.StatusOK, map[string]any{"users": users})
}

func platformPagination(request *http.Request) (int, int) {
	page, _ := strconv.Atoi(request.URL.Query().Get("page"))
	pageSize, _ := strconv.Atoi(request.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	return page, pageSize
}
