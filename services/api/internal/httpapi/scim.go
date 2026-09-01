package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"
	"strings"

	passwordauth "github.com/cankonix/xpace/api/internal/auth"
)

const scimUserSchema = "urn:ietf:params:scim:schemas:core:2.0:User"
const scimGroupSchema = "urn:ietf:params:scim:schemas:core:2.0:Group"
const scimListSchema = "urn:ietf:params:scim:api:messages:2.0:ListResponse"

type scimContext struct{ TenantID, TenantSlug, ActorID string }
type scimEmail struct {
	Value   string `json:"value"`
	Type    string `json:"type"`
	Primary bool   `json:"primary"`
}
type scimUserInput struct {
	Schemas     []string                                          `json:"schemas"`
	ID          string                                            `json:"id,omitempty"`
	ExternalID  string                                            `json:"externalId,omitempty"`
	UserName    string                                            `json:"userName"`
	DisplayName string                                            `json:"displayName"`
	Active      *bool                                             `json:"active,omitempty"`
	Name        struct{ Formatted, GivenName, FamilyName string } `json:"name"`
	Emails      []scimEmail                                       `json:"emails"`
}
type scimGroupInput struct {
	Schemas     []string                          `json:"schemas"`
	ID          string                            `json:"id,omitempty"`
	ExternalID  string                            `json:"externalId,omitempty"`
	DisplayName string                            `json:"displayName"`
	Members     []struct{ Value, Display string } `json:"members"`
}
type scimPatchInput struct {
	Schemas    []string             `json:"schemas"`
	Operations []scimPatchOperation `json:"Operations"`
}
type scimPatchOperation struct {
	Op    string          `json:"op"`
	Path  string          `json:"path"`
	Value json.RawMessage `json:"value"`
}

func (api *API) adminSCIMConfiguration(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "identity.manage") {
		errorJSON(writer, 403, "PERMISSION_REQUIRED", "identity.manage permission is required")
		return
	}
	if request.Method == http.MethodGet {
		var enabled bool
		var rotated any
		err := api.database.QueryRowContext(request.Context(), `SELECT enabled,rotated_at FROM tenant_scim_configurations WHERE tenant_id=$1`, actor.TenantID).Scan(&enabled, &rotated)
		respondJSON(writer, 200, map[string]any{"configured": err == nil, "enabled": err == nil && enabled, "rotatedAt": rotated, "baseUrl": scimBaseURL(actor.TenantSlug)})
		return
	}
	if request.Method == http.MethodDelete {
		_, err := api.database.ExecContext(request.Context(), `UPDATE tenant_scim_configurations SET enabled=FALSE,rotated_at=NOW() WHERE tenant_id=$1`, actor.TenantID)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not disable SCIM")
			return
		}
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "identity.scim.disable", "tenant", actor.TenantID, nil)
		writer.WriteHeader(204)
		return
	}
	token, err := randomToken(32)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create SCIM token")
		return
	}
	_, err = api.database.ExecContext(request.Context(), `INSERT INTO tenant_scim_configurations(tenant_id,token_hash,enabled,created_by,rotated_at) VALUES($1,$2,TRUE,$3,NOW()) ON CONFLICT(tenant_id) DO UPDATE SET token_hash=EXCLUDED.token_hash,enabled=TRUE,created_by=EXCLUDED.created_by,rotated_at=NOW()`, actor.TenantID, hashToken(token), actor.ID)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not save SCIM token")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "identity.scim.token.rotate", "tenant", actor.TenantID, nil)
	respondJSON(writer, 201, map[string]any{"token": token, "baseUrl": scimBaseURL(actor.TenantSlug)})
}

func (api *API) scimServiceProvider(writer http.ResponseWriter, request *http.Request) {
	if _, ok := api.requireSCIM(writer, request); !ok {
		return
	}
	scimJSON(writer, 200, map[string]any{"schemas": []string{"urn:ietf:params:scim:schemas:core:2.0:ServiceProviderConfig"}, "patch": map[string]bool{"supported": true}, "bulk": map[string]any{"supported": false, "maxOperations": 0, "maxPayloadSize": 0}, "filter": map[string]any{"supported": true, "maxResults": 200}, "changePassword": map[string]bool{"supported": false}, "sort": map[string]bool{"supported": false}, "etag": map[string]bool{"supported": false}, "authenticationSchemes": []map[string]any{{"type": "oauthbearertoken", "name": "Bearer Token", "description": "Tenant-scoped SCIM bearer token", "specUri": "https://www.rfc-editor.org/rfc/rfc6750", "primary": true}}})
}

