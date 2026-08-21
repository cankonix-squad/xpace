package httpapi

import (
	"net/http"
	"strings"
	"time"

	"github.com/cankonix/xpace/api/internal/auth"
)

type adminUserResponse struct {
	ID          string    `json:"id"`
	Email       string    `json:"email"`
	Username    string    `json:"username"`
	DisplayName string    `json:"displayName"`
	Role        userRole  `json:"role"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

func (api *API) adminUsers(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	if request.Method == http.MethodPost {
		api.createAdminUser(writer, request, actor)
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,email,username,display_name,role,status,created_at FROM users WHERE tenant_id=$1 ORDER BY created_at DESC LIMIT 200`, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load users")
		return
	}
	defer rows.Close()
	items := make([]adminUserResponse, 0)
	for rows.Next() {
		var item adminUserResponse
		if err = rows.Scan(&item.ID, &item.Email, &item.Username, &item.DisplayName, &item.Role, &item.Status, &item.CreatedAt); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load users")
			return
		}
		items = append(items, item)
	}
	if err = rows.Err(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load users")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"users": items})
}

func (api *API) createAdminUser(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	var input struct {
		Email, Username, DisplayName, Password string
		Role                                   userRole
		Status                                 string
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.Email = strings.ToLower(strings.TrimSpace(input.Email))
	input.Username = strings.ToLower(strings.TrimSpace(input.Username))
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if input.Status == "" {
		input.Status = "INVITED"
	}
	if input.Email == "" || input.Username == "" || len(input.DisplayName) < 2 || !validManagedRole(actor.Role, input.Role) || (input.Status != "ACTIVE" && input.Status != "INVITED") {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "email, username, displayName, allowed role, and ACTIVE or INVITED status are required")
		return
	}
	password := input.Password
	if input.Status == "INVITED" {
		password, _ = randomToken(32)
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "WEAK_PASSWORD", err.Error())
		return
	}
	var item adminUserResponse
	err = api.database.QueryRowContext(request.Context(), `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,email,username,display_name,role,status,created_at`, actor.TenantID, input.Email, input.Username, input.DisplayName, passwordHash, input.Role, input.Status).Scan(&item.ID, &item.Email, &item.Username, &item.DisplayName, &item.Role, &item.Status, &item.CreatedAt)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "USER_EXISTS", "email or username already exists in this tenant")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `INSERT INTO user_profiles (user_id,tenant_id) VALUES ($1,$2) ON CONFLICT (user_id) DO NOTHING`, item.ID, actor.TenantID)
	api.auditUserManagement(request, actor, "user.create", item.ID)
	respondJSON(writer, http.StatusCreated, map[string]any{"user": item})
}

func (api *API) updateAdminUser(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	targetID := request.PathValue("userID")
	var input struct {
		Role   userRole `json:"role"`
		Status string   `json:"status"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.Status = strings.ToUpper(strings.TrimSpace(input.Status))
	if targetID == actor.ID && (input.Status != "ACTIVE" || input.Role != actor.Role) {
		errorJSON(writer, http.StatusConflict, "SELF_PROTECTION", "administrators cannot change their own role or active status")
		return
	}
	var targetRole userRole
	if err := api.database.QueryRowContext(request.Context(), `SELECT role FROM users WHERE id=$1 AND tenant_id=$2`, targetID, actor.TenantID).Scan(&targetRole); err != nil {
		errorJSON(writer, http.StatusNotFound, "USER_NOT_FOUND", "user was not found")
		return
	}
	if targetRole == roleSuperAdmin && actor.Role != roleSuperAdmin {
		errorJSON(writer, http.StatusForbidden, "ROLE_NOT_ALLOWED", "tenant admins cannot modify super administrators")
		return
	}
	if !validManagedRole(actor.Role, input.Role) || !validUserStatus(input.Status) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", "role or status is not allowed")
		return
	}
	var item adminUserResponse
	err := api.database.QueryRowContext(request.Context(), `UPDATE users SET role=$1,status=$2,updated_at=NOW() WHERE id=$3 AND tenant_id=$4 RETURNING id,email,username,display_name,role,status,created_at`, input.Role, input.Status, targetID, actor.TenantID).Scan(&item.ID, &item.Email, &item.Username, &item.DisplayName, &item.Role, &item.Status, &item.CreatedAt)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update user")
		return
	}
	if input.Status != "ACTIVE" {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL AND EXISTS (SELECT 1 FROM users WHERE id=$1 AND tenant_id=$2)`, targetID, actor.TenantID)
	}
	api.auditUserManagement(request, actor, "user.update", targetID)
	respondJSON(writer, http.StatusOK, map[string]any{"user": item})
}

func validManagedRole(actorRole, targetRole userRole) bool {
	if _, exists := rolePermissions[targetRole]; !exists {
		return false
	}
	return targetRole != roleSuperAdmin || actorRole == roleSuperAdmin
}

func validUserStatus(status string) bool {
	return status == "ACTIVE" || status == "INVITED" || status == "SUSPENDED" || status == "DEACTIVATED"
}

func (api *API) auditUserManagement(request *http.Request, actor currentUser, action, targetID string) {
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, action, "user", targetID, nil)
}
