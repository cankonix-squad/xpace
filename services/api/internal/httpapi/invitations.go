package httpapi

import (
	"database/sql"
	"net/http"
	"strings"

	"github.com/cankonix/xpace/api/internal/auth"
)

func (api *API) acceptInvitation(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Token, Password, PasswordConfirm string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.Token = strings.TrimSpace(input.Token)
	if input.Token == "" || input.Password != input.PasswordConfirm {
		errorJSON(writer, http.StatusBadRequest, "PASSWORD_MISMATCH", "a valid invitation and matching passwords are required")
		return
	}
	passwordHash, err := auth.HashPassword(input.Password)
	if err != nil {
		errorJSON(writer, http.StatusBadRequest, "WEAK_PASSWORD", err.Error())
		return
	}
	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not accept invitation")
		return
	}
	defer tx.Rollback()
	var invitationID string
	var user currentUser
	err = tx.QueryRowContext(request.Context(), `SELECT i.id,u.id,u.tenant_id,t.slug,t.name,u.email,u.username,u.display_name,u.role FROM user_invitations i JOIN users u ON u.id=i.user_id JOIN tenants t ON t.id=u.tenant_id WHERE i.token_hash=$1 AND i.accepted_at IS NULL AND i.expires_at>NOW() AND u.status='INVITED' FOR UPDATE OF i,u`, hashToken(input.Token)).Scan(&invitationID, &user.ID, &user.TenantID, &user.TenantSlug, &user.TenantName, &user.Email, &user.Username, &user.DisplayName, &user.Role)
	if err != nil {
		errorJSON(writer, http.StatusGone, "INVITATION_INVALID", "invitation is invalid, expired, or already used")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE users SET password_hash=$1,status='ACTIVE',updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, passwordHash, user.ID, user.TenantID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not activate user")
		return
	}
	if _, err = tx.ExecContext(request.Context(), `UPDATE user_invitations SET accepted_at=NOW() WHERE id=$1`, invitationID); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not complete invitation")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not complete invitation")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "auth.invitation.accept", "user", user.ID, nil)
	if err = api.createSession(request.Context(), writer, request, user); err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "account activated but session could not be started")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"user": userResponse(user)})
}