func (api *API) scimUsers(writer http.ResponseWriter, request *http.Request) {
	ctx, ok := api.requireSCIM(writer, request)
	if !ok {
		return
	}
	if request.Method == http.MethodPost {
		api.scimCreateUser(writer, request, ctx)
		return
	}
	start, count := scimPagination(request)
	filter := strings.TrimSpace(request.URL.Query().Get("filter"))
	username := ""
	if strings.HasPrefix(filter, "userName eq \"") && strings.HasSuffix(filter, "\"") {
		username = strings.TrimSuffix(strings.TrimPrefix(filter, "userName eq \""), "\"")
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,COALESCE(scim_external_id,''),username,email,display_name,status='ACTIVE' FROM users WHERE tenant_id=$1 AND ($2='' OR LOWER(username)=LOWER($2)) ORDER BY created_at LIMIT $3 OFFSET $4`, ctx.TenantID, username, count, start-1)
	if err != nil {
		scimError(writer, 500, "could not list users")
		return
	}
	defer rows.Close()
	resources := []any{}
	for rows.Next() {
		var id, external, userName, email, display string
		var active bool
		if rows.Scan(&id, &external, &userName, &email, &display, &active) == nil {
			resources = append(resources, scimUserResource(ctx.TenantSlug, id, external, userName, email, display, active))
		}
	}
	scimJSON(writer, 200, map[string]any{"schemas": []string{scimListSchema}, "totalResults": len(resources), "startIndex": start, "itemsPerPage": len(resources), "Resources": resources})
}

func (api *API) scimUser(writer http.ResponseWriter, request *http.Request) {
	ctx, ok := api.requireSCIM(writer, request)
	if !ok {
		return
	}
	id := request.PathValue("resourceID")
	if request.Method == http.MethodPatch {
		api.scimPatchUser(writer, request, ctx, id)
		return
	}
	if request.Method == http.MethodDelete {
		result, err := api.database.ExecContext(request.Context(), `UPDATE users SET status='DEACTIVATED',updated_at=NOW() WHERE id=$1 AND tenant_id=$2 AND managed_by_scim=TRUE`, id, ctx.TenantID)
		if err != nil {
			scimError(writer, 500, "could not deactivate user")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			scimError(writer, 404, "user not found")
			return
		}
		_, _ = api.database.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, id)
		api.auditSCIM(request, ctx, "scim.user.deactivate", id)
		writer.WriteHeader(204)
		return
	}
	if request.Method == http.MethodPut {
		var input scimUserInput
		if !decodeSCIM(writer, request, &input) {
			return
		}
		api.scimReplaceUser(writer, request, ctx, id, input)
		return
	}
	var external, userName, email, display string
	var active bool
	err := api.database.QueryRowContext(request.Context(), `SELECT COALESCE(scim_external_id,''),username,email,display_name,status='ACTIVE' FROM users WHERE id=$1 AND tenant_id=$2`, id, ctx.TenantID).Scan(&external, &userName, &email, &display, &active)
	if err != nil {
		scimError(writer, 404, "user not found")
		return
	}
	scimJSON(writer, 200, scimUserResource(ctx.TenantSlug, id, external, userName, email, display, active))
}

func (api *API) scimCreateUser(writer http.ResponseWriter, request *http.Request, ctx scimContext) {
	var input scimUserInput
	if !decodeSCIM(writer, request, &input) {
		return
	}
	email, display, active, message := normalizeSCIMUser(input)
	if message != "" {
		scimError(writer, 400, message)
		return
	}
	password, _ := randomToken(32)
	hash, err := passwordauth.HashPassword(password)
	if err != nil {
		scimError(writer, 500, "could not create user")
		return
	}
	status := "ACTIVE"
	if !active {
		status = "DEACTIVATED"
	} else if err = api.enforceTenantQuota(request.Context(), ctx.TenantID, "users", 1); err != nil {
		var quota *entitlementError
		if errors.As(err, &quota) {
			scimError(writer, quota.status, quota.message)
		} else {
			scimError(writer, 500, "could not verify workspace quota")
		}
		return
	}
	var id string
	err = api.database.QueryRowContext(request.Context(), `INSERT INTO users(tenant_id,email,username,display_name,password_hash,role,status,scim_external_id,managed_by_scim) VALUES($1,$2,$3,$4,$5,'MEMBER',$6,$7,TRUE) RETURNING id`, ctx.TenantID, email, strings.ToLower(strings.TrimSpace(input.UserName)), display, hash, status, nullString(input.ExternalID)).Scan(&id)
	if err != nil {
		scimError(writer, 409, "userName, email, or externalId already exists")
		return
	}
	_, _ = api.database.ExecContext(request.Context(), `INSERT INTO user_profiles(user_id,tenant_id) VALUES($1,$2) ON CONFLICT DO NOTHING`, id, ctx.TenantID)
	api.auditSCIM(request, ctx, "scim.user.create", id)
	writer.Header().Set("Location", scimBaseURL(ctx.TenantSlug)+"/Users/"+id)
	scimJSON(writer, 201, scimUserResource(ctx.TenantSlug, id, input.ExternalID, input.UserName, email, display, active))
}

func (api *API) scimReplaceUser(writer http.ResponseWriter, request *http.Request, ctx scimContext, id string, input scimUserInput) {
	email, display, active, message := normalizeSCIMUser(input)
	if message != "" {
		scimError(writer, 400, message)
		return
	}
	status := "ACTIVE"
	if !active {
		status = "DEACTIVATED"
	}
	result, err := api.database.ExecContext(request.Context(), `UPDATE users SET email=$1,username=$2,display_name=$3,status=$4,scim_external_id=$5,updated_at=NOW() WHERE id=$6 AND tenant_id=$7 AND managed_by_scim=TRUE`, email, strings.ToLower(strings.TrimSpace(input.UserName)), display, status, nullString(input.ExternalID), id, ctx.TenantID)
	if err != nil {
		scimError(writer, 409, "userName, email, or externalId already exists")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		scimError(writer, 404, "user not found")
		return
	}
	if !active {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	}
	api.auditSCIM(request, ctx, "scim.user.replace", id)
	scimJSON(writer, 200, scimUserResource(ctx.TenantSlug, id, input.ExternalID, input.UserName, email, display, active))
}

func (api *API) scimGroups(writer http.ResponseWriter, request *http.Request) {
	ctx, ok := api.requireSCIM(writer, request)
	if !ok {
		return
	}
	if request.Method == http.MethodPost {
		var input scimGroupInput
		if !decodeSCIM(writer, request, &input) {
			return
		}
		api.scimCreateGroup(writer, request, ctx, input)
		return
	}
	start, count := scimPagination(request)
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,COALESCE(scim_external_id,''),name FROM groups WHERE tenant_id=$1 ORDER BY created_at LIMIT $2 OFFSET $3`, ctx.TenantID, count, start-1)
	if err != nil {
		scimError(writer, 500, "could not list groups")
		return
	}
	defer rows.Close()
	resources := []any{}
	for rows.Next() {
		var id, external, name string
		if rows.Scan(&id, &external, &name) == nil {
			resources = append(resources, scimGroupResource(ctx.TenantSlug, id, external, name, []map[string]string{}))
		}
	}
	scimJSON(writer, 200, map[string]any{"schemas": []string{scimListSchema}, "totalResults": len(resources), "startIndex": start, "itemsPerPage": len(resources), "Resources": resources})
}

