package httpapi

import (
	"database/sql"
	"net/http"
	"strings"
	"time"
	"unicode/utf8"
)

type profileResponse struct {
	UserID      string  `json:"userId"`
	TenantID    string  `json:"tenantId"`
	DisplayName string  `json:"displayName"`
	Email       string  `json:"email"`
	Username    string  `json:"username"`
	Role        string  `json:"role"`
	Timezone    string  `json:"timezone"`
	Locale      string  `json:"locale"`
	Bio         string  `json:"bio"`
	AvatarURL   *string `json:"avatarUrl"`
}

func (api *API) profile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	if request.Method == http.MethodPatch {
		api.updateProfile(writer, request, user)
		return
	}
	api.getProfile(writer, request, user)
}

func (api *API) getProfile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	profile, err := api.loadProfile(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not load profile")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"profile": profile})
}

func (api *API) loadProfile(request *http.Request, user currentUser) (profileResponse, error) {
	var profile profileResponse
	err := api.database.QueryRowContext(request.Context(), `
		SELECT u.id,u.tenant_id,u.display_name,u.email,u.username,u.role,
		       COALESCE(p.timezone,'Asia/Jakarta'),COALESCE(p.locale,'en-ID'),
		       COALESCE(p.bio,''),p.avatar_url
		FROM users u
		LEFT JOIN user_profiles p ON p.user_id=u.id AND p.tenant_id=u.tenant_id
		WHERE u.id=$1 AND u.tenant_id=$2`, user.ID, user.TenantID).
		Scan(&profile.UserID, &profile.TenantID, &profile.DisplayName, &profile.Email,
			&profile.Username, &profile.Role, &profile.Timezone, &profile.Locale,
			&profile.Bio, &profile.AvatarURL)
	return profile, err
}

func (api *API) updateProfile(writer http.ResponseWriter, request *http.Request, user currentUser) {
	var input struct {
		DisplayName string `json:"displayName"`
		Timezone    string `json:"timezone"`
		Locale      string `json:"locale"`
		Bio         string `json:"bio"`
	}
	if err := decodeJSON(writer, request, &input); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_INPUT", err.Error())
		return
	}
	input.DisplayName = strings.TrimSpace(input.DisplayName)
	input.Timezone = strings.TrimSpace(input.Timezone)
	input.Locale = strings.TrimSpace(input.Locale)
	input.Bio = strings.TrimSpace(input.Bio)
	if utf8.RuneCountInString(input.DisplayName) < 2 || utf8.RuneCountInString(input.DisplayName) > 80 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_DISPLAY_NAME", "displayName must be between 2 and 80 characters")
		return
	}
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		errorJSON(writer, http.StatusBadRequest, "INVALID_TIMEZONE", "timezone must be a valid IANA timezone")
		return
	}
	if !validLocale(input.Locale) {
		errorJSON(writer, http.StatusBadRequest, "INVALID_LOCALE", "locale must use a language or language-region format")
		return
	}
	if utf8.RuneCountInString(input.Bio) > 280 {
		errorJSON(writer, http.StatusBadRequest, "INVALID_BIO", "bio must not exceed 280 characters")
		return
	}

	tx, err := api.database.BeginTx(request.Context(), &sql.TxOptions{})
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update profile")
		return
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(request.Context(), `UPDATE users SET display_name=$1,updated_at=NOW() WHERE id=$2 AND tenant_id=$3`, input.DisplayName, user.ID, user.TenantID); err == nil {
		_, err = tx.ExecContext(request.Context(), `INSERT INTO user_profiles (user_id,tenant_id,timezone,locale,bio) VALUES ($1,$2,$3,$4,$5) ON CONFLICT (user_id) DO UPDATE SET timezone=EXCLUDED.timezone,locale=EXCLUDED.locale,bio=EXCLUDED.bio,updated_at=NOW() WHERE user_profiles.tenant_id=EXCLUDED.tenant_id`, user.ID, user.TenantID, input.Timezone, input.Locale, input.Bio)
	}
	if err != nil || tx.Commit() != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "could not update profile")
		return
	}
	_ = api.writeAuditEvent(request.Context(), request, user.TenantID, user.ID, "profile.update", "user", user.ID, nil)
	profile, err := api.loadProfile(request, user)
	if err != nil {
		errorJSON(writer, http.StatusInternalServerError, "INTERNAL_ERROR", "profile updated but could not be reloaded")
		return
	}
	respondJSON(writer, http.StatusOK, map[string]any{"profile": profile})
}

func validLocale(locale string) bool {
	if len(locale) < 2 || len(locale) > 16 {
		return false
	}
	for index, character := range locale {
		if character == '-' && index > 0 && index < len(locale)-1 {
			continue
		}
		if character < 'A' || character > 'Z' && character < 'a' || character > 'z' {
			return false
		}
	}
	return true
}
