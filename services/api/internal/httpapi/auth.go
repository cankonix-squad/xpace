package httpapi

import (
	"context"
	"database/sql"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/cankonix/xpace/api/internal/auth"
)

const sessionCookie = "xpace_session"

type currentUser struct {
	ID, TenantID, TenantSlug, TenantName, Email, Username, DisplayName string
	Role                                                               userRole
}
type sessionHandler func(http.ResponseWriter, *http.Request, currentUser)

func (api *API) bootstrap(writer http.ResponseWriter, request *http.Request) {
	var input struct{ TenantName, TenantSlug, DisplayName, Email, Username, Password string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	input.TenantName, input.TenantSlug, input.DisplayName = strings.TrimSpace(input.TenantName), strings.ToLower(strings.TrimSpace(input.TenantSlug)), strings.TrimSpace(input.DisplayName)
	input.Email, input.Username = strings.ToLower(strings.TrimSpace(input.Email)), strings.ToLower(strings.TrimSpace(input.Username))
	if input.TenantName == "" || input.DisplayName == "" || input.Email == "" || input.Username == "" || !validSlug(input.TenantSlug) {
		errorJSON(writer, 400, "INVALID_INPUT", "all fields are required and tenantSlug may only contain letters, numbers, and hyphens")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		errorJSON(writer, 400, "WEAK_PASSWORD", err.Error())
		return
	}
	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{Isolation: sql.LevelSerializable})
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not initialize workspace")
		return
	}
	defer tx.Rollback()
	var count int
	if err = tx.QueryRowContext(request.Context(), "SELECT COUNT(*) FROM tenants").Scan(&count); err != nil || count != 0 {
		errorJSON(writer, 409, "BOOTSTRAP_UNAVAILABLE", "workspace has already been initialized")
		return
	}
	var tenantID, userID string
	if err = tx.QueryRowContext(request.Context(), "INSERT INTO tenants (slug,name) VALUES ($1,$2) RETURNING id", input.TenantSlug, input.TenantName).Scan(&tenantID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create workspace")
		return
	}
	if err = tx.QueryRowContext(request.Context(), "INSERT INTO users (tenant_id,email,username,display_name,password_hash,role) VALUES ($1,$2,$3,$4,$5,'SUPER_ADMIN') RETURNING id", tenantID, input.Email, input.Username, input.DisplayName, hash).Scan(&userID); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create administrator")
		return
	}
	if _, err = tx.ExecContext(request.Context(), "INSERT INTO audit_events (tenant_id,actor_user_id,action,resource_type,resource_id,ip_address,user_agent) VALUES ($1,$2,'auth.bootstrap','tenant',$5,NULLIF($3,'')::inet,$4)", tenantID, userID, clientIP(request), request.UserAgent(), tenantID); err != nil {
		slog.Error("bootstrap audit event failed", "error", err)
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not create workspace audit event")
		return
	}
	if err = tx.Commit(); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not initialize workspace")
		return
	}
	user := currentUser{ID: userID, TenantID: tenantID, TenantSlug: input.TenantSlug, TenantName: input.TenantName, Email: input.Email, Username: input.Username, DisplayName: input.DisplayName, Role: roleSuperAdmin}
	if err = api.createSession(request.Context(), writer, request, user); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "workspace created but session could not be started")
		return
	}
	respondJSON(writer, 201, map[string]any{"user": userResponse(user)})
}

func (api *API) login(writer http.ResponseWriter, request *http.Request) {
	var input struct{ Tenant, Identity, Password string }
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, 400, "INVALID_INPUT", err.Error())
		return
	}
	var user currentUser
	var passwordHash, status string
	err := api.database.QueryRowContext(request.Context(), `SELECT u.id,u.tenant_id,t.slug,t.name,u.email,u.username,u.display_name,u.role,u.password_hash,u.status FROM users u JOIN tenants t ON t.id=u.tenant_id WHERE t.slug=$1 AND (LOWER(u.email)=LOWER($2) OR LOWER(u.username)=LOWER($2))`, strings.ToLower(strings.TrimSpace(input.Tenant)), strings.TrimSpace(input.Identity)).Scan(&user.ID, &user.TenantID, &user.TenantSlug, &user.TenantName, &user.Email, &user.Username, &user.DisplayName, &user.Role, &passwordHash, &status)
	if err != nil || status != "ACTIVE" {
		api.auditFailedLogin(request, input.Tenant, input.Identity, "unknown_or_inactive_account")
		errorJSON(writer, 401, "INVALID_CREDENTIALS", "tenant, username, or password is incorrect")
		return
	}
	valid, verifyErr := auth.VerifyPassword(input.Password, passwordHash)
	if verifyErr != nil || !valid {
		_ = api.writeAuditEvent(request.Context(), request, user.TenantID, "", "auth.login.failed", "user", user.ID, map[string]any{"reason": "invalid_password", "identityHash": shortIdentityHash(input.Identity)})
		errorJSON(writer, 401, "INVALID_CREDENTIALS", "tenant, username, or password is incorrect")
		return
	}
	if err = api.createSession(request.Context(), writer, request, user); err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not start session")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "auth.login", "session", user.ID, nil)
	respondJSON(writer, 200, map[string]any{"user": userResponse(user)})
}