func (api *API) scimGroup(writer http.ResponseWriter, request *http.Request) {
	ctx, ok := api.requireSCIM(writer, request)
	if !ok {
		return
	}
	id := request.PathValue("resourceID")
	if request.Method == http.MethodPatch {
		api.scimPatchGroup(writer, request, ctx, id)
		return
	}
	if request.Method == http.MethodDelete {
		result, err := api.database.ExecContext(request.Context(), `DELETE FROM groups WHERE id=$1 AND tenant_id=$2 AND managed_by_scim=TRUE`, id, ctx.TenantID)
		if err != nil {
			scimError(writer, 500, "could not delete group")
			return
		}
		count, _ := result.RowsAffected()
		if count == 0 {
			scimError(writer, 404, "SCIM-managed group not found")
			return
		}
		api.auditSCIM(request, ctx, "scim.group.delete", id)
		writer.WriteHeader(204)
		return
	}
	if request.Method == http.MethodPut {
		var input scimGroupInput
		if !decodeSCIM(writer, request, &input) {
			return
		}
		api.scimReplaceGroup(writer, request, ctx, id, input)
		return
	}
	resource, err := api.loadSCIMGroup(request, ctx, id)
	if err != nil {
		scimError(writer, 404, "group not found")
		return
	}
	scimJSON(writer, 200, resource)
}

