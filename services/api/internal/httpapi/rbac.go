package httpapi

import (
	"context"
	"net/http"
	"sort"
	"strings"
)

type customRoleResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	MemberCount int      `json:"memberCount"`
}

func (api *API) hasPermission(ctx context.Context, actor currentUser, permission string) bool {
	for _, item := range actor.Role.permissions() {
		if item == permission || item == "platform.manage" {
			return true
		}
	}
	var allowed bool
	err := api.database.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM user_custom_roles ur JOIN custom_roles r ON r.id=ur.role_id AND r.tenant_id=ur.tenant_id JOIN custom_role_permissions p ON p.role_id=r.id WHERE ur.user_id=$1 AND ur.tenant_id=$2 AND p.permission=$3)`, actor.ID, actor.TenantID, permission).Scan(&allowed)
	return err == nil && allowed
}

func (api *API) effectivePermissions(ctx context.Context, actor currentUser) []string {
	set := map[string]bool{}
	for _, item := range actor.Role.permissions() {
		set[item] = true
	}
	if api.database == nil {
		items := make([]string, 0, len(set))
		for item := range set {
			items = append(items, item)
		}
		sort.Strings(items)
		return items
	}
	rows, err := api.database.QueryContext(ctx, `SELECT DISTINCT p.permission FROM user_custom_roles ur JOIN custom_roles r ON r.id=ur.role_id AND r.tenant_id=ur.tenant_id JOIN custom_role_permissions p ON p.role_id=r.id WHERE ur.user_id=$1 AND ur.tenant_id=$2`, actor.ID, actor.TenantID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item string
			if rows.Scan(&item) == nil {
				set[item] = true
			}
		}
	}
	items := make([]string, 0, len(set))
	for item := range set {
		items = append(items, item)
	}
	sort.Strings(items)
	return items
}

func (api *API) adminCustomRoles(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "roles.manage") {
		errorJSON(writer, 403, "PERMISSION_REQUIRED", "roles.manage permission is required")
		return
	}
	if request.Method == http.MethodPost {
		api.createCustomRole(writer, request, actor)
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT r.id,r.name,r.description,COALESCE(ARRAY_AGG(DISTINCT p.permission) FILTER(WHERE p.permission IS NOT NULL),'{}'),COUNT(DISTINCT ur.user_id) FROM custom_roles r LEFT JOIN custom_role_permissions p ON p.role_id=r.id LEFT JOIN user_custom_roles ur ON ur.role_id=r.id AND ur.tenant_id=r.tenant_id WHERE r.tenant_id=$1 GROUP BY r.id ORDER BY r.name`, actor.TenantID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load custom roles")
		return
	}
	defer rows.Close()
	items := []customRoleResponse{}
	for rows.Next() {
		var item customRoleResponse
		if rows.Scan(&item.ID, &item.Name, &item.Description, &item.Permissions, &item.MemberCount) != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load custom roles")
			return
		}
		items = append(items, item)
	}
	respondJSON(writer, 200, map[string]any{"roles": items, "permissionCatalog": assignablePermissions})
}

func (api *API) createCustomRole(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	var input struct {
		Name, Description string
		Permissions       []string
	}
	if decodeJSON(writer, request, &input) != nil {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if len(input.Name) < 2 || len(input.Name) > 80 || len(input.Description) > 300 || !validPermissionSet(input.Permissions) {
		errorJSON(writer, 400, "INVALID_ROLE", "name and an allowed permission set are required")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create role")
		return
	}
	defer tx.Rollback()
	var item customRoleResponse
	item.Permissions = uniqueStrings(input.Permissions)
	err = tx.QueryRowContext(request.Context(), `INSERT INTO custom_roles(tenant_id,name,description,created_by) VALUES($1,$2,$3,$4) RETURNING id,name,description`, actor.TenantID, input.Name, input.Description, actor.ID).Scan(&item.ID, &item.Name, &item.Description)
	if err != nil {
		errorJSON(writer, 409, "ROLE_EXISTS", "a custom role with this name already exists")
		return
	}
	for _, permission := range item.Permissions {
		if _, err = tx.ExecContext(request.Context(), `INSERT INTO custom_role_permissions(role_id,permission) VALUES($1,$2)`, item.ID, permission); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not save role permissions")
			return
		}
	}
	if tx.Commit() != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create role")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "rbac.role.create", "custom_role", item.ID, map[string]any{"permissions": item.Permissions})
	respondJSON(writer, 201, map[string]any{"role": item})
}

