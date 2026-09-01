package httpapi

import (
	"net/http"
	"strings"
	"time"
)

type sessionDevice struct {
	ID         string    `json:"id"`
	DeviceName string    `json:"deviceName"`
	IPAddress  string    `json:"ipAddress"`
	UserAgent  string    `json:"userAgent"`
	CreatedAt  time.Time `json:"createdAt"`
	LastSeenAt time.Time `json:"lastSeenAt"`
	ExpiresAt  time.Time `json:"expiresAt"`
	Current    bool      `json:"current"`
}

func (api *API) sessionDevices(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	currentHash := currentSessionHash(request)
	if request.Method == http.MethodDelete {
		result, err := api.database.ExecContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>NOW() AND token_hash<>$2`, actor.ID, currentHash)
		if err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not revoke other sessions")
			return
		}
		count, _ := result.RowsAffected()
		_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "auth.sessions.revoke_others", "user", actor.ID, map[string]any{"count": count})
		respondJSON(writer, 200, map[string]any{"revoked": count})
		return
	}
	rows, err := api.database.QueryContext(request.Context(), `SELECT id,device_name,COALESCE(host(ip_address),''),user_agent,created_at,last_seen_at,expires_at,token_hash=$2 FROM sessions WHERE user_id=$1 AND revoked_at IS NULL AND expires_at>NOW() ORDER BY last_seen_at DESC`, actor.ID, currentHash)
	if err != nil {
		errorJSON(writer, 500, "INTERNAL_ERROR", "could not load active sessions")
		return
	}
	defer rows.Close()
	items := make([]sessionDevice, 0)
	for rows.Next() {
		var item sessionDevice
		if err = rows.Scan(&item.ID, &item.DeviceName, &item.IPAddress, &item.UserAgent, &item.CreatedAt, &item.LastSeenAt, &item.ExpiresAt, &item.Current); err != nil {
			errorJSON(writer, 500, "INTERNAL_ERROR", "could not load active sessions")
			return
		}
		items = append(items, item)
	}
	respondJSON(writer, 200, map[string]any{"sessions": items})
}

func (api *API) revokeSessionDevice(writer http.ResponseWriter, request *http.Request, actor currentUser) {
	sessionID := request.PathValue("sessionID")
	var revokedCurrent bool
	err := api.database.QueryRowContext(request.Context(), `UPDATE sessions SET revoked_at=NOW() WHERE id=$1 AND user_id=$2 AND revoked_at IS NULL RETURNING token_hash=$3`, sessionID, actor.ID, currentSessionHash(request)).Scan(&revokedCurrent)
	if err != nil {
		errorJSON(writer, 404, "SESSION_NOT_FOUND", "active session was not found")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, actor.TenantID, actor.ID, "auth.session.revoke", "session", sessionID, map[string]any{"current": revokedCurrent})
	if revokedCurrent {
		http.SetCookie(writer, &http.Cookie{Name: sessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: request.TLS != nil, SameSite: http.SameSiteLaxMode, MaxAge: -1, Expires: time.Unix(0, 0)})
	}
	writer.WriteHeader(http.StatusNoContent)
}

func currentSessionHash(request *http.Request) string {
	cookie, err := request.Cookie(sessionCookie)
	if err != nil {
		return ""
	}
	token, valid := verifySignedSessionToken(cookie.Value)
	if !valid {
		return ""
	}
	return hashToken(token)
}

func sessionDeviceName(userAgent string) string {
	lower := strings.ToLower(userAgent)
	browser := "Browser"
	switch {
	case strings.Contains(lower, "edg/"):
		browser = "Microsoft Edge"
	case strings.Contains(lower, "chrome/") || strings.Contains(lower, "crios/"):
		browser = "Google Chrome"
	case strings.Contains(lower, "firefox/") || strings.Contains(lower, "fxios/"):
		browser = "Mozilla Firefox"
	case strings.Contains(lower, "safari/"):
		browser = "Safari"
	}
	platform := "Unknown device"
	switch {
	case strings.Contains(lower, "iphone"):
		platform = "iPhone"
	case strings.Contains(lower, "ipad"):
		platform = "iPad"
	case strings.Contains(lower, "android"):
		platform = "Android"
	case strings.Contains(lower, "macintosh") || strings.Contains(lower, "mac os"):
		platform = "Mac"
	case strings.Contains(lower, "windows"):
		platform = "Windows"
	case strings.Contains(lower, "linux"):
		platform = "Linux"
	}
	return browser + " on " + platform
}