func (api *API) scimCreateGroup(writer http.ResponseWriter, request *http.Request, ctx scimContext, input scimGroupInput) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	if len(input.DisplayName) < 2 {
		scimError(writer, 400, "displayName is required")
		return
	}
	var id string
	err := api.database.QueryRowContext(request.Context(), `INSERT INTO groups(tenant_id,name,description,created_by,scim_external_id,managed_by_scim) VALUES($1,$2,'SCIM managed group',$3,$4,TRUE) RETURNING id`, ctx.TenantID, input.DisplayName, ctx.ActorID, nullString(input.ExternalID)).Scan(&id)
	if err != nil {
		scimError(writer, 409, "group name or externalId already exists")
		return
	}
	if !api.replaceSCIMMembers(request, ctx, id, input.Members) {
		_, _ = api.database.ExecContext(request.Context(), `DELETE FROM groups WHERE id=$1 AND tenant_id=$2 AND managed_by_scim=TRUE`, id, ctx.TenantID)
		scimError(writer, 400, "one or more members are invalid")
		return
	}
	api.auditSCIM(request, ctx, "scim.group.create", id)
	resource, _ := api.loadSCIMGroup(request, ctx, id)
	writer.Header().Set("Location", scimBaseURL(ctx.TenantSlug)+"/Groups/"+id)
	scimJSON(writer, 201, resource)
}
func (api *API) scimReplaceGroup(writer http.ResponseWriter, request *http.Request, ctx scimContext, id string, input scimGroupInput) {
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	result, err := api.database.ExecContext(request.Context(), `UPDATE groups SET name=$1,scim_external_id=$2,managed_by_scim=TRUE,updated_at=NOW() WHERE id=$3 AND tenant_id=$4`, input.DisplayName, nullString(input.ExternalID), id, ctx.TenantID)
	if err != nil {
		scimError(writer, 409, "group name or externalId already exists")
		return
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		scimError(writer, 404, "group not found")
		return
	}
	if !api.replaceSCIMMembers(request, ctx, id, input.Members) {
		scimError(writer, 400, "one or more members are invalid")
		return
	}
	api.auditSCIM(request, ctx, "scim.group.replace", id)
	resource, _ := api.loadSCIMGroup(request, ctx, id)
	scimJSON(writer, 200, resource)
}

func (api *API) replaceSCIMMembers(request *http.Request, ctx scimContext, groupID string, members []struct{ Value, Display string }) bool {
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		return false
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(request.Context(), `DELETE FROM group_members WHERE group_id=$1 AND tenant_id=$2`, groupID, ctx.TenantID); err != nil {
		return false
	}
	for _, member := range members {
		var valid bool
		_ = tx.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND tenant_id=$2 AND status!='DEACTIVATED')`, member.Value, ctx.TenantID).Scan(&valid)
		if !valid {
			return false
		}
		if _, err = tx.ExecContext(request.Context(), `INSERT INTO group_members(group_id,tenant_id,user_id,added_by) VALUES($1,$2,$3,$4)`, groupID, ctx.TenantID, member.Value, ctx.ActorID); err != nil {
			return false
		}
	}
	return tx.Commit() == nil
}
func (api *API) loadSCIMGroup(request *http.Request, ctx scimContext, id string) (any, error) {
	var external, name string
	err := api.database.QueryRowContext(request.Context(), `SELECT COALESCE(scim_external_id,''),name FROM groups WHERE id=$1 AND tenant_id=$2`, id, ctx.TenantID).Scan(&external, &name)
	if err != nil {
		return nil, err
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT u.id,u.display_name FROM group_members gm JOIN users u ON u.id=gm.user_id AND u.tenant_id=gm.tenant_id WHERE gm.group_id=$1 AND gm.tenant_id=$2`, id, ctx.TenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	members := []map[string]string{}
	for rows.Next() {
		var userID, display string
		if rows.Scan(&userID, &display) == nil {
			members = append(members, map[string]string{"value": userID, "display": display})
		}
	}
	return scimGroupResource(ctx.TenantSlug, id, external, name, members), nil
}