func (api *API) updateCustomRole(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "roles.manage") {
		errorJSON(writer, 403, "PERMISSION_REQUIRED", "roles.manage permission is required")
		return
	}
	roleID := request.PathValue("roleID")
	if request.Method == http.MethodDelete {
		result, err := api.database.ExecContext(request.Context(), `DELETE FROM custom_roles WHERE id=$1 AND tenant_id=$2`, roleID, actor.TenantID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not delete role")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			errorJSON(writer, 404, "ROLE_NOT_FOUND", "custom role was not found")
			return
		}
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "rbac.role.delete", "custom_role", roleID, nil)
		writer.WriteHeader(204)
		return
	}
	var input struct {
		Name, Description string
		Permissions       []string
	}
	if decodeJSON(writer, request, &input) != nil {
		return
	}
	input.Name = strings.TrimSpace(input.Name)
	input.Description = strings.TrimSpace(input.Description)
	if len(input.Name) < 2 || len(input.Name) > 80 || !validPermissionSet(input.Permissions) {
		errorJSON(writer, 400, "INVALID_ROLE", "invalid custom role")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update role")
		return
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(request.Context(), `UPDATE custom_roles SET name=$1,description=$2,updated_at=NOW() WHERE id=$3 AND tenant_id=$4`, input.Name, input.Description, roleID, actor.TenantID)
	if err != nil {
		errorJSON(writer, 409, "ROLE_EXISTS", "a custom role with this name already exists")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		errorJSON(writer, 404, "ROLE_NOT_FOUND", "custom role was not found")
		return
	}
	_, err = tx.ExecContext(request.Context(), `DELETE FROM custom_role_permissions WHERE role_id=$1`, roleID)
	for _, permission := range uniqueStrings(input.Permissions) {
		if err == nil {
			_, err = tx.ExecContext(request.Context(), `INSERT INTO custom_role_permissions(role_id,permission) VALUES($1,$2)`, roleID, permission)
		}
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update role")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "rbac.role.update", "custom_role", roleID, map[string]any{"permissions": input.Permissions})
	respondJSON(writer, 200, map[string]string{"status": "ok"})
}

func (api *API) customRoleAssignment(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "roles.manage") {
		errorJSON(writer, 403, "PERMISSION_REQUIRED", "roles.manage permission is required")
		return
	}
	roleID, userID := request.PathValue("roleID"), request.PathValue("userID")
	var valid bool
	_ = api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM custom_roles WHERE id=$1 AND tenant_id=$3) AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND tenant_id=$3 AND status='ACTIVE')`, roleID, userID, actor.TenantID).Scan(&valid)
	if !valid {
		errorJSON(writer, 404, "ASSIGNMENT_TARGET_NOT_FOUND", "role or active user was not found")
		return
	}
	action := "rbac.role.assign"
	var err error
	if request.Method == http.MethodPut {
		_, err = api.database.ExecContext(request.Context(), `INSERT INTO user_custom_roles(tenant_id,user_id,role_id,assigned_by) VALUES($1,$2,$3,$4) ON CONFLICT(user_id,role_id) DO NOTHING`, actor.TenantID, userID, roleID, actor.ID)
	} else {
		action = "rbac.role.unassign"
		_, err = api.database.ExecContext(request.Context(), `DELETE FROM user_custom_roles WHERE tenant_id=$1 AND user_id=$2 AND role_id=$3`, actor.TenantID, userID, roleID)
	}
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not update role assignment")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, action, "custom_role", roleID, map[string]any{"userId": userID})
	respondJSON(writer, 200, map[string]string{"status": "ok"})
}

func validPermissionSet(items []string) bool {
	allowed := map[string]bool{}
	for _, item := range assignablePermissions {
		allowed[item] = true
	}
	for _, item := range items {
		if !allowed[item] {
			return false
		}
	}
	return true
}
func uniqueStrings(items []string) []string {
	set := map[string]bool{}
	result := []string{}
	for _, item := range items {
		if !set[item] {
			set[item] = true
			result = append(result, item)
		}
	}
	sort.Strings(result)
	return result
}