func (api *API) createSession(ctx context.Context, writer http.ResponseWriter, request *http.Request, user currentUser) error {
	token, err := randomToken(32)
	if err != nil {
		return err
	}
	expires := time.Now().Add(24 * time.Hour)
	_, err = api.database.ExecContext(ctx, "INSERT INTO sessions (user_id,token_hash,expires_at) VALUES ($1,$2,$3)", user.ID, hashToken(token), expires)
	if err != nil {
		return err
	}
	http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: signedSessionToken(token), Path: "/", HttpOnly: true, Secure: os.Getenv("COOKIE_SECURE") == "true", SameSite: http.SameSiteLaxMode, Expires: expires, MaxAge: 86400})
	return nil
}

func (api *API) requireSession(next sessionHandler) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		cookie, err := request.Cookie(sessionCookie)
		if err != nil {
			errorJSON(writer, 401, "UNAUTHENTICATED", "sign in is required")
			return
		}
		token, valid := verifySignedSessionToken(cookie.Value)
		if !valid {
			slog.Warn("signed session token rejected", "remote_addr", request.RemoteAddr)
			errorJSON(writer, 401, "UNAUTHENTICATED", "session is invalid or expired")
			return
		}
		var user currentUser
		err = api.database.QueryRowContext(request.Context(), `SELECT u.id,u.tenant_id,t.slug,t.name,u.email,u.username,u.display_name,u.role FROM sessions s JOIN users u ON u.id=s.user_id JOIN tenants t ON t.id=u.tenant_id WHERE s.token_hash=$1 AND s.revoked_at IS NULL AND s.expires_at>NOW() AND u.status='ACTIVE'`, hashToken(token)).Scan(&user.ID, &user.TenantID, &user.TenantSlug, &user.TenantName, &user.Email, &user.Username, &user.DisplayName, &user.Role)
		if err != nil {
			api.auditRejectedSession(request, token)
			errorJSON(writer, 401, "UNAUTHENTICATED", "session is invalid or expired")
			return
		}
		next(writer, request, user)
	}
}

func (api *API) me(writer http.ResponseWriter, _ *http.Request, user currentUser) {
	respondJSON(writer, 200, map[string]any{"user": userResponse(user)})
}
func (api *API) logout(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(sessionCookie); err == nil {
		if token, valid := verifySignedSessionToken(cookie.Value); valid {
			var tenantID, userID string
			err = api.database.QueryRowContext(request.Context(), `UPDATE sessions s SET revoked_at=NOW() FROM users u WHERE s.user_id=u.id AND s.token_hash=$1 AND s.revoked_at IS NULL RETURNING u.tenant_id,u.id`, hashToken(token)).Scan(&tenantID, &userID)
			if err == nil {
				_ = api.writeAuditEvent(request.Context(), request, tenantID, userID, "auth.logout", "session", userID, nil)
			}
		}
	}
	http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, MaxAge: -1, Expires: time.Unix(0, 0)})
	writer.WriteHeader(http.StatusNoContent)
}

func (api *API) auditFailedLogin(request *http.Request, tenantSlug, identity, reason string) {
	var tenantID, userID string
	err := api.database.QueryRowContext(request.Context(), `SELECT t.id,COALESCE((SELECT u.id::text FROM users u WHERE u.tenant_id=t.id AND (LOWER(u.email)=LOWER($2) OR LOWER(u.username)=LOWER($2)) LIMIT 1),'') FROM tenants t WHERE t.slug=$1`, strings.ToLower(strings.TrimSpace(tenantSlug)), strings.TrimSpace(identity)).Scan(&tenantID, &userID)
	if err == nil {
		_ = api.writeAuditEvent(request.Context(), request, tenantID, "", "auth.login.failed", "user", userID, map[string]any{"reason": reason, "identityHash": shortIdentityHash(identity)})
	}
}

func (api *API) auditRejectedSession(request *http.Request, token string) {
	var tenantID, userID string
	err := api.database.QueryRowContext(request.Context(), `SELECT u.tenant_id,u.id FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1`, hashToken(token)).Scan(&tenantID, &userID)
	if err == nil {
		_ = api.writeAuditEvent(request.Context(), request, tenantID, "", "auth.session.rejected", "session", userID, map[string]any{"reason": "invalid_or_expired"})
	}
}

func shortIdentityHash(identity string) string {
	value := hashToken(strings.ToLower(strings.TrimSpace(identity)))
	return value[:16]
}
func userResponse(user currentUser) map[string]any {
	return map[string]any{"id": user.ID, "tenantId": user.TenantID, "tenantSlug": user.TenantSlug, "tenantName": user.TenantName, "email": user.Email, "username": user.Username, "displayName": user.DisplayName, "role": user.Role, "permissions": user.Role.permissions()}
}
func validSlug(value string) bool {
	if len(value) < 2 || len(value) > 48 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-') {
			return false
		}
	}
	return true
}
