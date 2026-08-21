package httpapi

import (
	"net/http"
	"strings"
	"unicode/utf8"
)

type adminGroupResponse struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	MemberCount int      `json:"memberCount"`
	MemberIDs   []string `json:"memberIds"`
}

func (api *API) adminGroups(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	if request.Method == http.MethodPost {
		api.createAdminGroup(writer, request, actor)
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `
		SELECT g.id,g.name,g.description,COUNT(gm.user_id),
		       COALESCE(ARRAY_AGG(gm.user_id::text ORDER BY gm.created_at) FILTER (WHERE gm.user_id IS NOT NULL),'{}')
		FROM groups g LEFT JOIN group_members gm ON gm.group_id=g.id AND gm.tenant_id=g.tenant_id
		WHERE g.tenant_id=$1 GROUP BY g.id ORDER BY g.name`, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load groups")
		return
	}
	defer rows.Close()
	items := make([]adminGroupResponse, 0)
	for rows.Next() {
		var item adminGroupResponse
		if err = rows.Scan(&item.ID, &item.Name, &item.Description, &item.MemberCount, &item.MemberIDs); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load groups")
			return
		}
		items = append(items, item)
	}
	respondJSON(writer, http.StatusOK, map[string]any{"groups": items})
}

func (api *API) createAdminGroup(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	name, description, ok := decodeGroupInput(writer, request)
	if !ok {
		return
	}
	var item adminGroupResponse
	item.MemberIDs = []string{}
	err := api.database.QueryRowContext(request.Context(), `INSERT INTO groups (tenant_id,name,description,created_by) VALUES ($1,$2,$3,$4) RETURNING id,name,description`, actor.TenantID, name, description, actor.ID).Scan(&item.ID, &item.Name, &item.Description)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "GROUP_EXISTS", "a group with this name already exists")
		return
	}
	api.auditGroup(request, actor, "group.create", item.ID)
	respondJSON(writer, http.StatusCreated, map[string]any{"group": item})
}

func (api *API) updateAdminGroup(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	groupID := request.PathValue("groupID")
	if request.Method == http.MethodDelete {
		result, err := api.database.ExecContext(request.Context(), `DELETE FROM groups WHERE id=$1 AND tenant_id=$2`, groupID, actor.TenantID)
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not delete group")
			return
		}
		affected, _ := result.RowsAffected()
		if affected == 0 {
			errorJSON(writer, http.StatusNotFound, "GROUP_NOT_FOUND", "group was not found")
			return
		}
		api.auditGroup(request, actor, "group.delete", groupID)
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	name, description, ok := decodeGroupInput(writer, request)
	if !ok {
		return
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE groups SET name=$1,description=$2,updated_at=NOW() WHERE id=$3 AND tenant_id=$4`, name, description, groupID, actor.TenantID)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "GROUP_EXISTS", "a group with this name already exists")
		return
	}
	affected, _ := result.RowsAffected()
	if affected == 0 {
		errorJSON(writer, http.StatusNotFound, "GROUP_NOT_FOUND", "group was not found")
		return
	}
	api.auditGroup(request, actor, "group.update", groupID)
	respondJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (api *API) updateAdminGroupMember(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !actor.Role.isWorkspaceAdmin() {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	groupID, userID := request.PathValue("groupID"), request.PathValue("userID")
	var exists bool
	err := api.database.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND tenant_id=$3) AND EXISTS(SELECT 1 FROM users WHERE id=$2 AND tenant_id=$3 AND status!='DEACTIVATED')`, groupID, userID, actor.TenantID).Scan(&exists)
	if err != nil || !exists {
		errorJSON(writer, http.StatusNotFound, "GROUP_MEMBER_TARGET_NOT_FOUND", "group or user was not found")
		return
	}
	action := "group.member.add"
	if request.Method == http.MethodPut {
		_, err = api.database.ExecContext(request.Context(), `INSERT INTO group_members (group_id,tenant_id,user_id,added_by) VALUES ($1,$2,$3,$4) ON CONFLICT (group_id,user_id) DO NOTHING`, groupID, actor.TenantID, userID, actor.ID)
	} else {
		action = "group.member.remove"
		_, err = api.database.ExecContext(request.Context(), `DELETE FROM group_members WHERE group_id=$1 AND tenant_id=$2 AND user_id=$3`, groupID, actor.TenantID, userID)
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update group membership")
		return
	}
	api.auditGroup(request, actor, action, groupID)
	respondJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func decodeGroupInput(writer http.ResponseWriter, request *http.Request) (string, string, bool) {
	var input struct{ Name, Description string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return "", "", false
	}
	input.Name, input.Description = strings.TrimSpace(input.Name), strings.TrimSpace(input.Description)
	if utf8.RuneCountInString(input.Name) < 2 || utf8.RuneCountInString(input.Name) > 80 || utf8.RuneCountInString(input.Description) > 240 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_GROUP", "name must be 2-80 characters and description at most 240 characters")
		return "", "", false
	}
	return input.Name, input.Description, true
}

func (api *API) auditGroup(request *http.Request, actor currentUser, action, groupID string) {
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, action, "group", groupID, nil)
}
