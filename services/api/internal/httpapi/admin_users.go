package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
	"time"

	"github.com/cankonix/xpace/api/internal/auth"
	"github.com/minio/minio-go/v7"
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

type createAdminUserResponse struct {
	User           adminUserResponse `json:"user"`
	InvitationPath string            `json:"invitationPath,omitempty"`
}

func (api *API) adminUsers(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "users.manage") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	if request.Method == http.MethodPost {
		api.createAdminUser(writer, request, actor)
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,email,username,display_name,role,status,created_at FROM users WHERE tenant_id=$1 AND deleted_at IS NULL ORDER BY created_at DESC LIMIT 200`, actor.TenantID)
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
		Email, Username, DisplayName, Password, PasswordConfirm string
		Role                                                    userRole
		Status                                                  string
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
	invitationToken := ""
	var err error
	if input.Status == "INVITED" {
		password, err = randomToken(32)
		if err == nil {
			invitationToken, err = randomToken(32)
		}
		if err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create invitation")
			return
		}
	} else if input.Password != input.PasswordConfirm {
		errorJSON(writer, http.StatusBadRequest, "PASSWORD_MISMATCH", "password and password confirmation must match")
		return
	}
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "WEAK_PASSWORD", err.Error())
		return
	}
	if err = api.enforceTenantQuota(request.Context(), actor.TenantID, "users", 1); err != nil {
		if !respondEntitlementError(writer, err) {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not verify workspace quota")
		}
		return
	}
	var item adminUserResponse
	tx, err := api.database.BeginTx(request.Context(), nil)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create user")
		return
	}
	defer tx.Rollback()
	err = tx.QueryRowContext(request.Context(), `INSERT INTO users (tenant_id,email,username,display_name,password_hash,role,status) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING id,email,username,display_name,role,status,created_at`, actor.TenantID, input.Email, input.Username, input.DisplayName, passwordHash, input.Role, input.Status).Scan(&item.ID, &item.Email, &item.Username, &item.DisplayName, &item.Role, &item.Status, &item.CreatedAt)
	if err != nil {
		errorJSON(writer, http.StatusConflict, "USER_EXISTS", "email or username already exists in this tenant")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `INSERT INTO user_profiles (user_id,tenant_id) VALUES ($1,$2) ON CONFLICT (user_id) DO NOTHING`, item.ID, actor.TenantID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create user profile")
		return
	}
	if invitationToken != "" {
		var invitationID string
		if err = tx.QueryRowContext(request.Context(), `INSERT INTO user_invitations (tenant_id,user_id,created_by,token_hash,expires_at) VALUES ($1,$2,$3,$4,NOW()+INTERVAL '72 hours') RETURNING id`, actor.TenantID, item.ID, actor.ID, hashToken(invitationToken)).Scan(&invitationID); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create invitation")
			return
		}
		if err = queueTransactionalEmail(request.Context(), tx, actor.TenantID, item.Email, "INVITATION", invitationToken, "invitation:"+invitationID, map[string]any{"workspace": actor.TenantName, "inviter": actor.DisplayName}); err != nil {
			errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not queue invitation email")
			return
		}
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not create user")
		return
	}
	api.auditUserManagement(request, actor, "user.create", item.ID)
	response := createAdminUserResponse{User: item}
	if invitationToken != "" {
		response.InvitationPath = "/accept-invite?token=" + invitationToken
	}
	respondJSON(writer, http.StatusCreated, response)
}

func (api *API) updateAdminUser(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "users.manage") {
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

func (api *API) deleteAdminUser(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	if !api.hasPermission(request.Context(), actor, "users.manage") {
		errorJSON(writer, http.StatusForbidden, "ADMIN_REQUIRED", "workspace administrator access is required")
		return
	}
	targetID := strings.TrimSpace(request.PathValue("userID"))
	if targetID == "" {
		errorJSON(writer, http.StatusNotFound, "USER_NOT_FOUND", "user was not found")
		return
	}
	if targetID == actor.ID {
		errorJSON(writer, http.StatusConflict, "SELF_PROTECTION", "administrators cannot permanently delete their own account")
		return
	}
	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not delete user")
		return
	}
	defer tx.Rollback()
	var role userRole
	var status, email, avatarObjectKey string
	err = tx.QueryRowContext(request.Context(), `SELECT u.role,u.status,u.email,COALESCE(p.avatar_url,'') FROM users u LEFT JOIN user_profiles p ON p.user_id=u.id AND p.tenant_id=u.tenant_id WHERE u.id=$1 AND u.tenant_id=$2 AND u.deleted_at IS NULL FOR UPDATE OF u`, targetID, actor.TenantID).Scan(&role, &status, &email, &avatarObjectKey)
	if err == sql.ErrNoRows {
		errorJSON(writer, http.StatusNotFound, "USER_NOT_FOUND", "user was not found")
		return
	}
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not delete user")
		return
	}
	if !userDeletionEligible(role, status) {
		if role == roleSuperAdmin {
			errorJSON(writer, http.StatusConflict, "PROTECTED_ACCOUNT", "super administrator accounts cannot be permanently deleted")
			return
		}
		errorJSON(writer, http.StatusConflict, "DEACTIVATION_REQUIRED", "deactivate this account before permanently deleting it")
		return
	}
	suffix := strings.ReplaceAll(targetID, "-", "")
	anonymousEmail, anonymousUsername := "deleted+"+suffix+"@xspace.invalid", "deleted-"+suffix
	if _, err = tx.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL`, targetID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not revoke user sessions")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE user_invitations SET accepted_at=COALESCE(accepted_at,NOW()) WHERE tenant_id=$1 AND user_id=$2`, actor.TenantID, targetID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not invalidate user invitations")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE email_verification_tokens SET used_at=COALESCE(used_at,NOW()) WHERE tenant_id=$1 AND user_id=$2`, actor.TenantID, targetID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not invalidate user credentials")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE password_reset_tokens SET used_at=COALESCE(used_at,NOW()) WHERE tenant_id=$1 AND user_id=$2`, actor.TenantID, targetID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not invalidate user credentials")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE email_outbox SET recipient_email=$1,token_encrypted='',status=CASE WHEN status IN ('PENDING','PROCESSING') THEN 'FAILED' ELSE status END,last_error=CASE WHEN status IN ('PENDING','PROCESSING') THEN 'account deleted' ELSE last_error END,updated_at=NOW() WHERE tenant_id=$2 AND LOWER(recipient_email)=LOWER($3)`, anonymousEmail, actor.TenantID, email); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not remove queued email identity")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE user_profiles SET bio='',avatar_url=NULL,updated_at=NOW() WHERE user_id=$1 AND tenant_id=$2`, targetID, actor.TenantID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not clear user profile")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE users SET email=$1,username=$2,display_name='Deleted user',password_hash='$deleted$',status='DEACTIVATED',deleted_at=NOW(),updated_at=NOW() WHERE id=$3 AND tenant_id=$4`, anonymousEmail, anonymousUsername, targetID, actor.TenantID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not anonymize user identity")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not delete user")
		return
	}
	if avatarObjectKey != "" {
		if client, bucket, storageErr := recordingObjectClient(); storageErr == nil {
			_ = client.RemoveObject(request.Context(), bucket, avatarObjectKey, minio.RemoveObjectOptions{})
		}
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "user.delete", "user", targetID, map[string]any{"identityAnonymized": true})
	writer.WriteHeader(http.StatusNoContent)
}

func userDeletionEligible(role userRole, status string) bool {
	return role != roleSuperAdmin && (status == "DEACTIVATED" || status == "INVITED")
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