func (api *API) scimPatchUser(writer http.ResponseWriter, request *http.Request, ctx scimContext, id string) {
	var input scimPatchInput
	if !decodeSCIM(writer, request, &input) || len(input.Operations) == 0 {
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		scimError(writer, 500, "could not patch user")
		return
	}
	defer tx.Rollback()
	var exists bool
	_ = tx.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM users WHERE id=$1 AND tenant_id=$2 AND managed_by_scim=TRUE)`, id, ctx.TenantID).Scan(&exists)
	if !exists {
		scimError(writer, 404, "SCIM-managed user not found")
		return
	}
	deactivated := false
	for _, operation := range input.Operations {
		if !strings.EqualFold(operation.Op, "replace") {
			scimError(writer, 400, "user PATCH supports replace operations")
			return
		}
		path := strings.ToLower(strings.TrimSpace(operation.Path))
		switch path {
		case "active":
			var value bool
			if json.Unmarshal(operation.Value, &value) != nil {
				scimError(writer, 400, "active must be boolean")
				return
			}
			status := "DEACTIVATED"
			if value {
				status = "ACTIVE"
			}
			_, err = tx.ExecContext(request.Context(), `UPDATE users SET status=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, status, id, ctx.TenantID)
			deactivated = !value
		case "username":
			var value string
			_ = json.Unmarshal(operation.Value, &value)
			value = strings.ToLower(strings.TrimSpace(value))
			if len(value) < 2 {
				scimError(writer, 400, "userName is invalid")
				return
			}
			_, err = tx.ExecContext(request.Context(), `UPDATE users SET username=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, value, id, ctx.TenantID)
		case "displayname":
			var value string
			_ = json.Unmarshal(operation.Value, &value)
			value = strings.TrimSpace(value)
			if len(value) < 2 {
				scimError(writer, 400, "displayName is invalid")
				return
			}
			_, err = tx.ExecContext(request.Context(), `UPDATE users SET display_name=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, value, id, ctx.TenantID)
		case "externalid":
			var value string
			_ = json.Unmarshal(operation.Value, &value)
			_, err = tx.ExecContext(request.Context(), `UPDATE users SET scim_external_id=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, nullString(value), id, ctx.TenantID)
		default:
			scimError(writer, 400, "unsupported user PATCH path")
			return
		}
		if err != nil {
			scimError(writer, 409, "patched value conflicts with another user")
			return
		}
	}
	if tx.Commit() != nil {
		scimError(writer, 500, "could not patch user")
		return
	}
	if deactivated {
		_, _ = api.database.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, id)
	}
	api.auditSCIM(request, ctx, "scim.user.patch", id)
	var external, userName, email, display string
	var active bool
	if api.database.QueryRowContext(request.Context(), `SELECT COALESCE(scim_external_id,''),username,email,display_name,status='ACTIVE' FROM users WHERE id=$1 AND tenant_id=$2`, id, ctx.TenantID).Scan(&external, &userName, &email, &display, &active) != nil {
		scimError(writer, 404, "user not found")
		return
	}
	scimJSON(writer, 200, scimUserResource(ctx.TenantSlug, id, external, userName, email, display, active))
}

func (api *API) scimPatchGroup(writer http.ResponseWriter, request *http.Request, ctx scimContext, id string) {
	var input scimPatchInput
	if !decodeSCIM(writer, request, &input) || len(input.Operations) == 0 {
		return
	}
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		scimError(writer, 500, "could not patch group")
		return
	}
	defer tx.Rollback()
	var exists bool
	_ = tx.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM groups WHERE id=$1 AND tenant_id=$2 AND managed_by_scim=TRUE)`, id, ctx.TenantID).Scan(&exists)
	if !exists {
		scimError(writer, 404, "SCIM-managed group not found")
		return
	}
	for _, operation := range input.Operations {
		op, path := strings.ToLower(operation.Op), strings.ToLower(strings.TrimSpace(operation.Path))
		if path == "displayname" && op == "replace" {
			var name string
			_ = json.Unmarshal(operation.Value, &name)
			name = strings.TrimSpace(name)
			if len(name) < 2 {
				scimError(writer, 400, "displayName is invalid")
				return
			}
			_, err = tx.ExecContext(request.Context(), `UPDATE groups SET name=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, name, id, ctx.TenantID)
			if err != nil {
				scimError(writer, 409, "group name already exists")
				return
			}
			continue
		}
		if !strings.HasPrefix(path, "members") {
			scimError(writer, 400, "unsupported group PATCH path")
			return
		}
		members := scimPatchMembers(operation.Value)
		if op == "replace" {
			if _, err = tx.ExecContext(request.Context(), `DELETE FROM group_members WHERE group_id=$1 AND tenant_id=$2`, id, ctx.TenantID); err != nil {
				scimError(writer, 500, "could not replace members")
				return
			}
		}
		if op == "remove" && len(members) == 0 {
			if value := memberFromFilter(path); value != "" {
				members = []string{value}
			}
		}
		for _, userID := range members {
			if op == "remove" {
				_, err = tx.ExecContext(request.Context(), `DELETE FROM group_members WHERE group_id=$1 AND tenant_id=$2 AND user_id=$3`, id, ctx.TenantID, userID)
			} else if op == "add" || op == "replace" {
				var count int
				err = tx.QueryRowContext(request.Context(), `WITH inserted AS (INSERT INTO group_members(group_id,tenant_id,user_id,added_by) SELECT $1,$2,id,$4 FROM users WHERE id=$3 AND tenant_id=$2 AND status!='DEACTIVATED' ON CONFLICT DO NOTHING RETURNING 1) SELECT COUNT(*) FROM inserted`, id, ctx.TenantID, userID, ctx.ActorID).Scan(&count)
				if err == nil && count == 0 {
					var already bool
					_ = tx.QueryRowContext(request.Context(), `SELECT EXISTS(SELECT 1 FROM group_members WHERE group_id=$1 AND user_id=$2)`, id, userID).Scan(&already)
					if !already {
						err = fmt.Errorf("invalid member")
					}
				}
			} else {
				scimError(writer, 400, "group PATCH supports add, remove, or replace")
				return
			}
			if err != nil {
				scimError(writer, 400, "one or more members are invalid")
				return
			}
		}
	}
	if tx.Commit() != nil {
		scimError(writer, 500, "could not patch group")
		return
	}
	api.auditSCIM(request, ctx, "scim.group.patch", id)
	resource, err := api.loadSCIMGroup(request, ctx, id)
	if err != nil {
		scimError(writer, 404, "group not found")
		return
	}
	scimJSON(writer, 200, resource)
}

func scimPatchMembers(raw json.RawMessage) []string {
	var direct []struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &direct) == nil {
		return memberIDs(direct)
	}
	var wrapper struct {
		Members []struct {
			Value string `json:"value"`
		} `json:"members"`
	}
	if json.Unmarshal(raw, &wrapper) == nil {
		return memberIDs(wrapper.Members)
	}
	return nil
}
func memberIDs(items []struct {
	Value string `json:"value"`
}) []string {
	result := []string{}
	for _, item := range items {
		if value := strings.TrimSpace(item.Value); value != "" {
			result = append(result, value)
		}
	}
	return result
}
func memberFromFilter(path string) string {
	start := strings.Index(path, "value eq \"")
	if start < 0 {
		return ""
	}
	value := path[start+10:]
	end := strings.Index(value, "\"")
	if end < 0 {
		return ""
	}
	return value[:end]
}

func (api *API) requireSCIM(writer http.ResponseWriter, request *http.Request) (scimContext, bool) {
	token := strings.TrimSpace(strings.TrimPrefix(request.Header.Get("Authorization"), "Bearer "))
	if token == "" || !strings.HasPrefix(request.Header.Get("Authorization"), "Bearer ") {
		scimError(writer, 401, "valid bearer token is required")
		return scimContext{}, false
	}
	var ctx scimContext
	err := api.database.QueryRowContext(request.Context(), `SELECT c.tenant_id,t.slug,c.created_by FROM tenant_scim_configurations c JOIN tenants t ON t.id=c.tenant_id WHERE t.slug=$1 AND c.token_hash=$2 AND c.enabled=TRUE`, strings.ToLower(request.PathValue("tenant")), hashToken(token)).Scan(&ctx.TenantID, &ctx.TenantSlug, &ctx.ActorID)
	if err != nil {
		scimError(writer, 401, "valid bearer token is required")
		return ctx, false
	}
	return ctx, true
}
func decodeSCIM(writer http.ResponseWriter, request *http.Request, value any) bool {
	mediaType, _, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || (mediaType != "application/scim+json" && mediaType != "application/json") {
		scimError(writer, 415, "Content-Type must be application/scim+json")
		return false
	}
	decoder := json.NewDecoder(io.LimitReader(request.Body, maxRequestBody))
	if decoder.Decode(value) != nil {
		scimError(writer, 400, "request body must be valid SCIM JSON")
		return false
	}
	return true
}
func normalizeSCIMUser(input scimUserInput) (string, string, bool, string) {
	email := ""
	for _, item := range input.Emails {
		if item.Primary || email == "" {
			email = strings.ToLower(strings.TrimSpace(item.Value))
		}
	}
	display := strings.TrimSpace(input.DisplayName)
	if display == "" {
		display = strings.TrimSpace(input.Name.Formatted)
	}
	active := true
	if input.Active != nil {
		active = *input.Active
	}
	if len(strings.TrimSpace(input.UserName)) < 2 || !strings.Contains(email, "@") || len(display) < 2 {
		return email, display, active, "userName, displayName, and a valid email are required"
	}
	return email, display, active, ""
}
func scimPagination(request *http.Request) (int, int) {
	start, _ := strconv.Atoi(request.URL.Query().Get("startIndex"))
	count, _ := strconv.Atoi(request.URL.Query().Get("count"))
	if start < 1 {
		start = 1
	}
	if count < 1 || count > 200 {
		count = 100
	}
	return start, count
}
func scimUserResource(slug, id, external, userName, email, display string, active bool) map[string]any {
	return map[string]any{"schemas": []string{scimUserSchema}, "id": id, "externalId": external, "userName": userName, "displayName": display, "active": active, "emails": []map[string]any{{"value": email, "type": "work", "primary": true}}, "meta": map[string]string{"resourceType": "User", "location": scimBaseURL(slug) + "/Users/" + id}}
}
func scimGroupResource(slug, id, external, name string, members []map[string]string) map[string]any {
	return map[string]any{"schemas": []string{scimGroupSchema}, "id": id, "externalId": external, "displayName": name, "members": members, "meta": map[string]string{"resourceType": "Group", "location": scimBaseURL(slug) + "/Groups/" + id}}
}
func scimBaseURL(slug string) string {
	base := strings.TrimRight(os.Getenv("XPACE_PUBLIC_URL"), "/")
	if base == "" {
		base = "http://localhost:3300"
	}
	return base + "/api/v1/scim/v2/" + slug
}
func scimJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/scim+json; charset=utf-8")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}
func scimError(writer http.ResponseWriter, status int, detail string) {
	scimJSON(writer, status, map[string]any{"schemas": []string{"urn:ietf:params:scim:api:messages:2.0:Error"}, "status": fmt.Sprint(status), "detail": detail})
}
func nullString(value string) any {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return value
}
func (api *API) auditSCIM(request *http.Request, ctx scimContext, action, id string) {
	_ = api.writeAuditEvent(request.Context(), request, ctx.TenantID, ctx.ActorID, action, "scim", id, nil)
}
